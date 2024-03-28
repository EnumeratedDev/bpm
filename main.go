package main

import (
	"bufio"
	"capcreepergr.me/bpm/bpm_utils"
	"fmt"
	"log"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
)

/* ---BPM | Bubble Package Manager--- */
/*        Made By CapCreeperGR        */
/*   A simple-to-use package manager  */
/* ---------------------------------- */

var bpmVer = "0.0.8"
var rootDir = "/"

func main() {
	errs, fixed := bpm_utils.FixInstalledPackages(rootDir)
	if len(errs) != 0 {
		for pkg, err := range errs {
			fmt.Printf("Package (%s) could not be read properly\nError: %s\n", pkg, err.Error())
		}
		fmt.Println("The aforementioned packages require manual fixing. Make sure their info files are valid in " + path.Join(rootDir, "var/lib/bpm/installed"))
		os.Exit(1)
	} else {
		if fixed != 0 {
			fmt.Println("Fixed " + strconv.Itoa(fixed) + " outdated package info files")
		}
	}
	if os.Getuid() != 0 {
		fmt.Println("BPM needs to be run with superuser permissions")
		os.Exit(0)
	}
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
			info := bpm_utils.GetPackageInfo(pkg, rootDir, false)
			if info == nil {
				fmt.Printf("Package (%s) could not be found\n", pkg)
				continue
			}
			fmt.Print("----------------\n" + bpm_utils.CreateInfoFile(*info))
			if n == len(packages)-1 {
				fmt.Println("----------------")
			}
		}
	case list:
		flags, _ := resolveFlags()
		packages, err := bpm_utils.GetInstalledPackages(rootDir)
		if err != nil {
			log.Fatalf("Could not get installed packages\nError: %s", err.Error())
			return
		}
		if len(packages) == 0 {
			fmt.Println("No packages have been installed")
			return
		}
		if slices.Contains(flags, "n") {
			fmt.Println(len(packages))
		} else if slices.Contains(flags, "l") {
			for _, pkg := range packages {
				fmt.Println(pkg)
			}
		} else {
			for n, pkg := range packages {
				info := bpm_utils.GetPackageInfo(pkg, rootDir, false)
				if info == nil {
					fmt.Printf("Package (%s) could not be found\n", pkg)
					continue
				}
				fmt.Print("----------------\n" + bpm_utils.CreateInfoFile(*info))
				if n == len(packages)-1 {
					fmt.Println("----------------")
				}
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
			pkgInfo, err := bpm_utils.ReadPackage(file)
			if err != nil {
				log.Fatalf("Could not read package\nError: %s\n", err)
			}
			fmt.Print("----------------\n" + bpm_utils.CreateInfoFile(*pkgInfo))
			fmt.Println("----------------")
			verb := "install"
			if pkgInfo.Type == "source" {
				verb = "build"
			}
			if !slices.Contains(flags, "f") {
				if pkgInfo.Arch != bpm_utils.GetArch() {
					fmt.Printf("skipping... cannot %s a package with a different architecture\n", verb)
					continue
				}
				if unresolved := bpm_utils.CheckDependencies(pkgInfo, rootDir); len(unresolved) != 0 {
					fmt.Printf("skipping... cannot %s package (%s) due to missing dependencies: %s\n", verb, pkgInfo.Name, strings.Join(unresolved, ", "))
					continue
				}
				if pkgInfo.Type == "source" {
					if unresolved := bpm_utils.CheckMakeDependencies(pkgInfo, rootDir); len(unresolved) != 0 {
						fmt.Printf("skipping... cannot %s package (%s) due to missing make dependencies: %s\n", verb, pkgInfo.Name, strings.Join(unresolved, ", "))
						continue
					}
				}
			}
			if bpm_utils.IsPackageInstalled(pkgInfo.Name, rootDir) {
				if !slices.Contains(flags, "y") {
					installedInfo := bpm_utils.GetPackageInfo(pkgInfo.Name, rootDir, false)
					if strings.Compare(pkgInfo.Version, installedInfo.Version) > 0 {
						fmt.Println("This file contains a newer version of this package (" + installedInfo.Version + " -> " + pkgInfo.Version + ")")
						fmt.Print("Do you wish to update this package? [y\\N] ")
					} else if strings.Compare(pkgInfo.Version, installedInfo.Version) < 0 {
						fmt.Println("This file contains an older version of this package (" + installedInfo.Version + " -> " + pkgInfo.Version + ")")
						fmt.Print("Do you wish to downgrade this package? (Not recommended) [y\\N] ")
					} else if strings.Compare(pkgInfo.Version, installedInfo.Version) == 0 {
						fmt.Println("This package is already installed on the system and is up to date")
						fmt.Printf("Do you wish to re%s this package? [y\\N] ", verb)
					}
					reader := bufio.NewReader(os.Stdin)
					text, _ := reader.ReadString('\n')
					if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
						fmt.Printf("Skipping package (%s)...\n", pkgInfo.Name)
						continue
					}
				}
				err := bpm_utils.RemovePackage(pkgInfo.Name, rootDir)
				if err != nil {
					log.Fatalf("Could not remove current version of the package\nError: %s\n", err)
				}
			} else if !slices.Contains(flags, "y") {
				reader := bufio.NewReader(os.Stdin)
				if pkgInfo.Type == "source" {
					fmt.Print("Would you like to view the source.sh file of this package? [Y\\n]")
					text, _ := reader.ReadString('\n')
					if strings.TrimSpace(strings.ToLower(text)) != "n" && strings.TrimSpace(strings.ToLower(text)) != "no" {
						script, err := bpm_utils.GetSourceScript(file)
						if err != nil {
							log.Fatalf("Could not read source script\nError: %s\n", err)
						}
						fmt.Println(script)
						fmt.Println("-------EOF-------")
					}
				}
				fmt.Printf("Do you wish to %s this package? [y\\N] ", verb)

				text, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
					fmt.Printf("Skipping package (%s)...\n", pkgInfo.Name)
					continue
				}
			}

			err = bpm_utils.InstallPackage(file, rootDir, slices.Contains(flags, "f"))
			if err != nil {
				log.Fatalf("Could not install package\nError: %s\n", err)
			}
			fmt.Printf("Package (%s) was successfully installed!\n", pkgInfo.Name)
		}
	case remove:
		flags, i := resolveFlags()
		packages := getArgs()[1+i:]
		if len(packages) == 0 {
			fmt.Println("No packages were given")
			return
		}
		for _, pkg := range packages {
			pkgInfo := bpm_utils.GetPackageInfo(pkg, rootDir, false)
			if pkgInfo == nil {
				fmt.Printf("Package (%s) could not be found\n", pkg)
				continue
			}
			fmt.Print("----------------\n" + bpm_utils.CreateInfoFile(*pkgInfo))
			fmt.Println("----------------")
			if !slices.Contains(flags, "y") {
				reader := bufio.NewReader(os.Stdin)
				fmt.Print("Do you wish to remove this package? [y\\N] ")
				text, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
					fmt.Printf("Skipping package (%s)...\n", pkgInfo.Name)
					continue
				}
			}
			err := bpm_utils.RemovePackage(pkg, rootDir)

			if err != nil {
				log.Fatalf("Could not remove package\nError: %s\n", err)
			}
			fmt.Printf("Package (%s) was successfully removed!\n", pkgInfo.Name)
		}
	default:
		fmt.Println("\033[1m------Help------\033[0m")
		fmt.Println("\033[1m\\ Command Format /\033[0m")
		fmt.Println("-> command format: bpm <subcommand> [-flags]...")
		fmt.Println("-> flags will be read if passed right after the subcommand otherwise they will be read as subcommand arguments")
		fmt.Println("\033[1m\\ Command List /\033[0m")
		fmt.Println("-> bpm version | shows information on the installed version of bpm")
		fmt.Println("-> bpm info | shows information on an installed package")
		fmt.Println("-> bpm list [-n, -l] | lists all installed packages. -n shows the number of packages. -l lists package names only")
		fmt.Println("-> bpm install [-y, -f] <files...> | installs the following files. -y skips the confirmation prompt. -f skips dependency and architecture checking")
		fmt.Println("-> bpm remove [-y] <packages...> | removes the following packages. -y skips the confirmation prompt")
		fmt.Println("-> bpm cleanup | removes all unneeded dependencies")
		fmt.Println("\033[1m----------------\033[0m")
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
				v := [...]string{"l", "n"}
				if !slices.Contains(v[:], f) {
					log.Fatalf("Invalid flag " + flag)
				}
				ret = append(ret, f)
			case install:
				v := [...]string{"y", "f"}
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
