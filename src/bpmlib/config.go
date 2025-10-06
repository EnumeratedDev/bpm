package bpmlib

import (
	"os"

	"gopkg.in/yaml.v3"
)

type MainBPMConfigStruct struct {
	IgnorePackages            []string         `yaml:"ignore_packages"`
	ShowSourcePackageContents string           `yaml:"show_source_package_contents"`
	CleanupMakeDependencies   bool             `yaml:"cleanup_make_dependencies"`
	Databases                 []configDatabase `yaml:"databases"`
}

type configDatabase struct {
	Name     string `yaml:"name"`
	Source   string `yaml:"source"`
	Disabled *bool  `yaml:"disabled"`
}

type CompilationBPMConfigStruct struct {
	PrivilegeEscalatorCmd  string   `yaml:"privilege_escalator_cmd"`
	CompilationEnvironment []string `yaml:"compilation_env"`
}

var MainBPMConfig MainBPMConfigStruct
var CompilationBPMConfig CompilationBPMConfigStruct

func ReadConfig() (err error) {
	var file *os.File

	// Set default config options
	MainBPMConfig = MainBPMConfigStruct{
		ShowSourcePackageContents: "always",
		CleanupMakeDependencies:   true,
	}

	// Read main BPM config
	file, err = os.Open("/etc/bpm.conf")
	if err != nil {
		return err
	}
	err = yaml.NewDecoder(file).Decode(&MainBPMConfig)
	if err != nil {
		return err
	}
	file.Close()

	// Read compilation BPM config
	if _, err := os.Stat("/etc/bpm-compilation.conf"); err == nil {
		file, err = os.Open("/etc/bpm-compilation.conf")
		if err != nil {
			return err
		}
		err = yaml.NewDecoder(file).Decode(&CompilationBPMConfig)
		if err != nil {
			return err
		}
		file.Close()
	}

	// Remove disabled databases from memory
	for i := len(MainBPMConfig.Databases) - 1; i >= 0; i-- {
		if MainBPMConfig.Databases[i].Disabled != nil && *MainBPMConfig.Databases[i].Disabled {
			MainBPMConfig.Databases = append(MainBPMConfig.Databases[:i], MainBPMConfig.Databases[i+1:]...)
		}
	}

	return nil
}
