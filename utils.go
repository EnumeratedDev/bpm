package main

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

func byteArrayToString(bs []int8) string {
	b := make([]byte, len(bs))
	for i, v := range bs {
		b[i] = byte(v)
	}
	return string(b)
}
