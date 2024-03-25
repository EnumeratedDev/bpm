package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
)

/* ---BPM | Bubble Package Manager--- */
/*        Made By CapCreeperGR        */
/*   A simple-to-use package manager  */
/* ---------------------------------- */

var bpmVer = "0.0.3"
var rootDir string = "/"

func main() {
	//fmt.Printf("Running %s %s\n", getKernel(), getArch())
	resolveCommand()
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
		resolveFlags()
		fmt.Println("Bubble Package Manager (BPM)")
		fmt.Println("Version: " + bpmVer)
	case info:
		_, i := resolveFlags()
		packages := getArgs()[1+i:]
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
			fmt.Print("----------------\n" + createInfoFile(*info))
			if n == len(packages)-1 {
				fmt.Println("----------------")
			}
		}
	case list:
		resolveFlags()
		packages, err := getInstalledPackages()
		if err != nil {
			log.Fatalf("Could not get installed packages\nError: %s", err.Error())
			return
		}
		if len(packages) == 0 {
			fmt.Println("No packages have been installed")
			return
		}
		for n, pkg := range packages {
			info := getPackageInfo(pkg)
			if info == nil {
				fmt.Printf("Package (%s) could not be found\n", pkg)
				continue
			}
			fmt.Print("----------------\n" + createInfoFile(*info))
			if n == len(packages)-1 {
				fmt.Println("----------------")
			}
		}
	case install:
		flags, i := resolveFlags()
		files := getArgs()[1+i:]
		if len(files) == 0 {
			fmt.Println("No files were given to install")
			return
		}
		for _, file := range files {
			pkgInfo, err := readPackage(file)
			if err != nil {
				log.Fatalf("Could not read package\nError: %s\n", err)
			}
			fmt.Print("----------------\n" + createInfoFile(*pkgInfo))
			fmt.Println("----------------")
			if isPackageInstalled(pkgInfo.name) {
				if !slices.Contains(flags, "y") {
					installedInfo := getPackageInfo(pkgInfo.name)
					if strings.Compare(pkgInfo.version, installedInfo.version) > 0 {
						fmt.Println("This file contains a newer version of this package (" + installedInfo.version + " -> " + pkgInfo.version + ")")
						fmt.Print("Do you wish to update this package? [y\\N] ")
					} else if strings.Compare(pkgInfo.version, installedInfo.version) < 0 {
						fmt.Println("This file contains an older version of this package (" + installedInfo.version + " -> " + pkgInfo.version + ")")
						fmt.Print("Do you wish to downgrade this package? (Not recommended) [y\\N] ")
					} else if strings.Compare(pkgInfo.version, installedInfo.version) == 0 {
						fmt.Println("This package is already installed on the system and is up to date")
						fmt.Print("Do you wish to reinstall this package? [y\\N] ")
					}
					reader := bufio.NewReader(os.Stdin)
					text, _ := reader.ReadString('\n')
					if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
						fmt.Printf("Skipping package (%s)...\n", pkgInfo.name)
						continue
					}
				}
				err := removePackage(pkgInfo.name)
				if err != nil {
					log.Fatalf("Could not remove current version of the package\nError: %s\n", err)
				}
			} else if !slices.Contains(flags, "y") {
				fmt.Print("Do you wish to install this package? [y\\N] ")
				reader := bufio.NewReader(os.Stdin)
				text, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
					fmt.Printf("Skipping package (%s)...\n", pkgInfo.name)
					continue
				}
			}

			err = installPackage(file, rootDir)
			if err != nil {
				log.Fatalf("Could not install package\nError: %s\n", err)
			}
			fmt.Printf("Package (%s) was successfully installed!\n", pkgInfo.name)
		}
	case remove:
		flags, i := resolveFlags()
		packages := getArgs()[1+i:]
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
			fmt.Print("----------------\n" + createInfoFile(*pkgInfo))
			fmt.Println("----------------")
			if !slices.Contains(flags, "y") {
				reader := bufio.NewReader(os.Stdin)
				fmt.Print("Do you wish to remove this package? [y\\N] ")
				text, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
					fmt.Printf("Skipping package (%s)...\n", pkgInfo.name)
					continue
				}
			}
			err := removePackage(pkg)
			if err != nil {
				log.Fatalf("Could not remove package\nError: %s\n", err)
			}
			fmt.Printf("Package (%s) was successfully removed!\n", pkgInfo.name)
		}
	default:
		fmt.Println("------Help------")
		fmt.Println("bpm version | shows information on the installed version of bpm")
		fmt.Println("bpm info | shows information on an installed package")
		fmt.Println("bpm list [-e] | lists all installed packages. -e lists only explicitly installed packages")
		fmt.Println("bpm install [-y] <files...> | installs the following files. -y skips the confirmation prompt")
		fmt.Println("bpm remove [-y] <packages...> | removes the following packages. -y skips the confirmation prompt")
		fmt.Println("bpm cleanup | removes all unneeded dependencies")
		fmt.Println("----------------")
	}
}

func resolveFlags() ([]string, int) {
	flags := getArgs()[1:]
	var ret []string
	for _, flag := range flags {
		if strings.HasPrefix(flag, "-") {
			f := strings.TrimPrefix(flag, "-")
			switch getCommandType() {
			default:
				log.Fatalf("Invalid flag " + flag)
			case list:
				v := [...]string{"e"}
				if !slices.Contains(v[:], f) {
					log.Fatalf("Invalid flag " + flag)
				}
				ret = append(ret, f)
			case install:
				v := [...]string{"y"}
				if !slices.Contains(v[:], f) {
					log.Fatalf("Invalid flag " + flag)
				}
				ret = append(ret, f)
			case remove:
				v := [...]string{"y"}
				if !slices.Contains(v[:], f) {
					log.Fatalf("Invalid flag " + flag)
				}
				ret = append(ret, f)
			}
		} else {
			break
		}
	}
	return ret, len(ret)
}
