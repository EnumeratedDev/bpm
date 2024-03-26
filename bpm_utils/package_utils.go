package bpm_utils

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
)

type PackageInfo struct {
	Name        string
	Description string
	Version     string
	Type        string
	Depends     []string
	Provides    []string
}

func ReadPackage(filename string) (*PackageInfo, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(archive)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Name == "pkg.info" {
			bs, _ := io.ReadAll(tr)
			err := file.Close()
			if err != nil {
				return nil, err
			}
			pkgInfo, err := ReadPackageInfo(string(bs))
			if err != nil {
				return nil, err
			}
			return pkgInfo, nil
		}
	}
	return nil, errors.New("pkg.info not found in archive")
}

func ReadPackageInfo(contents string) (*PackageInfo, error) {
	pkgInfo := PackageInfo{}
	lines := strings.Split(contents, "\n")
	for num, line := range lines {
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}
		split := strings.SplitN(line, ":", 2)
		if len(split) != 2 {
			return nil, errors.New("invalid pkg.info format at line " + strconv.Itoa(num))
		}
		split[0] = strings.Trim(split[0], " ")
		split[1] = strings.Trim(split[1], " ")
		switch split[0] {
		case "name":
			pkgInfo.Name = split[1]
		case "description":
			pkgInfo.Description = split[1]
		case "version":
			pkgInfo.Version = split[1]
		case "type":
			pkgInfo.Type = split[1]
		case "depends":
			pkgInfo.Depends = strings.Split(strings.Replace(split[1], " ", "", -1), ",")
			pkgInfo.Depends = stringSliceRemoveEmpty(pkgInfo.Depends)
		case "provides":
			pkgInfo.Provides = strings.Split(strings.Replace(split[1], " ", "", -1), ",")
			pkgInfo.Provides = stringSliceRemoveEmpty(pkgInfo.Depends)
		}
	}
	return &pkgInfo, nil
}

func CreateInfoFile(pkgInfo PackageInfo) string {
	ret := ""
	ret = ret + "name: " + pkgInfo.Name + "\n"
	ret = ret + "description: " + pkgInfo.Description + "\n"
	ret = ret + "version: " + pkgInfo.Version + "\n"
	ret = ret + "type: " + pkgInfo.Type + "\n"
	if len(pkgInfo.Depends) > 0 {
		ret = ret + "depends (" + strconv.Itoa(len(pkgInfo.Depends)) + "): " + strings.Join(pkgInfo.Depends, ",") + "\n"
	}
	if len(pkgInfo.Provides) > 0 {
		ret = ret + "provides (" + strconv.Itoa(len(pkgInfo.Provides)) + "): " + strings.Join(pkgInfo.Provides, ",") + "\n"
	}
	return ret
}

