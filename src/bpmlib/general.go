package bpmlib

import (
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
)

type ReinstallMethod uint8

const (
	ReinstallMethodNone      ReinstallMethod = iota
	ReinstallMethodSpecified ReinstallMethod = iota
	ReinstallMethodAll       ReinstallMethod = iota
)

// InstallPackages installs the specified packages into the given root directory by fetching them from databases or directly from local bpm archives
func InstallPackages(rootDir string, forceInstallationReason InstallationReason, reinstallMethod ReinstallMethod, installRuntimeDependencies, installOptionalDependencies, forceInstallation, runChecks bool, verbose bool, packages ...string) (operation *BPMOperation, err error) {
	// Setup operation struct
	operation = &BPMOperation{
		Actions:           make([]OperationAction, 0),
		UnresolvedDepends: make([]string, 0),
		Changes:           make(map[string]string),
		RunChecks:         runChecks,
		RootDir:           rootDir,
		compiledPackages:  make(map[string]string),
	}

	// Remove duplicates from packages
	packages = removeDuplicates(packages)

	// Resolve packages
	pkgsNotFound := make([]string, 0)
	for _, pkg := range packages {
		if stat, err := os.Stat(pkg); err == nil && !stat.IsDir() {
			bpmpkg, err := ReadPackage(pkg)
			if err != nil {
				return nil, fmt.Errorf("could not read package: %s", err)
			}

			if bpmpkg.PkgInfo.Type == "source" && bpmpkg.PkgInfo.IsSplitPackage() {
				for _, splitPkg := range bpmpkg.PkgInfo.SplitPackages {
					if reinstallMethod == ReinstallMethodNone && IsPackageInstalled(splitPkg.Name, rootDir) && GetPackageInfo(splitPkg.Name, rootDir).GetFullVersion() == splitPkg.GetFullVersion() {
						continue
					}

					// Set package installation reason
					installationReason := forceInstallationReason
					if installationReason == InstallationReasonUnknown {
						if IsPackageInstalled(splitPkg.Name, rootDir) {
							installationReason = GetPackage(splitPkg.Name, rootDir).LocalInfo.GetInstallationReason()
						} else {
							installationReason = InstallationReasonManual
						}
					}

					operation.AppendAction(&InstallPackageAction{
						File:                  pkg,
						InstallationReason:    installationReason,
						BpmPackage:            bpmpkg,
						SplitPackageToInstall: splitPkg.Name,
					})
				}
				continue
			}

			if reinstallMethod == ReinstallMethodNone && IsPackageInstalled(bpmpkg.PkgInfo.Name, rootDir) && GetPackageInfo(bpmpkg.PkgInfo.Name, rootDir).GetFullVersion() == bpmpkg.PkgInfo.GetFullVersion() {
				continue
			}

			// Set package installation reason
			installationReason := forceInstallationReason
			if installationReason == InstallationReasonUnknown {
				if IsPackageInstalled(bpmpkg.PkgInfo.Name, rootDir) {
					installationReason = GetPackage(bpmpkg.PkgInfo.Name, rootDir).LocalInfo.GetInstallationReason()
				} else {
					installationReason = InstallationReasonManual
				}
			}

			operation.AppendAction(&InstallPackageAction{
				File:               pkg,
				InstallationReason: installationReason,
				BpmPackage:         bpmpkg,
			})
		} else {
			var entry *BPMDatabaseEntry

			if e, _, err := GetDatabaseEntry(pkg); err == nil {
				entry = e
			} else if isVirtual, p := IsVirtualPackage(pkg, rootDir); isVirtual {
				entry, _, err = GetDatabaseEntry(p)
				if err != nil {
					pkgsNotFound = append(pkgsNotFound, pkg)
					continue
				}
			} else if e := ResolveVirtualPackage(pkg); e != nil {
				entry = e
			} else {
				pkgsNotFound = append(pkgsNotFound, pkg)
				continue
			}
			if reinstallMethod == ReinstallMethodNone && IsPackageInstalled(entry.Info.Name, rootDir) && GetPackageInfo(entry.Info.Name, rootDir).GetFullVersion() == entry.Info.GetFullVersion() {
				continue
			}

			// Set package installation reason
			installationReason := forceInstallationReason
			if installationReason == InstallationReasonUnknown {
				if IsPackageInstalled(entry.Info.Name, rootDir) {
					installationReason = GetPackage(entry.Info.Name, rootDir).LocalInfo.GetInstallationReason()
				} else {
					installationReason = InstallationReasonManual
				}
			}

			operation.AppendAction(&FetchPackageAction{
				InstallationReason: installationReason,
				DatabaseEntry:      entry,
			})
		}
	}

	// Return error if not all packages are found
	if len(pkgsNotFound) != 0 {
		return nil, PackageNotFoundErr{pkgsNotFound}
	}

	// Resolve dependencies
	err = operation.ResolveDependencies(reinstallMethod == ReinstallMethodAll, installRuntimeDependencies, installOptionalDependencies, verbose)
	if err != nil {
		return nil, fmt.Errorf("could not resolve dependencies: %s", err)
	}
	if len(operation.UnresolvedDepends) != 0 {
		if !forceInstallation {
			return nil, DependencyNotFoundErr{operation.UnresolvedDepends}
		} else if verbose {
			log.Printf("Warning: %s", DependencyNotFoundErr{operation.UnresolvedDepends})
		}
	}

	// Replace obsolete packages
	operation.ReplaceObsoletePackages()

	// Check for conflicts
	conflicts := operation.CheckForConflicts()
	if len(conflicts) > 0 {
		err = fmt.Errorf("conflicts detected")
		for pkg, conflict := range conflicts {
			err = errors.Join(err, PackageConflictErr{pkg, conflict})
		}
		if !forceInstallation {
			return nil, err
		} else {
			log.Printf("Warning: %s", err)
		}
	}

	// Check whether compiling source packages on different root directory
	if rootDir != "/" {
		sourcePackages := make([]string, 0)
		for _, action := range operation.Actions {
			switch action := action.(type) {
			case *InstallPackageAction:
				if action.BpmPackage.PkgInfo.Type == "source" {
					sourcePackages = append(sourcePackages, action.BpmPackage.PkgInfo.Name)
				}
			case *FetchPackageAction:
				if action.DatabaseEntry.Info.Type == "source" {
					sourcePackages = append(sourcePackages, action.DatabaseEntry.Info.Name)
				}
			}
		}

		// Return error if source packages are present in the operation
		if len(sourcePackages) != 0 {
			return nil, fmt.Errorf("cannot compile source packages in different root directory: %s", strings.Join(sourcePackages, ", "))
		}
	}

	return operation, nil
}

