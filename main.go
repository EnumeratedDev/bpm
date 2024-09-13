package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/elliotchance/orderedmap/v2"
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

var bpmVer = "0.4.1"

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
	search
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
			info = utils.GetPackageInfo(pkg, rootDir, false)
			if info == nil {
				log.Fatalf("Package (%s) is not installed\n", pkg)
			}
			fmt.Println("----------------")
			fmt.Println(utils.CreateReadableInfo(true, true, true, info, rootDir))
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
				fmt.Println("----------------\n" + utils.CreateReadableInfo(true, true, true, info, rootDir))
				if n == len(packages)-1 {
					fmt.Println("----------------")
				}
			}
		}
	case search:
		searchTerms := subcommandArgs
		if len(searchTerms) == 0 {
			fmt.Println("No search terms given")
			os.Exit(0)
		}

		for _, term := range searchTerms {
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
				log.Fatalf("No results for term (%s) were found\n", term)
			}
			fmt.Printf("Results for term (%s)\n", term)
			for i, result := range results {
				fmt.Println("----------------")
				fmt.Printf("%d) %s: %s (%s)\n", i+1, result.Name, result.Description, result.GetFullVersion())
			}
		}
	case install:
		if os.Getuid() != 0 {
			fmt.Println("This subcommand needs to be run with superuser permissions")
			os.Exit(0)
		}
		pkgs := subcommandArgs
		if len(pkgs) == 0 {
			fmt.Println("No packages or files were given to install")
			return
		}

		pkgsToInstall := orderedmap.NewOrderedMap[string, *struct {
			bpmFile      string
			isDependency bool
			shouldFetch  bool
			pkgInfo      *utils.PackageInfo
		}]()
		unresolvedDepends := make([]string, 0)

		// Search for packages
		for _, pkg := range pkgs {
			if stat, err := os.Stat(pkg); err == nil && !stat.IsDir() {
				pkgInfo, err := utils.ReadPackage(pkg)
				if err != nil {
					log.Fatalf("Could not read package. Error: %s\n", err)
				}
				if !reinstall && utils.IsPackageInstalled(pkgInfo.Name, rootDir) && utils.GetPackageInfo(pkgInfo.Name, rootDir, true).GetFullVersion() == pkgInfo.GetFullVersion() {
					continue
				}
				pkgsToInstall.Set(pkgInfo.Name, &struct {
					bpmFile      string
					isDependency bool
					shouldFetch  bool
					pkgInfo      *utils.PackageInfo
				}{bpmFile: pkg, isDependency: false, shouldFetch: false, pkgInfo: pkgInfo})
			} else {
				entry, _, err := utils.GetRepositoryEntry(pkg)
				if err != nil {
					log.Fatalf("Could not find package (%s) in any repository\n", pkg)
				}
				if !reinstall && utils.IsPackageInstalled(entry.Info.Name, rootDir) && utils.GetPackageInfo(entry.Info.Name, rootDir, true).GetFullVersion() == entry.Info.GetFullVersion() {
					continue
				}
				pkgsToInstall.Set(entry.Info.Name, &struct {
					bpmFile      string
					isDependency bool
					shouldFetch  bool
					pkgInfo      *utils.PackageInfo
				}{bpmFile: "", isDependency: false, shouldFetch: true, pkgInfo: entry.Info})
			}
		}

		clone := pkgsToInstall.Copy()
		pkgsToInstall = orderedmap.NewOrderedMap[string, *struct {
			bpmFile      string
			isDependency bool
			shouldFetch  bool
			pkgInfo      *utils.PackageInfo
		}]()
		for _, pkg := range clone.Keys() {
			value, _ := clone.Get(pkg)
			resolved, unresolved := value.pkgInfo.ResolveAll(&[]string{}, &[]string{}, value.pkgInfo.Type == "source", !noOptional, !reinstall, rootDir)
			unresolvedDepends = append(unresolvedDepends, unresolved...)
			for _, depend := range resolved {
				if _, ok := pkgsToInstall.Get(depend); !ok && depend != value.pkgInfo.Name {
					if !reinstallAll && utils.IsPackageInstalled(depend, rootDir) {
						continue
					}
					entry, _, err := utils.GetRepositoryEntry(depend)
					if err != nil {
						log.Fatalf("Could not find package (%s) in any repository\n", pkg)
					}
					pkgsToInstall.Set(depend, &struct {
						bpmFile      string
						isDependency bool
						shouldFetch  bool
						pkgInfo      *utils.PackageInfo
					}{bpmFile: "", isDependency: true, shouldFetch: true, pkgInfo: entry.Info})
				}
			}
			pkgsToInstall.Set(pkg, value)
		}

		// Show summary
		if len(unresolvedDepends) != 0 {
			if !force {
				log.Fatalf("The following dependencies could not be found in any repositories: %s\n", strings.Join(unresolvedDepends, ", "))
			} else {
				log.Println("Warning: The following dependencies could not be found in any repositories: " + strings.Join(unresolvedDepends, ", "))
			}
		}
		if pkgsToInstall.Len() == 0 {
			fmt.Println("All packages are up to date!")
			os.Exit(0)
		}

		for _, pkg := range pkgsToInstall.Keys() {
			value, _ := pkgsToInstall.Get(pkg)
			pkgInfo := value.pkgInfo
			installedInfo := utils.GetPackageInfo(pkgInfo.Name, rootDir, false)
			sourceInfo := ""
			if pkgInfo.Type == "source" {
				if rootDir != "/" && !force {
					log.Fatalf("Error: Cannot compile and install source packages to a different root directory")
				}
				sourceInfo = "(From Source)"
			}
			if installedInfo == nil {
				fmt.Printf("%s: %s (Install) %s\n", pkgInfo.Name, pkgInfo.GetFullVersion(), sourceInfo)
			} else if strings.Compare(pkgInfo.GetFullVersion(), installedInfo.GetFullVersion()) < 0 {
				fmt.Printf("%s: %s -> %s (Downgrade) %s\n", pkgInfo.Name, installedInfo.GetFullVersion(), pkgInfo.GetFullVersion(), sourceInfo)
			} else if strings.Compare(pkgInfo.GetFullVersion(), installedInfo.GetFullVersion()) > 0 {
				fmt.Printf("%s: %s -> %s (Upgrade) %s\n", pkgInfo.Name, installedInfo.GetFullVersion(), pkgInfo.GetFullVersion(), sourceInfo)
			} else {
				fmt.Printf("%s: %s (Reinstall) %s\n", pkgInfo.Name, pkgInfo.GetFullVersion(), sourceInfo)
			}
		}
		if rootDir != "/" {
			fmt.Println("Warning: Operating in " + rootDir)
		}
		if !yesAll {
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("Do you wish to install these %d packages? [y\\N] ", pkgsToInstall.Len())
			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Println("Cancelling...")
				os.Exit(1)
			}
		}

		// Fetch packages from repositories
		fmt.Println("Fetching packages from available repositories...")
		for _, pkg := range pkgsToInstall.Keys() {
			value, _ := pkgsToInstall.Get(pkg)
			if !value.shouldFetch {
				continue
			}
			entry, repo, err := utils.GetRepositoryEntry(pkg)
			if err != nil {
				log.Fatalf("Could not find package (%s) in any repository\n", pkg)
			}
			fetchedPackage, err := repo.FetchPackage(entry.Info.Name)
			if err != nil {
				log.Fatalf("Could not fetch package (%s). Error: %s\n", pkg, err)
			}
			fmt.Printf("Package (%s) was successfully fetched!\n", value.pkgInfo.Name)
			value.bpmFile = fetchedPackage
			pkgsToInstall.Set(pkg, value)
		}

		// Install fetched packages
		for _, pkg := range pkgsToInstall.Keys() {
			value, _ := pkgsToInstall.Get(pkg)
			pkgInfo := value.pkgInfo
			var err error
			if value.isDependency {
				err = utils.InstallPackage(value.bpmFile, rootDir, verbose, true, buildSource, skipCheck, keepTempDir)
			} else {
				err = utils.InstallPackage(value.bpmFile, rootDir, verbose, force, buildSource, skipCheck, keepTempDir)
			}

			if err != nil {
				if pkgInfo.Type == "source" && keepTempDir {
					fmt.Println("BPM temp directory was created at /var/tmp/bpm_source-" + pkgInfo.Name)
				}
				log.Fatalf("Could not install package (%s). Error: %s\n", pkg, err)
			}
			fmt.Printf("Package (%s) was successfully installed!\n", pkgInfo.Name)
			if value.isDependency {
				err := utils.SetInstallationReason(pkgInfo.Name, utils.Dependency, rootDir)
				if err != nil {
					log.Fatalf("Could not set installation reason for package\nError: %s\n", err)
				}
			}
			if pkgInfo.Type == "source" && keepTempDir {
				fmt.Println("** It is recommended you delete the temporary bpm folder in /var/tmp **")
			}
		}
	case update:
		if os.Getuid() != 0 {
			fmt.Println("This subcommand needs to be run with superuser permissions")
			os.Exit(0)
		}

		// Sync repositories
		if !nosync {
			for _, repo := range utils.BPMConfig.Repositories {
				fmt.Printf("Fetching package database for repository (%s)...\n", repo.Name)
				err := repo.SyncLocalDatabase()
				if err != nil {
					log.Fatal(err)
				}
			}
			fmt.Println("All package databases synced successfully!")
		}

		utils.ReadConfig()

		// Get installed packages and check for updates
		pkgs, err := utils.GetInstalledPackages(rootDir)
		if err != nil {
			log.Fatalf("Could not get installed packages! Error: %s\n", err)
		}
		toUpdate := orderedmap.NewOrderedMap[string, *struct {
			isDependency bool
			entry        *utils.RepositoryEntry
		}]()
		for _, pkg := range pkgs {
			entry, _, err := utils.GetRepositoryEntry(pkg)
			if err != nil {
				continue
			}
			installedInfo := utils.GetPackageInfo(pkg, rootDir, true)
			if installedInfo == nil {
				log.Fatalf(pkg)
			}
			if strings.Compare(entry.Info.GetFullVersion(), installedInfo.GetFullVersion()) > 0 {
				toUpdate.Set(entry.Info.Name, &struct {
					isDependency bool
					entry        *utils.RepositoryEntry
				}{isDependency: false, entry: entry})
			} else if reinstall {
				toUpdate.Set(entry.Info.Name, &struct {
					isDependency bool
					entry        *utils.RepositoryEntry
				}{isDependency: false, entry: entry})
			}
		}
		if toUpdate.Len() == 0 {
			fmt.Println("All packages are up to date!")
			os.Exit(0)
		}

		// Check for new dependencies in updated packages
		unresolved := make([]string, 0)
		clone := toUpdate.Copy()
		for _, key := range clone.Keys() {
			pkg, _ := clone.Get(key)
			r, u := pkg.entry.Info.ResolveAll(&[]string{}, &[]string{}, pkg.entry.Info.Type == "source", !noOptional, true, rootDir)
			unresolved = append(unresolved, u...)
			for _, depend := range r {
				if _, ok := toUpdate.Get(depend); !ok {
					entry, _, err := utils.GetRepositoryEntry(depend)
					if err != nil {
						log.Fatalf("Could not find package (%s) in any repository\n", depend)
					}
					toUpdate.Set(depend, &struct {
						isDependency bool
						entry        *utils.RepositoryEntry
					}{isDependency: true, entry: entry})
				}
			}
		}

		if len(unresolved) != 0 {
			if !force {
				log.Fatalf("The following dependencies could not be found in any repositories: %s\n", strings.Join(unresolved, ", "))
			} else {
				log.Println("Warning: The following dependencies could not be found in any repositories: " + strings.Join(unresolved, ", "))
			}
		}

		for _, key := range toUpdate.Keys() {
			value, _ := toUpdate.Get(key)
			installedInfo := utils.GetPackageInfo(value.entry.Info.Name, rootDir, true)
			sourceInfo := ""
			if value.entry.Info.Type == "source" {
				sourceInfo = "(From Source)"
			}
			if installedInfo == nil {
				fmt.Printf("%s: %s (Install) %s\n", value.entry.Info.Name, value.entry.Info.GetFullVersion(), sourceInfo)
				continue
			}
			if strings.Compare(value.entry.Info.GetFullVersion(), installedInfo.GetFullVersion()) > 0 {
				fmt.Printf("%s: %s -> %s (Upgrade) %s\n", value.entry.Info.Name, installedInfo.GetFullVersion(), value.entry.Info.GetFullVersion(), sourceInfo)
			} else if reinstall {
				fmt.Printf("%s: %s -> %s (Reinstall) %s\n", value.entry.Info.Name, installedInfo.GetFullVersion(), value.entry.Info.GetFullVersion(), sourceInfo)
			}
		}

		// Update confirmation prompt
		if !yesAll {
			fmt.Printf("Are you sure you wish to update all %d packages? [y\\N] ", toUpdate.Len())
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
				fmt.Println("Cancelling update...")
				os.Exit(0)
			}
		}

		// Fetch packages
		pkgsToInstall := orderedmap.NewOrderedMap[string, *struct {
			isDependency bool
			entry        *utils.RepositoryEntry
		}]()
		fmt.Println("Fetching packages from available repositories...")
		for _, pkg := range toUpdate.Keys() {
			value, _ := toUpdate.Get(pkg)
			entry, repo, err := utils.GetRepositoryEntry(pkg)
			if err != nil {
				log.Fatalf("Could not find package (%s) in any repository\n", pkg)
			}
			fetchedPackage, err := repo.FetchPackage(entry.Info.Name)
			if err != nil {
				log.Fatalf("Could not fetch package (%s). Error: %s\n", pkg, err)
			}
			fmt.Printf("Package (%s) was successfully fetched!\n", value.entry.Info.Name)
			pkgsToInstall.Set(fetchedPackage, value)
		}

		// Install fetched packages
		for _, pkg := range pkgsToInstall.Keys() {
			value, _ := pkgsToInstall.Get(pkg)
			pkgInfo := value.entry.Info
			var err error
			if value.isDependency {
				err = utils.InstallPackage(pkg, rootDir, verbose, true, buildSource, skipCheck, keepTempDir)
			} else {
				err = utils.InstallPackage(pkg, rootDir, verbose, force, buildSource, skipCheck, keepTempDir)
			}

			if err != nil {
				if pkgInfo.Type == "source" && keepTempDir {
					fmt.Println("BPM temp directory was created at /var/tmp/bpm_source-" + pkgInfo.Name)
				}
				log.Fatalf("Could not install package (%s). Error: %s\n", pkg, err)
			}
			fmt.Printf("Package (%s) was successfully installed!\n", pkgInfo.Name)
			if value.isDependency {
				err := utils.SetInstallationReason(pkgInfo.Name, utils.Dependency, rootDir)
				if err != nil {
					log.Fatalf("Could not set installation reason for package\nError: %s\n", err)
				}
			}
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
			fmt.Println("----------------\n" + utils.CreateReadableInfo(false, false, false, pkgInfo, rootDir))
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
	fmt.Println("-> bpm info [-R] <packages...> | shows information on an installed package")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("-> bpm list [-R, -c, -n] | lists all installed packages")
	fmt.Println("       -R=<path> lets you define the root path which will be used")
	fmt.Println("       -c lists the amount of installed packages")
	fmt.Println("       -n lists only the names of installed packages")
	fmt.Println("-> bpm search <search terms...> | Searches for packages through declared repositories")
	fmt.Println("-> bpm install [-R, -v, -y, -f, -o, -c, -b, -k, --reinstall, --reinstall-all, --no-optional] <packages...> | installs the following files")
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
		if reinstallAll {
			reinstall = true
		}
	}
}
