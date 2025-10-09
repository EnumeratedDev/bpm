package bpmlib

import (
	"errors"
	"fmt"
	"slices"
)

type pkgInstallationReason struct {
	PkgName            string
	InstallationReason InstallationReason
}

func (pkgInfo *PackageInfo) GetDependencies(includeMakeDepends, includeOptionalDepends bool) []pkgInstallationReason {
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
	return allDepends
}

func (pkgInfo *PackageInfo) GetDependenciesRecursive(includeMakeDepends bool, rootDir string) (resolved []string) {
	// Initialize slices
	resolved = make([]string, 0)
	unresolved := make([]string, 0)

	// Call unexported function
	pkgInfo.getDependenciesRecursive(&resolved, &unresolved, includeMakeDepends, rootDir)

	return resolved
}

func (pkgInfo *PackageInfo) getDependenciesRecursive(resolved *[]string, unresolved *[]string, includeMakeDepends bool, rootDir string) {
	// Add current package name to unresolved slice
	*unresolved = append(*unresolved, pkgInfo.Name)

	// Loop through all dependencies
	for _, pkgIR := range pkgInfo.GetDependencies(includeMakeDepends, false) {
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
				dependInfo.getDependenciesRecursive(resolved, unresolved, includeMakeDepends, rootDir)
			}
		}
	}
	if !slices.Contains(*resolved, pkgInfo.Name) {
		*resolved = append(*resolved, pkgInfo.Name)
	}
	*unresolved = stringSliceRemove(*unresolved, pkgInfo.Name)
}

func ResolveAllPackageDependenciesFromDatabases(pkgInfo *PackageInfo, checkMake, checkOptional, ignoreInstalled, verbose bool, rootDir string) (resolved []pkgInstallationReason, unresolved []string) {
	// Initialize slices
	resolved = make([]pkgInstallationReason, 0)
	unresolved = make([]string, 0)

	// Call unexported function
	resolvePackageDependenciesFromDatabase(&resolved, &unresolved, pkgInfo, checkMake, checkOptional, ignoreInstalled, verbose, rootDir)

	// Remove main package from unresolved slice
	unresolved = stringSliceRemove(unresolved, pkgInfo.Name)

	return resolved, unresolved
}

func resolvePackageDependenciesFromDatabase(resolved *[]pkgInstallationReason, unresolved *[]string, pkgInfo *PackageInfo, checkMake, checkOptional, ignoreInstalled, verbose bool, rootDir string) {
	// Add current package name to unresolved slice
	*unresolved = append(*unresolved, pkgInfo.Name)

	// Loop through all dependencies
	for _, pkgIR := range pkgInfo.GetDependencies(pkgInfo.Type == "source", checkOptional) {
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
			if entry = ResolveVirtualPackage(pkgIR.PkgName); entry == nil {
				if !slices.Contains(*unresolved, pkgIR.PkgName) {
					*unresolved = append(*unresolved, pkgIR.PkgName)
				}
				continue
			}
		}

		// Resolve the dependencies of this dependency
		resolvePackageDependenciesFromDatabase(resolved, unresolved, entry.Info, checkMake, false, ignoreInstalled, verbose, rootDir)

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

func GetPackageDependants(pkgName string, rootDir string) ([]string, error) {
	ret := make([]string, 0)

	// Get BPM package
	pkg := GetPackage(pkgName, rootDir)
	if pkg == nil {
		return nil, errors.New("package not found: " + pkgName)
	}

	// Get installed package names
	pkgs, err := GetInstalledPackages(rootDir)
	if err != nil {
		return nil, errors.New("could not get installed packages")
	}

	// Loop through all installed packages
	for _, installedPkgName := range pkgs {
		// Get installed BPM package
		installedPkg := GetPackage(installedPkgName, rootDir)
		if installedPkg == nil {
			return nil, errors.New("package not found: " + installedPkgName)
		}

		// Skip iteration if comparing the same packages
		if installedPkg.PkgInfo.Name == pkgName {
			continue
		}

		// Add installed package to list if its dependencies include pkgName
		if slices.ContainsFunc(installedPkg.PkgInfo.Depends, func(n string) bool {
			return n == pkgName
		}) {
			ret = append(ret, installedPkgName)
			continue
		}

		// Loop through each virtual package
		for _, vpkg := range pkg.PkgInfo.Provides {
			// Add installed package to list if its dependencies contain a provided virtual package
			if slices.ContainsFunc(installedPkg.PkgInfo.Depends, func(n string) bool {
				return n == vpkg
			}) {
				ret = append(ret, installedPkgName)
				break
			}
		}
	}

	return ret, nil
}
