package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

/* ---BPM | Bubble Package Manager--- */
/*        Made By CapCreeperGR        */
/*   A simple-to-use package manager  */
/* ---------------------------------- */

var bpmVer = "0.0.1"
var rootDir string = "/"

func main() {
	fmt.Printf("Running %s %s\n", getKernel(), getArch())
	resolveCommand()
	/*_, err := readPackage("test_hello_package/hello.bpm")
	err := installPackage("test_hello_package/hello.bpm", rootDir)
	if err != nil {
		log.Fatalf("Could not read package\nError: %s\n", err)
	}
	removePackage("hello")*/
}

func getArgs() []string {
	return os.Args[1:]
}

type commandType uint8

const (
	help commandType = iota
	version
	info
	list
	install
	remove
	cleanup
)

func getCommandType() commandType {
	if len(getArgs()) == 0 {
		return help
	}
	cmd := getArgs()[0]
	switch cmd {
	case "version":
		return version
	case "info":
		return info
	case "list":
		return list
	case "install":
		return install
	case "remove":
		return remove
	case "cleanup":
		return cleanup
	default:
		return help
	}
}

func resolveCommand() {
	switch getCommandType() {
	case version:
		fmt.Println("Bubble Package Manager (BPM)")
		fmt.Println("Version: " + bpmVer)
	case info:
		packages := getArgs()[1:]
		if len(packages) == 0 {
			fmt.Println("No packages were given")
			return
		}
		for n, pkg := range packages {
			info := getPackageInfo(pkg)
			if info == nil {
				fmt.Printf("Package (%s) could not be found\n", pkg)
				continue
			}
			fmt.Print("---------------\n" + createInfoFile(*info))
			if n == len(packages)-1 {
				fmt.Println("---------------")
			}
		}
	case install:
		files := getArgs()[1:]
		if len(files) == 0 {
			fmt.Println("No files were given to install")
			return
		}
		for _, file := range files {
			pkgInfo, err := readPackage(file)
			if err != nil {
				log.Fatalf("Could not read package\nError: %s\n", err)
			}
			fmt.Print("---------------\n" + createInfoFile(*pkgInfo))
			fmt.Println("---------------")
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Do you wish to install this package? [y\\N] ")
			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Printf("Skipping package (%s)...\n", pkgInfo.name)
				continue
			}
			err = installPackage(file, rootDir)
			if err != nil {
				log.Fatalf("Could not install package\nError: %s\n", err)
			}
			fmt.Printf("Package (%s) was successfully installed!\n", pkgInfo.name)
		}
	case remove:
		packages := getArgs()[1:]
		if len(packages) == 0 {
			fmt.Println("No packages were given")
			return
		}
		for _, pkg := range packages {
			pkgInfo := getPackageInfo(pkg)
			if pkgInfo == nil {
				fmt.Printf("Package (%s) could not be found\n", pkg)
				continue
			}
			fmt.Print("---------------\n" + createInfoFile(*pkgInfo))
			fmt.Println("---------------")
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Do you wish to remove this package? [y\\N] ")
			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Printf("Skipping package (%s)...\n", pkgInfo.name)
				continue
			}
			err := removePackage(pkg)
			if err != nil {
				log.Fatalf("Could not remove package\nError: %s\n", err)
			}
			fmt.Printf("Package (%s) was successfully removed!\n", pkgInfo.name)
		}
	default:
		fmt.Println("help...")
	}
}
