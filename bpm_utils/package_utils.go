package bpm_utils

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type PackageInfo struct {
	Name        string
	Description string
	Version     string
	Url         string
	License     string
	Arch        string
	Type        string
	Depends     []string
	MakeDepends []string
	Provides    []string
}

func GetPackageInfoRaw(filename string) (string, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return "", err
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(archive)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if header.Name == "pkg.info" {
			bs, _ := io.ReadAll(tr)
			err := file.Close()
			if err != nil {
				return "", err
			}
			return string(bs), nil
		}
	}
	return "", errors.New("pkg.info not found in archive")
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
			pkgInfo, err := ReadPackageInfo(string(bs), false)
			if err != nil {
				return nil, err
			}
			return pkgInfo, nil
		}
	}
	return nil, errors.New("pkg.info not found in archive")
}

func ReadPackageInfo(contents string, defaultValues bool) (*PackageInfo, error) {
	pkgInfo := PackageInfo{
		Name:        "",
		Description: "",
		Version:     "",
		Url:         "",
		License:     "",
		Arch:        "",
		Type:        "",
		Depends:     nil,
		MakeDepends: nil,
		Provides:    nil,
	}
	lines := strings.Split(contents, "\n")
	for num, line := range lines {
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}
		if line[0] == '#' {
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
			if strings.Contains(split[1], " ") {
				return nil, errors.New("the " + split[0] + " field cannot contain spaces")
			}
			pkgInfo.Name = split[1]
		case "description":
			pkgInfo.Description = split[1]
		case "version":
			if strings.Contains(split[1], " ") {
				return nil, errors.New("the " + split[0] + " field cannot contain spaces")
			}
			pkgInfo.Version = split[1]
		case "url":
			pkgInfo.Url = split[1]
		case "license":
			pkgInfo.License = split[1]
		case "architecture":
			pkgInfo.Arch = split[1]
		case "type":
			pkgInfo.Type = split[1]
		case "depends":
			pkgInfo.Depends = strings.Split(strings.Replace(split[1], " ", "", -1), ",")
			pkgInfo.Depends = stringSliceRemoveEmpty(pkgInfo.Depends)
		case "make_depends":
			pkgInfo.MakeDepends = strings.Split(strings.Replace(split[1], " ", "", -1), ",")
			pkgInfo.MakeDepends = stringSliceRemoveEmpty(pkgInfo.MakeDepends)
		case "provides":
			pkgInfo.Provides = strings.Split(strings.Replace(split[1], " ", "", -1), ",")
			pkgInfo.Provides = stringSliceRemoveEmpty(pkgInfo.Provides)
		}
	}
	if !defaultValues {
		if pkgInfo.Name == "" {
			return nil, errors.New("this package contains no name")
		} else if pkgInfo.Description == "" {
			return nil, errors.New("this package contains no description")
		} else if pkgInfo.Version == "" {
			return nil, errors.New("this package contains no version")
		} else if pkgInfo.Arch == "" {
			return nil, errors.New("this package contains no architecture")
		} else if pkgInfo.Type == "" {
			return nil, errors.New("this package contains no type")
		}
	}
	return &pkgInfo, nil
}

func CreateInfoFile(pkgInfo PackageInfo) string {
	ret := ""
	ret = ret + "name: " + pkgInfo.Name + "\n"
	ret = ret + "description: " + pkgInfo.Description + "\n"
	ret = ret + "version: " + pkgInfo.Version + "\n"
	if pkgInfo.Url != "" {
		ret = ret + "url: " + pkgInfo.Url + "\n"
	}
	if pkgInfo.License != "" {
		ret = ret + "license: " + pkgInfo.License + "\n"
	}
	ret = ret + "architecture: " + pkgInfo.Arch + "\n"
	ret = ret + "type: " + pkgInfo.Type + "\n"
	if len(pkgInfo.Depends) > 0 {
		ret = ret + "depends (" + strconv.Itoa(len(pkgInfo.Depends)) + "): " + strings.Join(pkgInfo.Depends, ",") + "\n"
	}
	if len(pkgInfo.Provides) > 0 {
		ret = ret + "provides (" + strconv.Itoa(len(pkgInfo.Provides)) + "): " + strings.Join(pkgInfo.Provides, ",") + "\n"
	}
	return ret
}

