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
	Repositories      []*Repository `yaml:"repositories"`
}

var BPMConfig BPMConfigStruct = BPMConfigStruct{
	CompilationEnv:    make([]string, 0),
	SilentCompilation: false,
	BinaryOutputDir:   "/var/lib/bpm/compiled/",
	CompilationDir:    "/var/tmp/",
}

func ReadConfig() {
	if _, err := os.Stat("/etc/bpm.conf"); os.IsNotExist(err) {
		log.Fatal(err)
	}
	bytes, err := os.ReadFile("/etc/bpm.conf")
	if err != nil {
		log.Fatal(err)
	}
	err = yaml.Unmarshal(bytes, &BPMConfig)
	if err != nil {
		log.Fatal(err)
	}
	for _, repo := range BPMConfig.Repositories {
		repo.Entries = make(map[string]*RepositoryEntry)
		err := repo.ReadLocalDatabase()
		if err != nil {
			log.Fatal(err)
		}
	}
}
