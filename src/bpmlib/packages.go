package bpmlib

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	version "github.com/knqyf263/go-rpm-version"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type BPMPackage struct {
	PkgInfo  *PackageInfo
	PkgFiles []*PackageFileEntry
}

type PackageInfo struct {
	Name            string         `yaml:"name,omitempty"`
	Description     string         `yaml:"description,omitempty"`
	Version         string         `yaml:"version,omitempty"`
	Revision        int            `yaml:"revision,omitempty"`
	Url             string         `yaml:"url,omitempty"`
	License         string         `yaml:"license,omitempty"`
	Arch            string         `yaml:"architecture,omitempty"`
	Type            string         `yaml:"type,omitempty"`
	Keep            []string       `yaml:"keep,omitempty"`
	Depends         []string       `yaml:"depends,omitempty"`
	MakeDepends     []string       `yaml:"make_depends,omitempty"`
	OptionalDepends []string       `yaml:"optional_depends,omitempty"`
	Conflicts       []string       `yaml:"conflicts,omitempty"`
	Replaces        []string       `yaml:"replaces,omitempty"`
	Provides        []string       `yaml:"provides,omitempty"`
	SplitPackages   []*PackageInfo `yaml:"split_packages,omitempty"`
}

type PackageFileEntry struct {
	Path        string
	OctalPerms  uint32
	UserID      int
	GroupID     int
	SizeInBytes uint64
}

func (pkg *BPMPackage) GetInstalledSize() uint64 {
	var totalSize uint64 = 0
	for _, entry := range pkg.PkgFiles {
		totalSize += entry.SizeInBytes
	}
	return totalSize
}

func (pkg *BPMPackage) ConvertFilesToString() string {
	str := ""
	for _, file := range pkg.PkgFiles {
		str += fmt.Sprintf("%s %d %d %d\n", file.Path, file.UserID, file.GroupID, file.SizeInBytes)
	}
	return str
}

func (pkgInfo *PackageInfo) GetFullVersion() string {
	return pkgInfo.Version + "-" + strconv.Itoa(pkgInfo.Revision)
}

func (pkgInfo *PackageInfo) IsSplitPackage() bool {
	// Return false if not a source package
	if pkgInfo.Type != "source" {
		return false
	}

	return len(pkgInfo.SplitPackages) > 0
}

func (pkgInfo *PackageInfo) GetSplitPackageInfo(splitPkg string) *PackageInfo {
	for _, splitPkgInfo := range pkgInfo.SplitPackages {
		if splitPkgInfo.Name == splitPkg {
			return splitPkgInfo
		}
	}

	return nil
}

type InstallationReason string

const (
	InstallationReasonManual     InstallationReason = "manual"
	InstallationReasonDependency InstallationReason = "dependency"
	InstallationReasonUnknown    InstallationReason = "unknown"
)

func ComparePackageVersions(info1, info2 PackageInfo) int {
	v1 := version.NewVersion(info1.GetFullVersion())
	v2 := version.NewVersion(info2.GetFullVersion())

	return v1.Compare(v2)
}

func GetInstallationReason(pkg, rootDir string) InstallationReason {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	if stat, err := os.Stat(path.Join(pkgDir, "installation_reason")); err != nil || stat.IsDir() {
		return InstallationReasonManual
	}
	b, err := os.ReadFile(path.Join(pkgDir, "installation_reason"))
	if err != nil {
		return InstallationReasonUnknown
	}
	reason := strings.TrimSpace(string(b))
	if reason == "manual" {
		return InstallationReasonManual
	} else if reason == "dependency" {
		return InstallationReasonDependency
	}
	return InstallationReasonUnknown
}

func SetInstallationReason(pkg string, reason InstallationReason, rootDir string) error {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	err := os.WriteFile(path.Join(pkgDir, "installation_reason"), []byte(reason), 0644)
	if err != nil {
		return err
	}
	return nil
}

func GetPackageInfoRaw(filename string) (string, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return "", err
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}

	tr := tar.NewReader(file)
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

