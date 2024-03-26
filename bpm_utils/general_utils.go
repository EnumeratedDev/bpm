package bpm_utils

import (
	"os/exec"
	"strings"
)

func GetArch() string {
	output, err := exec.Command("/usr/bin/uname", "-m").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(byteArrayToString(output))
}

func stringSliceRemove(s []string, r string) []string {
	for i, v := range s {
		if v == r {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

func stringSliceRemoveEmpty(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}

func byteArrayToString(bs []byte) string {
	b := make([]byte, len(bs))
	for i, v := range bs {
		b[i] = v
	}
	return string(b)
}
