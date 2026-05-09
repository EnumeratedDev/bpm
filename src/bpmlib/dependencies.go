package bpmlib

import (
	"slices"
	"strings"
)

func (pkgInfo *PackageInfo) GetPackageDependants(rootDir string, skipMultipleProviders bool) (dependants []string) {
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
			n, _, _ = SplitPkgNameAndVersion(n)
			return n == pkgInfo.Name
		}) {
			dependants = append(dependants, installedPkg.Name)
			continue
		}

		// Add installed package to list if its runtime dependencies include pkgName
		if slices.ContainsFunc(installedPkg.RuntimeDepends, func(n string) bool {
			n, _, _ = SplitPkgNameAndVersion(n)
			return n == pkgInfo.Name
		}) {
			dependants = append(dependants, installedPkg.Name)
			continue
		}

		// Loop through each virtual package
		for _, vpkg := range pkgInfo.Provides {
			if skipMultipleProviders && len(GetVirtualPackageInfo(vpkg, rootDir)) > 1 {
				continue
			}

			// Add installed package to list if its dependencies contain a provided virtual package
			if slices.ContainsFunc(installedPkg.Depends, func(n string) bool {
				n, _, _ = SplitPkgNameAndVersion(n)
				return n == vpkg
			}) {
				dependants = append(dependants, installedPkg.Name)
				break
			}

			// Add installed package to list if its runtime dependencies contain a provided virtual package
			if slices.ContainsFunc(installedPkg.RuntimeDepends, func(n string) bool {
				n, _, _ = SplitPkgNameAndVersion(n)
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
			// Remove optional dependency comment
			n = strings.SplitN(n, ":", 2)[0]

			// Remove required version
			n, _, _ = SplitPkgNameAndVersion(n)

			return n == pkgInfo.Name
		}) {
			dependants = append(dependants, installedPkg.Name)
			continue
		}

		// Loop through each virtual package
		for _, vpkg := range pkgInfo.Provides {
			// Add installed package to list if its optional dependencies contain a provided virtual package
			if slices.ContainsFunc(installedPkg.OptionalDepends, func(n string) bool {
				// Remove optional dependency comment
				n = strings.SplitN(n, ":", 2)[0]

				// Remove required version
				n, _, _ = SplitPkgNameAndVersion(n)

				return n == vpkg
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

func ResolveDependencies(pkgInfo *PackageInfo, resolvedVirtualPackages map[string]string, includeRuntimeDepends bool, rootDir string) (resolved []ResolvedPackage, unresolved []string) {
	visited := make([]string, 0)

	var dfs func(resolvedPkg *PackageInfo)
	dfs = func(pkgInfo *PackageInfo) {
		checkDependencies := func(dependencies []string, installationReason InstallationReason) {
			for _, depend := range dependencies {
				// Split dependency name and required version
				dependName, _, _ := SplitPkgNameAndVersion(depend)

				// Ignore if package is already installed
				if IsPackageInstalled(dependName, rootDir) && EvaluateDependency(depend, GetPackageInfo(dependName, rootDir).Version) {
					continue
				} else if providers := GetVirtualPackageInfo(dependName, rootDir); len(providers) > 0 {
					continue
				}

				// Find database entry for dependency
				var dependEntry *BPMDatabaseEntry
				if resolvedVpkg, ok := resolvedVirtualPackages[dependName]; ok {
					dependEntry, _, _ = GetDatabaseEntry(resolvedVpkg)
				} else if entry, _, _ := GetDatabaseEntry(dependName); entry != nil {
					dependEntry = entry
				} else if providers := GetDatabaseVirtualPackageEntry(dependName); len(providers) > 0 {
					dependEntry = providers[0]
				}

				if dependEntry == nil {
					unresolved = append(unresolved, depend)
					continue
				}

				// Ensure entry has required version
				if !EvaluateDependency(depend, dependEntry.Info.Version) {
					unresolved = append(unresolved, depend)
					continue
				}

				// Skip ignored packages in config
				if slices.Contains(MainBPMConfig.IgnorePackages, dependEntry.Info.Name) {
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
	}

	dfs(pkgInfo)

	return resolved, unresolved
}

func SplitPkgNameAndVersion(pkg string) (string, string, string) {
	if strings.Contains(pkg, ">=") {
		pkgSplit := strings.SplitN(pkg, ">=", 2)
		pkgName := pkgSplit[0]
		pkgVersion := pkgSplit[1]

		return pkgName, ">=", pkgVersion
	} else if strings.Contains(pkg, ">") {
		pkgSplit := strings.SplitN(pkg, ">", 2)
		pkgName := pkgSplit[0]
		pkgVersion := pkgSplit[1]

		return pkgName, ">", pkgVersion
	} else if strings.Contains(pkg, "<=") {
		pkgSplit := strings.SplitN(pkg, "<=", 2)
		pkgName := pkgSplit[0]
		pkgVersion := pkgSplit[1]

		return pkgName, "<=", pkgVersion
	} else if strings.Contains(pkg, "<") {
		pkgSplit := strings.SplitN(pkg, "<", 2)
		pkgName := pkgSplit[0]
		pkgVersion := pkgSplit[1]

		return pkgName, "<", pkgVersion
	} else if strings.Contains(pkg, "=") {
		pkgSplit := strings.SplitN(pkg, "=", 2)
		pkgName := pkgSplit[0]
		pkgVersion := pkgSplit[1]

		return pkgName, "=", pkgVersion
	}

	return pkg, "", ""
}

func EvaluateDependency(pkg, matchVersion string) bool {
	_, comparisonSymbol, pkgVersion := SplitPkgNameAndVersion(pkg)

	switch comparisonSymbol {
	case ">=":
		return CompareVersions(matchVersion, pkgVersion) >= 0
	case ">":
		return CompareVersions(matchVersion, pkgVersion) > 0
	case "<=":
		return CompareVersions(matchVersion, pkgVersion) <= 0
	case "<":
		return CompareVersions(matchVersion, pkgVersion) < 0
	case "=":
		if cutPkgVersion, ok := strings.CutSuffix(pkgVersion, "*"); ok {
			return strings.HasPrefix(matchVersion, cutPkgVersion)
		} else {
			return CompareVersions(matchVersion, pkgVersion) == 0
		}
	default:
		return true
	}
}
