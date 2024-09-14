package utils

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

type PackageInfo struct {
	Name            string   `yaml:"name,omitempty"`
	Description     string   `yaml:"description,omitempty"`
	Version         string   `yaml:"version,omitempty"`
	Revision        int      `yaml:"revision,omitempty"`
	Url             string   `yaml:"url,omitempty"`
	License         string   `yaml:"license,omitempty"`
	Arch            string   `yaml:"architecture,omitempty"`
	Type            string   `yaml:"type,omitempty"`
	Keep            []string `yaml:"keep,omitempty"`
	Depends         []string `yaml:"depends,omitempty"`
	MakeDepends     []string `yaml:"make_depends,omitempty"`
	OptionalDepends []string `yaml:"optional_depends,omitempty"`
	Conflicts       []string `yaml:"conflicts,omitempty"`
	Provides        []string `yaml:"provides,omitempty"`
}

func (pkgInfo *PackageInfo) GetFullVersion() string {
	return pkgInfo.Version + "-" + strconv.Itoa(pkgInfo.Revision)
}

type InstallationReason string

const (
	Manual     InstallationReason = "manual"
	Dependency InstallationReason = "dependency"
	Unknown    InstallationReason = "unknown"
)

func GetInstallationReason(pkg, rootDir string) InstallationReason {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	if stat, err := os.Stat(path.Join(pkgDir, "installation_reason")); err != nil || stat.IsDir() {
		return Manual
	}
	bytes, err := os.ReadFile(path.Join(pkgDir, "installation_reason"))
	if err != nil {
		return Unknown
	}
	reason := string(bytes)
	if reason == "manual" {
		return Manual
	} else if reason == "dependency" {
		return Dependency
	}
	return Unknown
}

func SetInstallationReason(pkg string, reason InstallationReason, rootDir string) error {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	err := os.WriteFile(path.Join(pkgDir, "installation_reason"), []byte(reason), 0644)
	if err != nil {
		return err
	}
	return nil
}

func GetPackageInfoRaw(filename string) (string, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return "", err
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(archive)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if header.Name == "pkg.info" {
			bs, _ := io.ReadAll(tr)
			err := file.Close()
			if err != nil {
				return "", err
			}
			return string(bs), nil
		}
	}
	return "", errors.New("pkg.info not found in archive")
}

func ReadPackage(filename string) (*PackageInfo, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(archive)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Name == "pkg.info" {
			bs, _ := io.ReadAll(tr)
			err := file.Close()
			if err != nil {
				return nil, err
			}
			pkgInfo, err := ReadPackageInfo(string(bs))
			if err != nil {
				return nil, err
			}
			return pkgInfo, nil
		}
	}
	return nil, errors.New("pkg.info not found in archive")
}

func ReadPackageScripts(filename string) (map[string]string, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(archive)
	ret := make(map[string]string)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Name == "pre_install.sh" {
			bs, _ := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			ret[header.Name] = string(bs)
		} else if header.Name == "post_install.sh" {
			bs, _ := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			ret[header.Name] = string(bs)
		} else if header.Name == "pre_update.sh" {
			bs, _ := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			ret[header.Name] = string(bs)
		} else if header.Name == "post_update.sh" {
			bs, _ := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			ret[header.Name] = string(bs)
		} else if header.Name == "post_remove.sh" {
			bs, _ := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			ret[header.Name] = string(bs)
		}
	}
	err = archive.Close()
	if err != nil {
		return nil, err
	}
	err = file.Close()
	if err != nil {
		return nil, err
	}
	return ret, nil
}

type Operation uint8

const (
	Install Operation = 0
	Update            = 1
	Remove            = 2
)

