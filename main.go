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

/* -------------BPM | Bubble Package Manager-------------- */
/*        Made By EnumDev (Previously CapCreeperGR)        */
/*             A simple-to-use package manager             */
/* ------------------------------------------------------- */

var bpmVer = "0.5.0"

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
var pkgListNumbers = false
var pkgListNames = false
var reinstall = false
var reinstallAll = false
var noOptional = false
var installationReason = ""
var nosync = true
var removeUnused = false
var doCleanup = false
var showRepoInfo = false

func main() {
	utils.ReadConfig()
	resolveFlags()
	resolveCommand()
}

type commandType uint8

const (
	_default commandType = iota
	help
	info
	list
	search
	install
	update
	sync
	remove
	cleanup
	file
)

func getCommandType() commandType {
	switch subcommand {
	case "version":
		return _default
	case "info":
		return info
	case "list":
		return list
	case "search":
		return search
	case "install":
		return install
	case "update":
		return update
	case "sync":
		return sync
	case "remove":
		return remove
	case "cleanup":
		return cleanup
	case "file":
		return file
	default:
		return help
	}
}

func resolveCommand() {
	switch getCommandType() {
	case _default:
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
			isFile := false
			if showRepoInfo {
				entry, _, err := utils.GetRepositoryEntry(pkg)
				if err != nil {
					log.Fatalf("Error: could not find package (%s) in any repository\n", pkg)
				}
				info = entry.Info
			} else if stat, err := os.Stat(pkg); err == nil && !stat.IsDir() {
				bpmpkg, err := utils.ReadPackage(pkg)
				if err != nil {
					log.Fatalf("Error: could not read package: %s\n", err)
				}
				info = bpmpkg.PkgInfo
				isFile = true
			} else {
				info = utils.GetPackageInfo(pkg, rootDir)
			}
			if info == nil {
				log.Fatalf("Error: package (%s) is not installed\n", pkg)
			}
			if n != 0 {
				fmt.Println()
			}
			if isFile {
				abs, err := filepath.Abs(pkg)
				if err != nil {
					log.Fatalf("Error: could not get absolute path of file (%s)\n", abs)
				}
				fmt.Println("File: " + abs)
			}
			fmt.Println(utils.CreateReadableInfo(true, true, true, info, rootDir))
		}
	case list:
		packages, err := utils.GetInstalledPackages(rootDir)
		if err != nil {
			log.Fatalf("Error: could not get installed packages: %s", err.Error())
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
				info := utils.GetPackageInfo(pkg, rootDir)
				if info == nil {
					fmt.Printf("Package (%s) could not be found\n", pkg)
					continue
				}
				if n != 0 {
					fmt.Println()
				}
				fmt.Println(utils.CreateReadableInfo(true, true, true, info, rootDir))
			}
		}
	case search:
		searchTerms := subcommandArgs
		if len(searchTerms) == 0 {
			log.Fatalf("Error: no search terms given")
		}
		for i, term := range searchTerms {
			nameResults := make([]*utils.PackageInfo, 0)
			descResults := make([]*utils.PackageInfo, 0)
			for _, repo := range utils.BPMConfig.Repositories {
				for _, entry := range repo.Entries {
					if strings.Contains(entry.Info.Name, term) {
						nameResults = append(nameResults, entry.Info)
					} else if strings.Contains(entry.Info.Description, term) {
						descResults = append(descResults, entry.Info)
					}
				}
			}
			results := append(nameResults, descResults...)
			if len(results) == 0 {
				log.Fatalf("Error: no results for term (%s) were found\n", term)
			}
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("Results for term (%s)\n", term)
			for j, result := range results {
				fmt.Printf("%d) %s: %s (%s)\n", j+1, result.Name, result.Description, result.GetFullVersion())
			}
		}
	case install:
		if os.Getuid() != 0 {
			log.Fatalf("Error: this subcommand needs to be run with superuser permissions")
		}
		pkgs := subcommandArgs
		if len(pkgs) == 0 {
			fmt.Println("No packages or files were given to install")
			return
		}

		// Check if installationReason argument is valid
		ir := utils.Unknown
		if installationReason == "manual" {
			ir = utils.Manual
		} else if installationReason == "dependency" {
			ir = utils.Dependency
		} else if installationReason != "" {
			log.Fatalf("Error: %s is not a valid installation reason", installationReason)
		}

		operation := utils.BPMOperation{
			Actions:                 make([]utils.OperationAction, 0),
			UnresolvedDepends:       make([]string, 0),
			RootDir:                 rootDir,
			ForceInstallationReason: ir,
		}

		// Search for packages
		for _, pkg := range pkgs {
			if stat, err := os.Stat(pkg); err == nil && !stat.IsDir() {
				bpmpkg, err := utils.ReadPackage(pkg)
				if err != nil {
					log.Fatalf("Error: could not read package: %s\n", err)
				}
				if !reinstall && utils.IsPackageInstalled(bpmpkg.PkgInfo.Name, rootDir) && utils.GetPackageInfo(bpmpkg.PkgInfo.Name, rootDir).GetFullVersion() == bpmpkg.PkgInfo.GetFullVersion() {
					continue
				}
				operation.Actions = append(operation.Actions, &utils.InstallPackageAction{
					File:         pkg,
					IsDependency: false,
					BpmPackage:   bpmpkg,
				})
			} else {
				entry, _, err := utils.GetRepositoryEntry(pkg)
				if err != nil {
					log.Fatalf("Error: could not find package (%s) in any repository\n", pkg)
				}
				if !reinstall && utils.IsPackageInstalled(entry.Info.Name, rootDir) && utils.GetPackageInfo(entry.Info.Name, rootDir).GetFullVersion() == entry.Info.GetFullVersion() {
					continue
				}
				operation.Actions = append(operation.Actions, &utils.FetchPackageAction{
					IsDependency:    false,
					RepositoryEntry: entry,
				})
			}
		}

		// Resolve dependencies
		err := operation.ResolveDependencies(reinstallAll, !noOptional, verbose)
		if err != nil {
			log.Fatalf("Error: could not resolve dependencies: %s\n", err)
		}
		if len(operation.UnresolvedDepends) != 0 {
			if !force {
				log.Fatalf("Error: the following dependencies could not be found in any repositories: %s\n", strings.Join(operation.UnresolvedDepends, ", "))
			} else {
				log.Println("Warning: The following dependencies could not be found in any repositories: " + strings.Join(operation.UnresolvedDepends, ", "))
			}
		}

		// Replace obsolete packages
		operation.ReplaceObsoletePackages()

		// Check for conflicts
		conflicts, err := operation.CheckForConflicts()
		if err != nil {
			log.Fatalf("Error: could not complete package conflict check: %s\n", err)
		}
		if len(conflicts) > 0 {
			if !force {
				log.Println("Error: conflicting packages found")
			} else {
				log.Fatalf("Warning: conflicting packages found")
			}
			for pkg, conflict := range conflicts {
				fmt.Printf("%s is in conflict with the following packages: %s\n", pkg, strings.Join(conflict, ", "))
			}
			if !force {
				os.Exit(0)
			}
		}

		// Show operation summary
		operation.ShowOperationSummary()

		// Confirmation Prompt
		if !yesAll {
			reader := bufio.NewReader(os.Stdin)
			if len(operation.Actions) == 1 {
				fmt.Printf("Do you wish to install this package? [y\\N] ")
			} else {
				fmt.Printf("Do you wish to install these %d packages? [y\\N] ", len(operation.Actions))
			}

			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Println("Cancelling package installation...")
				os.Exit(1)
			}
		}

		// Execute operation
		err = operation.Execute(verbose, force)
		if err != nil {
			log.Fatalf("Error: could not complete operation: %s\n", err)
		}
	case update:
		if os.Getuid() != 0 {
			log.Fatalf("Error: this subcommand needs to be run with superuser permissions")
		}

		// Sync repositories
		if !nosync {
			for _, repo := range utils.BPMConfig.Repositories {
				fmt.Printf("Fetching package database for repository (%s)...\n", repo.Name)
				err := repo.SyncLocalDatabase()
				if err != nil {
					log.Fatalf("Error: could not sync local database for repository (%s): %s\n", repo.Name, err)
				}
			}
			fmt.Println("All package databases synced successfully!")
		}

		utils.ReadConfig()

		// Get installed packages and check for updates
		pkgs, err := utils.GetInstalledPackages(rootDir)
		if err != nil {
			log.Fatalf("Error: could not get installed packages: %s\n", err)
		}

		operation := utils.BPMOperation{
			Actions:                 make([]utils.OperationAction, 0),
			UnresolvedDepends:       make([]string, 0),
			RootDir:                 rootDir,
			ForceInstallationReason: utils.Unknown,
		}

		// Search for packages
		for _, pkg := range pkgs {
			if slices.Contains(utils.BPMConfig.IgnorePackages, pkg) {
				continue
			}
			var entry *utils.RepositoryEntry
			// Check if installed package can be replaced and install that instead
			if e := utils.FindReplacement(pkg); e != nil {
				entry = e
			} else if entry, _, err = utils.GetRepositoryEntry(pkg); err != nil {
				continue
			}

			installedInfo := utils.GetPackageInfo(pkg, rootDir)
			if installedInfo == nil {
				log.Fatalf("Error: could not get package info for (%s)\n", pkg)
			} else {
				comparison := utils.ComparePackageVersions(*entry.Info, *installedInfo)
				if comparison > 0 || reinstall {
					operation.Actions = append(operation.Actions, &utils.FetchPackageAction{
						IsDependency:    false,
						RepositoryEntry: entry,
					})
				}
			}
		}

		// Check for new dependencies in updated packages
		err = operation.ResolveDependencies(reinstallAll, !noOptional, verbose)
		if err != nil {
			log.Fatalf("Error: could not resolve dependencies: %s\n", err)
		}
		if len(operation.UnresolvedDepends) != 0 {
			if !force {
				log.Fatalf("Error: the following dependencies could not be found in any repositories: %s\n", strings.Join(operation.UnresolvedDepends, ", "))
			} else {
				log.Println("Warning: The following dependencies could not be found in any repositories: " + strings.Join(operation.UnresolvedDepends, ", "))
			}
		}

		// Replace obsolete packages
		operation.ReplaceObsoletePackages()

		// Show operation summary
		operation.ShowOperationSummary()

		// Confirmation Prompt
		if !yesAll {
			fmt.Printf("Are you sure you wish to update all %d packages? [y\\N] ", len(operation.Actions))
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Println("Cancelling package update...")
				os.Exit(1)
			}
		}

		// Execute operation
		err = operation.Execute(verbose, force)
		if err != nil {
			log.Fatalf("Error: could not complete operation: %s\n", err)
		}
	case sync:
		if os.Getuid() != 0 {
			log.Fatalf("Error: this subcommand needs to be run with superuser permissions")
		}
		if !yesAll {
			fmt.Printf("Are you sure you wish to sync all databases? [y\\N] ")
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Println("Cancelling database synchronization...")
				os.Exit(1)
			}
		}
		for _, repo := range utils.BPMConfig.Repositories {
			fmt.Printf("Fetching package database for repository (%s)...\n", repo.Name)
			err := repo.SyncLocalDatabase()
			if err != nil {
				log.Fatalf("Error: could not sync local database for repository (%s): %s\n", repo.Name, err)
			}
		}
		fmt.Println("All package databases synced successfully!")
	case remove:
		if os.Getuid() != 0 {
			log.Fatalf("Error: this subcommand needs to be run with superuser permissions")
		}
		packages := subcommandArgs
		if len(packages) == 0 {
			fmt.Println("No packages were given")
			return
		}

		operation := &utils.BPMOperation{
			Actions:           make([]utils.OperationAction, 0),
			UnresolvedDepends: make([]string, 0),
			RootDir:           rootDir,
		}

		// Search for packages
		for _, pkg := range packages {
			bpmpkg := utils.GetPackage(pkg, rootDir)
			if bpmpkg == nil {
				continue
			}
			operation.Actions = append(operation.Actions, &utils.RemovePackageAction{BpmPackage: bpmpkg})
		}

		// Skip needed packages if the --unused flag is on
		if removeUnused {
			err := operation.RemoveNeededPackages()
			if err != nil {
				log.Fatalf("Error: could not skip needed packages: %s\n", err)
			}
		}

		// Do package cleanup
		if doCleanup {
			err := operation.Cleanup(verbose)
			if err != nil {
				log.Fatalf("Error: could not perform cleanup for operation: %s\n", err)
			}
		}

		// Show operation summary
		operation.ShowOperationSummary()

		// Confirmation Prompt
		if !yesAll {
			fmt.Printf("Are you sure you wish to remove all %d packages? [y\\N] ", len(operation.Actions))
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Println("Cancelling package removal...")
				os.Exit(1)
			}
		}

		// Execute operation
		err := operation.Execute(verbose, force)
		if err != nil {
			log.Fatalf("Error: could not complete operation: %s\n", err)
		}
	case cleanup:
		if os.Getuid() != 0 {
			log.Fatalf("Error: this subcommand needs to be run with superuser permissions")
		}

		operation := &utils.BPMOperation{
			Actions:           make([]utils.OperationAction, 0),
			UnresolvedDepends: make([]string, 0),
			RootDir:           rootDir,
		}

		// Do package cleanup
		err := operation.Cleanup(verbose)
		if err != nil {
			log.Fatalf("Error: could not perform cleanup for operation: %s\n", err)
		}

		// Show operation summary
		operation.ShowOperationSummary()

		// Confirmation Prompt
		if !yesAll {
			fmt.Printf("Are you sure you wish to remove all %d packages? [y\\N] ", len(operation.Actions))
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Println("Cancelling package removal...")
				os.Exit(1)
			}
		}

		// Execute operation
		err = operation.Execute(verbose, force)
		if err != nil {
			log.Fatalf("Error: could not complete operation: %s\n", err)
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
				log.Fatalf("Error: could not get absolute path of file (%s)\n", file)
			}
			stat, err := os.Stat(absFile)
			if os.IsNotExist(err) {
				log.Fatalf("Error: file (%s) does not exist!\n", absFile)
			}
			pkgs, err := utils.GetInstalledPackages(rootDir)
			if err != nil {
				log.Fatalf("Error: could not get installed packages: %s\n", err.Error())
			}

			if !strings.HasPrefix(absFile, rootDir) {
				log.Fatalf("Error: could not get path of file (%s) relative to root path", absFile)
			}
			absFile, err = filepath.Rel(rootDir, absFile)
			if err != nil {
				log.Fatalf("Error: could not get path of file (%s) relative to root path", absFile)
			}
			absFile = strings.TrimPrefix(absFile, "/")
			if stat.IsDir() {
				absFile = absFile + "/"
			}

			var pkgList []string
			for _, pkg := range pkgs {
				if slices.ContainsFunc(utils.GetPackageFiles(pkg, rootDir), func(entry *utils.PackageFileEntry) bool {
					return entry.Path == absFile
				}) {
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
	fmt.Println("-> bpm info [-R, --repos] <packages...> | shows information on an installed package")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       --repos show information on package in repository")
	fmt.Println("-> bpm list [-R, -c, -n] | lists all installed packages")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -c lists the amount of installed packages")
	fmt.Println("       -n lists only the names of installed packages")
	fmt.Println("-> bpm search <search terms...> | Searches for packages through declared repositories")
	fmt.Println("-> bpm install [-R, -v, -y, -f, -o, -c, -b, -k, --reinstall, --reinstall-all, --no-optional, --installation-reason] <packages...> | installs the following files")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("       -f skips dependency, conflict and architecture checking")
	fmt.Println("       -o=<path> set the binary package output directory (defaults to /var/lib/bpm/compiled)")
	fmt.Println("       -c=<path> set the compilation directory (defaults to /var/tmp)")
	fmt.Println("       -b creates a binary package from a source package after compilation and saves it in the binary package output directory")
	fmt.Println("       -k keeps the compilation directory created by BPM after source package installation")
	fmt.Println("       --reinstall Reinstalls packages even if they do not have a newer version available")
	fmt.Println("       --reinstall-all Same as --reinstall but also reinstalls dependencies")
	fmt.Println("       --no-optional Prevents installation of optional dependencies")
	fmt.Println("       --installation-reason=<manual/dependency> sets the installation reason for all newly installed packages")
	fmt.Println("-> bpm update [-R, -v, -y, -f, --reinstall, --no-sync] | updates all packages that are available in the repositories")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("       -f skips dependency, conflict and architecture checking")
	fmt.Println("       --reinstall Fetches and reinstalls all packages even if they do not have a newer version available")
	fmt.Println("       --no-sync Skips package database syncing")
	fmt.Println("-> bpm sync [-R, -v, -y] | Syncs package databases without updating packages")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("-> bpm remove [-R, -v, -y, --unused, --cleanup] <packages...> | removes the following packages")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("       -unused removes only packages that aren't required as dependencies by other packages")
	fmt.Println("       -cleanup performs a dependency cleanup")
	fmt.Println("-> bpm cleanup [-R, -v, -y] | remove all unused dependency packages")
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
	infoFlagSet.BoolVar(&showRepoInfo, "repos", false, "Show information on package in repository")
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
	installFlagSet.BoolVar(&reinstall, "reinstall", false, "Reinstalls packages even if they do not have a newer version available")
	installFlagSet.BoolVar(&reinstallAll, "reinstall-all", false, "Same as --reinstall but also reinstalls dependencies")
	installFlagSet.BoolVar(&noOptional, "no-optional", false, "Prevents installation of optional dependencies")
	installFlagSet.StringVar(&installationReason, "installation-reason", "", "Set the installation reason for all newly installed packages")
	installFlagSet.Usage = printHelp
	// Update flags
	updateFlagSet := flag.NewFlagSet("Update flags", flag.ExitOnError)
	updateFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	updateFlagSet.BoolVar(&verbose, "v", false, "Show additional information about what BPM is doing")
	updateFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")
	updateFlagSet.BoolVar(&force, "f", false, "Force update by skipping architecture and dependency resolution")
	updateFlagSet.BoolVar(&reinstall, "reinstall", false, "Fetches and reinstalls all packages even if they do not have a newer version available")
	updateFlagSet.BoolVar(&nosync, "no-sync", false, "Skips package database syncing")
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
	removeFlagSet.BoolVar(&removeUnused, "unused", false, "Removes only packages that aren't required as dependencies by other packages")
	removeFlagSet.BoolVar(&doCleanup, "cleanup", false, "Perform a dependency cleanup")
	removeFlagSet.Usage = printHelp
	// Cleanup flags
	cleanupFlagSet := flag.NewFlagSet("Cleanup flags", flag.ExitOnError)
	cleanupFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	cleanupFlagSet.BoolVar(&verbose, "v", false, "Show additional information about what BPM is doing")
	cleanupFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")
	cleanupFlagSet.Usage = printHelp
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
		if reinstallAll {
			reinstall = true
		}
	}
}
