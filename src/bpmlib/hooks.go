package bpmlib

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"
)

type BPMHook struct {
	SourcePath          string
	SourceContent       string
	TriggerActions      []string `yaml:"trigger_actions"`
	TriggerPreOperation bool     `yaml:"trigger_pre_operation"`
	Targets             []string `yaml:"targets"`
	Run                 string   `yaml:"run"`
	PassTargets         bool     `yaml:"pass_targets"`
}

// createHook returns a BPMHook instance based on the content of the given string
func createHook(sourcePath string) (*BPMHook, error) {
	// Read hook from source path
	bytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, err
	}

	// Create base hook structure
	hook := &BPMHook{
		SourcePath:     sourcePath,
		SourceContent:  string(bytes),
		TriggerActions: nil,
		Targets:        nil,
		Run:            "",
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
	if len(hook.TriggerActions) == 0 {
		return errors.New("no trigger operations specified")
	}
	for _, operation := range hook.TriggerActions {
		if !slices.Contains(ValidOperations, operation) {
			return errors.New("trigger operation '" + operation + "' is not valid")
		}
	}

	if len(hook.Run) == 0 {
		return errors.New("command to run is empty")
	}

	// Return nil as hook is valid
	return nil
}

// Execute hook if all conditions are met
func (hook *BPMHook) Execute(modifiedFiles map[string]string, preOperation bool, verbose bool, rootDir string) error {
	// Check if any targets are met
	targetsMet := make([]string, 0)
	for _, target := range hook.Targets {
		for modifiedFile, action := range modifiedFiles {
			// Check if this hook is triggered by this file's action
			if !slices.Contains(hook.TriggerActions, action) {
				continue
			}

			// Check if file has already been checked
			if slices.Contains(targetsMet, modifiedFile) {
				continue
			}

			if matched, _ := filepath.Match(target, modifiedFile); !matched {
				continue
			}

			targetsMet = append(targetsMet, modifiedFile)
		}
	}

	if len(targetsMet) == 0 {
		return nil
	}

	// Execute the command
	splitCommand := strings.Split(hook.Run, " ")
	cmd := exec.Command(splitCommand[0], splitCommand[1:]...)
	// Setup subprocess environment
	cmd.Dir = "/"
	// Pass targets
	if hook.PassTargets {
		buffer := bytes.Buffer{}
		buffer.WriteString(strings.Join(targetsMet, "\n") + "\n")

		cmd.Stdin = &buffer
	}
	// Run hook in chroot if using the -R flag
	if rootDir != "/" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: rootDir}
	}

	if !verbose {
		fmt.Printf("Running hook (%s)\n", filepath.Base(hook.SourcePath))
	} else {
		fmt.Printf("Running hook (%s) with run command: %s\n", filepath.Base(hook.SourcePath), strings.Join(splitCommand, " "))
	}

	err := cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
