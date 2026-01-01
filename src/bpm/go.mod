module bpm

go 1.24.0

require (
	github.com/EnumeratedDev/bpm/src/bpmlib v0.0.0
	github.com/lithammer/fuzzysearch v1.1.8
	github.com/spf13/pflag v1.0.10
)

replace github.com/EnumeratedDev/bpm/src/bpmlib => ../bpmlib

require (
	github.com/drone/envsubst v1.0.3 // indirect
	github.com/knqyf263/go-rpm-version v0.0.0-20240918084003-2afd7dc6a38f // indirect
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/schollz/progressbar/v3 v3.18.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/term v0.36.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