func InstallPackage(filename, installDir string, force, binaryPkgFromSrc, keepTempDir bool) error {
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
	var oldFiles []string
	var files []string
	pkgInfo, err := ReadPackage(filename)
	if err != nil {
		return err
	}
	if IsPackageInstalled(pkgInfo.Name, installDir) {
		oldFiles = GetPackageFiles(pkgInfo.Name, installDir)
	}
	if !force {
		if pkgInfo.Arch != "any" && pkgInfo.Arch != GetArch() {
			return errors.New("cannot install a package with a different architecture")
		}
		if unresolved := CheckDependencies(pkgInfo, installDir); len(unresolved) != 0 {
			return errors.New("Could not resolve all dependencies. Missing " + strings.Join(unresolved, ", "))
		}
	}

	if pkgInfo.Type == "binary" {
		seenHardlinks := make(map[string]string)
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
					if err := os.Mkdir(extractFilename, 0755); err != nil {
						if !os.IsExist(err) {
							return err
						}
					} else {
						fmt.Println("Creating Directory: " + extractFilename)
					}
				case tar.TypeReg:
					err := os.Remove(extractFilename)
					if err != nil && !os.IsNotExist(err) {
						return err
					}
					outFile, err := os.Create(extractFilename)
					fmt.Println("Creating File: " + extractFilename)
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
				case tar.TypeSymlink:
					fmt.Println("Creating Symlink: " + extractFilename + " -> " + header.Linkname)
					files = append(files, strings.TrimPrefix(header.Name, "files/"))
					err := os.Remove(extractFilename)
					if err != nil && !os.IsNotExist(err) {
						return err
					}
					err = os.Symlink(header.Linkname, extractFilename)
					if err != nil {
						return err
					}
				case tar.TypeLink:
					fmt.Println("Detected Hard Link: " + extractFilename + " -> " + path.Join(installDir, strings.TrimPrefix(header.Linkname, "files/")))
					files = append(files, strings.TrimPrefix(header.Name, "files/"))
					seenHardlinks[extractFilename] = path.Join(strings.TrimPrefix(header.Linkname, "files/"))
					err := os.Remove(extractFilename)
					if err != nil && !os.IsNotExist(err) {
						return err
					}
				default:
					return errors.New("ExtractTarGz: unknown type: " + strconv.Itoa(int(header.Typeflag)) + " in " + extractFilename)
				}
			}
		}
		for extractFilename, destination := range seenHardlinks {
			fmt.Println("Creating Hard Link: " + extractFilename + " -> " + path.Join(installDir, destination))
			err := os.Link(path.Join(installDir, destination), extractFilename)
			if err != nil {
				return err
			}
		}
	} else if pkgInfo.Type == "source" {
		temp := "/var/tmp/bpm_source-" + pkgInfo.Name
		err = os.RemoveAll(temp)
		if err != nil {
			return err
		}
		err = os.Mkdir(temp, 0755)
		fmt.Println("Creating temp directory at: " + temp)
		if err != nil {
			return err
		}
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if strings.HasPrefix(header.Name, "source-files/") && header.Name != "source-files/" {
				extractFilename := path.Join(temp, strings.TrimPrefix(header.Name, "source-files/"))
				switch header.Typeflag {
				case tar.TypeDir:
					if err := os.Mkdir(extractFilename, 0755); err != nil {
						if !os.IsExist(err) {
							return err
						}
					} else {
						fmt.Println("Creating Directory: " + extractFilename)
					}
				case tar.TypeReg:
					err := os.Remove(extractFilename)
					if err != nil && !os.IsNotExist(err) {
						return err
					}
					outFile, err := os.Create(extractFilename)
					fmt.Println("Creating File: " + extractFilename)
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
				case tar.TypeSymlink:
					fmt.Println("Skipping symlink (Bundling symlinks in source packages is not supported)")
				case tar.TypeLink:
					fmt.Println("Skipping hard link (Bundling hard links in source packages is not supported)")
				default:
					return errors.New("ExtractTarGz: unknown type: " + strconv.Itoa(int(header.Typeflag)) + " in " + extractFilename)
				}
			}
			if header.Name == "source.sh" {
				bs, err := io.ReadAll(tr)
				if err != nil {
					return err
				}
				err = os.WriteFile(path.Join(temp, "source.sh"), bs, 0644)
				if err != nil {
					return err
				}
			}
		}
		if _, err := os.Stat(path.Join(temp, "source.sh")); os.IsNotExist(err) {
			return errors.New("source.sh file could not be found in the temporary build directory")
		}
		if err != nil {
			return err
		}
		fmt.Println("Running source.sh file...")
		if err != nil {
			return err
		}
		cmd := exec.Command("/bin/bash", "-e", "source.sh")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = temp
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_NAME=%s", pkgInfo.Name))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DESC=%s", pkgInfo.Description))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_VERSION=%s", pkgInfo.Version))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_URL=%s", pkgInfo.Url))
		depends := make([]string, len(pkgInfo.Depends))
		copy(depends, pkgInfo.Depends)
		for i := 0; i < len(depends); i++ {
			depends[i] = fmt.Sprintf("\"%s\"", depends[i])
		}
		makeDepends := make([]string, len(pkgInfo.MakeDepends))
		copy(makeDepends, pkgInfo.MakeDepends)
		for i := 0; i < len(makeDepends); i++ {
			makeDepends[i] = fmt.Sprintf("\"%s\"", makeDepends[i])
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DEPENDS=(%s)", strings.Join(depends, " ")))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_MAKE_DEPENDS=(%s)", strings.Join(makeDepends, " ")))
		cmd.Env = append(cmd.Env, "BPM_PKG_TYPE=source")
		err = cmd.Run()
		if err != nil {
			return err
		}
		if _, err := os.Stat(path.Join(temp, "/output/")); err != nil {
			if os.IsNotExist(err) {
				return errors.New("Output directory not be found at " + path.Join(temp, "/output/"))
			}
			return err
		}
		fmt.Println("Copying all files...")
		err = filepath.WalkDir(path.Join(temp, "/output/"), func(fullpath string, d fs.DirEntry, err error) error {
			relFilename, err := filepath.Rel(path.Join(temp, "/output/"), fullpath)
			if relFilename == "." {
				return nil
			}
			extractFilename := path.Join(installDir, relFilename)
			if err != nil {
				return err
			}
			if d.Type() == os.ModeDir {
				files = append(files, relFilename+"/")
				if err := os.Mkdir(extractFilename, 0755); err != nil {
					if !os.IsExist(err) {
						return err
					}
				} else {
					fmt.Println("Creating Directory: " + extractFilename)
				}
			} else if d.Type().IsRegular() {
				err := os.Remove(extractFilename)
				if err != nil && !os.IsNotExist(err) {
					return err
				}
				outFile, err := os.Create(extractFilename)
				fmt.Println("Creating File: " + extractFilename)
				files = append(files, relFilename)
				if err != nil {
					return err
				}
				f, err := os.Open(fullpath)
				if err != nil {
					return err
				}
				if _, err := io.Copy(outFile, f); err != nil {
					return err
				}
				info, err := os.Stat(fullpath)
				if err != nil {
					return err
				}
				if err := os.Chmod(extractFilename, info.Mode()); err != nil {
					return err
				}
				err = outFile.Close()
				if err != nil {
					return err
				}
				err = f.Close()
				if err != nil {
					return err
				}
			} else if d.Type() == os.ModeSymlink {
				link, err := os.Readlink(fullpath)
				if err != nil {
					return err
				}
				err = os.Remove(extractFilename)
				if err != nil && !os.IsNotExist(err) {
					return err
				}
				fmt.Println("Creating Symlink: "+extractFilename, " -> "+link)
				files = append(files, relFilename)
				err = os.Symlink(link, extractFilename)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if binaryPkgFromSrc {
			compiledDir := path.Join(installDir, "var/lib/bpm/compiled/")
			err = os.MkdirAll(compiledDir, 755)
			compiledInfo := PackageInfo{}
			compiledInfo = *pkgInfo
			compiledInfo.Type = "binary"
			compiledInfo.Arch = GetArch()
			err = os.WriteFile(path.Join(compiledDir, "pkg.info"), []byte(CreateInfoFile(compiledInfo)), 0644)
			if err != nil {
				return err
			}
			sed := fmt.Sprintf("s/%s/files/", strings.Replace(strings.TrimPrefix(path.Join(temp, "/output/"), "/"), "/", `\/`, -1))
			cmd := exec.Command("/usr/bin/tar", "-czvf", compiledInfo.Name+"-"+compiledInfo.Version+".bpm", "pkg.info", path.Join(temp, "/output/"), "--transform", sed)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Dir = compiledDir
			fmt.Printf("running command: %s %s\n", cmd.Path, strings.Join(cmd.Args, " "))
			err := cmd.Run()
			if err != nil {
				return err
			}
			err = os.Remove(path.Join(compiledDir, "pkg.info"))
			if err != nil {
				return err
			}
		}
		if !keepTempDir {
			err := os.RemoveAll(temp)
			if err != nil {
				return err
			}
		}
		if len(files) == 0 {
			return errors.New("no output files for source package. Cancelling package installation")
		}
	} else {
		return errors.New("Unknown package type: " + pkgInfo.Type)
	}
	slices.Sort(files)
	slices.Reverse(files)

	filesDiff := slices.DeleteFunc(oldFiles, func(f string) bool {
		return slices.Contains(files, f)
	})

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
	raw, err := GetPackageInfoRaw(filename)
	if err != nil {
		return err
	}
	_, err = f.WriteString(raw)
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
	if len(filesDiff) != 0 {
		fmt.Println("Removing obsolete files")
		for _, f := range filesDiff {
			err := os.RemoveAll(path.Join(installDir, f))
			if err != nil {
				return err
			}
			fmt.Println("Removing: " + path.Join(installDir, f))
		}
	}
	return nil
}

func GetSourceScript(filename string) (string, error) {
	pkgInfo, err := ReadPackage(filename)
	if err != nil {
		return "", err
	}
	if pkgInfo.Type != "source" {
		return "", errors.New("package not of source type")
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(archive)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if header.Name == "source.sh" {
			err := archive.Close()
			if err != nil {
				return "", err
			}
			err = file.Close()
			if err != nil {
				return "", err
			}
			bs, err := io.ReadAll(tr)
			if err != nil {
				return "", err
			}
			return string(bs), nil
		}
	}
	return "", errors.New("package does not contain a source.sh file")
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
		_, err := os.Stat(path.Join(installedDir, item.Name(), "/info"))
		if err != nil {
			return nil
		}
		bs, err := os.ReadFile(path.Join(installedDir, item.Name(), "/info"))
		if err != nil {
			return nil
		}
		info, err := ReadPackageInfo(string(bs), false)
		if err != nil {
			return nil
		}
		if slices.Contains(unresolved, info.Name) {
			unresolved = stringSliceRemove(unresolved, info.Name)
		}
		for _, prov := range info.Provides {
			if slices.Contains(unresolved, prov) {
				unresolved = stringSliceRemove(unresolved, prov)
			}
		}
	}
	return unresolved
}

