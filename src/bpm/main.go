package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"git.enumerated.dev/bubble-package-manager/bpm/src/bpmlib"
)

/* -------------BPM | Bubble Package Manager-------------- */
/*        Made By EnumDev (Previously CapCreeperGR)        */
/*             A simple-to-use package manager             */
/* ------------------------------------------------------- */

var bpmVer = "0.6.0"

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
var installOptional = false
var installationReason = ""
var nosync = true
var removeUnused = false
var doCleanup = false
var showDatabaseInfo = false
var installSrcPkgDepends = false
var skipChecks = false
var outputDirectory = ""
var outputFd = -1
var cleanupDependencies = false
var cleanupMakeDependencies = false
var cleanupCompilationFiles = false
var cleanupCompiledPackages = false
var cleanupFetchedPackages = false

var exitCode = 0

func main() {
	err := bpmlib.ReadConfig()
	if err != nil {
		log.Fatalf("Error: could not read BPM config: %s", err)
	}
	resolveFlags()
	resolveCommand()

	if exitCode != 0 {
		os.Exit(exitCode)
	}
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
		err := bpmlib.ReadLocalDatabaseFiles()
		if err != nil {
			log.Printf("Error: could not read local databases: %s", err)
			exitCode = 1
			return
		}

		for n, pkg := range packages {
			var info *bpmlib.PackageInfo
			isFile := false
			showInstallationReason := false
			if showDatabaseInfo {
				var err error
				var entry *bpmlib.BPMDatabaseEntry
				entry, _, err = bpmlib.GetDatabaseEntry(pkg)
				if err != nil {
					if entry = bpmlib.ResolveVirtualPackage(pkg); entry == nil {
						log.Printf("Error: could not find package (%s) in any database\n", pkg)
						exitCode = 1
						return
					}
				}
				info = entry.Info
			} else if stat, err := os.Stat(pkg); err == nil && !stat.IsDir() {
				bpmpkg, err := bpmlib.ReadPackage(pkg)
				if err != nil {
					log.Printf("Error: could not read package: %s\n", err)
					exitCode = 1
					return
				}
				info = bpmpkg.PkgInfo
				isFile = true
			} else {
				if isVirtual, p := bpmlib.IsVirtualPackage(pkg, rootDir); isVirtual {
					info = bpmlib.GetPackageInfo(p, rootDir)
				} else {
					info = bpmlib.GetPackageInfo(pkg, rootDir)
				}
				showInstallationReason = true
			}
			if info == nil {
				log.Printf("Error: package (%s) is not installed\n", pkg)
				exitCode = 1
				return
			}
			if n != 0 {
				fmt.Println()
			}
			if isFile {
				abs, err := filepath.Abs(pkg)
				if err != nil {
					log.Printf("Error: could not get absolute path of file (%s)\n", abs)
					exitCode = 1
					return
				}
				fmt.Println("File: " + abs)
			}
			fmt.Println(bpmlib.CreateReadableInfo(true, true, true, showInstallationReason, info, rootDir))
		}
	case list:
		// Read local databases
		err := bpmlib.ReadLocalDatabaseFiles()
		if err != nil {
			log.Printf("Error: could not read local databases: %s", err)
			exitCode = 1
			return
		}

		packages, err := bpmlib.GetInstalledPackages(rootDir)
		if err != nil {
			log.Printf("Error: could not get installed packages: %s", err.Error())
			exitCode = 1
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
				fmt.Println(bpmlib.CreateReadableInfo(true, true, true, true, info, rootDir))
			}
		}
	case search:
		searchTerms := subcommandArgs
		if len(searchTerms) == 0 {
			log.Printf("Error: no search terms given")
			exitCode = 1
			return
		}

		// Read local databases
		err := bpmlib.ReadLocalDatabaseFiles()
		if err != nil {
			log.Printf("Error: could not read local databases: %s", err)
			exitCode = 1
			return
		}

		for i, term := range searchTerms {
			nameResults := make([]*bpmlib.PackageInfo, 0)
			descResults := make([]*bpmlib.PackageInfo, 0)
			for _, db := range bpmlib.BPMDatabases {
				for _, entry := range db.Entries {
					if strings.Contains(entry.Info.Name, term) {
						nameResults = append(nameResults, entry.Info)
					} else if strings.Contains(entry.Info.Description, term) {
						descResults = append(descResults, entry.Info)
					}
				}
			}
			results := append(nameResults, descResults...)
			if len(results) == 0 {
				log.Printf("Error: no results for term (%s) were found\n", term)
				exitCode = 1
				return
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
			log.Printf("Error: this subcommand needs to be run with superuser permissions")
			exitCode = 1
			return
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
		case "make-dependency":
			ir = bpmlib.InstallationReasonMakeDependency
		case "":
		default:
			log.Printf("Error: %s is not a valid installation reason", installationReason)
			exitCode = 1
			return
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

		// Create BPM Lock file
		fileLock, err := bpmlib.LockBPM(rootDir)
		if err != nil {
			log.Printf("Error: could not create BPM lock file: %s", err)
			exitCode = 1
			return
		}
		defer fileLock.Unlock()

		// Read local databases
		err = bpmlib.ReadLocalDatabaseFiles()
		if err != nil {
			log.Printf("Error: could not read local databases: %s", err)
			exitCode = 1
			return
		}

		// Create installation operation
		operation, err := bpmlib.InstallPackages(rootDir, ir, reinstallMethod, installOptional, force, verbose, subcommandArgs...)
		if errors.As(err, &bpmlib.PackageNotFoundErr{}) || errors.As(err, &bpmlib.DependencyNotFoundErr{}) || errors.As(err, &bpmlib.PackageConflictErr{}) {
			log.Printf("Error: %s", err)
			exitCode = 1
			return
		} else if err != nil {
			log.Printf("Error: could not setup operation: %s\n", err)
			exitCode = 1
			return
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
				exitCode = 1
				return
			}
		}

		// Execute operation
		err = operation.Execute(verbose, force)
		if err != nil {
			log.Printf("Error: could not complete operation: %s\n", err)
			exitCode = 1
			return
		}

		// Executing hooks
		fmt.Println("Running hooks...")
		err = operation.RunHooks(verbose)
		if err != nil {
			log.Printf("Error: could not run hooks: %s\n", err)
			exitCode = 1
			return
		}
	case update:
		// Check for required permissions
		if os.Getuid() != 0 {
			log.Printf("Error: this subcommand needs to be run with superuser permissions")
			exitCode = 1
			return
		}

		// Create BPM Lock file
		fileLock, err := bpmlib.LockBPM(rootDir)
		if err != nil {
			log.Printf("Error: could not create BPM lock file: %s", err)
			exitCode = 1
			return
		}
		defer fileLock.Unlock()

		// Read local databases if no sync
		if nosync {
			err := bpmlib.ReadLocalDatabaseFiles()
			if err != nil {
				log.Printf("Error: could not read local databases: %s", err)
				exitCode = 1
				return
			}
		}

		// Create update operation
		operation, err := bpmlib.UpdatePackages(rootDir, !nosync, installOptional, force, verbose)
		if errors.As(err, &bpmlib.PackageNotFoundErr{}) || errors.As(err, &bpmlib.DependencyNotFoundErr{}) || errors.As(err, &bpmlib.PackageConflictErr{}) {
			log.Printf("Error: %s", err)
			exitCode = 1
			return
		} else if err != nil {
			log.Printf("Error: could not setup operation: %s\n", err)
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
				exitCode = 1
				return
			}
		}

		// Execute operation
		err = operation.Execute(verbose, force)
		if err != nil {
			log.Printf("Error: could not complete operation: %s\n", err)
			exitCode = 1
			return
		}

		// Executing hooks
		fmt.Println("Running hooks...")
		err = operation.RunHooks(verbose)
		if err != nil {
			log.Printf("Error: could not run hooks: %s\n", err)
			exitCode = 1
			return
		}
	case sync:
		// Check for required permissions
		if os.Getuid() != 0 {
			log.Printf("Error: this subcommand needs to be run with superuser permissions")
			exitCode = 1
			return
		}

		// Create BPM Lock file
		fileLock, err := bpmlib.LockBPM(rootDir)
		if err != nil {
			log.Printf("Error: could not create BPM lock file: %s", err)
			exitCode = 1
			return
		}
		defer fileLock.Unlock()

		// Confirmation Prompt
		if !yesAll {
			fmt.Printf("Are you sure you wish to sync all databases? [y\\N] ")
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Println("Cancelling database synchronization...")
				exitCode = 1
				return
			}
		}

		// Sync databases
		err = bpmlib.SyncDatabase(verbose)
		if err != nil {
			log.Printf("Error: could not sync local database: %s\n", err)
			exitCode = 1
			return
		}

		fmt.Println("All package databases synced successfully!")
	case remove:
		// Check for required permissions
		if os.Getuid() != 0 {
			log.Printf("Error: this subcommand needs to be run with superuser permissions")
			exitCode = 1
			return
		}

		if len(subcommandArgs) == 0 {
			fmt.Println("No packages were given")
			return
		}

		// Create BPM Lock file
		fileLock, err := bpmlib.LockBPM(rootDir)
		if err != nil {
			log.Printf("Error: could not create BPM lock file: %s", err)
			exitCode = 1
			return
		}
		defer fileLock.Unlock()

		// Read local databases
		err = bpmlib.ReadLocalDatabaseFiles()
		if err != nil {
			log.Printf("Error: could not read local databases: %s", err)
			exitCode = 1
			return
		}

		// Create remove operation
		operation, err := bpmlib.RemovePackages(rootDir, removeUnused, doCleanup, subcommandArgs...)
		if errors.As(err, &bpmlib.PackageNotFoundErr{}) || errors.As(err, &bpmlib.DependencyNotFoundErr{}) || errors.As(err, &bpmlib.PackageConflictErr{}) {
			log.Printf("Error: %s", err)
			exitCode = 1
			return
		} else if err != nil {
			log.Printf("Error: could not setup operation: %s\n", err)
			exitCode = 1
			return
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
				exitCode = 1
				return
			}
		}

		// Execute operation
		err = operation.Execute(verbose, force)
		if err != nil {
			log.Printf("Error: could not complete operation: %s\n", err)
			exitCode = 1
			return
		}

		// Executing hooks
		fmt.Println("Running hooks...")
		err = operation.RunHooks(verbose)
		if err != nil {
			log.Printf("Error: could not run hooks: %s\n", err)
			exitCode = 1
			return
		}
	case cleanup:
		// Check for required permissions
		if os.Getuid() != 0 {
			log.Printf("Error: this subcommand needs to be run with superuser permissions")
			exitCode = 1
			return
		}

		// Create BPM Lock file
		fileLock, err := bpmlib.LockBPM(rootDir)
		if err != nil {
			log.Printf("Error: could not create BPM lock file: %s", err)
			exitCode = 1
			return
		}
		defer fileLock.Unlock()

		err = bpmlib.CleanupCache(rootDir, cleanupCompilationFiles, cleanupCompiledPackages, cleanupFetchedPackages, verbose)
		if err != nil {
			log.Printf("Error: could not complete cache cleanup: %s", err)
			exitCode = 1
			return
		}

		if cleanupDependencies || cleanupMakeDependencies {
			// Read local databases
			err := bpmlib.ReadLocalDatabaseFiles()
			if err != nil {
				log.Printf("Error: could not read local databases: %s", err)
				exitCode = 1
				return
			}

			// Create cleanup operation
			operation, err := bpmlib.CleanupPackages(cleanupMakeDependencies, rootDir)
			if errors.As(err, &bpmlib.PackageNotFoundErr{}) || errors.As(err, &bpmlib.DependencyNotFoundErr{}) || errors.As(err, &bpmlib.PackageConflictErr{}) {
				log.Printf("Error: %s", err)
				exitCode = 1
				return
			} else if err != nil {
				log.Printf("Error: could not setup operation: %s\n", err)
				exitCode = 1
				return
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
					exitCode = 1
					return
				}
			}

			// Execute operation
			err = operation.Execute(verbose, force)
			if err != nil {
				log.Printf("Error: could not complete operation: %s\n", err)
				exitCode = 1
				return
			}

			// Executing hooks
			fmt.Println("Running hooks...")
			err = operation.RunHooks(verbose)
			if err != nil {
				log.Printf("Error: could not run hooks: %s\n", err)
				exitCode = 1
				return
			}
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
				log.Printf("Error: could not get absolute path of file (%s)\n", file)
				exitCode = 1
				return
			}
			stat, err := os.Stat(absFile)
			if os.IsNotExist(err) {
				log.Printf("Error: file (%s) does not exist!\n", absFile)
				exitCode = 1
				return
			}
			pkgs, err := bpmlib.GetInstalledPackages(rootDir)
			if err != nil {
				log.Printf("Error: could not get installed packages: %s\n", err.Error())
				exitCode = 1
				return
			}

			if !strings.HasPrefix(absFile, rootDir) {
				log.Printf("Error: could not get path of file (%s) relative to root path", absFile)
				exitCode = 1
				return
			}
			absFile, err = filepath.Rel(rootDir, absFile)
			if err != nil {
				log.Printf("Error: could not get path of file (%s) relative to root path", absFile)
				exitCode = 1
				return
			}
			absFile = strings.TrimPrefix(absFile, "/")
			if stat.IsDir() {
				absFile = absFile + "/"
			}

			var pkgList []string
			for _, pkg := range pkgs {
				if slices.ContainsFunc(bpmlib.GetPackage(pkg, rootDir).PkgFiles, func(entry *bpmlib.PackageFileEntry) bool {
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
		err := bpmlib.ReadLocalDatabaseFiles()
		if err != nil {
			log.Printf("Error: could not read local databases: %s", err)
			exitCode = 1
			return
		}

		// Compile packages
		for _, sourcePackage := range subcommandArgs {
			if _, err := os.Stat(sourcePackage); os.IsNotExist(err) {
				log.Printf("Error: file (%s) does not exist!", sourcePackage)
				exitCode = 1
				return
			}

			// Read archive
			bpmpkg, err := bpmlib.ReadPackage(sourcePackage)
			if err != nil {
				log.Printf("Could not read package (%s): %s", sourcePackage, err)
				exitCode = 1
				return
			}

			// Ensure archive is source BPM package
			if bpmpkg.PkgInfo.Type != "source" {
				log.Printf("Error: cannot compile a non-source package!")
				exitCode = 1
				return
			}

			// Get direct runtime and make dependencies
			totalDepends := make([]string, 0)
			for _, depend := range bpmpkg.PkgInfo.GetDependencies(true, false) {
				if !slices.Contains(totalDepends, depend.PkgName) {
					totalDepends = append(totalDepends, depend.PkgName)
				}
			}

			// Get unmet dependencies
			unmetDepends := slices.Clone(totalDepends)
			installedPackages, err := bpmlib.GetInstalledPackages("/")
			if err != nil {
				log.Printf("Error: could not get installed packages: %s\n", err)
				exitCode = 1
				return
			}
			for i := len(unmetDepends) - 1; i >= 0; i-- {
				if slices.Contains(installedPackages, unmetDepends[i]) {
					unmetDepends = append(unmetDepends[:i], unmetDepends[i+1:]...)
				} else if ok, _ := bpmlib.IsVirtualPackage(unmetDepends[i], rootDir); ok {
					unmetDepends = append(unmetDepends[:i], unmetDepends[i+1:]...)
				}
			}

			// Install missing source package dependencies
			if installSrcPkgDepends && len(unmetDepends) > 0 {
				// Get path to current executable
				executable, err := os.Executable()
				if err != nil {
					log.Printf("Error: could not get path to executable: %s\n", err)
					exitCode = 1
					return
				}

				// Run 'bpm install' using the set privilege escalator command
				args := []string{executable, "install", "--installation-reason=make-dependency"}
				args = append(args, unmetDepends...)
				cmd := exec.Command(bpmlib.CompilationBPMConfig.PrivilegeEscalatorCmd, args...)
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
					log.Printf("Error: dependency installation command failed: %s\n", err)
					exitCode = 1
					return
				}
			} else {
				// Ensure the required dependencies are installed
				if len(unmetDepends) != 0 {
					log.Printf("Error: could not resolve dependencies: the following dependencies were not found in any databases: " + strings.Join(unmetDepends, ", "))
					exitCode = 1
					return
				}
			}

			// Get current working directory
			workdir, err := os.Getwd()
			if err != nil {
				log.Printf("Error: could not get working directory: %s", err)
				exitCode = 1
				return
			}

			// Get user home directory
			homedir, err := os.UserHomeDir()
			if err != nil {
				log.Printf("Error: could not get user home directory: %s", err)
				exitCode = 1
				return
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
				log.Printf("Error: could not stat output directory (%s): %s", outputDirectory, err)
				exitCode = 1
				return
			}
			if !stat.IsDir() {
				log.Printf("Error: output directory (%s) is not a directory", outputDirectory)
				exitCode = 1
				return
			}

			outputBpmPackages, err := bpmlib.CompileSourcePackage(sourcePackage, outputDirectory, skipChecks)
			if err != nil {
				log.Printf("Error: could not compile source package (%s): %s", sourcePackage, err)
				exitCode = 1
				return
			}

			for k, v := range outputBpmPackages {
				if outputFd < 0 {
					fmt.Printf("Package (%s) was successfully compiled! Binary package generated at: %s\n", k, v)
				} else {
					f := os.NewFile(uintptr(outputFd), "output_file_descrptor")
					defer f.Close()
					if f == nil {
						log.Printf("Warning: invalid file descriptor: %d", outputFd)
						break
					}
					fmt.Fprintln(f, v)
				}
			}

			// Remove unused packages
			if installSrcPkgDepends && len(unmetDepends) > 0 {
				// Get path to current executable
				executable, err := os.Executable()
				if err != nil {
					log.Printf("Error: could not get path to executable: %s\n", err)
					exitCode = 1
					return
				}

				// Run 'bpm cleanup' using the set privilege escalator command
				cmd := exec.Command(bpmlib.CompilationBPMConfig.PrivilegeEscalatorCmd, executable, "cleanup")
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
					log.Printf("Error: dependency cleanup command failed: %s\n", err)
					exitCode = 1
					return
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
	fmt.Println("-> bpm info [-R, --databases] <packages...> | shows information on an installed package")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       --databases show information on package in configured databases")
	fmt.Println("-> bpm list [-R, -c, -n] | lists all installed packages")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -c lists the amount of installed packages")
	fmt.Println("       -n lists only the names of installed packages")
	fmt.Println("-> bpm search <search terms...> | Searches for packages through configured databases")
	fmt.Println("-> bpm install [-R, -v, -y, -f, --reinstall, --reinstall-all, --no-optional, --installation-reason] <packages...> | installs the following files")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("       -f skips dependency, conflict and architecture checking")
	fmt.Println("       --reinstall Reinstalls packages even if they do not have a newer version available")
	fmt.Println("       --reinstall-all Same as --reinstall but also reinstalls dependencies")
	fmt.Println("       --optional Installs all optional dependencies")
	fmt.Println("       --installation-reason=<manual/dependency> sets the installation reason for all newly installed packages")
	fmt.Println("-> bpm update [-R, -v, -y, -f, --reinstall, --no-sync] | updates all packages that are available in the configured databases")
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
	fmt.Println("-> bpm cleanup [-R, -v, -y, --depends, --compilation-files, --compiled-pkgs, --fetched-pkgs] | remove all unused dependencies and cache directories")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("       --depends performs a dependency cleanup")
	fmt.Println("       --make-depends performs a make dependency cleanup")
	fmt.Println("       --compilation-files performs a cleanup of compilation files")
	fmt.Println("       --compiled-pkgs performs a cleanup of compilation compiled binary packages")
	fmt.Println("       --fetched-pkgs performs a cleanup of fetched packages from databases")
	fmt.Println("-> bpm file [-R] <files...> | shows what packages the following packages are managed by")
	fmt.Println("       -R=<root_path> lets you define the root path which will be used")
	fmt.Println("-> bpm compile [-d, -s, -o] <source packages...> | Compile source BPM package")
	fmt.Println("       -v Show additional information about what BPM is doing")
	fmt.Println("       -d installs required dependencies for package compilation")
	fmt.Println("       -s skips the check function in source.sh scripts")
	fmt.Println("       -o sets output directory")
	fmt.Println("       -y skips the confirmation prompt")
	fmt.Println("       --fd=<file descriptor> Set the file descriptor output package names will be written to")

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
	infoFlagSet.BoolVar(&showDatabaseInfo, "databases", false, "Show information on package in configured databases")
	infoFlagSet.Usage = printHelp
	// Install flags
	installFlagSet := flag.NewFlagSet("Install flags", flag.ExitOnError)
	installFlagSet.StringVar(&rootDir, "R", "/", "Set the destination root")
	installFlagSet.BoolVar(&verbose, "v", false, "Show additional information about what BPM is doing")
	installFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")
	installFlagSet.BoolVar(&force, "f", false, "Force installation by skipping architecture and dependency resolution")
	installFlagSet.BoolVar(&reinstall, "reinstall", false, "Reinstalls packages even if they do not have a newer version available")
	installFlagSet.BoolVar(&reinstallAll, "reinstall-all", false, "Same as --reinstall but also reinstalls dependencies")
	installFlagSet.BoolVar(&installOptional, "optional", false, "Installs all optional dependencies")
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
	cleanupFlagSet.BoolVar(&cleanupDependencies, "depends", false, "Perform a dependency cleanup")
	cleanupFlagSet.BoolVar(&cleanupMakeDependencies, "make-depends", false, "Perform a make dependency cleanup")
	cleanupFlagSet.BoolVar(&cleanupCompilationFiles, "compilation-files", false, "Perform a cleanup of compilation files")
	cleanupFlagSet.BoolVar(&cleanupCompiledPackages, "compiled-pkgs", false, "Perform a cleanup of compilation compiled binary packages")
	cleanupFlagSet.BoolVar(&cleanupFetchedPackages, "fetched-pkgs", false, "Perform a cleanup of fetched packages from databases")
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
	compileFlagSet.IntVar(&outputFd, "fd", -1, "Set the file descriptor output package names will be written to")
	compileFlagSet.BoolVar(&verbose, "v", false, "Show additional information about what BPM is doing")
	compileFlagSet.BoolVar(&yesAll, "y", false, "Skip confirmation prompts")
	compileFlagSet.Usage = printHelp

	isFlagSet := func(flagSet *flag.FlagSet, name string) bool {
		found := false
		flagSet.Visit(func(f *flag.Flag) {
			if f.Name == name {
				found = true
			}
		})
		return found
	}

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
		} else if getCommandType() == cleanup {
			err := cleanupFlagSet.Parse(subcommandArgs)
			if err != nil {
				return
			}
			if !isFlagSet(cleanupFlagSet, "depends") && !isFlagSet(cleanupFlagSet, "make-depends") && !isFlagSet(cleanupFlagSet, "compilation-files") && !isFlagSet(cleanupFlagSet, "compiled-pkgs") && !isFlagSet(cleanupFlagSet, "fetched-pkgs") {
				cleanupDependencies = true
				cleanupMakeDependencies = bpmlib.MainBPMConfig.CleanupMakeDependencies
				cleanupCompilationFiles = true
				cleanupCompiledPackages = true
				cleanupFetchedPackages = true
			}
			subcommandArgs = cleanupFlagSet.Args()
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
