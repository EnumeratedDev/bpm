package bpmlib

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

func CompileSourcePackage(archiveFilename, outputFilename string, skipChecks bool) (err error) {
	// Read BPM archive
	bpmpkg, err := ReadPackage(archiveFilename)
	if err != nil {
		return err
	}

	// Ensure package type is 'source'
	if bpmpkg.PkgInfo.Type != "source" {
		return errors.New("cannot compile a non-source package")
	}

	// Read compilation options file in current directory
	compilationOptions, err := readCompilationOptionsFile()
	if err != nil {
		return err
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
	err = extractTarballFile(archiveFilename, "source.sh", tempDirectory)
	if err != nil {
		return err
	}

	// Get package scripts and extract them
	packageScripts := getPackageScripts(archiveFilename)
	for _, script := range packageScripts {
		err = extractTarballFile(archiveFilename, script, tempDirectory)
		if err != nil {
			return err
		}
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
	// Check for architecture override in compilation options
	if val, ok := compilationOptions["ARCH"]; ok {
		env = append(env, "BPM_PKG_ARCH="+val)
	} else {
		env = append(env, "BPM_PKG_ARCH="+GetArch())
	}
	env = append(env, BPMConfig.CompilationEnvironment...)

	// Execute prepare and build functions in source.sh script
	cmd := exec.Command("bash", "-c",
		"set -a\n"+ // Source and export functions and variables in source.sh script
			". \"${BPM_WORKDIR}\"/source.sh\n"+
			"set +a\n"+
			"[[ $(type -t prepare) == \"function\" ]] && { echo \"Running prepare() function\"; bash -e -c 'cd \"$BPM_SOURCE\" && prepare' || exit 1; }\n"+ // Run prepare() function if it exists
			"[[ $(type -t build) == \"function\" ]] && { echo \"Running build() function\"; bash -e -c 'cd \"$BPM_SOURCE\" && build'  || exit 1; }\n"+ // Run build() function if it exists
			"exit 0")
	cmd.Dir = tempDirectory
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	err = cmd.Run()
	if err != nil {
		return err
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
		err = cmd.Run()
		if err != nil {
			return err
		}
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

	// Execute package function in source.sh script and generate package file list
	cmd = exec.Command("bash", "-c",
		"set -a\n"+ // Source and export functions and variables in source.sh script
			". \"${BPM_WORKDIR}\"/source.sh\n"+
			"set +a\n"+
			"echo \"Running package() function\"\n"+
			"( cd \"$BPM_SOURCE\" && fakeroot -s \"$BPM_WORKDIR\"/fakeroot_file bash -e -c 'package' ) || exit 1\n"+ // Run package() function
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

	// Copy pkgInfo struct
	pkgInfo := bpmpkg.PkgInfo

	// Set package type to binary
	pkgInfo.Type = "binary"

	// Set package architecture
	if val, ok := compilationOptions["ARCH"]; ok {
		pkgInfo.Arch = val
	} else {
		pkgInfo.Arch = GetArch()
	}

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

	// Get files to include in BPM archive
	bpmArchiveFiles := make([]string, 0)
	bpmArchiveFiles = append(bpmArchiveFiles, "pkg.info", "pkg.files", "files.tar.gz") // Base files
	bpmArchiveFiles = append(bpmArchiveFiles, packageScripts...)                       // Package scripts

	// Create final BPM archive
	cmd = exec.Command("bash", "-c", "tar -cf "+outputFilename+" --owner=0 --group=0 -C \"$BPM_WORKDIR\" "+strings.Join(bpmArchiveFiles, " "))
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

func readCompilationOptionsFile() (options map[string]string, err error) {
	// Initialize options map
	options = make(map[string]string)

	// Check if file compilation options file exists
	stat, err := os.Stat(".compilation-options")
	if err != nil {
		return nil, nil
	}

	// Ensure it is a regular file
	if !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", stat.Name())
	}

	// Read file data
	data, err := os.ReadFile(stat.Name())
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		// Trim line
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Split line
		split := strings.SplitN(line, "=", 2)

		// Throw error if line isn't valid
		if len(split) < 2 {
			return nil, fmt.Errorf("invalid line in compilation-options file: '%s'", line)
		}

		options[split[0]] = split[1]
	}

	return options, nil
}