func ExecutePackageScripts(filename, rootDir string, operation Operation, postOperation bool) error {
	pkgInfo, err := ReadPackage(filename)
	if err != nil {
		return err
	}
	scripts, err := ReadPackageScripts(filename)
	if err != nil {
		return err
	}

	run := func(name, content string) error {
		temp, err := os.CreateTemp("", name)
		if err != nil {
			return err
		}
		_, err = temp.WriteString(content)
		if err != nil {
			return err
		}

		cmd := exec.Command("/bin/bash", temp.Name())
		if !BPMConfig.SilentCompilation {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.Dir = rootDir
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_ROOT=%s", rootDir))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_NAME=%s", pkgInfo.Name))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DESC=%s", pkgInfo.Description))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_VERSION=%s", pkgInfo.Version))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_URL=%s", pkgInfo.Url))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_ARCH=%s", pkgInfo.Arch))
		depends := make([]string, len(pkgInfo.Depends))
		copy(depends, pkgInfo.Depends)
		for i := 0; i < len(depends); i++ {
			depends[i] = fmt.Sprintf("\"%s\"", depends[i])
		}
		makeDepends := make([]string, len(pkgInfo.MakeDepends))
		copy(makeDepends, pkgInfo.MakeDepends)
		for i := 0; i < len(makeDepends); i++ {
			makeDepends[i] = fmt.Sprintf("\"%s\"", makeDepends[i])
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DEPENDS=(%s)", strings.Join(depends, " ")))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_MAKE_DEPENDS=(%s)", strings.Join(makeDepends, " ")))
		cmd.Env = append(cmd.Env, "BPM_PKG_TYPE=source")
		err = cmd.Run()
		if err != nil {
			return err
		}
		return nil
	}

	if operation == Install {
		if val, ok := scripts["pre_install.sh"]; !postOperation && ok {
			err := run("pre_install.sh", val)
			if err != nil {
				return err
			}
		}
		if val, ok := scripts["post_install.sh"]; postOperation && ok {
			err := run("post_install.sh", val)
			if err != nil {
				return err
			}
		}
	} else if operation == Update {
		if val, ok := scripts["pre_update.sh"]; !postOperation && ok {
			err := run("pre_update.sh", val)
			if err != nil {
				return err
			}
		}
		if val, ok := scripts["post_update.sh"]; postOperation && ok {
			err := run("post_update.sh", val)
			if err != nil {
				return err
			}
		}
	} else if operation == Remove {
		if val, ok := scripts["post_remove.sh"]; postOperation && ok {
			err := run("post_remove.sh", val)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func ReadPackageInfo(contents string) (*PackageInfo, error) {
	pkgInfo := PackageInfo{
		Name:            "",
		Description:     "",
		Version:         "",
		Revision:        1,
		Url:             "",
		License:         "",
		Arch:            "",
		Type:            "",
		Keep:            make([]string, 0),
		Depends:         make([]string, 0),
		MakeDepends:     make([]string, 0),
		OptionalDepends: make([]string, 0),
		Conflicts:       make([]string, 0),
		Provides:        make([]string, 0),
	}
	err := yaml.Unmarshal([]byte(contents), &pkgInfo)
	if err != nil {
		return nil, err
	}
	if pkgInfo.Name == "" {
		return nil, errors.New("this package contains no name")
	} else if pkgInfo.Description == "" {
		return nil, errors.New("this package contains no description")
	} else if pkgInfo.Version == "" {
		return nil, errors.New("this package contains no version")
	} else if pkgInfo.Revision <= 0 {
		return nil, errors.New("this package contains a revision number less or equal to 0")
	} else if pkgInfo.Arch == "" {
		return nil, errors.New("this package contains no architecture")
	} else if pkgInfo.Type == "" {
		return nil, errors.New("this package contains no type")
	}
	for i := 0; i < len(pkgInfo.Keep); i++ {
		pkgInfo.Keep[i] = strings.TrimPrefix(pkgInfo.Keep[i], "/")
	}
	return &pkgInfo, nil
}

func CreateInfoFile(pkgInfo *PackageInfo) string {
	bytes, err := yaml.Marshal(&pkgInfo)
	if err != nil {
		return ""
	}
	return string(bytes)
}

func CreateReadableInfo(showArchitecture, showType, showPackageRelations bool, pkgInfo *PackageInfo, rootDir string) string {
	ret := make([]string, 0)
	appendArray := func(label string, array []string) {
		if len(array) == 0 {
			return
		}
		ret = append(ret, fmt.Sprintf("%s: %s", label, strings.Join(array, ", ")))
	}
	ret = append(ret, "Name: "+pkgInfo.Name)
	ret = append(ret, "Description: "+pkgInfo.Description)
	ret = append(ret, "Version: "+pkgInfo.GetFullVersion())
	ret = append(ret, "URL: "+pkgInfo.Url)
	ret = append(ret, "License: "+pkgInfo.License)
	if showArchitecture {
		ret = append(ret, "Architecture: "+pkgInfo.Arch)
	}
	if showType {
		ret = append(ret, "Type: "+pkgInfo.Type)
	}
	if showPackageRelations {
		appendArray("Dependencies", pkgInfo.Depends)
		appendArray("Make Dependencies", pkgInfo.MakeDepends)
		appendArray("Optional dependencies", pkgInfo.OptionalDepends)
		appendArray("Conflicting packages", pkgInfo.Conflicts)
		appendArray("Provided packages", pkgInfo.Provides)

	}
	ret = append(ret, "Installation Reason: "+string(GetInstallationReason(pkgInfo.Name, rootDir)))
	return strings.Join(ret, "\n")
}

func extractPackage(pkgInfo *PackageInfo, verbose bool, filename, rootDir string) (error, []string) {
	var files []string
	if !IsPackageInstalled(pkgInfo.Name, rootDir) {
		err := ExecutePackageScripts(filename, rootDir, Install, false)
		if err != nil {
			return err, nil
		}
	} else {
		err := ExecutePackageScripts(filename, rootDir, Update, false)
		if err != nil {
			return err, nil
		}
	}
	seenHardlinks := make(map[string]string)
	file, err := os.Open(filename)
	if err != nil {
		return err, nil
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return err, nil
	}
	tr := tar.NewReader(archive)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err, nil
		}
		if strings.HasPrefix(header.Name, "files/") && header.Name != "files/" {
			trimmedName := strings.TrimPrefix(header.Name, "files/")
			extractFilename := path.Join(rootDir, trimmedName)
			switch header.Typeflag {
			case tar.TypeDir:
				files = append(files, strings.TrimPrefix(header.Name, "files/"))
				if err := os.Mkdir(extractFilename, 0755); err != nil {
					if !os.IsExist(err) {
						return err, nil
					}
				} else {
					if verbose {
						fmt.Println("Creating Directory: " + extractFilename)
					}
				}
			case tar.TypeReg:
				skip := false
				if _, err := os.Stat(extractFilename); err == nil {
					for _, k := range pkgInfo.Keep {
						if strings.HasSuffix(k, "/") {
							if strings.HasPrefix(trimmedName, k) {
								if verbose {
									fmt.Println("Skipping File: " + extractFilename + " (Containing directory is set to be kept during installs/updates)")
								}
								files = append(files, strings.TrimPrefix(header.Name, "files/"))
								skip = true
								continue
							}
						} else {
							if trimmedName == k {
								if verbose {
									fmt.Println("Skipping File: " + extractFilename + " (File is configured to be kept during installs/updates)")
								}
								files = append(files, strings.TrimPrefix(header.Name, "files/"))
								skip = true
								continue
							}
						}
					}
				}
				if skip {
					continue
				}
				err := os.Remove(extractFilename)
				if err != nil && !os.IsNotExist(err) {
					return err, nil
				}
				outFile, err := os.Create(extractFilename)
				if verbose {
					fmt.Println("Creating File: " + extractFilename)
				}
				files = append(files, strings.TrimPrefix(header.Name, "files/"))
				if err != nil {
					return err, nil
				}
				if _, err := io.Copy(outFile, tr); err != nil {
					return err, nil
				}
				if err := os.Chmod(extractFilename, header.FileInfo().Mode()); err != nil {
					return err, nil
				}
				err = outFile.Close()
				if err != nil {
					return err, nil
				}
			case tar.TypeSymlink:
				if verbose {
					fmt.Println("Creating Symlink: " + extractFilename + " -> " + header.Linkname)
				}
				files = append(files, strings.TrimPrefix(header.Name, "files/"))
				err := os.Remove(extractFilename)
				if err != nil && !os.IsNotExist(err) {
					return err, nil
				}
				err = os.Symlink(header.Linkname, extractFilename)
				if err != nil {
					return err, nil
				}
			case tar.TypeLink:
				if verbose {
					fmt.Println("Detected Hard Link: " + extractFilename + " -> " + path.Join(rootDir, strings.TrimPrefix(header.Linkname, "files/")))
				}
				files = append(files, strings.TrimPrefix(header.Name, "files/"))
				seenHardlinks[extractFilename] = path.Join(strings.TrimPrefix(header.Linkname, "files/"))
				err := os.Remove(extractFilename)
				if err != nil && !os.IsNotExist(err) {
					return err, nil
				}
			default:
				return errors.New("unknown type (" + strconv.Itoa(int(header.Typeflag)) + ") in " + extractFilename), nil
			}
		}
	}
	for extractFilename, destination := range seenHardlinks {
		if verbose {
			fmt.Println("Creating Hard Link: " + extractFilename + " -> " + path.Join(rootDir, destination))
		}
		err := os.Link(path.Join(rootDir, destination), extractFilename)
		if err != nil {
			return err, nil
		}
	}
	defer archive.Close()
	defer file.Close()
	return nil, files
}

