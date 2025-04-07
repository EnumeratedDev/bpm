package bpmlib

import (
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
)

type ReinstallMethod uint8

const (
	ReinstallMethodNone      ReinstallMethod = iota
	ReinstallMethodSpecified ReinstallMethod = iota
	ReinstallMethodAll       ReinstallMethod = iota
)

// InstallPackages installs the specified packages into the given root directory by fetching them from repositories or directly from local bpm archives
func InstallPackages(rootDir string, installationReason InstallationReason, reinstallMethod ReinstallMethod, installOptionalDependencies, forceInstallation, verbose bool, packages ...string) (operation *BPMOperation, err error) {
	// Setup operation struct
	operation = &BPMOperation{
		Actions:                 make([]OperationAction, 0),
		UnresolvedDepends:       make([]string, 0),
		Changes:                 make(map[string]string),
		RootDir:                 rootDir,
		ForceInstallationReason: installationReason,
	}

	// Resolve packages
	for _, pkg := range packages {
		if stat, err := os.Stat(pkg); err == nil && !stat.IsDir() {
			bpmpkg, err := ReadPackage(pkg)
			if err != nil {
				return nil, fmt.Errorf("could not read package: %s", err)
			}
			if reinstallMethod == ReinstallMethodNone && IsPackageInstalled(bpmpkg.PkgInfo.Name, rootDir) && GetPackageInfo(bpmpkg.PkgInfo.Name, rootDir).GetFullVersion() == bpmpkg.PkgInfo.GetFullVersion() {
				continue
			}
			operation.AppendAction(&InstallPackageAction{
				File:         pkg,
				IsDependency: false,
				BpmPackage:   bpmpkg,
			})
		} else {
			var entry *RepositoryEntry

			if e, _, err := GetRepositoryEntry(pkg); err == nil {
				entry = e
			} else if isVirtual, p := IsVirtualPackage(pkg, rootDir); isVirtual {
				entry, _, err = GetRepositoryEntry(p)
				if err != nil {
					return nil, fmt.Errorf("could not find package (%s) in any repositor", p)
				}
			} else if e := ResolveVirtualPackage(pkg); e != nil {
				entry = e
			} else {
				return nil, fmt.Errorf("could not find package (%s) in any repository", pkg)
			}
			if reinstallMethod == ReinstallMethodNone && IsPackageInstalled(entry.Info.Name, rootDir) && GetPackageInfo(entry.Info.Name, rootDir).GetFullVersion() == entry.Info.GetFullVersion() {
				continue
			}
			operation.AppendAction(&FetchPackageAction{
				IsDependency:    false,
				RepositoryEntry: entry,
			})
		}
	}

	// Resolve dependencies
	err = operation.ResolveDependencies(reinstallMethod == ReinstallMethodAll, installOptionalDependencies, verbose)
	if err != nil {
		return nil, fmt.Errorf("could not resolve dependencies: %s", err)
	}
	if len(operation.UnresolvedDepends) != 0 {
		if !forceInstallation {
			return nil, fmt.Errorf("dependencies (%s) could not be found in any repositories", strings.Join(operation.UnresolvedDepends, ", "))
		} else if verbose {
			log.Println("Warning: The following dependencies could not be found in any repositories: " + strings.Join(operation.UnresolvedDepends, ", "))
		}
	}

	// Replace obsolete packages
	operation.ReplaceObsoletePackages()

	// Check for conflicts
	conflicts, err := operation.CheckForConflicts()
	if err != nil {
		return nil, fmt.Errorf("could not complete package conflict check: %s", err)
	}
	if len(conflicts) > 0 {
		if verbose {
			for pkg, conflict := range conflicts {
				fmt.Printf("%s is in conflict with packages (%s)\n", pkg, strings.Join(conflict, ", "))
			}
		}
		if !forceInstallation {
			return nil, fmt.Errorf("conflicting packages found")
		} else {
			log.Println("Warning: conflicting packages found")
		}
	}

	return operation, nil
}

