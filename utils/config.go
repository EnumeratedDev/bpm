package utils

import (
	"gopkg.in/yaml.v3"
	"os"
)

type BPMConfigStruct struct {
	CompilationEnv    []string `yaml:"compilation_env"`
	SilentCompilation bool     `yaml:"silent_compilation"`
	BinaryOutputDir   string   `yaml:"binary_output_dir"`
	CompilationDir    string   `yaml:"compilation_dir"`
}

var BPMConfig BPMConfigStruct = BPMConfigStruct{
	CompilationEnv:    make([]string, 0),
	SilentCompilation: false,
	BinaryOutputDir:   "/var/lib/bpm/compiled/",
	CompilationDir:    "/var/tmp/",
}

func ReadConfig() {
	if _, err := os.Stat("/etc/bpm.conf"); os.IsNotExist(err) {
		return
	}
	bytes, err := os.ReadFile("/etc/bpm.conf")
	if err != nil {
		return
	}
	err = yaml.Unmarshal(bytes, &BPMConfig)
	if err != nil {
		return
	}
}
