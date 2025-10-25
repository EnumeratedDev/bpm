module git.enumerated.dev/bubble-package-manager/bpm/src/bpm

go 1.23

toolchain go1.23.7

require (
	git.enumerated.dev/bubble-package-manager/bpm/src/bpmlib v0.5.0
	github.com/lithammer/fuzzysearch v1.1.8
	github.com/spf13/pflag v1.0.10
)

replace git.enumerated.dev/bubble-package-manager/bpm/src/bpmlib => ../bpmlib

require (
	github.com/drone/envsubst v1.0.3 // indirect
	github.com/knqyf263/go-rpm-version v0.0.0-20240918084003-2afd7dc6a38f // indirect
	golang.org/x/text v0.9.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
