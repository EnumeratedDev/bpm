package bpmlib

import (
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
)

var localPackageInformation map[string]map[string]*BPMPackage = make(map[string]map[string]*BPMPackage)

func initializeLocalPackageInformation(rootDir string) (err error) {
	// Return if information is already initialized
	if _, ok := localPackageInformation[rootDir]; ok {
		return nil
	}

	tempPackageInformation := make(map[string]*BPMPackage)

	// Get path to installed package information directory
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")

	// Get directory content
	items, err := os.ReadDir(installedDir)
	if os.IsNotExist(err) {
		localPackageInformation[rootDir] = make(map[string]*BPMPackage)
		return nil
	}
	if err != nil {
		return err
	}

	// Loop through each subdirectory
	for _, item := range items {
		// Skip if not a directory
		if !item.IsDir() {
			continue
		}

		// Read package info
		infoData, err := os.ReadFile(path.Join(installedDir, item.Name(), "info"))
		if err != nil {
			return err
		}
		info, err := ReadPackageInfo(string(infoData))
		if err != nil {
			return err
		}

		// Read package files
		files := getPackageFiles(info.Name, rootDir)

		// Add package to slice
		tempPackageInformation[info.Name] = &BPMPackage{
			PkgInfo:  info,
			PkgFiles: files,
		}
	}

	localPackageInformation[rootDir] = tempPackageInformation
	return nil
}

func GetInstalledPackages(rootDir string) (ret []string, err error) {
	// Initialize local package information
	err = initializeLocalPackageInformation(rootDir)
	if err != nil {
		return nil, err
	}

	// Loop through each package and add it to slice
	for _, bpmpkg := range localPackageInformation[rootDir] {
		ret = append(ret, bpmpkg.PkgInfo.Name)
	}

	return ret, nil
}

func IsPackageInstalled(pkg, rootDir string) bool {
	// Initialize local package information
	err := initializeLocalPackageInformation(rootDir)
	if err != nil {
		return false
	}

	if _, ok := localPackageInformation[rootDir][pkg]; !ok {
		return false
	}
	return true
}

func GetPackageInfo(pkg string, rootDir string) *PackageInfo {
	// Get BPM package
	bpmpkg := GetPackage(pkg, rootDir)

	// Return nil if not found
	if bpmpkg == nil {
		return nil
	}

	return bpmpkg.PkgInfo
}

func IsVirtualPackage(pkg, rootDir string) (bool, string) {
	pkgs, err := GetInstalledPackages(rootDir)
	if err != nil {
		return false, ""
	}
	for _, p := range pkgs {
		if p == pkg {
			return false, ""
		}
		i := GetPackageInfo(p, rootDir)
		if i == nil {
			continue
		}
		if slices.Contains(i.Provides, pkg) {
			return true, p
		}
	}
	return false, ""
}

func IsPackageProvided(pkg, rootDir string) bool {
	pkgs, err := GetInstalledPackages(rootDir)
	if err != nil {
		return false
	}
	for _, p := range pkgs {
		if p == pkg {
			return true
		}
		i := GetPackageInfo(p, rootDir)
		if i == nil {
			continue
		}
		if slices.Contains(i.Provides, pkg) {
			return true
		}
	}
	return false
}

func GetPackage(pkg, rootDir string) *BPMPackage {
	err := initializeLocalPackageInformation(rootDir)
	if err != nil {
		return nil
	}

	bpmpkg := localPackageInformation[rootDir][pkg]

	return bpmpkg
}

func GetAllPackageFiles(rootDir string, excludePackages ...string) (map[string][]*BPMPackage, error) {
	ret := make(map[string][]*BPMPackage)

	pkgNames, err := GetInstalledPackages(rootDir)
	if err != nil {
		return nil, err
	}

	for _, pkgName := range pkgNames {
		if slices.Contains(excludePackages, pkgName) {
			continue
		}
		bpmpkg := GetPackage(pkgName, rootDir)
		if bpmpkg == nil {
			return nil, errors.New(fmt.Sprintf("could not get BPM package (%s)", pkgName))
		}
		for _, entry := range bpmpkg.PkgFiles {
			if _, ok := ret[entry.Path]; ok {
				ret[entry.Path] = append(ret[entry.Path], bpmpkg)
			} else {
				ret[entry.Path] = []*BPMPackage{bpmpkg}
			}
		}
	}

	return ret, nil
}

func getPackageFiles(pkg, rootDir string) []*PackageFileEntry {
	var pkgFiles []*PackageFileEntry
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	files := path.Join(pkgDir, "files")
	if _, err := os.Stat(installedDir); os.IsNotExist(err) {
		return nil
	}
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		return nil
	}
	bs, err := os.ReadFile(files)
	if err != nil {
		return nil
	}

	for _, line := range strings.Split(string(bs), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		stringEntry := strings.Split(strings.TrimSpace(line), " ")
		if len(stringEntry) < 5 {
			pkgFiles = append(pkgFiles, &PackageFileEntry{
				Path:        strings.TrimSuffix(line, "/"),
				OctalPerms:  0,
				UserID:      0,
				GroupID:     0,
				SizeInBytes: 0,
			})
			continue
		}
		uid, err := strconv.ParseInt(stringEntry[len(stringEntry)-4], 0, 32)
		if err != nil {
			return nil
		}
		octalPerms, err := strconv.ParseInt(stringEntry[len(stringEntry)-3], 0, 32)
		if err != nil {
			return nil
		}
		gid, err := strconv.ParseInt(stringEntry[len(stringEntry)-2], 0, 32)
		if err != nil {
			return nil
		}
		size, err := strconv.ParseUint(stringEntry[len(stringEntry)-1], 0, 64)
		if err != nil {
			return nil
		}
		pkgFiles = append(pkgFiles, &PackageFileEntry{
			Path:        strings.TrimSuffix(strings.Join(stringEntry[:len(stringEntry)-4], " "), "/"),
			OctalPerms:  uint32(octalPerms),
			UserID:      int(uid),
			GroupID:     int(gid),
			SizeInBytes: size,
		})
	}

	return pkgFiles
}