func InstallPackage(filename, installDir string, force bool) error {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return err
	}
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	tr := tar.NewReader(archive)
	var files []string
	pkgInfo, err := ReadPackage(filename)
	if err != nil {
		return err
	}
	if !force {
		if unresolved := CheckDependencies(pkgInfo, installDir); len(unresolved) != 0 {
			return errors.New("Could not resolve all dependencies. Missing " + strings.Join(unresolved, ", "))
		}
	}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if strings.HasPrefix(header.Name, "files/") && header.Name != "files/" {
			extractFilename := path.Join(installDir, strings.TrimPrefix(header.Name, "files/"))
			switch header.Typeflag {
			case tar.TypeDir:
				files = append(files, strings.TrimPrefix(header.Name, "files/"))
				if err := os.Mkdir(extractFilename, 0755); err != nil && !os.IsExist(err) {
					return err
				}
			case tar.TypeReg:
				outFile, err := os.Create(extractFilename)
				files = append(files, strings.TrimPrefix(header.Name, "files/"))
				if err != nil {
					return err
				}
				if _, err := io.Copy(outFile, tr); err != nil {
					return err
				}
				if err := os.Chmod(extractFilename, header.FileInfo().Mode()); err != nil {
					return err
				}
				err = outFile.Close()
				if err != nil {
					return err
				}
			default:
				return errors.New("ExtractTarGz: uknown type: " + strconv.Itoa(int(header.Typeflag)) + " in " + extractFilename)
			}
		}
	}
	if pkgInfo == nil {
		return errors.New("pkg.info not found in archive")
	}
	slices.Sort(files)
	slices.Reverse(files)

	installedDir := path.Join(installDir, "var/lib/bpm/installed/")
	err = os.MkdirAll(installedDir, 755)
	if err != nil {
		return err
	}
	pkgDir := path.Join(installedDir, pkgInfo.Name)
	err = os.RemoveAll(pkgDir)
	if err != nil {
		return err
	}
	err = os.Mkdir(pkgDir, 755)
	if err != nil {
		return err
	}

	f, err := os.Create(path.Join(pkgDir, "files"))
	if err != nil {
		return err
	}
	for _, line := range files {
		_, err := f.WriteString(line + "\n")
		if err != nil {
			return err
		}
	}
	err = f.Close()
	if err != nil {
		return err
	}

	f, err = os.Create(path.Join(pkgDir, "info"))
	if err != nil {
		return err
	}
	_, err = f.WriteString(CreateInfoFile(*pkgInfo))
	if err != nil {
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}

	err = archive.Close()
	if err != nil {
		return err
	}
	err = file.Close()
	if err != nil {
		return err
	}
	return nil
}

func CheckDependencies(pkgInfo *PackageInfo, rootDir string) []string {
	unresolved := make([]string, len(pkgInfo.Depends))
	copy(unresolved, pkgInfo.Depends)
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	if _, err := os.Stat(installedDir); err != nil {
		return nil
	}
	items, err := os.ReadDir(installedDir)
	if err != nil {
		return nil
	}

	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		if slices.Contains(unresolved, item.Name()) {
			unresolved = stringSliceRemove(unresolved, item.Name())
		}
	}
	return unresolved
}

func IsPackageInstalled(pkg, rootDir string) bool {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	if _, err := os.Stat(pkgDir); err != nil {
		return false
	}
	return true
}

func GetInstalledPackages(rootDir string) ([]string, error) {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	items, err := os.ReadDir(installedDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ret []string
	for _, item := range items {
		ret = append(ret, item.Name())
	}
	return ret, nil
}

func GetPackageFiles(pkg, rootDir string) []string {
	var ret []string
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	files := path.Join(pkgDir, "files")
	if _, err := os.Stat(installedDir); os.IsNotExist(err) {
		return nil
	}
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		return nil
	}
	file, err := os.Open(files)
	if err != nil {
		return nil
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ret = append(ret, scanner.Text())
	}
	return ret
}

func GetPackageInfo(pkg, rootDir string) *PackageInfo {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	files := path.Join(pkgDir, "info")
	if _, err := os.Stat(installedDir); os.IsNotExist(err) {
		return nil
	}
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		return nil
	}
	file, err := os.Open(files)
	if err != nil {
		return nil
	}
	bs, err := io.ReadAll(file)
	if err != nil {
		return nil
	}
	info, err := ReadPackageInfo(string(bs))
	if err != nil {
		return nil
	}
	return info
}

func RemovePackage(pkg, rootDir string) error {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	files := GetPackageFiles(pkg, rootDir)
	for _, file := range files {
		file = path.Join(rootDir, file)
		stat, err := os.Stat(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if stat.IsDir() {
			dir, err := os.ReadDir(file)
			if err != nil {
				return err
			}
			if len(dir) == 0 {
				fmt.Println("Removing: " + file)
				err := os.Remove(file)
				if err != nil {
					return err
				}
			}
		} else {
			fmt.Println("Removing: " + file)
			err := os.Remove(file)
			if err != nil {
				return err
			}
		}
	}
	err := os.RemoveAll(pkgDir)
	if err != nil {
		return err
	}
	fmt.Println("Removing: " + pkgDir)
	return nil
}
