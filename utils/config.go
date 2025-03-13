package utils

import (
	"gopkg.in/yaml.v3"
	"log"
	"os"
)

type BPMConfigStruct struct {
	CompilationEnv    []string      `yaml:"compilation_env"`
	SilentCompilation bool          `yaml:"silent_compilation"`
	BinaryOutputDir   string        `yaml:"binary_output_dir"`
	CompilationDir    string        `yaml:"compilation_dir"`
	IgnorePackages    []string      `yaml:"ignore_packages"`
	Repositories      []*Repository `yaml:"repositories"`
}

var BPMConfig BPMConfigStruct

func ReadConfig() {
	if _, err := os.Stat("/etc/bpm.conf"); os.IsNotExist(err) {
		log.Fatal(err)
	}
	bytes, err := os.ReadFile("/etc/bpm.conf")
	if err != nil {
		log.Fatal(err)
	}
	BPMConfig = BPMConfigStruct{
		CompilationEnv:    make([]string, 0),
		SilentCompilation: false,
		BinaryOutputDir:   "/var/lib/bpm/compiled/",
		CompilationDir:    "/var/tmp/",
	}
	err = yaml.Unmarshal(bytes, &BPMConfig)
	if err != nil {
		log.Fatal(err)
	}
	for i := len(BPMConfig.Repositories) - 1; i >= 0; i-- {
		if BPMConfig.Repositories[i].Disabled != nil && *BPMConfig.Repositories[i].Disabled {
			BPMConfig.Repositories = append(BPMConfig.Repositories[:i], BPMConfig.Repositories[i+1:]...)
		}
	}
	for _, repo := range BPMConfig.Repositories {
		repo.Entries = make(map[string]*RepositoryEntry)
		err := repo.ReadLocalDatabase()
		if err != nil {
			log.Fatal(err)
		}
	}
}
