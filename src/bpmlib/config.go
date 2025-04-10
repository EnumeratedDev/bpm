package bpmlib

import (
	"gopkg.in/yaml.v3"
	"os"
)

type BPMConfigStruct struct {
	IgnorePackages []string      `yaml:"ignore_packages"`
	Repositories   []*Repository `yaml:"repositories"`
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

	BPMConfig = BPMConfigStruct{}
	err = yaml.Unmarshal(bytes, &BPMConfig)
	if err != nil {
		return err
	}

	for i := len(BPMConfig.Repositories) - 1; i >= 0; i-- {
		if BPMConfig.Repositories[i].Disabled != nil && *BPMConfig.Repositories[i].Disabled {
			BPMConfig.Repositories = append(BPMConfig.Repositories[:i], BPMConfig.Repositories[i+1:]...)
		}
	}

	for _, repo := range BPMConfig.Repositories {
		repo.Entries = make(map[string]*RepositoryEntry)
		repo.VirtualPackages = make(map[string][]string)
		err := repo.ReadLocalDatabase()
		if err != nil {
			return err
		}
	}

	return nil
}
