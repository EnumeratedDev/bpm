package bpmlib

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/drone/envsubst"
	"gopkg.in/yaml.v3"
)

var rootCompilationUID = "65534"
var rootCompilationGID = "65534"

func CompileSourcePackage(archiveFilename, outputDirectory string, skipChecks bool) (outputBpmPackages map[string]string, err error) {
	// Initialize map
	outputBpmPackages = make(map[string]string)

	// Read BPM archive
	bpmpkg, err := ReadPackage(archiveFilename)
	if err != nil {
		return nil, err
	}

	// Ensure package type is 'source'
	if bpmpkg.PkgInfo.Type != "source" {
		return nil, errors.New("cannot compile a non-source package")
	}

	// Get HOME directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// Get UID and GID to use for compilation
	var uid, gid int
	if os.Getuid() == 0 {
		_uid, err := strconv.ParseInt(rootCompilationUID, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("could not convert UID '%s' to int", rootCompilationUID)
		}
		_gid, err := strconv.ParseInt(rootCompilationGID, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("could not convert GID '%s' to int", rootCompilationGID)
		}
		uid = int(_uid)
		gid = int(_gid)
	} else {
		uid = os.Getuid()
		gid = os.Getgid()
	}

	// Set temporary directory
	var tempDirectory string
	if os.Getuid() == 0 {
		tempDirectory = path.Join("/var/cache/bpm/compilation/", bpmpkg.PkgInfo.Name)
	} else {
		tempDirectory = path.Join(homeDir, ".cache/bpm/compilation/", bpmpkg.PkgInfo.Name)
	}

	// Ensure temporary directory does not exist
	if _, err := os.Stat(tempDirectory); err == nil {
		err := os.RemoveAll(tempDirectory)
		if err != nil {
			return nil, err
		}
	}

	// Create temporary directory
	err = os.MkdirAll(tempDirectory, 0755)
	if err != nil {
		return nil, err
	}

	// Change temporary directory owner
	err = os.Chown(tempDirectory, uid, gid)
	if err != nil {
		return nil, err
	}

	// Extract source.sh file
	err = extractTarballFile(archiveFilename, "source.sh", tempDirectory, uid, gid)
	if err != nil {
		return nil, err
	}

	// Get package scripts and extract them
	packageScripts := getPackageScripts(archiveFilename)
	for _, script := range packageScripts {
		err = extractTarballFile(archiveFilename, script, tempDirectory, uid, gid)
		if err != nil {
			return nil, err
		}
	}

	// Extract source files
	err = extractTarballDirectory(archiveFilename, "source-files", tempDirectory, uid, gid)
	if err != nil {
		return nil, err
	}

	// Create source directory
	err = os.Mkdir(path.Join(tempDirectory, "source"), 0755)
	if err != nil {
		return nil, err
	}

	// Change source directory owner
	err = os.Chown(path.Join(tempDirectory, "source"), uid, gid)
	if err != nil {
		return nil, err
	}

	// Download files
	err = downloadPackageFiles(bpmpkg.PkgInfo, tempDirectory)
	if err != nil {
		return nil, err
	}

	// Setup environment for commands
	env := os.Environ()
	env = append(env, "HOME="+tempDirectory)
	env = append(env, "BPM_WORKDIR="+tempDirectory)
	env = append(env, "BPM_SOURCE="+path.Join(tempDirectory, "source"))
	env = append(env, "BPM_OUTPUT="+path.Join(tempDirectory, "output"))
	env = append(env, "BPM_PKG_NAME="+bpmpkg.PkgInfo.Name)
	env = append(env, "BPM_PKG_VERSION="+bpmpkg.PkgInfo.Version)
	env = append(env, "BPM_PKG_REVISION="+strconv.Itoa(bpmpkg.PkgInfo.Revision))
	env = append(env, "BPM_PKG_ARCH="+bpmpkg.PkgInfo.OutputArch)
	env = append(env, CompilationBPMConfig.CompilationEnvironment...)

	// Execute prepare and build functions in source.sh script
	cmd := exec.Command("bash", "-c",
		"set -a\n"+ // Source and export functions and variables in source.sh script
			". \"${BPM_WORKDIR}\"/source.sh\n"+
			"set +a\n"+
			"[[ $(type -t prepare) == \"function\" ]] && { echo \"Running prepare() function\"; bash -e -c 'cd \"$BPM_WORKDIR\" && prepare' || exit 1; }\n"+ // Run prepare() function if it exists
			"[[ $(type -t build) == \"function\" ]] && { echo \"Running build() function\"; bash -e -c 'cd \"$BPM_SOURCE\" && build'  || exit 1; }\n"+ // Run build() function if it exists
			"exit 0")
	cmd.Dir = tempDirectory
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	if os.Getuid() == 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	}
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	// Execute check function in source.sh script if not skipping checks
	if !skipChecks {
		cmd = exec.Command("bash", "-c",
			"set -a\n"+ // Source and export functions and variables in source.sh script
				". \"${BPM_WORKDIR}\"/source.sh\n"+
				"set +a\n"+
				"[[ $(type -t check) == \"function\" ]] && { echo \"Running check() function\"; bash -e -c 'cd \"$BPM_SOURCE\" && check' || exit 1; }\n"+ // Run check() function if it exists
				"exit 0")
		cmd.Dir = tempDirectory
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = env
		if os.Getuid() == 0 {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
			cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
		}
		err = cmd.Run()
		if err != nil {
			return nil, err
		}
	}

	// Get all packages to compile
	packagesToCompile := bpmpkg.PkgInfo.SplitPackages
	if !bpmpkg.PkgInfo.IsSplitPackage() {
		packagesToCompile = append(packagesToCompile, bpmpkg.PkgInfo)
	}

	// Compile each package
	for _, pkg := range packagesToCompile {
		// Get package function name
		packageFunctionName := "package"
		if bpmpkg.PkgInfo.IsSplitPackage() {
			packageFunctionName = "package_" + pkg.Name
		}

		// Remove output directory if it already exists
		if _, err := os.Stat(path.Join(tempDirectory, "output")); err == nil {
			err := os.RemoveAll(path.Join(tempDirectory, "output"))
			if err != nil {
				return nil, err
			}
		}

		// Create new output directory
		err = os.Mkdir(path.Join(tempDirectory, "output"), 0755)
		if err != nil {
			return nil, err
		}

		// Change output directory owner
		err = os.Chown(path.Join(tempDirectory, "output"), uid, gid)
		if err != nil {
			return nil, err
		}

		// Execute package function in source.sh script and generate package file list
		cmd = exec.Command("bash", "-c",
			"set -a\n"+ // Source and export functions and variables in source.sh script
				". \"${BPM_WORKDIR}\"/source.sh\n"+
				"set +a\n"+
				"echo \"Running "+packageFunctionName+"() function\"\n"+
				"( cd \"$BPM_SOURCE\" && fakeroot -s \"$BPM_WORKDIR\"/fakeroot_file bash -e -c '"+packageFunctionName+"' ) || exit 1\n"+ // Run package() function
				"fakeroot -i \"$BPM_WORKDIR\"/fakeroot_file find \"$BPM_OUTPUT\" -mindepth 1 -printf \"%P %#m %U %G %s\\n\" > \"$BPM_WORKDIR\"/pkg.files") // Create package file list
		cmd.Dir = tempDirectory
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = env
		if os.Getuid() == 0 {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
			cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
		}
		err = cmd.Run()
		if err != nil {
			return nil, err
		}

		// Create gzip-compressed archive for the package files
		cmd = exec.Command("bash", "-c", "find output -printf \"%P\\n\" | fakeroot -i \"$BPM_WORKDIR\"/fakeroot_file tar -czf files.tar.gz --no-recursion -C output -T -")
		cmd.Dir = tempDirectory
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = env
		if os.Getuid() == 0 {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
			cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
		}
		err = cmd.Run()
		if err != nil {
			return nil, fmt.Errorf("files.tar.gz archive could not be created: %s", err)
		}

		// Clone source package info
		pkgInfo := *pkg

		// Set package type to binary
		pkgInfo.Type = "binary"

		// Set package architecture
		pkgInfo.Arch = pkg.OutputArch
		pkgInfo.OutputArch = ""

		// Remove split package and downloads fields
		pkgInfo.SplitPackages = nil
		pkgInfo.Downloads = nil

		// Marshal package info
		pkgInfoBytes, err := yaml.Marshal(pkgInfo)
		if err != nil {
			return nil, err
		}
		pkgInfoBytes = append(pkgInfoBytes, '\n')

		// Create pkg.info file
		err = os.WriteFile(path.Join(tempDirectory, "pkg.info"), pkgInfoBytes, 0644)
		if err != nil {
			return nil, err
		}

		// Change pkg.info file owner
		err = os.Chown(path.Join(tempDirectory, "pkg.info"), uid, gid)
		if err != nil {
			return nil, err
		}

		// Get files to include in BPM archive
		bpmArchiveFiles := make([]string, 0)
		bpmArchiveFiles = append(bpmArchiveFiles, "pkg.info", "pkg.files", "files.tar.gz") // Base files
		bpmArchiveFiles = append(bpmArchiveFiles, packageScripts...)                       // Package scripts

		// Create final BPM archive
		cmd = exec.Command("bash", "-c", "tar -cf final-archive.bpm --owner=0 --group=0 -C \"$BPM_WORKDIR\" "+strings.Join(bpmArchiveFiles, " "))
		cmd.Dir = tempDirectory
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		currentDir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cmd.Env = append(env, "CURRENT_DIR="+currentDir)
		if os.Getuid() == 0 {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
			cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
		}
		err = cmd.Run()
		if err != nil {
			return nil, fmt.Errorf("BPM archive could not be created: %s", err)
		}

		// Remove pkg.info file
		err = os.Remove(path.Join(tempDirectory, "pkg.info"))
		if err != nil {
			return nil, err
		}

		// Set output filename
		outputFilename := path.Join(outputDirectory, fmt.Sprintf("%s-%s-%d-%s.bpm", pkgInfo.Name, pkgInfo.Version, pkgInfo.Revision, pkgInfo.Arch))

		// Move final BPM archive
		err = os.Rename(path.Join(tempDirectory, "final-archive.bpm"), outputFilename)
		if err != nil {
			return nil, err
		}

		// Set final BPM archive owner
		err = os.Chown(outputFilename, os.Getuid(), os.Getgid())
		if err != nil {
			return nil, err
		}

		outputBpmPackages[pkgInfo.Name] = outputFilename
	}

	return outputBpmPackages, nil
}

