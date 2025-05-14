package bpmlib

import (
	"errors"
	"fmt"
	"slices"
)

func (pkgInfo *PackageInfo) GetDependencies(includeMakeDepends, includeOptionalDepends bool) []string {
	allDepends := make([]string, 0)
	allDepends = append(allDepends, pkgInfo.Depends...)
	if includeMakeDepends {
		allDepends = append(allDepends, pkgInfo.MakeDepends...)
	}
	if includeOptionalDepends {
		allDepends = append(allDepends, pkgInfo.OptionalDepends...)
	}
	return allDepends
}

func (pkgInfo *PackageInfo) GetAllDependencies(includeMakeDepends, includeOptionalDepends bool, rootDir string) (resolved []string) {
	// Initialize slices
	resolved = make([]string, 0)
	unresolved := make([]string, 0)

	// Call unexported function
	pkgInfo.getAllDependencies(&resolved, &unresolved, includeMakeDepends, includeOptionalDepends, rootDir)

	return resolved
}

func (pkgInfo *PackageInfo) getAllDependencies(resolved *[]string, unresolved *[]string, includeMakeDepends, includeOptionalDepends bool, rootDir string) {
	// Add current package name to unresolved slice
	*unresolved = append(*unresolved, pkgInfo.Name)

	// Loop through all dependencies
	for _, depend := range pkgInfo.GetDependencies(includeMakeDepends, includeOptionalDepends) {
		if !slices.Contains(*resolved, depend) {
			// Add current dependency to resolved slice when circular dependency is detected
			if slices.Contains(*unresolved, depend) {
				if !slices.Contains(*resolved, depend) {
					*resolved = append(*resolved, depend)
				}
				continue
			}

			var dependInfo *PackageInfo

			if isVirtual, p := IsVirtualPackage(depend, rootDir); isVirtual {
				dependInfo = GetPackageInfo(p, rootDir)
			} else {
				dependInfo = GetPackageInfo(depend, rootDir)
			}

			if dependInfo != nil {
				dependInfo.getAllDependencies(resolved, unresolved, includeMakeDepends, includeOptionalDepends, rootDir)
			}
		}
	}
	if !slices.Contains(*resolved, pkgInfo.Name) {
		*resolved = append(*resolved, pkgInfo.Name)
	}
	*unresolved = stringSliceRemove(*unresolved, pkgInfo.Name)
}

func ResolvePackageDependenciesFromDatabases(pkgInfo *PackageInfo, checkMake, checkOptional, ignoreInstalled, verbose bool, rootDir string) (resolved []string, unresolved []string) {
	// Initialize slices
	resolved = make([]string, 0)
	unresolved = make([]string, 0)

	// Call unexported function
	resolvePackageDependenciesFromDatabase(&resolved, &unresolved, pkgInfo, checkMake, checkOptional, ignoreInstalled, verbose, rootDir)

	return resolved, unresolved
}

func resolvePackageDependenciesFromDatabase(resolved, unresolved *[]string, pkgInfo *PackageInfo, checkMake, checkOptional, ignoreInstalled, verbose bool, rootDir string) {
	// Add current package name to unresolved slice
	*unresolved = append(*unresolved, pkgInfo.Name)

	// Loop through all dependencies
	for _, depend := range pkgInfo.GetDependencies(checkMake, checkOptional) {
		if !slices.Contains(*resolved, depend) {
			// Add current dependency to resolved slice when circular dependency is detected
			if slices.Contains(*unresolved, depend) {
				if verbose {
					fmt.Printf("Circular dependency was detected (%s -> %s). Installing %s first\n", pkgInfo.Name, depend, depend)
				}
				if !slices.Contains(*resolved, depend) {
					*resolved = append(*resolved, depend)
				}
				continue
			} else if ignoreInstalled && IsPackageProvided(depend, rootDir) {
				continue
			}
			var err error
			var entry *BPMDatabaseEntry
			entry, _, err = GetDatabaseEntry(depend)
			if err != nil {
				if entry = ResolveVirtualPackage(depend); entry == nil {
					if !slices.Contains(*unresolved, depend) {
						*unresolved = append(*unresolved, depend)
					}
					continue
				}
			}
			resolvePackageDependenciesFromDatabase(resolved, unresolved, entry.Info, checkMake, checkOptional, ignoreInstalled, verbose, rootDir)
		}
	}
	if !slices.Contains(*resolved, pkgInfo.Name) {
		*resolved = append(*resolved, pkgInfo.Name)
	}
	*unresolved = stringSliceRemove(*unresolved, pkgInfo.Name)
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

		// Get installed package dependencies
		dependencies := installedPkg.PkgInfo.GetDependencies(false, true)

		// Add installed package to list if its dependencies include pkgName
		if slices.Contains(dependencies, pkgName) {
			ret = append(ret, installedPkgName)
			continue
		}

		// Loop through each virtual package
		for _, vpkg := range pkg.PkgInfo.Provides {
			// Add installed package to list if its dependencies contain a provided virtual package
			if slices.Contains(dependencies, vpkg) {
				ret = append(ret, installedPkgName)
				break
			}
		}
	}

	return ret, nil
}
