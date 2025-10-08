package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"git.enumerated.dev/bubble-package-manager/bpm/src/bpmlib"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
	flag "github.com/spf13/pflag"
)

/* -------------BPM | Bubble Package Manager-------------- */
/*        Made By EnumDev (Previously CapCreeperGR)        */
/*             A simple-to-use package manager             */
/* ------------------------------------------------------- */

var bpmVer = "0.6.0"

var currentFlagSet *flag.FlagSet

var exitCode = 0

func main() {
	err := bpmlib.ReadConfig()
	if err != nil {
		log.Fatalf("Error: could not read BPM config: %s", err)
	}

	// Execute subcommand
	subcommand := "help"
	if len(os.Args) >= 2 {
		subcommand = os.Args[1]
	}

	switch subcommand {
	case "version":
		fmt.Println("Bubble Package Manager (BPM)")
		fmt.Println("Version: " + bpmVer)
	case "q", "query":
		// Setup flags and help
		currentFlagSet = flag.NewFlagSet("query", flag.ExitOnError)
		currentFlagSet.StringP("root", "R", "/", "Operate on specified root directory")
		currentFlagSet.BoolP("database", "d", false, "Show package information from remote databases")
		setupFlagsAndHelp(currentFlagSet, fmt.Sprintf("bpm %s <options>", subcommand), "Show information on the specified packages", os.Args[2:])

		showPackageInfo()
	case "l", "list":
		// Setup flags and help
		currentFlagSet = flag.NewFlagSet("list", flag.ExitOnError)
		currentFlagSet.StringP("root", "R", "/", "Operate on specified root directory")
		currentFlagSet.BoolP("count", "c", false, "Show total package count")
		currentFlagSet.BoolP("names", "n", false, "Show all package names")
		setupFlagsAndHelp(currentFlagSet, fmt.Sprintf("bpm %s <options>", subcommand), "List packages", os.Args[2:])

		showPackageList()
	case "s", "search":
		// Setup flags and help
		currentFlagSet = flag.NewFlagSet("search", flag.ExitOnError)
		setupFlagsAndHelp(currentFlagSet, fmt.Sprintf("bpm %s <options>", subcommand), "Search for packages in remote databases", os.Args[2:])

		searchForPackages()
	case "i", "install":
		// Setup flags and help
		currentFlagSet = flag.NewFlagSet("install", flag.ExitOnError)
		currentFlagSet.StringP("root", "R", "/", "Operate on specified root directory")
		currentFlagSet.BoolP("verbose", "v", false, "Show additional information about the current operation")
		currentFlagSet.BoolP("force", "f", false, "Bypass warnings during package installation")
		currentFlagSet.BoolP("yes", "y", false, "Enter 'yes' in all prompts")
		currentFlagSet.BoolP("optional", "o", false, "Install all optional dependencies")
		currentFlagSet.String("installation-reason", "", "Specify the installation reason to use for the specified packages")
		currentFlagSet.BoolP("reinstall", "r", false, "Reinstall the specified packages")
		currentFlagSet.BoolP("reinstall-all", "a", false, "Reinstall the specified packages and their dependencies")
		setupFlagsAndHelp(currentFlagSet, fmt.Sprintf("bpm %s <options>", subcommand), "Install the specified packages", os.Args[2:])

		installPackages()
	case "r", "remove":
		// Setup flags and help
		currentFlagSet = flag.NewFlagSet("remove", flag.ExitOnError)
		currentFlagSet.StringP("root", "R", "/", "Operate on specified root directory")
		currentFlagSet.BoolP("verbose", "v", false, "Show additional information about the current operation")
		currentFlagSet.BoolP("force", "f", false, "Bypass warnings during package removal")
		currentFlagSet.BoolP("yes", "y", false, "Enter 'yes' in all prompts")
		currentFlagSet.BoolP("unused", "u", false, "Remove packages only if they are not required as dependencies")
		currentFlagSet.BoolP("cleanup", "n", false, "Additionally remove all unused dependencies")
		setupFlagsAndHelp(currentFlagSet, fmt.Sprintf("bpm %s <options>", subcommand), "Remove the specified packages", os.Args[2:])

		removePackages()
	case "n", "cleanup":
		// Setup flags and help
		currentFlagSet = flag.NewFlagSet("cleanup", flag.ExitOnError)
		currentFlagSet.StringP("root", "R", "/", "Operate on specified root directory")
		currentFlagSet.BoolP("verbose", "v", false, "Show additional information about the current operation")
		currentFlagSet.BoolP("force", "f", false, "Bypass warnings during package cleanup")
		currentFlagSet.BoolP("yes", "y", false, "Enter 'yes' in all prompts")
		currentFlagSet.BoolP("all", "a", false, "Perform all types of cleanup")
		currentFlagSet.BoolP("depends", "d", false, "Perform a dependency cleanup")
		currentFlagSet.BoolP("make-depends", "m", false, "Perform a make dependency cleanup")
		currentFlagSet.BoolP("compilation-files", "c", false, "Perform a cleanup of compilation files")
		currentFlagSet.BoolP("binary-packages", "b", false, "Perform a cleanup of compilation compiled binary packages")
		currentFlagSet.BoolP("fetched-packages", "p", false, "Perform a cleanup of fetched packages from databases")
		setupFlagsAndHelp(currentFlagSet, fmt.Sprintf("bpm %s <options>", subcommand), "Remove unused dependencies, files and directories", os.Args[2:])

		doCleanup()
	case "y", "sync":
		// Setup flags and help
		currentFlagSet = flag.NewFlagSet("cleanup", flag.ExitOnError)
		currentFlagSet.StringP("root", "R", "/", "Operate on specified root directory")
		currentFlagSet.BoolP("verbose", "v", false, "Show additional information about the current operation")
		currentFlagSet.BoolP("yes", "y", false, "Enter 'yes' in all prompts")
		setupFlagsAndHelp(currentFlagSet, fmt.Sprintf("bpm %s <options>", subcommand), "Sync all databases", os.Args[2:])

		syncDatabases()
	case "u", "update":
		// Setup flags and help
		currentFlagSet = flag.NewFlagSet("update", flag.ExitOnError)
		currentFlagSet.StringP("root", "R", "/", "Operate on specified root directory")
		currentFlagSet.BoolP("verbose", "v", false, "Show additional information about the current operation")
		currentFlagSet.BoolP("force", "f", false, "Bypass warnings during package update")
		currentFlagSet.BoolP("yes", "y", false, "Enter 'yes' in all prompts")
		currentFlagSet.BoolP("no-sync", "n", false, "Do not sync databases")
		currentFlagSet.BoolP("optional", "o", false, "Install all optional dependencies")
		setupFlagsAndHelp(currentFlagSet, fmt.Sprintf("bpm %s <options>", subcommand), "Update installed packages", os.Args[2:])

		updatePackages()
	case "o", "owner":
		// Setup flags and help
		currentFlagSet = flag.NewFlagSet("owner", flag.ExitOnError)
		currentFlagSet.StringP("root", "R", "/", "Operate on specified root directory")
		setupFlagsAndHelp(currentFlagSet, fmt.Sprintf("bpm %s <options>", subcommand), "Show what packages own the specified paths", os.Args[2:])

		getFileOwner()
	case "c", "compile":
		// Setup flags and help
		currentFlagSet = flag.NewFlagSet("compile", flag.ExitOnError)
		currentFlagSet.StringP("root", "R", "/", "Operate on specified root directory")
		currentFlagSet.BoolP("verbose", "v", false, "Show additional information about the current operation")
		currentFlagSet.BoolP("force", "f", false, "Bypass warnings during package compilation")
		currentFlagSet.BoolP("yes", "y", false, "Enter 'yes' in all prompts")
		currentFlagSet.BoolP("depends", "d", false, "Install required dependencies for package compilation")
		currentFlagSet.BoolP("skip-checks", "s", false, "Skip the check function in source.sh scripts")
		currentFlagSet.BoolP("output-directory", "o", false, "Set the output directory for the binary packages")
		currentFlagSet.Int("output-fd", -1, "Set the file descriptor output package names will be written to")
		setupFlagsAndHelp(currentFlagSet, fmt.Sprintf("bpm %s <options>", subcommand), "Compile source packages and convert them to binary ones", os.Args[2:])

		compilePackage()
	default:
		listSubcommands()
	}

	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func showPackageInfo() {
	// Get flags
	rootDir, _ := currentFlagSet.GetString("root")
	showDatabaseInfo, _ := currentFlagSet.GetBool("database")

	// Get packages
	packages := currentFlagSet.Args()
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
}

func showPackageList() {
	// Get flags
	rootDir, _ := currentFlagSet.GetString("root")
	showPkgCount, _ := currentFlagSet.GetBool("count")
	showPkgNames, _ := currentFlagSet.GetBool("names")

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
	if showPkgCount {
		fmt.Println(len(packages))
	} else if showPkgNames {
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
}

func searchForPackages() {
	// Get search terms
	searchTerms := currentFlagSet.Args()
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
		resultsMap := make(map[*bpmlib.PackageInfo]float64, 0)

		// Loop through all packages
		for _, db := range bpmlib.BPMDatabases {
			for _, entry := range db.Entries {
				// Calculate string similarity and add to map
				similarity := strutil.Similarity(entry.Info.Name, term, metrics.NewSmithWatermanGotoh())
				if similarity > 0.8 {
					resultsMap[entry.Info] = similarity
					continue
				}
				similarity = strutil.Similarity(entry.Info.Description, term, metrics.NewSmithWatermanGotoh())
				if similarity > 0.8 {
					resultsMap[entry.Info] = similarity
				}
			}
		}
		if len(resultsMap) == 0 {
			log.Printf("Error: no results for term (%s) were found\n", term)
			exitCode = 1
			return
		}

		// Sort results
		results := slices.Collect(maps.Keys(resultsMap))
		sort.Slice(results, func(i, j int) bool {
			return resultsMap[results[i]] > resultsMap[results[j]]
		})

		// Print results
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("Results for term (%s)\n", term)
		for j := 0; j < 10 && j < len(results); j++ {
			result := results[j]
			fmt.Printf("%d) %s: %s (%s)\n", j+1, result.Name, result.Description, result.GetFullVersion())
		}
	}
}

func installPackages() {
	// Get flags
	rootDir, _ := currentFlagSet.GetString("root")
	verbose, _ := currentFlagSet.GetBool("verbose")
	force, _ := currentFlagSet.GetBool("force")
	yesAll, _ := currentFlagSet.GetBool("yes")
	installOptional, _ := currentFlagSet.GetBool("optional")
	installationReason, _ := currentFlagSet.GetString("installation-reason")
	reinstall, _ := currentFlagSet.GetBool("reinstall")
	reinstallAll, _ := currentFlagSet.GetBool("reinstall-all")

	// Get packages
	packages := currentFlagSet.Args()
	if len(packages) == 0 {
		fmt.Println("No packages or files were given to install")
		return
	}

	// Check for required permissions
	if os.Getuid() != 0 {
		log.Printf("Error: this subcommand needs to be run with superuser permissions")
		exitCode = 1
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
	operation, err := bpmlib.InstallPackages(rootDir, ir, reinstallMethod, installOptional, force, verbose, packages...)
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
		prompt := "Do you wish to install this package?"
		if len(operation.Actions) != 1 {
			prompt = fmt.Sprintf("Do you wish to install all %d packages?", len(operation.Actions))
		}

		if !showConfirmationPrompt(prompt, false) {
			fmt.Println("Cancelling package installation...")
			exitCode = 1
			return
		}
	}

	// Fetch packages
	err = operation.FetchPackages()
	if err != nil {
		log.Printf("Error: could not fetch packages for operation: %s\n", err)
		exitCode = 1
		return
	}

	if bpmlib.MainBPMConfig.ShowSourcePackageContents == "always" || bpmlib.MainBPMConfig.ShowSourcePackageContents == "install-only" {
		// Show source package contents
		sourcePackagesShown, err := operation.ShowSourcePackageContent()
		if err != nil {
			log.Printf("Error: could not show source package content: %s\n", err)
			exitCode = 1
			return
		}

		// Confirmation Prompt
		if sourcePackagesShown > 0 && !yesAll {
			if !showConfirmationPrompt("Do you wish to continue?", false) {
				fmt.Println("Cancelling package installation...")
				exitCode = 1
				return
			}
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

func removePackages() {
	// Get flags
	rootDir, _ := currentFlagSet.GetString("root")
	verbose, _ := currentFlagSet.GetBool("verbose")
	force, _ := currentFlagSet.GetBool("force")
	yesAll, _ := currentFlagSet.GetBool("yes")
	removeUnused, _ := currentFlagSet.GetBool("unused")
	cleanupPackages, _ := currentFlagSet.GetBool("cleanup")

	// Get packages
	packages := currentFlagSet.Args()

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

	// Read local databases
	err = bpmlib.ReadLocalDatabaseFiles()
	if err != nil {
		log.Printf("Error: could not read local databases: %s", err)
		exitCode = 1
		return
	}

	// Create remove operation
	operation, err := bpmlib.RemovePackages(rootDir, removeUnused, cleanupPackages, packages...)
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
		prompt := "Do you wish to remove this package?"
		if len(operation.Actions) != 1 {
			prompt = fmt.Sprintf("Do you wish to remove all %d packages?", len(operation.Actions))
		}

		if !showConfirmationPrompt(prompt, false) {
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

func doCleanup() {
	// Get flags
	rootDir, _ := currentFlagSet.GetString("root")
	verbose, _ := currentFlagSet.GetBool("verbose")
	force, _ := currentFlagSet.GetBool("force")
	yesAll, _ := currentFlagSet.GetBool("yes")
	all, _ := currentFlagSet.GetBool("all")
	cleanupDepends, _ := currentFlagSet.GetBool("depends")
	cleanupMakeDepends, _ := currentFlagSet.GetBool("make-depends")
	cleanupCompilationFiles, _ := currentFlagSet.GetBool("compilation-files")
	cleanupBinaryPackages, _ := currentFlagSet.GetBool("binary-packages")
	cleanupFetchedPackages, _ := currentFlagSet.GetBool("fetched-packages")

	// Set default behaviour
	if all {
		cleanupDepends = true
		cleanupMakeDepends = bpmlib.MainBPMConfig.CleanupMakeDependencies
		cleanupCompilationFiles = true
		cleanupBinaryPackages = true
		cleanupFetchedPackages = true
	} else if !isFlagSet(currentFlagSet, "depends") && !isFlagSet(currentFlagSet, "make-depends") && !isFlagSet(currentFlagSet, "compilation-files") && !isFlagSet(currentFlagSet, "binary-packages") && !isFlagSet(currentFlagSet, "fetched-packages") {
		cleanupDepends = true
		cleanupMakeDepends = bpmlib.MainBPMConfig.CleanupMakeDependencies
		cleanupCompilationFiles = false
		cleanupBinaryPackages = false
		cleanupFetchedPackages = false
	}

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

	err = bpmlib.CleanupCache(rootDir, cleanupCompilationFiles, cleanupBinaryPackages, cleanupFetchedPackages, verbose)
	if err != nil {
		log.Printf("Error: could not complete cache cleanup: %s", err)
		exitCode = 1
		return
	}

	if cleanupDepends || cleanupMakeDepends {
		// Read local databases
		err := bpmlib.ReadLocalDatabaseFiles()
		if err != nil {
			log.Printf("Error: could not read local databases: %s", err)
			exitCode = 1
			return
		}

		// Create cleanup operation
		operation, err := bpmlib.CleanupPackages(cleanupMakeDepends, rootDir)
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
			prompt := "Do you wish to remove this package?"
			if len(operation.Actions) != 1 {
				prompt = fmt.Sprintf("Do you wish to remove all %d packages?", len(operation.Actions))
			}

			if !showConfirmationPrompt(prompt, false) {
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
}

func syncDatabases() {
	// Get flags
	rootDir, _ := currentFlagSet.GetString("root")
	verbose, _ := currentFlagSet.GetBool("verbose")
	yesAll, _ := currentFlagSet.GetBool("yes")

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
		if !showConfirmationPrompt("Do you wish to sync all databases?", false) {
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
}

func updatePackages() {
	// Get flags
	rootDir, _ := currentFlagSet.GetString("root")
	verbose, _ := currentFlagSet.GetBool("verbose")
	force, _ := currentFlagSet.GetBool("force")
	yesAll, _ := currentFlagSet.GetBool("yes")
	noSync, _ := currentFlagSet.GetBool("no-sync")
	installOptional, _ := currentFlagSet.GetBool("optional")

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
	if noSync {
		err := bpmlib.ReadLocalDatabaseFiles()
		if err != nil {
			log.Printf("Error: could not read local databases: %s", err)
			exitCode = 1
			return
		}
	}

	// Create update operation
	operation, err := bpmlib.UpdatePackages(rootDir, !noSync, installOptional, force, verbose)
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
		prompt := "Do you wish to update this package?"
		if len(operation.Actions) != 1 {
			prompt = fmt.Sprintf("Do you wish to update all %d packages?", len(operation.Actions))
		}

		if !showConfirmationPrompt(prompt, false) {
			fmt.Println("Cancelling package update...")
			exitCode = 1
			return
		}
	}

	// Fetch packages
	err = operation.FetchPackages()
	if err != nil {
		log.Printf("Error: could not fetch packages for operation: %s\n", err)
		exitCode = 1
		return
	}

	if bpmlib.MainBPMConfig.ShowSourcePackageContents == "always" {
		// Show source package contents
		sourcePackagesShown, err := operation.ShowSourcePackageContent()
		if err != nil {
			log.Printf("Error: could not show source package content: %s\n", err)
			exitCode = 1
			return
		}

		// Confirmation Prompt
		if sourcePackagesShown > 0 && !yesAll {
			if !showConfirmationPrompt("Do you wish to continue?", false) {
				fmt.Println("Cancelling package installation...")
				exitCode = 1
				return
			}
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

func getFileOwner() {
	// Get flags
	rootDir, _ := currentFlagSet.GetString("root")

	// Get files
	files := currentFlagSet.Args()
	if len(files) == 0 {
		fmt.Println("No files were given to get which packages own it")
		return
	}

	for _, path := range files {
		// Ensure file exists
		stat, err := os.Lstat(path)
		if os.IsNotExist(err) {
			log.Printf("Error: file (%s) does not exist!\n", path)
			exitCode = 1
			return
		}

		// Get path type
		pathType := "File"
		if stat.IsDir() {
			pathType = "Directory"
		} else if stat.Mode()&os.ModeSymlink != 0 {
			pathType = "Symlink"
		}

		// Get absolte path to path
		absPath, err := filepath.Abs(path)
		if err != nil {
			log.Printf("Error: could not get absolute path of file (%s)\n", path)
			exitCode = 1
			return
		}

		// Get path relative to rootDir
		if !strings.HasPrefix(absPath, rootDir) {
			log.Printf("Error: could not get path of file (%s) relative to root path", absPath)
			exitCode = 1
			return
		}
		absPath, err = filepath.Rel(rootDir, absPath)
		if err != nil {
			log.Printf("Error: could not get path of file (%s) relative to root path", absPath)
			exitCode = 1
			return
		}

		// Trim leading and trailing slashes
		absPath = strings.TrimLeft(absPath, "/")
		absPath = strings.TrimRight(absPath, "/")

		// Get installed packages
		pkgs, err := bpmlib.GetInstalledPackages(rootDir)
		if err != nil {
			log.Printf("Error: could not get installed packages: %s\n", err.Error())
			exitCode = 1
			return
		}

		// Add packages that own path to list
		var pkgList []string
		for _, pkg := range pkgs {
			if slices.ContainsFunc(bpmlib.GetPackage(pkg, rootDir).PkgFiles, func(entry *bpmlib.PackageFileEntry) bool {
				return entry.Path == absPath
			}) {
				pkgList = append(pkgList, pkg)
			}
		}

		// Print packages
		if len(pkgList) == 0 {
			fmt.Printf("%s (%s) is not owned by any packages!\n", absPath, pathType)
		} else {
			fmt.Printf("%s (%s) is owned by the following packages:\n", absPath, pathType)
			for _, pkg := range pkgList {
				fmt.Println("- " + pkg)
			}
		}
	}
}

func compilePackage() {
	// Get flags
	rootDir, _ := currentFlagSet.GetString("root")
	verbose, _ := currentFlagSet.GetBool("verbose")
	yesAll, _ := currentFlagSet.GetBool("yes")
	installSrcPkgDepends, _ := currentFlagSet.GetBool("depends")
	skipChecks, _ := currentFlagSet.GetBool("skip-checks")
	outputDirectory, _ := currentFlagSet.GetString("output-directory")
	outputFd, _ := currentFlagSet.GetInt("output-fd")

	// Get files
	sourcePackages := currentFlagSet.Args()
	if len(sourcePackages) == 0 {
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
	for _, sourcePackage := range sourcePackages {
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

		// Setup cleanup function
		cleanupFunc := func() {
			if installSrcPkgDepends && len(unmetDepends) > 0 {
				// Get path to current executable
				executable, err := os.Executable()
				if err != nil {
					log.Printf("Warning: could not get path to executable: %s\n", err)
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
					log.Printf("Warning: dependency cleanup command failed: %s\n", err)
				}
			}
		}

		// Get current working directory
		workdir, err := os.Getwd()
		if err != nil {
			// Remove unused packages
			cleanupFunc()

			log.Printf("Error: could not get working directory: %s", err)
			exitCode = 1
			return
		}

		// Get user home directory
		homedir, err := os.UserHomeDir()
		if err != nil {
			// Remove unused packages
			cleanupFunc()

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
			// Remove unused packages
			cleanupFunc()

			log.Printf("Error: could not stat output directory (%s): %s", outputDirectory, err)
			exitCode = 1
			return
		}
		if !stat.IsDir() {
			// Remove unused packages
			cleanupFunc()

			log.Printf("Error: output directory (%s) is not a directory", outputDirectory)
			exitCode = 1
			return
		}

		outputBpmPackages, err := bpmlib.CompileSourcePackage(sourcePackage, outputDirectory, skipChecks)
		if err != nil {
			// Remove unused packages
			cleanupFunc()

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
		cleanupFunc()
	}
}

func listSubcommands() {
	fmt.Printf("Usage: %s <subcommand> [options]\n", os.Args[0])
	fmt.Println("Description: Manage system packages")
	fmt.Println("Subcommands:")
	fmt.Println("  q, query     Show information on the specified packages")
	fmt.Println("  l, list      List packages")
	fmt.Println("  s, search    Search for packages in remote databases")
	fmt.Println("  i, install   Install the specified packages")
	fmt.Println("  r, remove    Remove the specified packages")
	fmt.Println("  n, cleanup   Remove unused dependencies, files and directories")
	fmt.Println("  y, sync      Sync all databases")
	fmt.Println("  u, update    Update installed packages")
	fmt.Println("  o, owner     Show what packages own the specified paths")
	fmt.Println("  c, compile   Compile source packages and convert them to binary ones")
}

func setupFlagsAndHelp(flagset *flag.FlagSet, usage, desc string, args []string) {
	flagset.Usage = func() {
		fmt.Println("Usage: " + usage)
		fmt.Println("Description: " + desc)
		fmt.Println("Options:")
		if !flagset.HasFlags() {
			fmt.Println("  No flags defined")
		}
		flagset.PrintDefaults()
	}
	flagset.Parse(args)
}

func isFlagSet(flagSet *flag.FlagSet, name string) bool {
	found := false
	flagSet.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func showConfirmationPrompt(prompt string, defaultTo bool) bool {
	reader := bufio.NewReader(os.Stdin)
	if defaultTo {
		fmt.Printf("%s [Y/n] ", prompt)
	} else {
		fmt.Printf("%s [y/N] ", prompt)
	}

	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)

	if len(text) > 0 {
		switch text[0] {
		case 'y', 'Y':
			return true
		case 'n', 'N':
			return false
		}
	}

	return defaultTo
}
