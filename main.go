package main

import (
	"fmt"
	"log"
)

/* ---BPM | Bubble Package Manager--- */
/*        Made By CapCreeperGR        */
/*   A simple-to-use package manager  */
/* ---------------------------------- */

var rootDir string = "/"

func main() {
	fmt.Printf("Running %s %s\n", getKernel(), getArch())
	//_, err := readPackage("test_hello_package/hello.bpm")
	err := installPackage("test_hello_package/hello.bpm", rootDir)
	if err != nil {
		log.Fatalf("Could not read package\nError: %s\n", err)
	}
	pkgs, err := getInstalledPackages(rootDir)
	if err != nil {
		log.Fatalf("Could not get installed Packages!\nError: %s\n", err)
	}
	for _, pkg := range pkgs {
		fmt.Println(pkg)
	}
	fmt.Println(getPackageFiles("hello"))
}
