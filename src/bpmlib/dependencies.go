package bpmlib

import (
	"errors"
	"fmt"
	"slices"
)

func (pkgInfo *PackageInfo) GetDependencies(includeMakeDepends, includeOptionalDepends bool) map[string]InstallationReason {
	allDepends := make(map[string]InstallationReason)

	for _, depend := range pkgInfo.Depends {
		allDepends[depend] = InstallationReasonDependency
	}
	if includeOptionalDepends {
		for _, depend := range pkgInfo.OptionalDepends {
			if _, ok := allDepends[depend]; !ok {
				allDepends[depend] = InstallationReasonDependency
			}
		}
	}
	if includeMakeDepends {
		for _, depend := range pkgInfo.MakeDepends {
			if _, ok := allDepends[depend]; !ok {
				allDepends[depend] = InstallationReasonMakeDependency
			}
		}
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
	for depend := range pkgInfo.GetDependencies(includeMakeDepends, includeOptionalDepends) {
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
				dependInfo.getAllDependencies(resolved, unresolved, includeMakeDepends, includeOptionalDepends, rootDir)
			}
		}
	}
	if !slices.Contains(*resolved, pkgInfo.Name) {
		*resolved = append(*resolved, pkgInfo.Name)
	}
	*unresolved = stringSliceRemove(*unresolved, pkgInfo.Name)
}

func ResolveAllPackageDependenciesFromDatabases(pkgInfo *PackageInfo, checkMake, checkOptional, ignoreInstalled, verbose bool, rootDir string) (resolved map[string]InstallationReason, unresolved []string) {
	// Initialize slices
	resolved = make(map[string]InstallationReason)
	unresolved = make([]string, 0)

	// Call unexported function
	resolvePackageDependenciesFromDatabase(resolved, &unresolved, pkgInfo, InstallationReasonDependency, checkMake, checkOptional, ignoreInstalled, verbose, rootDir)

	return resolved, unresolved
}

func resolvePackageDependenciesFromDatabase(resolved map[string]InstallationReason, unresolved *[]string, pkgInfo *PackageInfo, installationReason InstallationReason, checkMake, checkOptional, ignoreInstalled, verbose bool, rootDir string) {
	// Add current package name to unresolved slice
	*unresolved = append(*unresolved, pkgInfo.Name)

	// Loop through all dependencies
	for depend, ir := range pkgInfo.GetDependencies(pkgInfo.Type == "source", checkOptional) {
		if _, ok := resolved[depend]; !ok {
			// Add current dependency to resolved slice when circular dependency is detected
			if slices.Contains(*unresolved, depend) {
				if verbose {
					fmt.Printf("Circular dependency was detected (%s -> %s). Installing %s first\n", pkgInfo.Name, depend, depend)
				}
				if _, ok := resolved[depend]; !ok {
					resolved[depend] = ir
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
			resolvePackageDependenciesFromDatabase(resolved, unresolved, entry.Info, ir, checkMake, checkOptional, ignoreInstalled, verbose, rootDir)
		}
	}

	if _, ok := resolved[pkgInfo.Name]; !ok {
		resolved[pkgInfo.Name] = installationReason
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
		if _, ok := dependencies[pkgName]; ok {
			ret = append(ret, installedPkgName)
			continue
		}

		// Loop through each virtual package
		for _, vpkg := range pkg.PkgInfo.Provides {
			// Add installed package to list if its dependencies contain a provided virtual package
			if _, ok := dependencies[vpkg]; ok {
				ret = append(ret, installedPkgName)
				break
			}
		}
	}

	return ret, nil
}