func isSplitPackage(filename string) bool {
	pkgInfo, err := ReadPackage(filename)
	if err != nil {
		return false
	}
	if pkgInfo.Type != "source" {
		return false
	}
	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf("test $(tar -tf %s | grep '^pkg.info' | wc -l) -eq 1", filename))
	if err := cmd.Run(); err == nil {
		return false
	}
	return true
}

func compilePackage(pkgInfo *PackageInfo, filename, rootDir string, verbose, binaryPkgFromSrc, skipCheck, keepTempDir bool) (error, []string) {
	var files []string
	if !IsPackageInstalled(pkgInfo.Name, rootDir) {
		err := ExecutePackageScripts(filename, rootDir, Install, false)
		if err != nil {
			return err, nil
		}
	} else {
		err := ExecutePackageScripts(filename, rootDir, Update, false)
		if err != nil {
			return err, nil
		}
	}
	//seenHardlinks := make(map[string]string)
	file, err := os.Open(filename)
	if err != nil {
		return err, nil
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return err, nil
	}
	tr := tar.NewReader(archive)

	temp := path.Join(BPMConfig.CompilationDir, "bpm_source-"+pkgInfo.Name)
	err = os.RemoveAll(temp)
	if err != nil {
		return err, nil
	}
	if verbose {
		fmt.Println("Creating temp directory at: " + temp)
	}
	err = os.Mkdir(temp, 0755)
	if err != nil {
		return err, nil
	}
	err = os.Chown(temp, 65534, 65534)
	if err != nil {
		return err, nil
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err, nil
		}
		if strings.HasPrefix(header.Name, "source-files/") && header.Name != "source-files/" {
			extractFilename := path.Join(temp, strings.TrimPrefix(header.Name, "source-files/"))
			switch header.Typeflag {
			case tar.TypeDir:
				if err := os.Mkdir(extractFilename, 0755); err != nil {
					if !os.IsExist(err) {
						return err, nil
					}
				} else {
					if verbose {
						fmt.Println("Creating Directory: " + extractFilename)
					}
					err = os.Chown(extractFilename, 65534, 65534)
					if err != nil {
						return err, nil
					}
				}
			case tar.TypeReg:
				err := os.Remove(extractFilename)
				if err != nil && !os.IsNotExist(err) {
					return err, nil
				}
				outFile, err := os.Create(extractFilename)
				if verbose {
					fmt.Println("Creating File: " + extractFilename)
				}
				if err != nil {
					return err, nil
				}
				err = os.Chown(extractFilename, 65534, 65534)
				if err != nil {
					return err, nil
				}
				if _, err := io.Copy(outFile, tr); err != nil {
					return err, nil
				}
				if err := os.Chmod(extractFilename, header.FileInfo().Mode()); err != nil {
					return err, nil
				}
				err = outFile.Close()
				if err != nil {
					return err, nil
				}
			case tar.TypeSymlink:
				if verbose {
					fmt.Println("Skipping symlink (Bundling symlinks in source packages is not supported)")
				}
			case tar.TypeLink:
				if verbose {
					fmt.Println("Skipping hard link (Bundling hard links in source packages is not supported)")
				}
			default:
				return errors.New("unknown type (" + strconv.Itoa(int(header.Typeflag)) + ") in " + extractFilename), nil
			}
		}
		if header.Name == "source.sh" {
			bs, err := io.ReadAll(tr)
			if err != nil {
				return err, nil
			}
			err = os.WriteFile(path.Join(temp, "source.sh"), bs, 0644)
			if err != nil {
				return err, nil
			}
			err = os.Chown(path.Join(temp, "source.sh"), 65534, 65534)
			if err != nil {
				return err, nil
			}
		}
	}
	if _, err := os.Stat(path.Join(temp, "source.sh")); os.IsNotExist(err) {
		return errors.New("source.sh file could not be found in the temporary build directory"), nil
	}
	if err != nil {
		return err, nil
	}
	fmt.Println("Running source.sh file...")
	if !IsPackageInstalled(pkgInfo.Name, rootDir) {
		err = ExecutePackageScripts(filename, rootDir, Install, false)
		if err != nil {
			return err, nil
		}
	} else {
		err = ExecutePackageScripts(filename, rootDir, Update, false)
		if err != nil {
			return err, nil
		}
	}
	bs, err := os.ReadFile(path.Join(temp, "source.sh"))
	if err != nil {
		return err, nil
	}

	if !strings.Contains(string(bs), "package()") {
		fmt.Print("This package does not seem to have the required 'package' function\nThe source.sh file may have been created for an older BPM version\nPlease update the source.sh file")
		return errors.New("invalid source.sh format"), nil
	}

	runScript := `
cd "$BPM_WORKDIR"

set -a
source "source.sh"
set +a

if [[ $(type -t prepare) == function ]]; then
  echo "Running prepare() function..."
  bash -e -c prepare
  if [ $? -ne 0 ]; then
    echo "Failed to run prepare() function in source.sh"
    exit 1
  fi
fi

cd "$BPM_SOURCE"
if [[ $(type -t build) == function ]]; then
  echo "Running build() function..."
  bash -e -c build
  if [ $? -ne 0 ]; then
    echo "Failed to run build() function in source.sh"
    exit 1
  fi
fi

cd "$BPM_SOURCE"
if [[ $(type -t check) == function ]] && [ -z "$SKIPCHECK" ]; then
  echo "Running check() function..."
  bash -e -c check
  if [ $? -ne 0 ]; then
    echo "Failed to run check() function in source.sh"
    exit 1
  fi
fi


cd "$BPM_SOURCE"
if ! [[ $(type -t package) == function ]]; then
  echo "Failed to locate package() function in source.sh"
  exit 1
fi
echo "Running package() function..."
touch "$BPM_WORKDIR"/fakeroot_file
fakeroot -s "$BPM_WORKDIR"/fakeroot_file bash -e -c package
bash -e -c package
if [ $? -ne 0 ]; then
  echo "Failed to run package() function in source.sh"
fi
`
	err = os.WriteFile(path.Join(temp, "run.sh"), []byte(runScript), 0644)
	if err != nil {
		return err, nil
	}
	err = os.Chown(path.Join(temp, "run.sh"), 65534, 65534)
	if err != nil {
		return err, nil
	}

	cmd := exec.Command("/bin/bash", "-e", "run.sh")
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: 65534, Gid: 65534}
	cmd.Dir = temp
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "USER=nobody")
	cmd.Env = append(cmd.Env, "HOME="+temp)

	err = os.Mkdir(path.Join(temp, "source"), 0755)
	if err != nil {
		return err, nil
	}
	err = os.Chown(path.Join(temp, "source"), 65534, 65534)
	if err != nil {
		return err, nil
	}
	err = os.Mkdir(path.Join(temp, "output"), 0755)
	if err != nil {
		return err, nil
	}
	err = os.Chown(path.Join(temp, "output"), 65534, 65534)
	if err != nil {
		return err, nil
	}
	cmd.Env = append(cmd.Env, "BPM_WORKDIR="+temp)
	cmd.Env = append(cmd.Env, "BPM_SOURCE="+path.Join(temp, "source"))
	cmd.Env = append(cmd.Env, "BPM_OUTPUT="+path.Join(temp, "output"))
	cmd.Env = append(cmd.Env, "SKIPCHECK="+strconv.FormatBool(skipCheck))

	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_ROOT=%s", rootDir))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_NAME=%s", pkgInfo.Name))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DESC=%s", pkgInfo.Description))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_VERSION=%s", pkgInfo.Version))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_REVISION=%d", pkgInfo.Revision))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_URL=%s", pkgInfo.Url))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_ARCH=%s", pkgInfo.Arch))
	depends := make([]string, len(pkgInfo.Depends))
	copy(depends, pkgInfo.Depends)
	for i := 0; i < len(depends); i++ {
		depends[i] = fmt.Sprintf("\"%s\"", depends[i])
	}
	makeDepends := make([]string, len(pkgInfo.MakeDepends))
	copy(makeDepends, pkgInfo.MakeDepends)
	for i := 0; i < len(makeDepends); i++ {
		makeDepends[i] = fmt.Sprintf("\"%s\"", makeDepends[i])
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DEPENDS=(%s)", strings.Join(depends, " ")))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_MAKE_DEPENDS=(%s)", strings.Join(makeDepends, " ")))
	for _, value := range BPMConfig.CompilationEnv {
		cmd.Env = append(cmd.Env, value)
	}
	cmd.Env = append(cmd.Env, "BPM_PKG_TYPE=source")

	if !BPMConfig.SilentCompilation {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	err = cmd.Run()
	if err != nil {
		return err, nil
	}
	if _, err := os.Stat(path.Join(temp, "output/")); err != nil {
		if os.IsNotExist(err) {
			return errors.New("output directory not be found at " + path.Join(temp, "output/")), nil
		}
		return err, nil
	}
	if dir, _ := os.ReadDir(path.Join(temp, "output/")); len(dir) == 0 {
		return errors.New("output directory is empty"), nil
	}
	fmt.Println("Copying all files...")
	err = filepath.WalkDir(path.Join(temp, "/output/"), func(fullpath string, d fs.DirEntry, err error) error {
		relFilename, err := filepath.Rel(path.Join(temp, "/output/"), fullpath)
		if relFilename == "." {
			return nil
		}
		extractFilename := path.Join(rootDir, relFilename)
		if err != nil {
			return err
		}
		if d.Type() == os.ModeDir {
			files = append(files, relFilename+"/")
			if err := os.Mkdir(extractFilename, 0755); err != nil {
				if !os.IsExist(err) {
					return err
				}
			} else {
				if verbose {
					fmt.Println("Creating Directory: " + extractFilename)
				}
			}
		} else if d.Type().IsRegular() {
			if _, err := os.Stat(extractFilename); err == nil {
				if slices.Contains(pkgInfo.Keep, relFilename) {
					if verbose {
						fmt.Println("Skipping File: " + extractFilename + "(File is configured to be kept during installs/updates)")
					}
					files = append(files, relFilename)
					return nil
				}
			}
			err := os.Remove(extractFilename)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			outFile, err := os.Create(extractFilename)
			if verbose {
				fmt.Println("Creating File: " + extractFilename)
			}
			files = append(files, relFilename)
			if err != nil {
				return err
			}
			f, err := os.Open(fullpath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, f); err != nil {
				return err
			}
			info, err := os.Stat(fullpath)
			if err != nil {
				return err
			}
			if err := os.Chmod(extractFilename, info.Mode()); err != nil {
				return err
			}
			err = outFile.Close()
			if err != nil {
				return err
			}
			err = f.Close()
			if err != nil {
				return err
			}
		} else if d.Type() == os.ModeSymlink {
			link, err := os.Readlink(fullpath)
			if err != nil {
				return err
			}
			err = os.Remove(extractFilename)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if verbose {
				fmt.Println("Creating Symlink: "+extractFilename, " -> "+link)
			}
			files = append(files, relFilename)
			err = os.Symlink(link, extractFilename)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err, nil
	}
	if binaryPkgFromSrc {
		compiledDir := path.Join(BPMConfig.BinaryOutputDir)
		err = os.MkdirAll(compiledDir, 0755)
		compiledInfo := PackageInfo{}
		compiledInfo = *pkgInfo
		compiledInfo.Type = "binary"
		compiledInfo.Arch = GetArch()
		err = os.WriteFile(path.Join(temp, "pkg.info"), []byte(CreateInfoFile(&compiledInfo)), 0644)
		if err != nil {
			return err, nil
		}
		err = os.Chown(path.Join(temp, "pkg.info"), 65534, 65534)
		if err != nil {
			return err, nil
		}
		scripts, err := ReadPackageScripts(filename)
		for key, val := range scripts {
			err = os.WriteFile(path.Join(temp, key), []byte(val), 0644)
			if err != nil {
				return err, nil
			}
			err = os.Chown(path.Join(temp, key), 65534, 65534)
			if err != nil {
				return err, nil
			}
		}
		sed := fmt.Sprintf("s/output/files/")
		fileName := compiledInfo.Name + "-" + compiledInfo.GetFullVersion() + "-" + compiledInfo.Arch + ".bpm"
		cmd := exec.Command("/usr/bin/fakeroot", "-i fakeroot_file", "tar", "-czvpf", fileName, "pkg.info", "output/", "--transform", sed)
		if !BPMConfig.SilentCompilation {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: 65534, Gid: 65534}
		cmd.Dir = temp
		cmd.Env = os.Environ()
		fmt.Printf("running command: %s\n", strings.Join(cmd.Args, " "))
		err = cmd.Run()
		if err != nil {
			return err, nil
		}
		err = copyFileContents(path.Join(temp, fileName), path.Join(compiledDir, fileName))
		if err != nil {
			return err, nil
		}
		err = os.Chown(path.Join(compiledDir, fileName), 0, 0)
		if err != nil {
			return err, nil
		}
	}
	if !keepTempDir {
		err := os.RemoveAll(temp)
		if err != nil {
			return err, nil
		}
	}
	if len(files) == 0 {
		return errors.New("no output files for source package. Cancelling package installation"), nil
	}

	defer archive.Close()
	defer file.Close()
	return nil, files
}

func InstallPackage(filename, rootDir string, verbose, force, binaryPkgFromSrc, skipCheck, keepTempDir bool) error {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return err
	}
	var oldFiles []string
	var files []string
	pkgInfo, err := ReadPackage(filename)
	if err != nil {
		return err
	}
	packageInstalled := IsPackageInstalled(pkgInfo.Name, rootDir)
	if packageInstalled {
		oldFiles = GetPackageFiles(pkgInfo.Name, rootDir)
	}
	if !force {
		if pkgInfo.Arch != "any" && pkgInfo.Arch != GetArch() {
			return errors.New("cannot install a package with a different architecture")
		}
		if unresolved := pkgInfo.CheckDependencies(pkgInfo.Type == "source", true, rootDir); len(unresolved) != 0 {
			return errors.New("the following dependencies are not installed: " + strings.Join(unresolved, ", "))
		}
	}
	if pkgInfo.Type == "binary" {
		err, i := extractPackage(pkgInfo, verbose, filename, rootDir)
		if err != nil {
			return err
		}
		files = i
	} else if pkgInfo.Type == "source" {
		if isSplitPackage(filename) {
			return errors.New("BPM is unable to install split source packages")
		}
		err, i := compilePackage(pkgInfo, filename, rootDir, verbose, binaryPkgFromSrc, skipCheck, keepTempDir)
		if err != nil {
			return err
		}
		files = i
	} else {
		return errors.New("unknown package type: " + pkgInfo.Type)
	}
	slices.Sort(files)
	slices.Reverse(files)

	filesDiff := slices.DeleteFunc(oldFiles, func(f string) bool {
		return slices.Contains(files, f)
	})

	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	err = os.MkdirAll(installedDir, 0755)
	if err != nil {
		return err
	}
	pkgDir := path.Join(installedDir, pkgInfo.Name)
	err = os.RemoveAll(pkgDir)
	if err != nil {
		return err
	}
	err = os.Mkdir(pkgDir, 0755)
	if err != nil {
		return err
	}

	f, err := os.Create(path.Join(pkgDir, "files"))
	if err != nil {
		return err
	}
	for _, line := range files {
		_, err := f.WriteString(line + "\n")
		if err != nil {
			return err
		}
	}
	err = f.Close()
	if err != nil {
		return err
	}

	f, err = os.Create(path.Join(pkgDir, "info"))
	if err != nil {
		return err
	}
	raw, err := GetPackageInfoRaw(filename)
	if err != nil {
		return err
	}
	_, err = f.WriteString(raw)
	if err != nil {
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}

	scripts, err := ReadPackageScripts(filename)
	if err != nil {
		return err
	}
	if val, ok := scripts["post_remove.sh"]; ok {
		f, err = os.Create(path.Join(pkgDir, "post_remove.sh"))
		if err != nil {
			return err
		}
		_, err = f.WriteString(val)
		if err != nil {
			return err
		}
		err = f.Close()
		if err != nil {
			return err
		}
	}

	if len(filesDiff) != 0 {
		fmt.Println("Removing obsolete files...")
		var symlinks []string
		for _, f := range filesDiff {
			f = path.Join(rootDir, f)
			lstat, err := os.Lstat(f)
			if os.IsNotExist(err) {
				continue
			} else if err != nil {
				return err
			}
			if lstat.Mode()&os.ModeSymlink != 0 {
				symlinks = append(symlinks, f)
				continue
			}
			stat, err := os.Stat(f)
			if os.IsNotExist(err) {
				continue
			} else if err != nil {
				return err
			}
			if stat.IsDir() {
				dir, err := os.ReadDir(f)
				if err != nil {
					return err
				}
				if len(dir) == 0 {
					if verbose {
						fmt.Println("Removing: " + f)
					}
					err := os.Remove(f)
					if err != nil {
						return err
					}
				}
			} else {
				if verbose {
					fmt.Println("Removing: " + f)
				}
				err := os.Remove(f)
				if err != nil {
					return err
				}
			}
		}
		removals := -1
		for len(symlinks) > 0 && removals != 0 {
			removals = 0
			for i := len(symlinks) - 1; i >= 0; i-- {
				f := symlinks[i]
				f = path.Join(rootDir, f)
				_, err := os.Lstat(f)
				if os.IsNotExist(err) {
					continue
				} else if err != nil {
					return err
				}
				_, err = filepath.EvalSymlinks(f)
				if os.IsNotExist(err) {
					err := os.Remove(f)
					if err != nil {
						return err
					}
					removals++
					if verbose {
						fmt.Println("Removing: " + f)
					}
				} else if err != nil {
					return err
				}
			}
		}
	}
	if !packageInstalled {
		err = ExecutePackageScripts(filename, rootDir, Install, true)
		if err != nil {
			return err
		}
	} else {
		err = ExecutePackageScripts(filename, rootDir, Update, true)
		if err != nil {
			return err
		}
	}
	return nil
}

func GetSourceScript(filename string) (string, error) {
	pkgInfo, err := ReadPackage(filename)
	if err != nil {
		return "", err
	}
	if pkgInfo.Type != "source" {
		return "", errors.New("package not of source type")
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(archive)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if header.Name == "source.sh" {
			err := archive.Close()
			if err != nil {
				return "", err
			}
			err = file.Close()
			if err != nil {
				return "", err
			}
			bs, err := io.ReadAll(tr)
			if err != nil {
				return "", err
			}
			return string(bs), nil
		}
	}
	return "", errors.New("package does not contain a source.sh file")
}

func (pkgInfo *PackageInfo) GetAllDependencies(checkMake, checkOptional bool) []string {
	allDepends := make([]string, 0)
	allDepends = append(allDepends, pkgInfo.Depends...)
	if checkMake {
		allDepends = append(allDepends, pkgInfo.MakeDepends...)
	}
	if checkOptional {
		allDepends = append(allDepends, pkgInfo.OptionalDepends...)
	}
	return allDepends
}

func (pkgInfo *PackageInfo) CheckDependencies(checkMake, checkOptional bool, rootDir string) []string {
	var ret []string
	for _, dependency := range pkgInfo.Depends {
		if !IsPackageProvided(dependency, rootDir) {
			ret = append(ret, dependency)
		}
	}
	if checkMake {
		for _, dependency := range pkgInfo.MakeDepends {
			if !IsPackageProvided(dependency, rootDir) {
				ret = append(ret, dependency)
			}
		}
	}
	if checkOptional {
		for _, dependency := range pkgInfo.OptionalDepends {
			if !IsPackageProvided(dependency, rootDir) {
				ret = append(ret, dependency)
			}
		}
	}

	return ret
}

func (pkgInfo *PackageInfo) CheckConflicts(rootDir string) []string {
	var ret []string
	for _, conflict := range pkgInfo.Conflicts {
		if IsPackageInstalled(conflict, rootDir) {
			ret = append(ret, conflict)
		}
	}
	return ret
}

func (pkgInfo *PackageInfo) ResolveAll(resolved, unresolved *[]string, checkMake, checkOptional, ignoreInstalled bool, rootDir string) ([]string, []string) {
	*unresolved = append(*unresolved, pkgInfo.Name)
	for _, depend := range pkgInfo.GetAllDependencies(checkMake, checkOptional) {
		if !slices.Contains(*resolved, depend) {
			if slices.Contains(*unresolved, depend) || (ignoreInstalled && IsPackageInstalled(depend, rootDir)) {
				continue
			}
			entry, _, err := GetRepositoryEntry(depend)
			if err != nil {
				if !slices.Contains(*unresolved, depend) {
					*unresolved = append(*unresolved, depend)
				}
				continue
			}
			entry.Info.ResolveAll(resolved, unresolved, checkMake, checkOptional, ignoreInstalled, rootDir)
		}
	}
	if !slices.Contains(*resolved, pkgInfo.Name) {
		*resolved = append(*resolved, pkgInfo.Name)
	}
	*unresolved = stringSliceRemove(*unresolved, pkgInfo.Name)
	return *resolved, *unresolved
}

func IsPackageInstalled(pkg, rootDir string) bool {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	if _, err := os.Stat(pkgDir); err != nil {
		return false
	}
	return true
}

func IsPackageProvided(pkg, rootDir string) bool {
	pkgs, err := GetInstalledPackages(rootDir)
	if err != nil {
		return false
	}
	for _, p := range pkgs {
		if p == pkg {
			return true
		}
		i := GetPackageInfo(p, rootDir, true)
		if i == nil {
			continue
		}
		if slices.Contains(i.Provides, pkg) {
			return true
		}
	}
	return false
}

func GetInstalledPackages(rootDir string) ([]string, error) {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	items, err := os.ReadDir(installedDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ret []string
	for _, item := range items {
		ret = append(ret, item.Name())
	}
	return ret, nil
}

func GetPackageFiles(pkg, rootDir string) []string {
	var ret []string
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	files := path.Join(pkgDir, "files")
	if _, err := os.Stat(installedDir); os.IsNotExist(err) {
		return nil
	}
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		return nil
	}
	file, err := os.Open(files)
	if err != nil {
		return nil
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ret = append(ret, scanner.Text())
	}
	return ret
}

func GetPackageInfo(pkg, rootDir string, defaultValues bool) *PackageInfo {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	files := path.Join(pkgDir, "info")
	if _, err := os.Stat(installedDir); os.IsNotExist(err) {
		return nil
	}
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		return nil
	}
	file, err := os.Open(files)
	if err != nil {
		return nil
	}
	bs, err := io.ReadAll(file)
	if err != nil {
		return nil
	}
	info, err := ReadPackageInfo(string(bs))
	if err != nil {
		return nil
	}
	return info
}

