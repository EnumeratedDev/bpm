package bpmlib

import (
	"fmt"
	"slices"
	"strings"
)

type pkgInstallationReason struct {
	PkgName            string
	InstallationReason InstallationReason
}

func (pkgInfo *PackageInfo) GetDependencies(includeMakeDepends, includeRuntimeDepends, includeOptionalDepends bool) []pkgInstallationReason {
	allDepends := make([]pkgInstallationReason, 0)

	for _, depend := range pkgInfo.Depends {
		if !slices.ContainsFunc(allDepends, func(p pkgInstallationReason) bool {
			return p.PkgName == depend
		}) {
			allDepends = append(allDepends, pkgInstallationReason{
				PkgName:            depend,
				InstallationReason: InstallationReasonDependency,
			})
		}
	}
	if includeOptionalDepends {
		for _, depend := range pkgInfo.OptionalDepends {
			depend = strings.SplitN(depend, ":", 2)[0]
			if !slices.ContainsFunc(allDepends, func(p pkgInstallationReason) bool {
				return p.PkgName == depend
			}) {
				allDepends = append(allDepends, pkgInstallationReason{
					PkgName:            depend,
					InstallationReason: InstallationReasonManual,
				})
			}
		}
	}
	if includeRuntimeDepends {
		for _, depend := range pkgInfo.RuntimeDepends {
			if !slices.ContainsFunc(allDepends, func(p pkgInstallationReason) bool {
				return p.PkgName == depend
			}) {
				allDepends = append(allDepends, pkgInstallationReason{
					PkgName:            depend,
					InstallationReason: InstallationReasonDependency,
				})
			}
		}
	}
	if includeMakeDepends {
		for _, depend := range pkgInfo.MakeDepends {
			if !slices.ContainsFunc(allDepends, func(p pkgInstallationReason) bool {
				return p.PkgName == depend
			}) {
				allDepends = append(allDepends, pkgInstallationReason{
					PkgName:            depend,
					InstallationReason: InstallationReasonMakeDependency,
				})
			}
		}
	}

	// Skip ignored packages
	allDepends = slices.DeleteFunc(allDepends, func(depend pkgInstallationReason) bool {
		return slices.Contains(MainBPMConfig.IgnorePackages, depend.PkgName)
	})

	return allDepends
}

func (pkgInfo *PackageInfo) GetDependenciesRecursive(includeRuntimeDepends, includeMakeDepends bool, rootDir string) (resolved []string) {
	// Initialize slices
	resolved = make([]string, 0)
	unresolved := make([]string, 0)

	// Call unexported function
	pkgInfo.getDependenciesRecursive(&resolved, &unresolved, includeRuntimeDepends, includeMakeDepends, rootDir)

	return resolved
}

func (pkgInfo *PackageInfo) getDependenciesRecursive(resolved *[]string, unresolved *[]string, includeRuntimeDepends, includeMakeDepends bool, rootDir string) {
	// Add current package name to unresolved slice
	*unresolved = append(*unresolved, pkgInfo.Name)

	// Loop through all dependencies
	for _, pkgIR := range pkgInfo.GetDependencies(includeMakeDepends, includeRuntimeDepends, false) {
		depend := pkgIR.PkgName

		if isVirtual, p := IsVirtualPackage(depend, rootDir); isVirtual {
			depend = p
		}

		if !slices.Contains(*resolved, depend) {
			// Add current dependency to resolved slice when circular dependency is detected
			if slices.Contains(*unresolved, depend) {
				if !slices.Contains(*resolved, depend) {
					*resolved = append(*resolved, depend)
				}
				continue
			}

			dependInfo := GetPackageInfo(depend, rootDir)

			if dependInfo != nil {
				dependInfo.getDependenciesRecursive(resolved, unresolved, includeRuntimeDepends, includeMakeDepends, rootDir)
			}
		}
	}
	if !slices.Contains(*resolved, pkgInfo.Name) {
		*resolved = append(*resolved, pkgInfo.Name)
	}
	*unresolved = stringSliceRemove(*unresolved, pkgInfo.Name)
}