func ReadPackage(filename string) (*BPMPackage, error) {
	var pkgInfo *PackageInfo
	var pkgFiles []*PackageFileEntry

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, err
	}

	file, err := os.Open(filename)
	defer file.Close()
	if err != nil {
		return nil, err
	}

	tr := tar.NewReader(file)
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
			pkgInfo, err = ReadPackageInfo(string(bs))
			if err != nil {
				return nil, err
			}
		} else if header.Name == "pkg.files" {
			bs, _ := io.ReadAll(tr)
			for _, line := range strings.Split(string(bs), "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				stringEntry := strings.Split(strings.TrimSpace(line), " ")
				if len(stringEntry) < 5 {
					return nil, errors.New("pkg.files is not formatted correctly")
				}
				octalPerms, err := strconv.ParseUint(stringEntry[len(stringEntry)-4], 8, 32)
				if err != nil {
					return nil, err
				}
				uid, err := strconv.ParseInt(stringEntry[len(stringEntry)-3], 0, 32)
				if err != nil {
					return nil, err
				}
				gid, err := strconv.ParseInt(stringEntry[len(stringEntry)-2], 0, 32)
				if err != nil {
					return nil, err
				}
				size, err := strconv.ParseUint(stringEntry[len(stringEntry)-1], 0, 64)
				if err != nil {
					return nil, err
				}
				pkgFiles = append(pkgFiles, &PackageFileEntry{
					Path:        strings.Join(stringEntry[:len(stringEntry)-4], " "),
					OctalPerms:  uint32(octalPerms),
					UserID:      int(uid),
					GroupID:     int(gid),
					SizeInBytes: size,
				})
			}
		}
	}

	if pkgInfo == nil {
		return nil, errors.New("pkg.info not found in archive")
	}
	return &BPMPackage{
		PkgInfo:  pkgInfo,
		PkgFiles: pkgFiles,
	}, nil
}

func getPackageScripts(filename string) (packageScripts []string) {
	content, err := listTarballContent(filename)
	if err != nil {
		return
	}

	for _, file := range content {
		if file == "pre_install.sh" {
			packageScripts = append(packageScripts, "pre_install.sh")
		} else if file == "post_install.sh" {
			packageScripts = append(packageScripts, "post_install.sh")
		} else if file == "pre_update.sh" {
			packageScripts = append(packageScripts, "pre_update.sh")
		} else if file == "post_update.sh" {
			packageScripts = append(packageScripts, "post_update.sh")
		} else if file == "pre_remove.sh" {
			packageScripts = append(packageScripts, "pre_remove.sh")
		} else if file == "post_remove.sh" {
			packageScripts = append(packageScripts, "post_remove.sh")
		}
	}

	return packageScripts
}

func ReadPackageScripts(filename string) (map[string]string, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	tr := tar.NewReader(file)
	ret := make(map[string]string)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Name == "pre_install.sh" || header.Name == "post_install.sh" || header.Name == "pre_update.sh" || header.Name == "post_update.sh" || header.Name == "pre_remove.sh" || header.Name == "post_remove.sh" {
			bs, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			ret[header.Name] = string(bs)
		}
	}

	err = file.Close()
	if err != nil {
		return nil, err
	}
	return ret, nil
}

type packageOperation uint8

const (
	packageOperationInstall packageOperation = 0
	packageOperationUpdate                   = 1
	packageOperationRemove                   = 2
)

