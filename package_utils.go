package main

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

type packageInfo struct {
	name        string
	description string
	version     string
	pkgType     string
	depends     []string
	provides    []string
}

func readPackage(filename string) (*packageInfo, error) {
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
			pkgInfo, err := readPackageInfo(string(bs))
			if err != nil {
				return nil, err
			}
			return pkgInfo, nil
		}
	}
	return nil, errors.New("pkg.info not found in archive")
}

func readPackageInfo(contents string) (*packageInfo, error) {
	pkgInfo := packageInfo{}
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
			pkgInfo.name = split[1]
		case "description":
			pkgInfo.description = split[1]
		case "version":
			pkgInfo.version = split[1]
		case "type":
			pkgInfo.pkgType = split[1]
		}
	}
	return &pkgInfo, nil
}

func createInfoFile(pkgInfo packageInfo) string {
	ret := ""
	ret = ret + "name: " + pkgInfo.name + "\n"
	ret = ret + "description: " + pkgInfo.description + "\n"
	ret = ret + "version: " + pkgInfo.version + "\n"
	ret = ret + "type: " + pkgInfo.pkgType + "\n"
	return ret
}

func installPackage(filename, installDir string) error {
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
	var pkgInfo *packageInfo
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Name == "pkg.info" {
			bs, _ := io.ReadAll(tr)
			if err != nil {
				return err
			}
			pkgInfo, err = readPackageInfo(string(bs))
			if err != nil {
				return err
			}
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

	dataDir := path.Join(installDir, "var/lib/bpm/installed/")
	err = os.MkdirAll(dataDir, 755)
	if err != nil {
		return err
	}
	pkgDir := path.Join(dataDir, pkgInfo.name)
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
	f.Close()

	f, err = os.Create(path.Join(pkgDir, "info"))
	if err != nil {
		return err
	}
	_, err = f.WriteString(createInfoFile(*pkgInfo))
	if err != nil {
		return err
	}
	f.Close()

	archive.Close()
	file.Close()
	return nil
}

func getInstalledPackages() ([]string, error) {
	dataDir := path.Join(rootDir, "var/lib/bpm/installed/")
	items, err := os.ReadDir(dataDir)
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

func getPackageFiles(pkg string) []string {
	var ret []string
	dataDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(dataDir, pkg)
	files := path.Join(pkgDir, "files")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
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

func getPackageInfo(pkg string) *packageInfo {
	dataDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(dataDir, pkg)
	files := path.Join(pkgDir, "info")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
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
	info, err := readPackageInfo(string(bs))
	if err != nil {
		return nil
	}
	return info
}

func removePackage(pkg string) error {
	dataDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(dataDir, pkg)
	files := getPackageFiles(pkg)
	for _, file := range files {
		file = path.Join(rootDir, file)
		stat, err := os.Stat(file)
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