// RemovePackages removes the specified packages from the given root directory
func RemovePackages(rootDir string, removeUnusedPackagesOnly, cleanupDependencies, verbose bool, packages ...string) (operation *BPMOperation, err error) {
	operation = &BPMOperation{
		Actions:           make([]OperationAction, 0),
		UnresolvedDepends: make([]string, 0),
		Changes:           make(map[string]string),
		RootDir:           rootDir,
	}

	// Search for packages
	for _, pkg := range packages {
		bpmpkg := GetPackage(pkg, rootDir)
		if bpmpkg == nil {
			continue
		}
		operation.AppendAction(&RemovePackageAction{BpmPackage: bpmpkg})
	}

	// Do not remove packages which other packages depend on
	if removeUnusedPackagesOnly {
		err := operation.RemoveNeededPackages()
		if err != nil {
			return nil, fmt.Errorf("could not skip needed packages: %s", err)
		}
	}

	// Do package cleanup
	if cleanupDependencies {
		err := operation.Cleanup(verbose)
		if err != nil {
			return nil, fmt.Errorf("could not perform cleanup for operation: %s", err)
		}
	}
	return operation, nil
}

// CleanupPackages finds packages installed as dependencies which are no longer required by the rest of the system in the given root directory
func CleanupPackages(rootDir string, verbose bool) (operation *BPMOperation, err error) {
	operation = &BPMOperation{
		Actions:           make([]OperationAction, 0),
		UnresolvedDepends: make([]string, 0),
		Changes:           make(map[string]string),
		RootDir:           rootDir,
	}

	// Do package cleanup
	err = operation.Cleanup(verbose)
	if err != nil {
		return nil, fmt.Errorf("could not perform cleanup for operation: %s", err)
	}

	return operation, nil
}

// UpdatePackages fetches the newest versions of all installed packages from
func UpdatePackages(rootDir string, syncDatabase bool, installOptionalDependencies, forceInstallation, verbose bool) (operation *BPMOperation, err error) {
	// Sync repositories
	if syncDatabase {
		err := SyncDatabase(verbose)
		if err != nil {
			return nil, fmt.Errorf("could not sync local database: %s", err)
		}
		if verbose {
			fmt.Println("All package databases synced successfully!")
		}
	}

	// Reload config and local databases
	ReadConfig()

	// Get installed packages and check for updates
	pkgs, err := GetInstalledPackages(rootDir)
	if err != nil {
		return nil, fmt.Errorf("could not get installed packages: %s", err)
	}

	operation = &BPMOperation{
		Actions:                 make([]OperationAction, 0),
		UnresolvedDepends:       make([]string, 0),
		Changes:                 make(map[string]string),
		RootDir:                 rootDir,
		ForceInstallationReason: Unknown,
	}

	// Search for packages
	for _, pkg := range pkgs {
		if slices.Contains(BPMConfig.IgnorePackages, pkg) {
			continue
		}
		var entry *RepositoryEntry
		// Check if installed package can be replaced and install that instead
		if e := FindReplacement(pkg); e != nil {
			entry = e
		} else if entry, _, err = GetRepositoryEntry(pkg); err != nil {
			continue
		}

		installedInfo := GetPackageInfo(pkg, rootDir)
		if installedInfo == nil {
			return nil, fmt.Errorf("could not get package info for package (%s)", pkg)
		} else {
			comparison := ComparePackageVersions(*entry.Info, *installedInfo)
			if comparison > 0 {
				operation.AppendAction(&FetchPackageAction{
					IsDependency:    false,
					RepositoryEntry: entry,
				})
			}
		}
	}

	// Check for new dependencies in updated packages
	err = operation.ResolveDependencies(false, installOptionalDependencies, verbose)
	if err != nil {
		return nil, fmt.Errorf("could not resolve dependencies: %s", err)
	}
	if len(operation.UnresolvedDepends) != 0 {
		if !forceInstallation {
			return nil, fmt.Errorf("dependencies (%s) could not be found in any repositories", strings.Join(operation.UnresolvedDepends, ", "))
		} else if verbose {
			log.Printf("Warning: dependencies (%s) could not be found in any repositories\n", strings.Join(operation.UnresolvedDepends, ", "))
		}
	}

	// Replace obsolete packages
	operation.ReplaceObsoletePackages()
	return operation, nil
}

// SyncDatabase syncs all databases declared in /etc/bpm.conf
func SyncDatabase(verbose bool) (err error) {
	for _, repo := range BPMConfig.Repositories {
		if verbose {
			fmt.Printf("Fetching package database for repository (%s)...\n", repo.Name)
		}

		err := repo.SyncLocalDatabase()
		if err != nil {
			return err
		}
	}

	return nil
}
