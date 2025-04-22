package bpmlib

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
)

func CompileSourcePackage(archiveFilename, outputFilename string) (err error) {
	// Read BPM archive
	bpmpkg, err := ReadPackage(archiveFilename)
	if err != nil {
		return err
	}

	// Ensure package type is 'source'
	if bpmpkg.PkgInfo.Type != "source" {
		return errors.New("cannot compile a non-source package")
	}

	// Get HOME directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	tempDirectory := path.Join(homeDir, ".cache/bpm/compilation/", bpmpkg.PkgInfo.Name)

	// Ensure temporary directory does not exist
	if _, err := os.Stat(tempDirectory); err == nil {
		err := os.RemoveAll(tempDirectory)
		if err != nil {
			return err
		}
	}

	// Create temporary directory
	err = os.MkdirAll(tempDirectory, 0755)
	if err != nil {
		return err
	}

	// Extract source.sh file
	content, err := readTarballContent(archiveFilename, "source.sh")
	if err != nil {
		return err
	}
	sourceFile, err := os.Create(path.Join(tempDirectory, "source.sh"))
	if err != nil {
		return err
	}
	_, err = io.Copy(sourceFile, content.tarReader)
	if err != nil {
		return err
	}
	err = sourceFile.Close()
	if err != nil {
		return err
	}
	err = content.file.Close()
	if err != nil {
		return err
	}

	// Extract source files
	err = extractTarballDirectory(archiveFilename, "source-files", tempDirectory)
	if err != nil {
		return err
	}

	// Create source directory
	err = os.Mkdir(path.Join(tempDirectory, "source"), 0755)
	if err != nil {
		return err
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
	env = append(env, "BPM_PKG_ARCH="+GetArch())
	env = append(env, BPMConfig.CompilationEnvironment...)

	// Execute prepare and build functions in source.sh script
	cmd := exec.Command("bash", "-c",
		"set -a\n"+ // Source and export functions and variables in source.sh script
			". \"${BPM_WORKDIR}\"/source.sh\n"+
			"set +a\n"+
			"[[ $(type -t prepare) == \"function\" ]] && (echo \"Running prepare() function\" && cd \"$BPM_SOURCE\" && set -e && prepare)\n"+ // Run prepare() function if it exists
			"[[ $(type -t build) == \"function\" ]] && (echo \"Running build() function\" && cd \"$BPM_SOURCE\" && set -e && build)\n"+ // Run build() function if it exists
			"[[ $(type -t check) == \"function\" ]] && (echo \"Running check() function\" && cd \"$BPM_SOURCE\" && set -e && check)\n"+ // Run check() function if it exists
			"exit 0")
	cmd.Dir = tempDirectory
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	err = cmd.Run()
	if err != nil {
		return err
	}

	// Remove 'output' directory if it already exists
	if _, err := os.Stat(path.Join(tempDirectory, "output")); err == nil {
		err := os.RemoveAll(path.Join(tempDirectory, "output"))
		if err != nil {
			return err
		}
	}

	// Create new 'output' directory
	err = os.Mkdir(path.Join(tempDirectory, "output"), 0755)
	if err != nil {
		return err
	}

	// Run bash command
	cmd = exec.Command("bash", "-e", "-c",
		"set -a\n"+ // Source and export functions and variables in source.sh script
			". \"${BPM_WORKDIR}\"/source.sh\n"+
			"set +a\n"+
			"(echo \"Running package() function\" && cd \"$BPM_SOURCE\" && fakeroot -s \"$BPM_WORKDIR\"/fakeroot_file package)\n"+ // Run package() function
			"fakeroot -i \"$BPM_WORKDIR\"/fakeroot_file find \"$BPM_OUTPUT\" -mindepth 1 -printf \"%P %#m %U %G %s\\n\" > \"$BPM_WORKDIR\"/pkg.files") // Create package file list
	cmd.Dir = tempDirectory
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	err = cmd.Run()
	if err != nil {
		return err
	}

	// Create gzip-compressed archive for the package files
	cmd = exec.Command("bash", "-c", "find output -printf \"%P\\n\" | fakeroot -i \"$BPM_WORKDIR\"/fakeroot_file tar -czf files.tar.gz --no-recursion -C output -T -")
	cmd.Dir = tempDirectory
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("files.tar.gz archive could not be created: %s", err)
	}

	// Copy pkgInfo struct and set package type to binary
	pkgInfo := bpmpkg.PkgInfo
	pkgInfo.Type = "binary"

	// Marshal package info
	pkgInfoBytes, err := yaml.Marshal(pkgInfo)
	if err != nil {
		return err
	}
	pkgInfoBytes = append(pkgInfoBytes, '\n')

	// Create pkg.info file
	err = os.WriteFile(path.Join(tempDirectory, "pkg.info"), pkgInfoBytes, 0644)
	if err != nil {
		return err
	}

	// Create final BPM archive
	cmd = exec.Command("bash", "-c", "tar -cf "+outputFilename+" --owner=0 --group=0 -C \"$BPM_WORKDIR\" pkg.info pkg.files ${PACKAGE_SCRIPTS[@]} files.tar.gz")
	cmd.Dir = tempDirectory
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	cmd.Env = append(env, "CURRENT_DIR="+currentDir)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("BPM archive could not be created: %s", err)
	}

	return nil
}
