package bpm_utils

import "syscall"

func getArch() string {
	u := syscall.Utsname{}
	err := syscall.Uname(&u)
	if err != nil {
		return "unknown"
	}
	return byteArrayToString(u.Machine[:])
}

func getKernel() string {
	u := syscall.Utsname{}
	err := syscall.Uname(&u)
	if err != nil {
		return "unknown"
	}
	return byteArrayToString(u.Sysname[:]) + " " + byteArrayToString(u.Release[:])
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

func byteArrayToString(bs []int8) string {
	b := make([]byte, len(bs))
	for i, v := range bs {
		b[i] = byte(v)
	}
	return string(b)
}
