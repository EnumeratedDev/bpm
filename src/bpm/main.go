package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"git.enumerated.dev/bubble-package-manager/bpm/src/bpmlib"
	"log"
	"os"
	"os/exec"
	"path"
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
var installSrcPkgDepends = false
var skipChecks = false
var outputDirectory = ""

func main() {
	err := bpmlib.ReadConfig()
	if err != nil {
		log.Fatalf("Error: could not read BPM config: %s", err)
	}
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
	compile
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
	case "compile":
		return compile
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

		// Read local databases
		err := bpmlib.ReadLocalDatabases()
		if err != nil {
			log.Fatalf("Error: could not read local databases: %s", err)
		}

		for n, pkg := range packages {
			var info *bpmlib.PackageInfo
			isFile := false
			if showRepoInfo {
				var err error
				var entry *bpmlib.RepositoryEntry
				entry, _, err = bpmlib.GetRepositoryEntry(pkg)
				if err != nil {
					if entry = bpmlib.ResolveVirtualPackage(pkg); entry == nil {
						log.Fatalf("Error: could not find package (%s) in any repository\n", pkg)
					}
				}
				info = entry.Info
			} else if stat, err := os.Stat(pkg); err == nil && !stat.IsDir() {
				bpmpkg, err := bpmlib.ReadPackage(pkg)
				if err != nil {
					log.Fatalf("Error: could not read package: %s\n", err)
				}
				info = bpmpkg.PkgInfo
				isFile = true
			} else {
				if isVirtual, p := bpmlib.IsVirtualPackage(pkg, rootDir); isVirtual {
					info = bpmlib.GetPackageInfo(p, rootDir)
				} else {
					info = bpmlib.GetPackageInfo(pkg, rootDir)
				}
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
			fmt.Println(bpmlib.CreateReadableInfo(true, true, true, info, rootDir))
		}
	case list:
		// Read local databases
		err := bpmlib.ReadLocalDatabases()
		if err != nil {
			log.Fatalf("Error: could not read local databases: %s", err)
		}

		packages, err := bpmlib.GetInstalledPackages(rootDir)
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
				info := bpmlib.GetPackageInfo(pkg, rootDir)
				if info == nil {
					fmt.Printf("Package (%s) could not be found\n", pkg)
					continue
				}
				if n != 0 {
					fmt.Println()
				}
				fmt.Println(bpmlib.CreateReadableInfo(true, true, true, info, rootDir))
			}
		}
	case search:
		searchTerms := subcommandArgs
		if len(searchTerms) == 0 {
			log.Fatalf("Error: no search terms given")
		}

		// Read local databases
		err := bpmlib.ReadLocalDatabases()
		if err != nil {
			log.Fatalf("Error: could not read local databases: %s", err)
		}

		for i, term := range searchTerms {
			nameResults := make([]*bpmlib.PackageInfo, 0)
			descResults := make([]*bpmlib.PackageInfo, 0)
			for _, repo := range bpmlib.BPMConfig.Repositories {
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
		// Check for required permissions
		if os.Getuid() != 0 {
			log.Fatalf("Error: this subcommand needs to be run with superuser permissions")
		}

		// Return if no packages are specified
		if len(subcommandArgs) == 0 {
			fmt.Println("No packages or files were given to install")
			return
		}

		// Check if installationReason argument is valid
		ir := bpmlib.InstallationReasonUnknown
		switch installationReason {
		case "manual":
			ir = bpmlib.InstallationReasonManual
		case "dependency":
			ir = bpmlib.InstallationReasonDependency
		case "":
		default:
			log.Fatalf("Error: %s is not a valid installation reason", installationReason)
		}

		// Get reinstall method
		var reinstallMethod bpmlib.ReinstallMethod
		if reinstallAll {
			reinstallMethod = bpmlib.ReinstallMethodAll
		} else if reinstall {
			reinstallMethod = bpmlib.ReinstallMethodSpecified
		} else {
			reinstallMethod = bpmlib.ReinstallMethodNone
		}

		// Read local databases
		err := bpmlib.ReadLocalDatabases()
		if err != nil {
			log.Fatalf("Error: could not read local databases: %s", err)
		}

		// Create installation operation
		operation, err := bpmlib.InstallPackages(rootDir, ir, reinstallMethod, !noOptional, force, verbose, subcommandArgs...)
		if errors.As(err, &bpmlib.PackageNotFoundErr{}) || errors.As(err, &bpmlib.DependencyNotFoundErr{}) || errors.As(err, &bpmlib.PackageConflictErr{}) {
			log.Fatalf("Error: %s", err)
		} else if err != nil {
			log.Fatalf("Error: could not setup operation: %s\n", err)
		}

		// Exit if operation contains no actions
		if len(operation.Actions) == 0 {
			fmt.Println("No action needs to be taken")
			return
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

		// Executing hooks
		fmt.Println("Running hooks...")
		err = operation.RunHooks(verbose)
		if err != nil {
			log.Fatalf("Error: could not run hooks: %s\n", err)
		}
	case update:
		// Check for required permissions
		if os.Getuid() != 0 {
			log.Fatalf("Error: this subcommand needs to be run with superuser permissions")
		}

		// Read local databases if no sync
		if nosync {
			err := bpmlib.ReadLocalDatabases()
			if err != nil {
				log.Fatalf("Error: could not read local databases: %s", err)
			}
		}

		// Create update operation
		operation, err := bpmlib.UpdatePackages(rootDir, !nosync, !noOptional, force, verbose)
		if errors.As(err, &bpmlib.PackageNotFoundErr{}) || errors.As(err, &bpmlib.DependencyNotFoundErr{}) || errors.As(err, &bpmlib.PackageConflictErr{}) {
			log.Fatalf("Error: %s", err)
		} else if err != nil {
			log.Fatalf("Error: could not setup operation: %s\n", err)
		}

		// Exit if operation contains no actions
		if len(operation.Actions) == 0 {
			fmt.Println("No action needs to be taken")
			return
		}

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

		// Executing hooks
		fmt.Println("Running hooks...")
		err = operation.RunHooks(verbose)
		if err != nil {
			log.Fatalf("Error: could not run hooks: %s\n", err)
		}
	case sync:
		// Check for required permissions
		if os.Getuid() != 0 {
			log.Fatalf("Error: this subcommand needs to be run with superuser permissions")
		}

		// Confirmation Prompt
		if !yesAll {
			fmt.Printf("Are you sure you wish to sync all databases? [y\\N] ")
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Println("Cancelling database synchronization...")
				os.Exit(1)
			}
		}

		// Sync databases
		err := bpmlib.SyncDatabase(verbose)
		if err != nil {
			log.Fatalf("Error: could not sync local database: %s\n", err)
		}

		fmt.Println("All package databases synced successfully!")
	case remove:
		// Check for required permissions
		if os.Getuid() != 0 {
			log.Fatalf("Error: this subcommand needs to be run with superuser permissions")
		}

		if len(subcommandArgs) == 0 {
			fmt.Println("No packages were given")
			return
		}

		// Read local databases
		err := bpmlib.ReadLocalDatabases()
		if err != nil {
			log.Fatalf("Error: could not read local databases: %s", err)
		}

		// Create remove operation
		operation, err := bpmlib.RemovePackages(rootDir, removeUnused, doCleanup, verbose, subcommandArgs...)
		if errors.As(err, &bpmlib.PackageNotFoundErr{}) || errors.As(err, &bpmlib.DependencyNotFoundErr{}) || errors.As(err, &bpmlib.PackageConflictErr{}) {
			log.Fatalf("Error: %s", err)
		} else if err != nil {
			log.Fatalf("Error: could not setup operation: %s\n", err)
		}

		// Exit if operation contains no actions
		if len(operation.Actions) == 0 {
			fmt.Println("No action needs to be taken")
			return
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

		// Executing hooks
		fmt.Println("Running hooks...")
		err = operation.RunHooks(verbose)
		if err != nil {
			log.Fatalf("Error: could not run hooks: %s\n", err)
		}
	case cleanup:
		// Check for required permissions
		if os.Getuid() != 0 {
			log.Fatalf("Error: this subcommand needs to be run with superuser permissions")
		}

		// Read local databases
		err := bpmlib.ReadLocalDatabases()
		if err != nil {
			log.Fatalf("Error: could not read local databases: %s", err)
		}

		// Create cleanup operation
		operation, err := bpmlib.CleanupPackages(rootDir, verbose)
		if errors.As(err, &bpmlib.PackageNotFoundErr{}) || errors.As(err, &bpmlib.DependencyNotFoundErr{}) || errors.As(err, &bpmlib.PackageConflictErr{}) {
			log.Fatalf("Error: %s", err)
		} else if err != nil {
			log.Fatalf("Error: could not setup operation: %s\n", err)
		}

		// Exit if operation contains no actions
		if len(operation.Actions) == 0 {
			fmt.Println("No action needs to be taken")
			return
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

		// Executing hooks
		fmt.Println("Running hooks...")
		err = operation.RunHooks(verbose)
		if err != nil {
			log.Fatalf("Error: could not run hooks: %s\n", err)
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
			pkgs, err := bpmlib.GetInstalledPackages(rootDir)
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
				if slices.ContainsFunc(bpmlib.GetPackageFiles(pkg, rootDir), func(entry *bpmlib.PackageFileEntry) bool {
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
	case compile:
		if len(subcommandArgs) == 0 {
			fmt.Println("No source packages were given")
			return
		}

		// Read local databases
		err := bpmlib.ReadLocalDatabases()
		if err != nil {
			log.Fatalf("Error: could not read local databases: %s", err)
		}

		// Compile packages
		for _, sourcePackage := range subcommandArgs {
			if _, err := os.Stat(sourcePackage); os.IsNotExist(err) {
				log.Fatalf("Error: file (%s) does not exist!", sourcePackage)
			}

			// Read archive
			bpmpkg, err := bpmlib.ReadPackage(sourcePackage)
			if err != nil {
				log.Fatalf("Could not read package (%s): %s", sourcePackage, err)
			}

			// Ensure archive is source BPM package
			if bpmpkg.PkgInfo.Type != "source" {
				log.Fatalf("Error: cannot compile a non-source package!")
			}

			// Get direct runtime and make dependencies
			totalDepends := make([]string, 0)
			for _, depend := range bpmpkg.PkgInfo.GetAllDependencies(true, false) {
				if !slices.Contains(totalDepends, depend) {
					totalDepends = append(totalDepends, depend)
				}
			}

			// Get unmet dependencies
			unmetDepends := slices.Clone(totalDepends)
			installedPackages, err := bpmlib.GetInstalledPackages("/")
			if err != nil {
				log.Fatalf("Error: could not get installed packages: %s\n", err)
			}
			for i := len(unmetDepends) - 1; i >= 0; i-- {
				if slices.Contains(installedPackages, unmetDepends[i]) {
					unmetDepends = append(unmetDepends[:i], unmetDepends[i+1:]...)
				}
			}

			// Install missing source package dependencies
			if installSrcPkgDepends && len(unmetDepends) > 0 {
				// Get path to current executable
				executable, err := os.Executable()
				if err != nil {
					log.Fatalf("Error: could not get path to executable: %s\n", err)
				}

				// Run 'bpm install' using the set privilege escalator command
				cmd := exec.Command(bpmlib.BPMConfig.PrivilegeEscalatorCmd, executable, "install", "--installation-reason=dependency", strings.Join(unmetDepends, " "))
				if yesAll {
					cmd.Args = slices.Insert(cmd.Args, 3, "-y")
				}
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Stdin = os.Stdin
				if verbose {
					fmt.Println("Running command: " + cmd.String())
				}
				err = cmd.Run()
				if err != nil {
					log.Fatalf("Error: dependency installation command failed: %s\n", err)
				}
			} else {
				// Ensure the required dependencies are installed
				if len(unmetDepends) != 0 {
					log.Fatalf("Error: could not resolve dependencies: the following dependencies were not found in any repositories: " + strings.Join(unmetDepends, ", "))
				}
			}

			// Get current working directory
			workdir, err := os.Getwd()
			if err != nil {
				log.Fatalf("Error: could not get working directory: %s", err)
			}

			// Get user home directory
			homedir, err := os.UserHomeDir()
			if err != nil {
				log.Fatalf("Error: could not get user home directory: %s", err)
			}

			// Trim output directory
			outputDirectory = strings.TrimSpace(outputDirectory)
			if outputDirectory != "/" {
				outputDirectory = strings.TrimSuffix(outputDirectory, "/")
			}

			// Set output directory if empty
			if outputDirectory == "" {
				outputDirectory = workdir
			}

			// Replace first tilde with user home directory
			if strings.Split(outputDirectory, "/")[0] == "~" {
				outputDirectory = strings.Replace(outputDirectory, "~", homedir, 1)
			}

			// Prepend current working directory to output directory if not an absolute path
			if outputDirectory != "" && !strings.HasPrefix(outputDirectory, "/") {
				outputDirectory = filepath.Join(workdir, outputDirectory)
			}

			// Clean path
			path.Clean(outputDirectory)

			// Ensure output directory exists and is a directory
			stat, err := os.Stat(outputDirectory)
			if err != nil {
				log.Fatalf("Error: could not stat output directory (%s): %s", outputDirectory, err)
			}
			if !stat.IsDir() {
				log.Fatalf("Error: output directory (%s) is not a directory", outputDirectory)
			}

			outputBpmPackages, err := bpmlib.CompileSourcePackage(sourcePackage, outputDirectory, skipChecks)
			if err != nil {
				log.Fatalf("Error: could not compile source package (%s): %s", sourcePackage, err)
			}

			for k, v := range outputBpmPackages {
				fmt.Printf("Package (%s) was successfully compiled! Binary package generated at: %s\n", k, v)
			}

			// Remove unused packages
			if installSrcPkgDepends && len(unmetDepends) > 0 {
				// Get path to current executable
				executable, err := os.Executable()
				if err != nil {
					log.Fatalf("Error: could not get path to executable: %s\n", err)
				}

				// Run 'bpm cleanup' using the set privilege escalator command
				cmd := exec.Command(bpmlib.BPMConfig.PrivilegeEscalatorCmd, executable, "cleanup")
				if yesAll {
					cmd.Args = slices.Insert(cmd.Args, 3, "-y")
				}
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Stdin = os.Stdin
				if verbose {
					fmt.Println("Running command: " + cmd.String())
				}
				err = cmd.Run()
				if err != nil {
					log.Fatalf("Error: dependency cleanup command failed: %s\n", err)
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
	fmt.Println("       --unused removes only packages that aren't required as dependencies by other packages")
	fmt.Println("       --cleanup performs a dependency cleanup")
	fmt.Println("-> bpm cleanup [-R, -v, -y] | remove all unused dependency packages")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("-> bpm file [-R] <files...> | shows what packages the following packages are managed by")
	fmt.Println("       -R=<root_path> lets you define the root path which will be used")
	fmt.Println("-> bpm compile [-d, -s, -o] <source packages...> | Compile source BPM package")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -d installs required dependencies for package compilation")
	fmt.Println("       -s skips the check function in source.sh scripts")
	fmt.Println("       -o sets output directory")
	fmt.Println("       -y skips the confirmation prompt")

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
	// Compile flags
	compileFlagSet := flag.NewFlagSet("Compile flags", flag.ExitOnError)
	compileFlagSet.BoolVar(&installSrcPkgDepends, "d", false, "Install required dependencies for package compilation")
	compileFlagSet.BoolVar(&skipChecks, "s", false, "Skip the check function in source.sh scripts")
	compileFlagSet.StringVar(&outputDirectory, "o", "", "Set output directory")
	compileFlagSet.BoolVar(&verbose, "v", false, "Show additional information about what BPM is doing")
	compileFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")

	compileFlagSet.Usage = printHelp
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
		} else if getCommandType() == compile {
			err := compileFlagSet.Parse(subcommandArgs)
			if err != nil {
				return
			}
			subcommandArgs = compileFlagSet.Args()
		}
		if reinstallAll {
			reinstall = true
		}
	}
}