func CheckMakeDependencies(pkgInfo *PackageInfo, rootDir string) []string {
	unresolved := make([]string, len(pkgInfo.MakeDepends))
	copy(unresolved, pkgInfo.MakeDepends)
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
		_, err := os.Stat(path.Join(installedDir, item.Name(), "/info"))
		if err != nil {
			return nil
		}
		bs, err := os.ReadFile(path.Join(installedDir, item.Name(), "/info"))
		if err != nil {
			return nil
		}
		info, err := ReadPackageInfo(string(bs), false)
		if err != nil {
			return nil
		}
		if slices.Contains(unresolved, info.Name) {
			unresolved = stringSliceRemove(unresolved, info.Name)
		}
		for _, prov := range info.Provides {
			if slices.Contains(unresolved, prov) {
				unresolved = stringSliceRemove(unresolved, prov)
			}
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

func GetPackageInfo(pkg, rootDir string, defaultValues bool) *PackageInfo {
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
	info, err := ReadPackageInfo(string(bs), defaultValues)
	if err != nil {
		return nil
	}
	return info
}

func setPackageInfo(pkg, contents, rootDir string) error {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	info := path.Join(pkgDir, "info")
	if _, err := os.Stat(installedDir); os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		return err
	}
	bs := []byte(contents)
	err := os.WriteFile(info, bs, 0644)
	if err != nil {
		return err
	}
	return nil
}

func RemovePackage(pkg, rootDir string) error {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	files := GetPackageFiles(pkg, rootDir)
	for _, file := range files {
		file = path.Join(rootDir, file)
		stat, err := os.Lstat(file)
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
