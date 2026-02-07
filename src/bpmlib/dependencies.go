package bpmlib

import (
	"slices"
	"strings"
)

func (pkgInfo *PackageInfo) GetPackageDependants(rootDir string) (dependants []string) {
	// Get installed package names
	pkgs, ok := localPackageInformation[rootDir]
	if !ok {
		return nil
	}

	// Loop through all installed packages
	for _, installedPkg := range pkgs {
		// Skip iteration if comparing the same packages
		if installedPkg.Name == pkgInfo.Name {
			continue
		}

		// Add installed package to list if its dependencies include pkgName
		if slices.ContainsFunc(installedPkg.Depends, func(n string) bool {
			return n == pkgInfo.Name
		}) {
			dependants = append(dependants, installedPkg.Name)
			continue
		}

		// Add installed package to list if its runtime dependencies include pkgName
		if slices.ContainsFunc(installedPkg.RuntimeDepends, func(n string) bool {
			return n == pkgInfo.Name
		}) {
			dependants = append(dependants, installedPkg.Name)
			continue
		}

		// Loop through each virtual package
		for _, vpkg := range pkgInfo.Provides {
			// Add installed package to list if its dependencies contain a provided virtual package
			if slices.ContainsFunc(installedPkg.Depends, func(n string) bool {
				return n == vpkg
			}) {
				dependants = append(dependants, installedPkg.Name)
				break
			}

			// Add installed package to list if its runtime dependencies contain a provided virtual package
			if slices.ContainsFunc(installedPkg.RuntimeDepends, func(n string) bool {
				return n == vpkg
			}) {
				dependants = append(dependants, installedPkg.Name)
				break
			}
		}
	}

	return dependants
}

func (pkgInfo *PackageInfo) GetPackageOptionalDependants(rootDir string) (dependants []string) {
	// Get installed package names
	pkgs, ok := localPackageInformation[rootDir]
	if !ok {
		return nil
	}

	// Loop through all installed packages
	for _, installedPkg := range pkgs {
		// Skip iteration if comparing the same packages
		if installedPkg.Name == pkgInfo.Name {
			continue
		}

		// Add installed package to list if its optional dependencies include pkgName
		if slices.ContainsFunc(installedPkg.OptionalDepends, func(n string) bool {
			return strings.SplitN(n, ":", 2)[0] == pkgInfo.Name
		}) {
			dependants = append(dependants, installedPkg.Name)
			continue
		}

		// Loop through each virtual package
		for _, vpkg := range pkgInfo.Provides {
			// Add installed package to list if its optional dependencies contain a provided virtual package
			if slices.ContainsFunc(installedPkg.OptionalDepends, func(n string) bool {
				return strings.SplitN(n, ":", 2)[0] == vpkg
			}) {
				dependants = append(dependants, installedPkg.Name)
				break
			}
		}
	}

	return dependants
}

type ResolvedPackage struct {
	DatabaseEntry      *BPMDatabaseEntry
	InstallationReason InstallationReason
}

func ResolveDependencies(pkgInfo *PackageInfo, resolvedVirtualPackages map[string]string, includeRuntimeDepends, includeOptionalDepends bool, rootDir string) (resolved []ResolvedPackage, unresolved []string) {
	visited := make([]string, 0)

	var dfs func(resolvedPkg *PackageInfo)
	dfs = func(pkgInfo *PackageInfo) {
		checkDependencies := func(dependencies []string, installationReason InstallationReason) {
			for _, depend := range dependencies {
				// Ignore if package is already installed
				if IsPackageInstalled(depend, rootDir) {
					continue
				} else if providers := GetVirtualPackageInfo(depend, rootDir); len(providers) > 0 {
					continue
				}

				// Find database entry for dependency
				var dependEntry *BPMDatabaseEntry
				if resolvedVpkg, ok := resolvedVirtualPackages[depend]; ok {
					dependEntry, _, _ = GetDatabaseEntry(resolvedVpkg)
				} else if entry, _, _ := GetDatabaseEntry(depend); entry != nil {
					dependEntry = entry
				} else if providers := GetDatabaseVirtualPackageEntry(depend); len(providers) > 0 {
					dependEntry = providers[0]
				}

				if dependEntry == nil {
					unresolved = append(unresolved, depend)
					continue
				}

				if !slices.Contains(visited, dependEntry.Info.Name) {
					dfs(dependEntry.Info)
					resolved = append(resolved, ResolvedPackage{DatabaseEntry: dependEntry, InstallationReason: installationReason})
				}
			}
		}

		visited = append(visited, pkgInfo.Name)

		checkDependencies(pkgInfo.Depends, InstallationReasonDependency)
		if includeRuntimeDepends {
			checkDependencies(pkgInfo.RuntimeDepends, InstallationReasonDependency)
		}
		if pkgInfo.Type == "source" {
			checkDependencies(pkgInfo.MakeDepends, InstallationReasonMakeDependency)
			checkDependencies(pkgInfo.CheckDepends, InstallationReasonMakeDependency)
		}
		if includeOptionalDepends {
			checkDependencies(pkgInfo.OptionalDepends, InstallationReasonManual)
		}
	}

	dfs(pkgInfo)

	return resolved, unresolved
}
