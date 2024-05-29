package bpm_utils

import (
	"gopkg.in/yaml.v3"
	"os"
)

type BPMConfigStruct struct {
	CompilationEnv    []string `yaml:"compilation_env"`
	SilentCompilation bool     `yaml:"silent_compilation"`
}

var BPMConfig BPMConfigStruct = BPMConfigStruct{
	CompilationEnv:    make([]string, 0),
	SilentCompilation: false}

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