func ResolveAllPackageDependenciesFromDatabases(pkgInfo *PackageInfo, resolvedVirtualPkgs map[string]string, checkMake, checkRuntime, checkOptional, ignoreInstalled, verbose bool, rootDir string) (resolved []pkgInstallationReason, unresolved []string) {
	// Initialize slices and maps
	resolved = make([]pkgInstallationReason, 0)
	unresolved = make([]string, 0)
	if resolvedVirtualPkgs == nil {
		resolvedVirtualPkgs = make(map[string]string)
	}

	// Call unexported function
	resolvePackageDependenciesFromDatabase(&resolved, &unresolved, resolvedVirtualPkgs, pkgInfo, checkMake, checkRuntime, checkOptional, ignoreInstalled, verbose, rootDir)

	// Remove main package from unresolved slice
	unresolved = stringSliceRemove(unresolved, pkgInfo.Name)

	return resolved, unresolved
}

func resolvePackageDependenciesFromDatabase(resolved *[]pkgInstallationReason, unresolved *[]string, resolvedVirtualPkgs map[string]string, pkgInfo *PackageInfo, checkMake, checkRuntime, checkOptional, ignoreInstalled, verbose bool, rootDir string) {
	// Add current package name to unresolved slice
	*unresolved = append(*unresolved, pkgInfo.Name)

	for _, vpkg := range pkgInfo.Provides {
		if _, ok := resolvedVirtualPkgs[vpkg]; !ok {
			resolvedVirtualPkgs[vpkg] = pkgInfo.Name
		}
	}

	// Loop through all dependencies
	for _, pkgIR := range pkgInfo.GetDependencies(pkgInfo.Type == "source", checkRuntime, checkOptional) {
		// Skip dependency if it has already been resolved
		if slices.ContainsFunc(*resolved, func(p pkgInstallationReason) bool {
			return p.PkgName == pkgIR.PkgName
		}) {
			continue
		}

		// Add current dependency to resolved slice when circular dependency is detected
		if slices.Contains(*unresolved, pkgIR.PkgName) {
			if verbose {
				fmt.Printf("Circular dependency was detected (%s -> %s). Installing %s first\n", pkgInfo.Name, pkgIR.PkgName, pkgIR.PkgName)
			}

			*resolved = append(*resolved, pkgInstallationReason{
				PkgName:            pkgIR.PkgName,
				InstallationReason: pkgIR.InstallationReason,
			})
			continue
		}

		// Skip dependency if it is already installed or provided
		if ignoreInstalled && IsPackageProvided(pkgIR.PkgName, rootDir) {
			continue
		}

		// Get database entry for dependency
		var err error
		var entry *BPMDatabaseEntry
		entry, _, err = GetDatabaseEntry(pkgIR.PkgName)
		if err != nil {
			if resolvedVirtualPkg, ok := resolvedVirtualPkgs[pkgIR.PkgName]; ok {
				// Virtual package already resolved

				// Move dependency from the unresolved slice to the resolved slice
				if !slices.ContainsFunc(*resolved, func(p pkgInstallationReason) bool {
					return p.PkgName == resolvedVirtualPkg
				}) {
					*resolved = append(*resolved, pkgInstallationReason{
						PkgName:            resolvedVirtualPkg,
						InstallationReason: pkgIR.InstallationReason,
					})
				}
				*unresolved = stringSliceRemove(*unresolved, resolvedVirtualPkg)

				continue
			} else if entry = ResolveVirtualPackage(pkgIR.PkgName); entry != nil {
				// Virtual package found in database
			} else {
				// Virtual package not found
				if !slices.Contains(*unresolved, pkgIR.PkgName) {
					*unresolved = append(*unresolved, pkgIR.PkgName)
				}
				continue
			}
		}

		// Resolve the dependencies of this dependency
		resolvePackageDependenciesFromDatabase(resolved, unresolved, resolvedVirtualPkgs, entry.Info, checkMake, checkRuntime, false, ignoreInstalled, verbose, rootDir)

		// Move dependency from the unresolved slice to the resolved slice
		if !slices.ContainsFunc(*resolved, func(p pkgInstallationReason) bool {
			return p.PkgName == entry.Info.Name
		}) {
			*resolved = append(*resolved, pkgInstallationReason{
				PkgName:            entry.Info.Name,
				InstallationReason: pkgIR.InstallationReason,
			})
		}
		*unresolved = stringSliceRemove(*unresolved, entry.Info.Name)
	}
}

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
