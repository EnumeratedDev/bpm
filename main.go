package main

import (
	"bufio"
	"flag"
	"fmt"
	"gitlab.com/bubble-package-manager/bpm/utils"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

/* ---BPM | Bubble Package Manager--- */
/*        Made By CapCreeperGR        */
/*   A simple-to-use package manager  */
/* ---------------------------------- */

var bpmVer = "0.4"

var subcommand = "help"
var subcommandArgs []string

// Flags
var rootDir = "/"
var verbose = false
var yesAll = false
var buildSource = false
var skipCheck = false
var keepTempDir = false
var force = false
var showInstalled = false
var pkgListNumbers = false
var pkgListNames = false
var reinstall = false
var nosync = true

func main() {
	utils.ReadConfig()
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
	update
	sync
	remove
	file
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
	case "update":
		return update
	case "sync":
		return sync
	case "remove":
		return remove
	case "file":
		return file
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
			var info *utils.PackageInfo
			if _, err := os.Stat(pkg); err == nil && !showInstalled {
				info, err = utils.ReadPackage(pkg)
				if err != nil {
					fmt.Printf("File (%s) could not be read\n", pkg)
					continue
				}

			} else if showInstalled {
				info = utils.GetPackageInfo(pkg, rootDir, false)
				if info == nil {
					fmt.Printf("Package (%s) is not installed\n", pkg)
					continue
				}
			} else {
				entry, err := utils.GetRepositoryEntry(pkg)
				if err != nil {
					fmt.Printf("Package (%s) could not be found in any repository\n", pkg)
					continue
				}
				info = &entry.Info
			}
			fmt.Println("----------------")
			if showInstalled {
				fmt.Println(utils.CreateReadableInfo(true, true, true, false, true, info, rootDir))
			} else {
				fmt.Println(utils.CreateReadableInfo(true, true, true, true, true, info, rootDir))
			}
			if n == len(packages)-1 {
				fmt.Println("----------------")
			}
		}
	case list:
		packages, err := utils.GetInstalledPackages(rootDir)
		if err != nil {
			log.Fatalf("Could not get installed packages\nError: %s", err.Error())
			return
		}
		if pkgListNumbers {
			fmt.Println(len(packages))
		} else if pkgListNames {
			for _, pkg := range packages {
				fmt.Println(pkg)
			}
		} else {
			if len(packages) == 0 {
				fmt.Println("No packages have been installed")
				return
			}
			for n, pkg := range packages {
				info := utils.GetPackageInfo(pkg, rootDir, false)
				if info == nil {
					fmt.Printf("Package (%s) could not be found\n", pkg)
					continue
				}
				fmt.Println("----------------\n" + utils.CreateReadableInfo(true, true, true, true, true, info, rootDir))
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
			pkgInfo, err := utils.ReadPackage(file)
			if err != nil {
				log.Fatalf("Could not read package\nError: %s\n", err)
			}
			if !yesAll {
				fmt.Println("----------------\n" + utils.CreateReadableInfo(true, true, true, true, false, pkgInfo, rootDir))
				fmt.Println("----------------")
			}
			verb := "install"
			if pkgInfo.Type == "source" {
				if _, err := os.Stat("/bin/fakeroot"); os.IsNotExist(err) {
					fmt.Printf("Skipping... cannot %s package (%s) due to fakeroot not being installed", verb, pkgInfo.Name)
					continue
				}
				verb = "build"
			}
			if !force {
				if pkgInfo.Arch != "any" && pkgInfo.Arch != utils.GetArch() {
					fmt.Printf("skipping... cannot %s a package with a different architecture\n", verb)
					continue
				}
				if unresolved := utils.CheckDependencies(pkgInfo, true, true, rootDir); len(unresolved) != 0 {
					fmt.Printf("skipping... cannot %s package (%s) due to missing dependencies: %s\n", verb, pkgInfo.Name, strings.Join(unresolved, ", "))
					continue
				}
				if conflicts := utils.CheckConflicts(pkgInfo, true, rootDir); len(conflicts) != 0 {
					fmt.Printf("skipping... cannot %s package (%s) due to conflicting packages: %s\n", verb, pkgInfo.Name, strings.Join(conflicts, ", "))
					continue
				}
			}
			if rootDir != "/" {
				fmt.Println("Warning: Operating in " + rootDir)
			}
			if !yesAll {
				reader := bufio.NewReader(os.Stdin)
				if pkgInfo.Type == "source" {
					fmt.Print("Would you like to view the source.sh file of this package? [Y\\n] ")
					text, _ := reader.ReadString('\n')
					if strings.TrimSpace(strings.ToLower(text)) != "n" && strings.TrimSpace(strings.ToLower(text)) != "no" {
						script, err := utils.GetSourceScript(file)
						if err != nil {
							log.Fatalf("Could not read source script\nError: %s\n", err)
						}
						fmt.Println(script)
						fmt.Println("-------EOF-------")
					}
				}
			}
			if utils.IsPackageInstalled(pkgInfo.Name, rootDir) {
				if !yesAll {
					installedInfo := utils.GetPackageInfo(pkgInfo.Name, rootDir, false)
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

			err = utils.InstallPackage(file, rootDir, verbose, force, buildSource, skipCheck, keepTempDir)
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
	case sync:
		if os.Getuid() != 0 {
			fmt.Println("This subcommand needs to be run with superuser permissions")
			os.Exit(0)
		}
		if !yesAll {
			fmt.Printf("Are you sure you wish to sync all databases? [y\\N] ")
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Println("Cancelling sync...")
				os.Exit(0)
			}
		}
		for _, repo := range utils.BPMConfig.Repositories {
			fmt.Printf("Fetching package database for repository (%s)...\n", repo.Name)
			err := repo.SyncLocalDatabase()
			if err != nil {
				log.Fatal(err)
			}
		}
		fmt.Println("All package databases synced successfully!")
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
			pkgInfo := utils.GetPackageInfo(pkg, rootDir, false)
			if pkgInfo == nil {
				fmt.Printf("Package (%s) could not be found\n", pkg)
				continue
			}
			fmt.Println("----------------\n" + utils.CreateReadableInfo(true, true, true, true, true, pkgInfo, rootDir))
			fmt.Println("----------------")
			if rootDir != "/" {
				fmt.Println("Warning: Operating in " + rootDir)
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
			err := utils.RemovePackage(pkg, verbose, rootDir)

			if err != nil {
				log.Fatalf("Could not remove package\nError: %s\n", err)
			}
			fmt.Printf("Package (%s) was successfully removed!\n", pkgInfo.Name)
		}
	case file:
		files := subcommandArgs
		if len(files) == 0 {
			fmt.Println("No files were given to get which packages manage it")
			return
		}
		for _, file := range files {
			absFile, err := filepath.Abs(file)
			if err != nil {
				log.Fatalf("Could not get absolute path of %s", file)
			}
			stat, err := os.Stat(absFile)
			if os.IsNotExist(err) {
				log.Fatalf(absFile + " does not exist!")
			}
			pkgs, err := utils.GetInstalledPackages(rootDir)
			if err != nil {
				log.Fatalf("Could not get installed packages. Error %s", err.Error())
			}

			if !strings.HasPrefix(absFile, rootDir) {
				log.Fatalf("Could not get relative path of %s to root path", absFile)
			}
			absFile, err = filepath.Rel(rootDir, absFile)
			if err != nil {
				log.Fatalf("Could not get relative path of %s to root path", absFile)
			}
			absFile = strings.TrimPrefix(absFile, "/")
			if stat.IsDir() {
				absFile = absFile + "/"
			}

			var pkgList []string
			for _, pkg := range pkgs {
				if slices.Contains(utils.GetPackageFiles(pkg, rootDir), absFile) {
					pkgList = append(pkgList, pkg)
				}
			}
			if len(pkgList) == 0 {
				fmt.Println(absFile + " is not managed by any packages")
			} else {
				fmt.Println(absFile + " is managed by the following packages:")
				for _, pkg := range pkgList {
					fmt.Println("- " + pkg)
				}
			}
		}
	default:
		printHelp()
	}
}

func printHelp() {
	fmt.Println("\033[1m---- Command Format ----\033[0m")
	fmt.Println("-> command format: bpm <subcommand> [-flags]...")
	fmt.Println("-> flags will be read if passed right after the subcommand otherwise they will be read as subcommand arguments")
	fmt.Println("\033[1m---- Command List ----\033[0m")
	fmt.Println("-> bpm version | shows information on the installed version of bpm")
	fmt.Println("-> bpm info [-R] | shows information on an installed package")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -i shows information about the currently installed package")
	fmt.Println("-> bpm list [-R, -c, -n] | lists all installed packages")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -c lists the amount of installed packages")
	fmt.Println("       -n lists only the names of installed packages")
	fmt.Println("-> bpm install [-R, -v, -y, -f, -o, -c, -b, -k] <files...> | installs the following files")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("       -f skips dependency, conflict and architecture checking")
	fmt.Println("       -o=<path> set the binary package output directory (defaults to /var/lib/bpm/compiled)")
	fmt.Println("       -c=<path> set the compilation directory (defaults to /var/tmp)")
	fmt.Println("       -b creates a binary package from a source package after compilation and saves it in the binary package output directory")
	fmt.Println("       -k keeps the compilation directory created by BPM after source package installation")
	fmt.Println("-> bpm update [-R, -v, -y, -f, --reinstall, --nosync] | updates all packages that are available in the repositories")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("       -f skips dependency, conflict and architecture checking")
	fmt.Println("       --reinstall Fetches and reinstalls all packages even if they do not have a newer version available")
	fmt.Println("       --nosync Skips package database syncing")
	fmt.Println("-> bpm sync [-R, -v] | Syncs package databases without updating packages")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("-> bpm remove [-R, -v, -y] <packages...> | removes the following packages")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("-> bpm file [-R] <files...> | shows what packages the following packages are managed by")
	fmt.Println("       -R=<root_path> lets you define the root path which will be used")
	fmt.Println("\033[1m----------------\033[0m")
}

func resolveFlags() {
	// List flags
	listFlagSet := flag.NewFlagSet("List flags", flag.ExitOnError)
	listFlagSet.Usage = printHelp
	listFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	listFlagSet.BoolVar(&pkgListNumbers, "c", false, "List the number of all packages installed with BPM")
	listFlagSet.BoolVar(&pkgListNames, "n", false, "List the names of all packages installed with BPM")
	// Info flags
	infoFlagSet := flag.NewFlagSet("Info flags", flag.ExitOnError)
	infoFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	infoFlagSet.BoolVar(&showInstalled, "i", false, "Shows information about the currently installed package")
	infoFlagSet.Usage = printHelp
	// Install flags
	installFlagSet := flag.NewFlagSet("Install flags", flag.ExitOnError)
	installFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	installFlagSet.BoolVar(&verbose, "v", false, "Show additional information about what BPM is doing")
	installFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")
	installFlagSet.StringVar(&utils.BPMConfig.BinaryOutputDir, "o", utils.BPMConfig.BinaryOutputDir, "Set the binary output directory")
	installFlagSet.StringVar(&utils.BPMConfig.CompilationDir, "c", utils.BPMConfig.CompilationDir, "Set the compilation directory")
	installFlagSet.BoolVar(&buildSource, "b", false, "Build binary package from source package")
	installFlagSet.BoolVar(&skipCheck, "s", false, "Skip check function during source compilation")
	installFlagSet.BoolVar(&keepTempDir, "k", false, "Keep temporary directory after source compilation")
	installFlagSet.BoolVar(&force, "f", false, "Force installation by skipping architecture and dependency resolution")
	installFlagSet.Usage = printHelp
	// Update flags
	updateFlagSet := flag.NewFlagSet("Update flags", flag.ExitOnError)
	updateFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	updateFlagSet.BoolVar(&verbose, "v", false, "Show additional information about what BPM is doing")
	updateFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")
	updateFlagSet.BoolVar(&force, "f", false, "Force update by skipping architecture and dependency resolution")
	updateFlagSet.BoolVar(&reinstall, "reinstall", false, "Fetches and reinstalls all packages even if they do not have a newer version available")
	updateFlagSet.BoolVar(&nosync, "nosync", false, "Skips package database syncing")
	updateFlagSet.Usage = printHelp
	// Sync flags
	syncFlagSet := flag.NewFlagSet("Sync flags", flag.ExitOnError)
	syncFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	syncFlagSet.BoolVar(&verbose, "v", false, "Show additional information about what BPM is doing")
	syncFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")
	syncFlagSet.Usage = printHelp
	// Remove flags
	removeFlagSet := flag.NewFlagSet("Remove flags", flag.ExitOnError)
	removeFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	removeFlagSet.BoolVar(&verbose, "v", false, "Show additional information about what BPM is doing")
	removeFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")
	removeFlagSet.Usage = printHelp
	// File flags
	fileFlagSet := flag.NewFlagSet("Remove flags", flag.ExitOnError)
	fileFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	fileFlagSet.Usage = printHelp
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
		} else if getCommandType() == update {
			err := updateFlagSet.Parse(subcommandArgs)
			if err != nil {
				return
			}
			subcommandArgs = updateFlagSet.Args()
		} else if getCommandType() == sync {
			err := syncFlagSet.Parse(subcommandArgs)
			if err != nil {
				return
			}
			subcommandArgs = syncFlagSet.Args()
		} else if getCommandType() == remove {
			err := removeFlagSet.Parse(subcommandArgs)
			if err != nil {
				return
			}
			subcommandArgs = removeFlagSet.Args()
		} else if getCommandType() == file {
			err := fileFlagSet.Parse(subcommandArgs)
			if err != nil {
				return
			}
			subcommandArgs = fileFlagSet.Args()
		}
	}
}