func executePackageScripts(filename, rootDir string, operation packageOperation, postOperation bool) error {
	pkgInfo, err := ReadPackage(filename)
	if err != nil {
		return err
	}
	scripts, err := ReadPackageScripts(filename)
	if err != nil {
		return err
	}

	run := func(name, content string) error {
		temp, err := os.CreateTemp("", name)
		if err != nil {
			return err
		}
		_, err = temp.WriteString(content)
		if err != nil {
			return err
		}

		cmd := exec.Command("/bin/bash", temp.Name())
		cmd.Dir = rootDir
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_ROOT=%s", rootDir))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_NAME=%s", pkgInfo.PkgInfo.Name))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DESC=%s", pkgInfo.PkgInfo.Description))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_VERSION=%s", pkgInfo.PkgInfo.Version))
		if operation != packageOperationInstall {
			cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_OLD_VERSION=%s", GetPackageInfo(pkgInfo.PkgInfo.Name, rootDir).Version))
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_REVISION=%d", pkgInfo.PkgInfo.Revision))
		if operation != packageOperationInstall {
			cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_OLD_REVISION=%d", GetPackageInfo(pkgInfo.PkgInfo.Name, rootDir).Revision))
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_URL=%s", pkgInfo.PkgInfo.Url))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_ARCH=%s", pkgInfo.PkgInfo.Arch))
		depends := make([]string, len(pkgInfo.PkgInfo.Depends))
		copy(depends, pkgInfo.PkgInfo.Depends)
		for i := 0; i < len(depends); i++ {
			depends[i] = fmt.Sprintf("\"%s\"", depends[i])
		}
		makeDepends := make([]string, len(pkgInfo.PkgInfo.MakeDepends))
		copy(makeDepends, pkgInfo.PkgInfo.MakeDepends)
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
		return nil
	}

	if operation == packageOperationInstall {
		if val, ok := scripts["pre_install.sh"]; !postOperation && ok {
			err := run("pre_install.sh", val)
			if err != nil {
				return err
			}
		}
		if val, ok := scripts["post_install.sh"]; postOperation && ok {
			err := run("post_install.sh", val)
			if err != nil {
				return err
			}
		}
	} else if operation == packageOperationUpdate {
		if val, ok := scripts["pre_update.sh"]; !postOperation && ok {
			err := run("pre_update.sh", val)
			if err != nil {
				return err
			}
		}
		if val, ok := scripts["post_update.sh"]; postOperation && ok {
			err := run("post_update.sh", val)
			if err != nil {
				return err
			}
		}
	} else if operation == packageOperationRemove {
		if val, ok := scripts["pre_remove.sh"]; !postOperation && ok {
			err := run("pre_remove.sh", val)
			if err != nil {
				return err
			}
		}
		if val, ok := scripts["post_remove.sh"]; postOperation && ok {
			err := run("post_remove.sh", val)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func ReadPackageInfo(contents string) (*PackageInfo, error) {
	pkgInfo := &PackageInfo{
		Name:            "",
		Description:     "",
		Version:         "",
		Revision:        1,
		Url:             "",
		License:         "",
		Arch:            "",
		Type:            "",
		Keep:            make([]string, 0),
		Depends:         make([]string, 0),
		MakeDepends:     make([]string, 0),
		OptionalDepends: make([]string, 0),
		Conflicts:       make([]string, 0),
		Replaces:        make([]string, 0),
		Provides:        make([]string, 0),
		SplitPackages:   make([]*PackageInfo, 0),
	}
	err := yaml.Unmarshal([]byte(contents), &pkgInfo)
	if err != nil {
		return nil, err
	}

	// Ensure required fields are set properly
	if pkgInfo.Name == "" {
		return nil, errors.New("this package contains no name")
	} else if pkgInfo.Description == "" {
		return nil, errors.New("this package contains no description")
	} else if pkgInfo.Version == "" {
		return nil, errors.New("this package contains no version")
	} else if pkgInfo.Revision <= 0 {
		return nil, errors.New("this package contains a revision number less or equal to 0")
	} else if pkgInfo.Arch == "" {
		return nil, errors.New("this package contains no architecture")
	} else if pkgInfo.Type == "" {
		return nil, errors.New("this package contains no type")
	}
	for _, val := range pkgInfo.Keep {
		if strings.HasPrefix(val, "/") {
			return nil, fmt.Errorf("cannot keep file (%s) after update because it starts with a slash", val)
		}
	}

	// Setup split package information
	for i, splitPkg := range pkgInfo.SplitPackages {
		// Ensure split package contains a name
		if splitPkg.Name == "" {
			return nil, fmt.Errorf("invalid split package name: %s", splitPkg.Name)
		}

		// Turn split package into json data
		splitPkgJson, err := yaml.Marshal(splitPkg)
		if err != nil {
			return nil, err
		}

		// Clone all main package fields onto split package
		pkgInfoClone := *pkgInfo
		pkgInfo.SplitPackages[i] = &pkgInfoClone

		// Set make depends and split package field of split package to nil
		pkgInfo.SplitPackages[i].MakeDepends = nil
		pkgInfo.SplitPackages[i].SplitPackages = nil

		// Unmarshal json data back to struct
		err = yaml.Unmarshal(splitPkgJson, &pkgInfo.SplitPackages[i])
		if err != nil {
			return nil, err
		}

		// Force set split package version, revision
		pkgInfo.SplitPackages[i].Version = pkgInfo.Version
		pkgInfo.SplitPackages[i].Revision = pkgInfo.Revision
	}

	return pkgInfo, nil
}

func CreateReadableInfo(showArchitecture, showType, showPackageRelations bool, pkgInfo *PackageInfo, rootDir string) string {
	ret := make([]string, 0)
	appendArray := func(label string, array []string) {
		if len(array) == 0 {
			return
		}
		ret = append(ret, fmt.Sprintf("%s: %s", label, strings.Join(array, ", ")))
	}
	ret = append(ret, "Name: "+pkgInfo.Name)
	ret = append(ret, "Description: "+pkgInfo.Description)
	ret = append(ret, "Version: "+pkgInfo.GetFullVersion())
	ret = append(ret, "URL: "+pkgInfo.Url)
	ret = append(ret, "License: "+pkgInfo.License)
	if showArchitecture {
		ret = append(ret, "Architecture: "+pkgInfo.Arch)
	}
	if showType {
		ret = append(ret, "Type: "+pkgInfo.Type)
	}
	if showPackageRelations {
		appendArray("Dependencies", pkgInfo.Depends)
		if pkgInfo.Type == "source" {
			appendArray("Make Dependencies", pkgInfo.MakeDepends)
		}
		appendArray("Optional dependencies", pkgInfo.OptionalDepends)
		dependants, err := GetPackageDependants(pkgInfo.Name, rootDir)
		if err == nil {
			appendArray("Dependant packages", dependants)
		}
		appendArray("Conflicting packages", pkgInfo.Conflicts)
		appendArray("Provided packages", pkgInfo.Provides)
		appendArray("Replaces packages", pkgInfo.Replaces)
	}
	if pkgInfo.Type == "source" && len(pkgInfo.SplitPackages) != 0 {
		splitPkgs := make([]string, len(pkgInfo.SplitPackages))
		for i, splitPkgInfo := range pkgInfo.SplitPackages {
			splitPkgs[i] = splitPkgInfo.Name
		}
		appendArray("Split Packages", splitPkgs)
	}
	ret = append(ret, "Installation Reason: "+string(GetInstallationReason(pkgInfo.Name, rootDir)))
	return strings.Join(ret, "\n")
}

func extractPackage(bpmpkg *BPMPackage, verbose bool, filename, rootDir string) error {
	if !IsPackageInstalled(bpmpkg.PkgInfo.Name, rootDir) {
		err := executePackageScripts(filename, rootDir, packageOperationInstall, false)
		if err != nil {
			return err
		}
	} else {
		err := executePackageScripts(filename, rootDir, packageOperationUpdate, false)
		if err != nil {
			return err
		}
	}
	seenHardlinks := make(map[string]string)
	file, err := os.Open(filename)
	if err != nil {
		return err
	}

	tarballFile, err := readTarballFile(filename, "files.tar.gz")
	if err != nil {
		return err
	}
	defer tarballFile.file.Close()

	archive, err := gzip.NewReader(tarballFile.tarReader)
	if err != nil {
		return err
	}
	packageFilesReader := tar.NewReader(archive)
	for {
		header, err := packageFilesReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		extractFilename := path.Join(rootDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.Mkdir(extractFilename, 0755); err != nil {
				if !os.IsExist(err) {
					return err
				}
			} else {
				if verbose {
					fmt.Println("Creating Directory: " + extractFilename)
				}
			}
		case tar.TypeReg:
			skip := false
			if _, err := os.Stat(extractFilename); err == nil {
				for _, k := range bpmpkg.PkgInfo.Keep {
					if strings.HasSuffix(k, "/") {
						if strings.HasPrefix(header.Name, k) {
							if verbose {
								fmt.Println("Skipping File: " + extractFilename + " (Containing directory is set to be kept during reinstalls/updates)")
							}
							skip = true
							continue
						}
					} else {
						if header.Name == k {
							if verbose {
								fmt.Println("Skipping File: " + extractFilename + " (File is configured to be kept during reinstalls/updates)")
							}
							skip = true
							continue
						}
					}
				}
			}
			if skip {
				continue
			}
			err := os.Remove(extractFilename)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			outFile, err := os.Create(extractFilename)
			if verbose {
				fmt.Println("Creating File: " + extractFilename)
			}
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, packageFilesReader); err != nil {
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
			if verbose {
				fmt.Println("Creating Symlink: " + extractFilename + " -> " + header.Linkname)
			}
			err := os.Remove(extractFilename)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			err = os.Symlink(header.Linkname, extractFilename)
			if err != nil {
				return err
			}
		case tar.TypeLink:
			if verbose {
				fmt.Println("Detected Hard Link: " + extractFilename + " -> " + path.Join(rootDir, strings.TrimPrefix(header.Linkname, "files/")))
			}
			seenHardlinks[extractFilename] = path.Join(strings.TrimPrefix(header.Linkname, "files/"))
			err := os.Remove(extractFilename)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		default:
			return errors.New("unknown type (" + strconv.Itoa(int(header.Typeflag)) + ") in " + extractFilename)
		}
	}
	for extractFilename, destination := range seenHardlinks {
		if verbose {
			fmt.Println("Creating Hard Link: " + extractFilename + " -> " + path.Join(rootDir, destination))
		}
		err := os.Link(path.Join(rootDir, destination), extractFilename)
		if err != nil {
			return err
		}
	}
	defer archive.Close()
	defer file.Close()
	return nil
}

func installPackage(filename, rootDir string, verbose, force bool) error {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return err
	}
	bpmpkg, err := ReadPackage(filename)
	if err != nil {
		return err
	}

	// Ensure package type is 'binary'
	if bpmpkg.PkgInfo.Type != "binary" {
		return fmt.Errorf("can only extract binary packages")
	}

	packageInstalled := IsPackageInstalled(bpmpkg.PkgInfo.Name, rootDir)
	// Check if package is installed and remove current files
	if packageInstalled {
		// Fetching and reversing package file entry list
		fileEntries := GetPackage(bpmpkg.PkgInfo.Name, rootDir).PkgFiles
		sort.Slice(fileEntries, func(i, j int) bool {
			return fileEntries[i].Path < fileEntries[j].Path
		})
		slices.Reverse(fileEntries)
		files, err := GetAllPackageFiles(rootDir, bpmpkg.PkgInfo.Name)
		if err != nil {
			return err
		}

		// Removing old package files
		if verbose {
			fmt.Printf("Removing old files for package (%s)...\n", bpmpkg.PkgInfo.Name)
		}
		for _, entry := range fileEntries {
			file := path.Join(rootDir, entry.Path)
			stat, err := os.Lstat(file)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if len(files[entry.Path]) != 0 {
				if verbose {
					fmt.Println("Skipping path: " + file + " (Path is managed by multiple packages)")
				}
				continue
			}
			shouldContinue := false
			for _, value := range bpmpkg.PkgInfo.Keep {
				if strings.HasSuffix(value, "/") {
					if strings.HasPrefix(entry.Path, value) || entry.Path == strings.TrimSuffix(value, "/") {
						if verbose {
							fmt.Println("Skipping path: " + file + " (Path is set to be kept during reinstalls/updates)")
						}
						shouldContinue = true
						continue
					}
				} else {
					if entry.Path == value {
						if verbose {
							fmt.Println("Skipping path: " + file + " (Path is set to be kept during reinstalls/updates)")
						}
						shouldContinue = true
						continue
					}
				}
			}
			if shouldContinue {
				continue
			}
			if stat.Mode()&os.ModeSymlink != 0 {
				if verbose {
					fmt.Println("Removing: " + file)
				}
				err := os.Remove(file)
				if err != nil {
					return err
				}
				continue
			}
			if stat.IsDir() {
				dir, err := os.ReadDir(file)
				if err != nil {
					return err
				}
				if len(dir) != 0 {
					if verbose {
						fmt.Println("Skipping non-empty directory: " + file)
					}
					continue
				}
				if verbose {
					fmt.Println("Removing: " + file)
				}
				err = os.Remove(file)
				if err != nil {
					return err
				}
			} else {
				if verbose {
					fmt.Println("Removing: " + file)
				}
				err := os.Remove(file)
				if err != nil {
					return err
				}
			}
		}
	}
	if !force {
		if bpmpkg.PkgInfo.Arch != "any" && bpmpkg.PkgInfo.Arch != GetArch() {
			return errors.New("cannot install a package with a different architecture")
		}
	}

	if verbose {
		fmt.Printf("Extracting files for package (%s)...\n", bpmpkg.PkgInfo.Name)
	}

	// Extract package files into rootDir
	err = extractPackage(bpmpkg, verbose, filename, rootDir)
	if err != nil {
		return err
	}

	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	err = os.MkdirAll(installedDir, 0755)
	if err != nil {
		return err
	}
	pkgDir := path.Join(installedDir, bpmpkg.PkgInfo.Name)

	err = os.MkdirAll(pkgDir, 0755)
	if err != nil {
		return err
	}

	f, err := os.Create(path.Join(pkgDir, "files"))
	if err != nil {
		return err
	}

	tarballFile, err := readTarballFile(filename, "pkg.files")
	if err != nil {
		return err
	}
	defer tarballFile.file.Close()

	_, err = io.Copy(f, tarballFile.tarReader)
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

	// Save remove package scripts
	packageScripts, err := ReadPackageScripts(filename)
	if err != nil {
		return err
	}
	for script, content := range packageScripts {
		if !strings.HasSuffix(script, "_remove.sh") {
			continue
		}

		// Create file
		f, err = os.Create(path.Join(pkgDir, script))
		if err != nil {
			return err
		}

		// Write script contents to file
		_, err = f.WriteString(content)
		if err != nil {
			return err
		}

		// Close file
		f.Close()
	}

	if !packageInstalled {
		err = executePackageScripts(filename, rootDir, packageOperationInstall, true)
		if err != nil {
			return err
		}
	} else {
		err = executePackageScripts(filename, rootDir, packageOperationUpdate, true)
		if err != nil {
			return err
		}
	}
	return nil
}

func removePackage(pkg string, verbose bool, rootDir string) error {
	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
	pkgDir := path.Join(installedDir, pkg)
	pkgInfo := GetPackageInfo(pkg, rootDir)
	if pkgInfo == nil {
		return errors.New("could not get package info")
	}

	// Executing pre_remove script
	if _, err := os.Stat(path.Join(pkgDir, "pre_remove.sh")); err == nil {
		cmd := exec.Command("/bin/bash", path.Join(pkgDir, "pre_remove.sh"))
		cmd.Dir = rootDir
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_ROOT=%s", rootDir))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_NAME=%s", pkgInfo.Name))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_VERSION=%s", pkgInfo.Version))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_REVISION=%d", pkgInfo.Revision))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_URL=%s", pkgInfo.Url))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_ARCH=%s", pkgInfo.Arch))

		err = cmd.Run()
		if err != nil {
			log.Printf("Warning: could not run pre_remove.sh package script: %s", err)
		}
	}

	// Fetching and reversing package file entry list
	fileEntries := GetPackage(pkg, rootDir).PkgFiles
	sort.Slice(fileEntries, func(i, j int) bool {
		return fileEntries[i].Path < fileEntries[j].Path
	})
	slices.Reverse(fileEntries)
	files, err := GetAllPackageFiles(rootDir, pkg)
	if err != nil {
		return err
	}

	// Removing package files
	for _, entry := range fileEntries {
		file := path.Join(rootDir, entry.Path)
		lstat, err := os.Lstat(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if len(files[entry.Path]) != 0 {
			if verbose {
				fmt.Println("Skipping path: " + file + "(Path is managed by multiple packages)")
			}
			continue
		}
		if lstat.Mode()&os.ModeSymlink != 0 {
			if verbose {
				fmt.Println("Removing: " + file)
			}
			err := os.Remove(file)
			if err != nil {
				return err
			}
			continue
		}
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
			if len(dir) != 0 {
				if verbose {
					fmt.Println("Skipping non-empty directory: " + file)
				}
				continue
			}
			if verbose {
				fmt.Println("Removing: " + file)
			}
			err = os.Remove(file)
			if err != nil {
				return err
			}
		} else {
			if verbose {
				fmt.Println("Removing: " + file)
			}
			err := os.Remove(file)
			if err != nil {
				return err
			}
		}
	}

	// Executing post_remove script
	if _, err := os.Stat(path.Join(pkgDir, "post_remove.sh")); err == nil {
		cmd := exec.Command("/bin/bash", path.Join(pkgDir, "post_remove.sh"))
		cmd.Dir = rootDir
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_ROOT=%s", rootDir))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_NAME=%s", pkgInfo.Name))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_VERSION=%s", pkgInfo.Version))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_REVISION=%d", pkgInfo.Revision))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_URL=%s", pkgInfo.Url))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_ARCH=%s", pkgInfo.Arch))

		err = cmd.Run()
		if err != nil {
			log.Printf("Warning: could not run pre_remove.sh package script: %s", err)
		}
	}

	// Removing package directory
	if verbose {
		fmt.Println("Removing: " + pkgDir)
	}
	err = os.RemoveAll(pkgDir)
	if err != nil {
		return err
	}

	return nil
}