func downloadPackageFiles(pkgInfo *PackageInfo, tempDirectory string) error {
	for _, download := range pkgInfo.Downloads {
		// Replace variables
		replaceVars := func(s string) string {
			switch s {
			case "BPM_PKG_VERSION":
				return pkgInfo.Version
			case "BPM_PKG_NAME":
				return pkgInfo.Name
			case "BPM_SOURCE":
				return path.Join(tempDirectory, "source/")
			default:
				return ""
			}
		}

		downloadUrl, err := envsubst.Eval(strings.TrimSpace(download.Url), replaceVars)
		if err != nil {
			return err
		}
		extractTo, err := envsubst.Eval(strings.TrimSpace(download.ExtractTo), replaceVars)
		if err != nil {
			return err
		}
		cloneTo, err := envsubst.Eval(strings.TrimSpace(download.CloneTo), replaceVars)
		if err != nil {
			return err
		}

		// Make relative paths absolute
		if extractTo != "" && extractTo[0] != '/' {
			extractTo = path.Join(tempDirectory, extractTo)
		}
		// Make relative paths absolute
		if cloneTo != "" && cloneTo[0] != '/' {
			cloneTo = path.Join(tempDirectory, cloneTo)
		}

		switch download.Type {
		case "", "file":
			filepath := path.Join(tempDirectory, path.Base(downloadUrl))
			if download.Filepath != "" && download.Filepath[0] != '/' {
				filepath = path.Join(tempDirectory, download.Filepath)
			}

			err := downloadFile(downloadUrl, filepath, 0644)
			if err != nil {
				return err
			}

			if download.Checksum != "skip" {
				f, err := os.Open(filepath)
				if err != nil {
					return err
				}
				defer f.Close()

				h := sha256.New()
				_, err = io.Copy(h, f)
				if err != nil {
					return err
				}

				if hex.EncodeToString(h.Sum(nil)) != download.Checksum {
					fmt.Printf("Downloaded file checksum: %x\n", h.Sum(nil))
					return fmt.Errorf("downloaded file checksums did not match")
				}
			} else {
				fmt.Println("Skipping checksum checking...")
			}

			if !download.NoExtract && (strings.Contains(filepath, ".tar") || strings.HasSuffix(filepath, ".tgz")) {
				cmd := exec.Command("tar", "xvf", filepath, "--strip-components="+strconv.Itoa(download.ExtractStripComponents))
				cmd.Dir = tempDirectory
				if extractTo != "" {
					err := os.MkdirAll(extractTo, 0755)
					if err != nil {
						return err
					}
					cmd.Args = append(cmd.Args, "-C", extractTo)
				}

				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr

				err := cmd.Run()
				if err != nil {
					return err
				}
			} else if !download.NoExtract && strings.HasSuffix(filepath, ".zip") {
				cmd := exec.Command("unzip", filepath)
				if extractTo != "" {
					err := os.MkdirAll(extractTo, 0755)
					if err != nil {
						return err
					}
					cmd.Args = append(cmd.Args, "-d", extractTo)
				} else {
					err := os.Mkdir(path.Join(tempDirectory, strings.TrimSuffix(path.Base(filepath), ".zip")), 0755)
					if err != nil {
						return err
					}

					cmd.Args = append(cmd.Args, "-d", path.Join(tempDirectory, strings.TrimSuffix(path.Base(filepath), ".zip")))
				}

				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr

				err := cmd.Run()
				if err != nil {
					return err
				}
			}
		case "git":
			// Replace variables in git branch
			gitBranch := download.GitBranch
			gitBranch, err := envsubst.Eval(gitBranch, func(s string) string {
				switch s {
				case "BPM_PKG_VERSION":
					return pkgInfo.Version
				case "BPM_PKG_NAME":
					return pkgInfo.Name
				default:
					return ""
				}
			})
			if err != nil {
				return err
			}

			cmd := exec.Command("git", "clone", "--depth=1", downloadUrl)
			cmd.Dir = tempDirectory
			if gitBranch != "" {
				cmd.Args = slices.Insert(cmd.Args, len(cmd.Args)-1, "--branch="+gitBranch)
			}
			if cloneTo != "" {
				err := os.MkdirAll(cloneTo, 0755)
				if err != nil {
					return err
				}
				cmd.Args = append(cmd.Args, cloneTo)
			}

			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			err = cmd.Run()
			if err != nil {
				return err
			}

			if download.Checksum != "skip" {
				cmd := exec.Command("git", "rev-parse", "HEAD")
				if cloneTo != "" {
					cmd.Dir = cloneTo
				} else {
					cmd.Dir = path.Join(tempDirectory, strings.TrimSuffix(path.Base(downloadUrl), ".git"))
				}

				branchChecksum, err := cmd.Output()
				if err != nil {
					return err
				}

				if strings.TrimSpace(string(branchChecksum)) != download.Checksum {
					fmt.Printf("Git branch checksum: %s\n", strings.TrimSpace(string(branchChecksum)))
					return fmt.Errorf("Cloned git repository checksum did not match")
				}
			} else {
				fmt.Println("Skipping checksum checking...")
			}
		default:
			return fmt.Errorf("unknown download type (%s)", download.Type)
		}
	}

	return nil
}
