package utils

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

type BPMHook struct {
	SourcePath        string
	SourceContent     string
	TriggerOperations []string `yaml:"trigger_operations"`
	TargetType        string   `yaml:"target_type"`
	Targets           []string `yaml:"targets"`
	Depends           []string `yaml:"depends"`
	Run               string   `yaml:"run"`
}

// CreateHook returns a BPMHook instance based on the content of the given string
func CreateHook(sourcePath string) (*BPMHook, error) {
	// Read hook from source path
	bytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, err
	}

	// Create base hook structure
	hook := &BPMHook{
		SourcePath:        sourcePath,
		SourceContent:     string(bytes),
		TriggerOperations: nil,
		TargetType:        "",
		Targets:           nil,
		Depends:           nil,
		Run:               "",
	}

	// Unmarshal yaml string
	err = yaml.Unmarshal(bytes, hook)
	if err != nil {
		return nil, err
	}

	// Ensure hook is valid
	if err := hook.IsValid(); err != nil {
		return nil, err
	}

	return hook, nil
}

// IsValid ensures hook is valid
func (hook *BPMHook) IsValid() error {
	ValidOperations := []string{"install", "upgrade", "remove"}

	// Return error if any trigger operation is not valid or none are given
	if len(hook.TriggerOperations) == 0 {
		return errors.New("no trigger operations specified")
	}
	for _, operation := range hook.TriggerOperations {
		if !slices.Contains(ValidOperations, operation) {
			return errors.New("trigger operation '" + operation + "' is not valid")
		}
	}

	if hook.TargetType != "package" && hook.TargetType != "path" {
		return errors.New("target type '" + hook.TargetType + "' is not valid")
	}

	if len(hook.Run) == 0 {
		return errors.New("command to run is empty")
	}

	// Return nil as hook is valid
	return nil
}

// Execute hook if all conditions are met
func (hook *BPMHook) Execute(packageChanges map[string]string, verbose bool, rootDir string) error {
	// Check if package dependencies are met
	installedPackages, err := GetInstalledPackages(rootDir)
	if err != nil {
		return err
	}

	for _, depend := range hook.Depends {
		if !slices.Contains(installedPackages, depend) {
			return nil
		}
	}

	// Get modified files slice
	modifiedFiles := make([]*PackageFileEntry, 0)
	for pkg, _ := range packageChanges {
		modifiedFiles = append(modifiedFiles, GetPackageFiles(pkg, rootDir)...)
	}

	// Check if any targets are met
	targetMet := false
	for _, target := range hook.Targets {
		if targetMet {
			break
		}
		if hook.TargetType == "package" {
			for change, operation := range packageChanges {
				if target == change && slices.Contains(hook.TriggerOperations, operation) {
					targetMet = true
					break
				}
			}
		} else {
			glob, err := filepath.Glob(path.Join(rootDir, target))
			if err != nil {
				return err
			}
			for _, change := range modifiedFiles {
				if slices.Contains(glob, path.Join(rootDir, change.Path)) {
					targetMet = true
					break
				}
			}
		}
	}
	if !targetMet {
		return nil
	}

	// Execute the command
	splitCommand := strings.Split(hook.Run, " ")
	cmd := exec.Command(splitCommand[0], splitCommand[1:]...)
	// Setup subprocess environment
	cmd.Dir = "/"
	// Run hook in chroot if using the -R flag
	if rootDir != "/" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: rootDir}
	}

	if verbose {
		fmt.Printf("Running hook (%s) with run command: %s\n", hook.SourcePath, strings.Join(splitCommand, " "))
	}

	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