// RemovePackages removes the specified packages from the given root directory
func RemovePackages(rootDir string, force, cleanupDependencies bool, packages ...string) (operation *BPMOperation, err error) {
	operation = &BPMOperation{
		Actions:           make([]OperationAction, 0),
		UnresolvedDepends: make([]string, 0),
		Changes:           make(map[string]string),
		RootDir:           rootDir,
		compiledPackages:  make(map[string]string),
	}

	// Remove duplicates from packages
	packages = removeDuplicates(packages)

	// Search for packages
	for _, pkg := range packages {
		bpmpkg := GetPackage(pkg, rootDir)
		if isVirutal, vpkg := IsVirtualPackage(pkg, rootDir); isVirutal {
			bpmpkg = GetPackage(vpkg, rootDir)
		}
		if bpmpkg == nil {
			continue
		}
		operation.AppendAction(&RemovePackageAction{BpmPackage: bpmpkg})
	}

	// Do package cleanup
	if cleanupDependencies {
		err := operation.Cleanup(MainBPMConfig.CleanupMakeDependencies)
		if err != nil {
			return nil, fmt.Errorf("could not perform cleanup for operation: %s", err)
		}
	}

	// Return error if other packages depend on removed ones
	if !force {
		// Get packages and their dependants
		packageDepndants := make(map[string][]string, 0)
		for _, action := range operation.Actions {
			// Skip package if ignored
			if slices.Contains(MainBPMConfig.IgnorePackages, action.(*RemovePackageAction).BpmPackage.PkgInfo.Name) {
				continue
			}

			dependants := action.(*RemovePackageAction).BpmPackage.PkgInfo.GetPackageDependants(rootDir)
			packageDepndants[action.(*RemovePackageAction).BpmPackage.PkgInfo.Name] = dependants
		}

		// Remove dependant packages from map if they are to be removed by this operation
		for pkg, required := range packageDepndants {
			required = slices.DeleteFunc(required, func(pkgName string) bool {
				_, ok := packageDepndants[pkgName]
				return ok
			})
			packageDepndants[pkg] = required
		}

		// Remove dependant packages if ignored
		for pkg, required := range packageDepndants {
			required = slices.DeleteFunc(required, func(pkgName string) bool {
				return slices.Contains(MainBPMConfig.IgnorePackages, pkgName)
			})
			packageDepndants[pkg] = required
		}

		// Remove empty keys from map
		maps.DeleteFunc(packageDepndants, func(pkg string, required []string) bool {
			return len(required) == 0
		})

		// Return error
		if len(packageDepndants) != 0 {
			return nil, PackageRemovalDependencyErr{RequiredPackages: packageDepndants}
		}
	}

	return operation, nil
}