func RemovePackage(pkg string, verbose bool, rootDir string) error {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	pkgInfo := GetPackageInfo(pkg, rootDir, false)
	if pkgInfo == nil {
		return errors.New("could not get package info")
	}
	files := GetPackageFiles(pkg, rootDir)
	var symlinks []string
	for _, file := range files {
		file = path.Join(rootDir, file)
		lstat, err := os.Lstat(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if lstat.Mode()&os.ModeSymlink != 0 {
			symlinks = append(symlinks, file)
			continue
		}
		stat, err := os.Stat(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if stat.IsDir() {
			dir, err := os.ReadDir(file)
			if err != nil {
				return err
			}
			if len(dir) == 0 {
				if verbose {
					fmt.Println("Removing: " + file)
				}
				err := os.Remove(file)
				if err != nil {
					return err
				}
			}
		} else {
			if verbose {
				fmt.Println("Removing: " + file)
			}
			err := os.Remove(file)
			if err != nil {
				return err
			}
		}
	}
	removals := -1
	for len(symlinks) > 0 && removals != 0 {
		removals = 0
		for i := len(symlinks) - 1; i >= 0; i-- {
			file := symlinks[i]
			file = path.Join(rootDir, file)
			_, err := os.Lstat(file)
			if os.IsNotExist(err) {
				continue
			} else if err != nil {
				return err
			}
			_, err = filepath.EvalSymlinks(file)
			if os.IsNotExist(err) {
				err := os.Remove(file)
				if err != nil {
					return err
				}
				removals++
				if verbose {
					fmt.Println("Removing: " + file)
				}
			} else if err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(path.Join(pkgDir, "post_remove.sh")); err == nil {
		cmd := exec.Command("/bin/bash", path.Join(pkgDir, "post_remove.sh"))
		if !BPMConfig.SilentCompilation {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.Dir = rootDir
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_ROOT=%s", rootDir))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_NAME=%s", pkgInfo.Name))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DESC=%s", pkgInfo.Description))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_VERSION=%s", pkgInfo.Version))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_REVISION=%d", pkgInfo.Revision))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_URL=%s", pkgInfo.Url))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_ARCH=%s", pkgInfo.Arch))
		depends := make([]string, len(pkgInfo.Depends))
		copy(depends, pkgInfo.Depends)
		for i := 0; i < len(depends); i++ {
			depends[i] = fmt.Sprintf("\"%s\"", depends[i])
		}
		makeDepends := make([]string, len(pkgInfo.MakeDepends))
		copy(makeDepends, pkgInfo.MakeDepends)
		for i := 0; i < len(makeDepends); i++ {
			makeDepends[i] = fmt.Sprintf("\"%s\"", makeDepends[i])
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DEPENDS=(%s)", strings.Join(depends, " ")))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_MAKE_DEPENDS=(%s)", strings.Join(makeDepends, " ")))
		cmd.Env = append(cmd.Env, "BPM_PKG_TYPE=source")
		err = cmd.Run()
		if err != nil {
			return err
		}
	}
	err := os.RemoveAll(pkgDir)
	if err != nil {
		return err
	}
	if verbose {
		fmt.Println("Removing: " + pkgDir)
	}
	return nil
}
