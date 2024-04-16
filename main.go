package main

import (
	"bufio"
	"capcreepergr.me/bpm/bpm_utils"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

/* ---BPM | Bubble Package Manager--- */
/*        Made By CapCreeperGR        */
/*   A simple-to-use package manager  */
/* ---------------------------------- */

var bpmVer = "0.1.2"

var subcommand = "help"
var subcommandArgs []string

// Flags
var rootDir = "/"
var yesAll = false
var buildSource = false
var keepTempDir = false
var forceInstall = false
var pkgListNumbers = false
var pkgListNames = false

func main() {
	resolveFlags()
	resolveCommand()
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
	switch subcommand {
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
		packages := subcommandArgs
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
		packages, err := bpm_utils.GetInstalledPackages(rootDir)
		if err != nil {
			log.Fatalf("Could not get installed packages\nError: %s", err.Error())
			return
		}
		if len(packages) == 0 {
			fmt.Println("No packages have been installed")
			return
		}
		if pkgListNumbers {
			fmt.Println(len(packages))
		} else if pkgListNames {
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
		if os.Getuid() != 0 {
			fmt.Println("This subcommand needs to be run with superuser permissions")
			os.Exit(0)
		}
		files := subcommandArgs
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
			if !forceInstall {
				if pkgInfo.Arch != "any" && pkgInfo.Arch != bpm_utils.GetArch() {
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
			if rootDir != "/" {
				fmt.Println("Warning: Installing to " + rootDir)
			}
			if !yesAll {
				reader := bufio.NewReader(os.Stdin)
				if pkgInfo.Type == "source" {
					fmt.Print("Would you like to view the source.sh file of this package? [Y\\n] ")
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
			}
			if bpm_utils.IsPackageInstalled(pkgInfo.Name, rootDir) {
				if !yesAll {
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
			} else if !yesAll {
				reader := bufio.NewReader(os.Stdin)
				fmt.Printf("Do you wish to %s this package? [y\\N] ", verb)
				text, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
					fmt.Printf("Skipping package (%s)...\n", pkgInfo.Name)
					continue
				}
			}

			err = bpm_utils.InstallPackage(file, rootDir, forceInstall, buildSource, keepTempDir)
			if err != nil {
				if pkgInfo.Type == "source" && keepTempDir {
					fmt.Println("BPM temp directory was created at /var/tmp/bpm_source-" + pkgInfo.Name)
				}
				log.Fatalf("Could not install package\nError: %s\n", err)
			}
			fmt.Printf("Package (%s) was successfully installed!\n", pkgInfo.Name)
			if pkgInfo.Type == "source" && keepTempDir {
				fmt.Println("** It is recommended you delete the temporary bpm folder in /var/tmp **")
			}
		}
	case remove:
		if os.Getuid() != 0 {
			fmt.Println("This subcommand needs to be run with superuser permissions")
			os.Exit(0)
		}
		packages := subcommandArgs
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
			if rootDir != "/" {
				fmt.Println("Warning: Installing to " + rootDir)
			}
			if !yesAll {
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
		printHelp()
	}
}

func printHelp() {
	fmt.Println("\033[1m------Help------\033[0m")
	fmt.Println("\033[1m\\ Command Format /\033[0m")
	fmt.Println("-> command format: bpm <subcommand> [-flags]...")
	fmt.Println("-> flags will be read if passed right after the subcommand otherwise they will be read as subcommand arguments")
	fmt.Println("\033[1m\\ Command List /\033[0m")
	fmt.Println("-> bpm version | shows information on the installed version of bpm")
	fmt.Println("-> bpm info | shows information on an installed package")
	fmt.Println("-> bpm list [-n, -l] | lists all installed packages")
	fmt.Println("       -n shows the number of packages")
	fmt.Println("       -l lists package names only")
	fmt.Println("-> bpm install [-y, -f, -b] <files...> | installs the following files")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("       -f skips dependency and architecture checking")
	fmt.Println("       -b creates a binary package for a source package after compilation and saves it in /var/lib/bpm/compiled")
	fmt.Println("       -k keeps the temp directory created by BPM after source package installation")
	fmt.Println("-> bpm remove [-y] <packages...> | removes the following packages")
	fmt.Println("       -y skips the confirmation prompt")
	//fmt.Println("-> bpm cleanup | removes all unneeded dependencies")
	fmt.Println("\033[1m----------------\033[0m")
}

func resolveFlags() {
	// List flags
	listFlagSet := flag.NewFlagSet("List flags", flag.ExitOnError)
	listFlagSet.Usage = printHelp
	listFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	listFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")
	listFlagSet.BoolVar(&pkgListNumbers, "n", false, "List the number of all packages installed with BPM")
	listFlagSet.BoolVar(&pkgListNames, "l", false, "List the names of all packages installed with BPM")
	// Info flags
	infoFlagSet := flag.NewFlagSet("Info flags", flag.ExitOnError)
	infoFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	infoFlagSet.Usage = printHelp
	// Install flags
	installFlagSet := flag.NewFlagSet("Install flags", flag.ExitOnError)
	installFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	installFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")
	installFlagSet.BoolVar(&buildSource, "b", false, "Build binary package from source package")
	installFlagSet.BoolVar(&keepTempDir, "k", false, "Keep temporary directory after source compilation")
	installFlagSet.BoolVar(&forceInstall, "f", false, "Force installation by skipping architecture and dependency resolution")
	installFlagSet.Usage = printHelp
	// Remove flags
	removeFlagSet := flag.NewFlagSet("Remove flags", flag.ExitOnError)
	removeFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	removeFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")
	removeFlagSet.Usage = printHelp
	if len(os.Args[1:]) <= 0 {
		subcommand = "help"
	} else {
		subcommand = os.Args[1]
		subcommandArgs = os.Args[2:]
		if getCommandType() == list {
			err := listFlagSet.Parse(subcommandArgs)
			if err != nil {
				return
			}
			subcommandArgs = listFlagSet.Args()
		} else if getCommandType() == info {
			err := infoFlagSet.Parse(subcommandArgs)
			if err != nil {
				return
			}
			subcommandArgs = infoFlagSet.Args()
		} else if getCommandType() == install {
			err := installFlagSet.Parse(subcommandArgs)
			if err != nil {
				return
			}
			subcommandArgs = installFlagSet.Args()
		} else if getCommandType() == remove {
			err := removeFlagSet.Parse(subcommandArgs)
			if err != nil {
				return
			}
			subcommandArgs = removeFlagSet.Args()
		}
	}
}

/*func resolveFlags() ([]string, int) {
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
				v := [...]string{"y", "f", "b", "k"}
				if !slices.Contains(v[:], f) {
					log.Fatalf("Invalid flag " + flag)
				}
				ret = append(ret, f)
			case remove:
				v := [...]string{"y", "r"}
				if !slices.Contains(v[:], f) {
					log.Fatalf("Invalid flag " + flag)
				}
				ret = append(ret, f)
			case info:
				v := [...]string{"r"}
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
}*/