// CleanupPackages finds packages installed as dependencies which are no longer required by the rest of the system in the given root directory
func CleanupPackages(cleanupMakeDepends bool, rootDir string) (operation *BPMOperation, err error) {
	operation = &BPMOperation{
		Actions:           make([]OperationAction, 0),
		UnresolvedDepends: make([]string, 0),
		Changes:           make(map[string]string),
		RootDir:           rootDir,
		compiledPackages:  make(map[string]string),
	}

	// Do package cleanup
	err = operation.Cleanup(cleanupMakeDepends)
	if err != nil {
		return nil, fmt.Errorf("could not perform cleanup for operation: %s", err)
	}

	return operation, nil
}

func CleanupCache(rootDir string, cleanupCompilationFiles, cleanupCompiledPackages, cleanupFetchedPackages, verbose bool) error {
	if cleanupCompilationFiles {
		globalCompilationCacheDir := path.Join(rootDir, "var/cache/bpm/compilation")

		// Ensure path exists and is a directory
		if stat, err := os.Stat(globalCompilationCacheDir); err == nil && stat.IsDir() {
			// Delete directory
			if verbose {
				log.Printf("Removing directory (%s)\n", globalCompilationCacheDir)
			}
			err = os.RemoveAll(globalCompilationCacheDir)
			if err != nil {
				return err
			}
		}

		// Get home directories of users in root directory
		homeDirs, err := os.ReadDir(path.Join(rootDir, "home"))
		if err != nil {
			return err
		}

		// Loop through all home directories
		for _, homeDir := range homeDirs {
			// Skip if not a directory
			if !homeDir.IsDir() {
				continue
			}

			localCompilationDir := path.Join(rootDir, "home", homeDir.Name(), ".cache/bpm/compilation")

			// Ensure path exists and is a directory
			if stat, err := os.Stat(localCompilationDir); err != nil || !stat.IsDir() {
				continue
			}

			// Delete directory
			if verbose {
				log.Printf("Removing directory (%s)\n", localCompilationDir)
			}
			err = os.RemoveAll(localCompilationDir)
			if err != nil {
				return err
			}
		}
	}

	if cleanupCompiledPackages {
		dirToRemove := path.Join(rootDir, "var/cache/bpm/compiled")

		// Ensure path exists and is a directory
		if stat, err := os.Stat(dirToRemove); err == nil && stat.IsDir() {
			// Delete directory
			if verbose {
				log.Printf("Removing directory (%s)\n", dirToRemove)
			}
			err = os.RemoveAll(dirToRemove)
			if err != nil {
				return err
			}
		}
	}

	if cleanupFetchedPackages {
		dirToRemove := path.Join(rootDir, "var/cache/bpm/fetched")

		// Ensure path exists and is a directory
		if stat, err := os.Stat(dirToRemove); err == nil && stat.IsDir() {
			// Delete directory
			if verbose {
				log.Printf("Removing directory (%s)\n", dirToRemove)
			}
			err = os.RemoveAll(dirToRemove)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// UpdatePackages fetches the newest versions of all installed packages from
func UpdatePackages(rootDir string, syncDatabase bool, allowDowngrades bool, installOptionalDependencies, forceInstallation, runChecks, verbose bool) (operation *BPMOperation, err error) {
	// Sync databases
	if syncDatabase {
		err := SyncDatabase(verbose)
		if err != nil {
			return nil, fmt.Errorf("could not sync local database: %s", err)
		}
		if verbose {
			fmt.Println("All package databases synced successfully!")
		}

		// Reload config and local databases
		err = ReadConfig()
		if err != nil {
			return nil, fmt.Errorf("could not read BPM config: %s", err)
		}
		err = ReadLocalDatabaseFiles()
		if err != nil {
			return nil, fmt.Errorf("could not read local databases: %s", err)
		}
	}

	// Get installed packages and check for updates
	pkgs, err := GetInstalledPackages(rootDir)
	if err != nil {
		return nil, fmt.Errorf("could not get installed packages: %s", err)
	}

	operation = &BPMOperation{
		Actions:           make([]OperationAction, 0),
		UnresolvedDepends: make([]string, 0),
		Changes:           make(map[string]string),
		RunChecks:         runChecks,
		RootDir:           rootDir,
		compiledPackages:  make(map[string]string),
	}

	// Search for packages
	for _, pkg := range pkgs {
		if slices.Contains(MainBPMConfig.IgnorePackages, pkg) {
			continue
		}
		var entry *BPMDatabaseEntry
		// Check if installed package can be replaced and install that instead
		if e := FindReplacement(pkg); e != nil {
			entry = e
		} else if entry, _, err = GetDatabaseEntry(pkg); err != nil {
			continue
		}

		installedInfo := GetPackageInfo(pkg, rootDir)
		if installedInfo == nil {
			return nil, fmt.Errorf("could not get package info for package (%s)", pkg)
		} else {
			comparison := CompareVersions(entry.Info.GetFullVersion(), installedInfo.GetFullVersion())
			if (!allowDowngrades && comparison > 0) || (allowDowngrades && comparison != 0) {
				operation.AppendAction(&FetchPackageAction{
					InstallationReason: GetPackage(pkg, rootDir).LocalInfo.GetInstallationReason(),
					DatabaseEntry:      entry,
				})
			}
		}
	}

	// Check for new dependencies in updated packages
	err = operation.ResolveDependencies(false, true, installOptionalDependencies, verbose)
	if err != nil {
		return nil, fmt.Errorf("could not resolve dependencies: %s", err)
	}
	if len(operation.UnresolvedDepends) != 0 {
		if !forceInstallation {
			return nil, DependencyNotFoundErr{operation.UnresolvedDepends}
		} else if verbose {
			log.Printf("Warning: %s", DependencyNotFoundErr{operation.UnresolvedDepends})
		}
	}

	// Replace obsolete packages
	operation.ReplaceObsoletePackages()

	// Check for conflicts
	conflicts := operation.CheckForConflicts()
	if len(conflicts) > 0 {
		err = fmt.Errorf("conflicts detected")
		for pkg, conflict := range conflicts {
			err = errors.Join(err, PackageConflictErr{pkg, conflict})
		}
		if !forceInstallation {
			return nil, err
		} else {
			log.Printf("Warning: %s", err)
		}
	}

	return operation, nil
}

// SyncDatabase syncs all databases declared in /etc/bpm.conf
func SyncDatabase(verbose bool) (err error) {
	for _, db := range MainBPMConfig.Databases {
		if verbose {
			fmt.Printf("Fetching package database file for database (%s)...\n", db.Name)
		}

		err := db.SyncLocalDatabaseFile()
		if err != nil {
			return err
		}
	}

	return nil
}
