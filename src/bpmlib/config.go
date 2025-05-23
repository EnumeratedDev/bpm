package bpmlib

import (
	"gopkg.in/yaml.v3"
	"os"
)

type BPMConfigStruct struct {
	IgnorePackages          []string       `yaml:"ignore_packages"`
	PrivilegeEscalatorCmd   string         `yaml:"privilege_escalator_cmd"`
	CompilationEnvironment  []string       `yaml:"compilation_env"`
	CleanupMakeDependencies bool           `yaml:"cleanup_make_dependencies"`
	Databases               []*BPMDatabase `yaml:"databases"`
}

var BPMConfig BPMConfigStruct

func ReadConfig() (err error) {
	if _, err = os.Stat("/etc/bpm.conf"); os.IsNotExist(err) {
		return err
	}

	bytes, err := os.ReadFile("/etc/bpm.conf")
	if err != nil {
		return err
	}

	BPMConfig = BPMConfigStruct{
		CleanupMakeDependencies: true,
	}
	err = yaml.Unmarshal(bytes, &BPMConfig)
	if err != nil {
		return err
	}

	for i := len(BPMConfig.Databases) - 1; i >= 0; i-- {
		if BPMConfig.Databases[i].Disabled != nil && *BPMConfig.Databases[i].Disabled {
			BPMConfig.Databases = append(BPMConfig.Databases[:i], BPMConfig.Databases[i+1:]...)
		}
	}

	return nil
}
