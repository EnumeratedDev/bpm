package bpmlib

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"

	version "github.com/knqyf263/go-rpm-version"
	"gopkg.in/yaml.v3"
)

type BPMPackage struct {
	PkgInfo  *PackageInfo
	PkgFiles []*PackageFileEntry
}

type PackageInfo struct {
	Name            string            `yaml:"name"`
	Description     string            `yaml:"description,omitempty"`
	Version         string            `yaml:"version,omitempty"`
	Revision        int               `yaml:"revision,omitempty"`
	Url             string            `yaml:"url,omitempty"`
	License         string            `yaml:"license,omitempty"`
	Arch            string            `yaml:"architecture,omitempty"`
	OutputArch      string            `yaml:"output_architecture,omitempty"`
	Type            string            `yaml:"type,omitempty"`
	Keep            []string          `yaml:"keep,omitempty"`
	Depends         []string          `yaml:"depends,omitempty"`
	OptionalDepends []string          `yaml:"optional_depends,omitempty"`
	MakeDepends     []string          `yaml:"make_depends,omitempty"`
	Conflicts       []string          `yaml:"conflicts,omitempty"`
	Replaces        []string          `yaml:"replaces,omitempty"`
	Provides        []string          `yaml:"provides,omitempty"`
	Options         []string          `yaml:"options,omitempty"`
	Downloads       []PackageDownload `yaml:"downloads,omitempty"`
	SplitPackages   []*PackageInfo    `yaml:"split_packages,omitempty"`
}

type PackageDownload struct {
	Url      string `yaml:"url"`
	Type     string `yaml:"type,omitempty"`
	Filepath string `yaml:"filepath,omitempty,omitempty"`

	// Archive options
	NoExtract              bool   `yaml:"no_extract,omitempty"`
	ExtractTo              string `yaml:"extract_to,omitempty"`
	ExtractStripComponents int    `yaml:"extract_strip_components,omitempty"`

	// Git options
	CloneTo   string `yaml:"clone_to,omitempty"`
	GitBranch string `yaml:"git_branch,omitempty"`

	Checksum string `yaml:"checksum,omitempty"`
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
	InstallationReasonManual         InstallationReason = "manual"
	InstallationReasonDependency     InstallationReason = "dependency"
	InstallationReasonMakeDependency InstallationReason = "make_dependency"
	InstallationReasonUnknown        InstallationReason = "unknown"
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
	} else if reason == "make_dependency" {
		return InstallationReasonMakeDependency
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
	if err != nil {
		return nil, err
	}
	defer file.Close()

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

func executePackageScript(pkg, rootDir string, verbose bool, packageScript string) error {
	var bpmpkg *BPMPackage
	var err error
	scripts := make(map[string]string)

	// Fetch bpmpkg variable from file or installed package
	if strings.HasSuffix(pkg, ".bpm") {
		bpmpkg, err = ReadPackage(pkg)
		if err != nil {
			return err
		}

		// Read package scripts from tarball
		scripts, err = ReadPackageScripts(pkg)
		if err != nil {
			return err
		}
	} else {
		bpmpkg = GetPackage(pkg, rootDir)
		if bpmpkg == nil {
			return fmt.Errorf("Package not found: %s", pkg)
		}
	}

	// Read installed remove package scripts
	if IsPackageInstalled(bpmpkg.PkgInfo.Name, rootDir) {
		pkgDir := path.Join(rootDir, "var/lib/bpm/installed", bpmpkg.PkgInfo.Name)
		if _, err := os.Stat(path.Join(pkgDir, "pre_remove.sh")); err == nil {
			data, err := os.ReadFile(path.Join(pkgDir, "pre_remove.sh"))
			if err != nil {
				return err
			}
			scripts["pre_remove.sh"] = string(data)
		}
		if _, err := os.Stat(path.Join(pkgDir, "post_remove.sh")); err == nil {
			data, err := os.ReadFile(path.Join(pkgDir, "post_remove.sh"))
			if err != nil {
				return err
			}
			scripts["post_remove.sh"] = string(data)
		}
	}

	// Ensure package script exists
	content, ok := scripts[packageScript]
	if !ok {
		return nil
	}

	cmd := exec.Command("/bin/bash", "-c", content)
	// Setup subprocess environment
	cmd.Dir = "/"
	// Run hook in chroot if using the -R flag
	if rootDir != "/" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: rootDir}
	}
	// Show output if verbose
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	// Setup command environment
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "BPM_ROOT=/") // Setting to "/" for backwards compatibility
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_NAME=%s", bpmpkg.PkgInfo.Name))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DESC=%s", bpmpkg.PkgInfo.Description))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_VERSION=%s", bpmpkg.PkgInfo.Version))
	if IsPackageInstalled(bpmpkg.PkgInfo.Name, rootDir) {
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_OLD_VERSION=%s", GetPackageInfo(bpmpkg.PkgInfo.Name, rootDir).Version))
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_REVISION=%d", bpmpkg.PkgInfo.Revision))
	if IsPackageInstalled(bpmpkg.PkgInfo.Name, rootDir) {
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_OLD_REVISION=%d", GetPackageInfo(bpmpkg.PkgInfo.Name, rootDir).Revision))
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_URL=%s", bpmpkg.PkgInfo.Url))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_ARCH=%s", bpmpkg.PkgInfo.Arch))
	depends := make([]string, len(bpmpkg.PkgInfo.Depends))
	copy(depends, bpmpkg.PkgInfo.Depends)
	for i := 0; i < len(depends); i++ {
		depends[i] = fmt.Sprintf("\"%s\"", depends[i])
	}
	makeDepends := make([]string, len(bpmpkg.PkgInfo.MakeDepends))
	copy(makeDepends, bpmpkg.PkgInfo.MakeDepends)
	for i := 0; i < len(makeDepends); i++ {
		makeDepends[i] = fmt.Sprintf("\"%s\"", makeDepends[i])
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DEPENDS=(%s)", strings.Join(depends, " ")))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_MAKE_DEPENDS=(%s)", strings.Join(makeDepends, " ")))
	cmd.Env = append(cmd.Env, "BPM_PKG_TYPE=source")
	// Run command
	if verbose {
		fmt.Printf("Running package script (%s) for package (%s)\n", packageScript, bpmpkg.PkgInfo.Name)
	}
	err = cmd.Run()
	if err != nil {
		return PackageScriptErr{err: err, packageName: bpmpkg.PkgInfo.Name, packageScript: packageScript}
	}
	return nil
}

func ReadPackageInfo(contents string) (*PackageInfo, error) {
	pkgInfo := &PackageInfo{
		Revision:        1,
		OutputArch:      GetArch(),
		Keep:            make([]string, 0),
		Depends:         make([]string, 0),
		MakeDepends:     make([]string, 0),
		OptionalDepends: make([]string, 0),
		Conflicts:       make([]string, 0),
		Replaces:        make([]string, 0),
		Provides:        make([]string, 0),
		Options:         make([]string, 0),
		Downloads:       make([]PackageDownload, 0),
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

	// Ensure package name is valid
	if match, _ := regexp.MatchString("^[a-zA-Z0-9._-]+$", pkgInfo.Name); !match {
		return nil, fmt.Errorf("package name (%s) is invalid", pkgInfo.Name)
	}

	// Setup split package information
	for i, splitPkg := range pkgInfo.SplitPackages {
		// Ensure split package contains a name
		if splitPkg.Name == "" {
			return nil, fmt.Errorf("package name (%s) is invalid", splitPkg.Name)
		}

		// Ensure split package name is valid
		if match, _ := regexp.MatchString("^[a-zA-Z0-9._-]+$", splitPkg.Name); !match {
			return nil, fmt.Errorf("package name (%s) is invalid", splitPkg.Name)
		}

		// Turn split package into yaml data
		splitPkgYaml, err := yaml.Marshal(splitPkg)
		if err != nil {
			return nil, err
		}

		// Clone all main package fields onto split package
		*splitPkg = *pkgInfo

		// Set split package field of split package to nil
		splitPkg.SplitPackages = nil

		// Unmarshal json data back to struct
		err = yaml.Unmarshal(splitPkgYaml, splitPkg)
		if err != nil {
			return nil, err
		}

		// Force set split package version, revision
		splitPkg.Version = pkgInfo.Version
		splitPkg.Revision = pkgInfo.Revision

		pkgInfo.SplitPackages[i] = splitPkg
	}

	return pkgInfo, nil
}

func (bpmpkg *BPMPackage) CreateReadableInfo(rootDir string, humanReadableSize bool) string {
	ret := make([]string, 0)
	appendArray := func(label string, array []string) {
		if len(array) == 0 {
			return
		}

		// Sort array
		slices.Sort(array)

		ret = append(ret, fmt.Sprintf("%s: %s", label, strings.Join(array, ", ")))
	}

	ret = append(ret, "Name: "+bpmpkg.PkgInfo.Name)
	ret = append(ret, "Description: "+bpmpkg.PkgInfo.Description)
	ret = append(ret, "Version: "+bpmpkg.PkgInfo.GetFullVersion())
	if bpmpkg.PkgInfo.Url != "" {
		ret = append(ret, "URL: "+bpmpkg.PkgInfo.Url)
	}
	if bpmpkg.PkgInfo.License != "" {
		ret = append(ret, "License: "+bpmpkg.PkgInfo.License)
	}
	ret = append(ret, "Architecture: "+bpmpkg.PkgInfo.Arch)
	if bpmpkg.PkgInfo.Type == "source" && bpmpkg.PkgInfo.OutputArch != "" && bpmpkg.PkgInfo.OutputArch != GetArch() {
		ret = append(ret, "Output architecture: "+bpmpkg.PkgInfo.Arch)
	}
	ret = append(ret, "Type: "+bpmpkg.PkgInfo.Type)
	appendArray("Dependencies", bpmpkg.PkgInfo.Depends)
	if bpmpkg.PkgInfo.Type == "source" {
		appendArray("Make Dependencies", bpmpkg.PkgInfo.MakeDepends)
	}
	appendArray("Optional dependencies", bpmpkg.PkgInfo.OptionalDepends)
	dependants := bpmpkg.PkgInfo.GetPackageDependants(rootDir)
	if len(dependants) > 0 {
		appendArray("Dependant packages", dependants)
	}
	appendArray("Conflicting packages", bpmpkg.PkgInfo.Conflicts)
	appendArray("Provided packages", bpmpkg.PkgInfo.Provides)
	appendArray("Replaces packages", bpmpkg.PkgInfo.Replaces)

	if bpmpkg.PkgInfo.Type == "source" && len(bpmpkg.PkgInfo.SplitPackages) != 0 {
		splitPkgs := make([]string, len(bpmpkg.PkgInfo.SplitPackages))
		for i, splitPkgInfo := range bpmpkg.PkgInfo.SplitPackages {
			splitPkgs[i] = splitPkgInfo.Name
		}
		appendArray("Split Packages", splitPkgs)
	}

	if rootDir != "" && IsPackageInstalled(bpmpkg.PkgInfo.Name, rootDir) {
		installationReason := GetInstallationReason(bpmpkg.PkgInfo.Name, rootDir)
		var installationReasonString string
		switch installationReason {
		case InstallationReasonManual:
			installationReasonString = "Manual"
		case InstallationReasonDependency:
			installationReasonString = "Dependency"
		case InstallationReasonMakeDependency:
			installationReasonString = "Make dependency"
		default:
			installationReasonString = "Unknown"
		}
		ret = append(ret, "Installation Reason: "+installationReasonString)
	}
	if bpmpkg.PkgInfo.Type == "binary" {
		installedSize := int64(bpmpkg.GetInstalledSize())
		var installedSizeStr string
		if humanReadableSize {
			installedSizeStr = bytesToHumanReadable(installedSize)
		} else {
			installedSizeStr = strconv.FormatInt(installedSize, 10)
		}
		ret = append(ret, "Installed size: "+installedSizeStr)
	}
	return strings.Join(ret, "\n")
}

func extractPackage(bpmpkg *BPMPackage, verbose bool, filename, rootDir string) error {
	if !IsPackageInstalled(bpmpkg.PkgInfo.Name, rootDir) {
		err := executePackageScript(filename, rootDir, verbose, "pre_install.sh")
		if err != nil {
			log.Printf("Warning: %s\n", err)
		}
	} else {
		err := executePackageScript(filename, rootDir, verbose, "pre_update.sh")
		if err != nil {
			log.Printf("Warning: %s\n", err)
		}
	}
	seenHardlinks := make(map[string]string)
	file, err := os.Open(filename)
	if err != nil {
		return err
	}

	// Initialize progress bar
	bar := createProgressBar(int64(bpmpkg.GetInstalledSize()), "Installing "+bpmpkg.PkgInfo.Name, verbose)
	defer bar.Close()

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
			if _, err := os.Stat(extractFilename); err == nil {
				if verbose {
					fmt.Printf("Skipping Directory: %s (Directory already exists)\n", extractFilename)
				}
				continue
			}

			if err := os.Mkdir(extractFilename, 0755); err != nil && !os.IsExist(err) {
				return err
			}

			err = os.Chown(extractFilename, header.Uid, header.Gid)
			if err != nil {
				return err
			}

			// Using syscall instead of os.Chmod because it seems to strip the setuid, setgid and sticky bits
			err := syscall.Chmod(extractFilename, uint32(header.Mode))
			if err != nil {
				return err
			}

			if verbose {
				fmt.Printf("Created directory %s (%o)\n", extractFilename, header.Mode)
			}
			bar.Add64(header.Size)
		case tar.TypeReg:
			skip := false
			if _, err := os.Stat(extractFilename); err == nil {
				for _, k := range bpmpkg.PkgInfo.Keep {
					if strings.HasSuffix(k, "/") {
						if strings.HasPrefix(header.Name, k) {
							if verbose {
								fmt.Printf("Skipping File: %s (Containing directory is set to be kept during reinstalls/updates)\n", extractFilename)
							}
							skip = true
							continue
						}
					} else {
						if header.Name == k {
							if verbose {
								fmt.Printf("Skipping File: %s (File is configured to be kept during reinstalls/updates)\n", extractFilename)
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
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, packageFilesReader); err != nil {
				return err
			}
			err = outFile.Close()
			if err != nil {
				return err
			}

			err = os.Chown(extractFilename, header.Uid, header.Gid)
			if err != nil {
				return err
			}

			// Using syscall instead of os.Chmod because it seems to strip the setuid, setgid and sticky bits
			err = syscall.Chmod(extractFilename, uint32(header.Mode))
			if err != nil {
				return err
			}

			if verbose {
				fmt.Printf("Created File: %s (%o)\n", extractFilename, header.Mode)
			}
			bar.Add64(header.Size)
		case tar.TypeSymlink:
			err := os.Remove(extractFilename)
			if err != nil && !os.IsNotExist(err) {
				return err
			}

			err = os.Symlink(header.Linkname, extractFilename)
			if err != nil {
				return err
			}

			if verbose {
				fmt.Println("Created Symlink: " + extractFilename + " -> " + header.Linkname)
			}
			bar.Add64(header.Size)
		case tar.TypeLink:
			if verbose {
				fmt.Println("Detected Hard Link: " + extractFilename + " -> " + path.Join(rootDir, strings.TrimPrefix(header.Linkname, "files/")))
			}
			seenHardlinks[extractFilename] = path.Join(strings.TrimPrefix(header.Linkname, "files/"))
			err := os.Remove(extractFilename)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			bar.Add64(header.Size)
		default:
			return errors.New("unknown type (" + strconv.Itoa(int(header.Typeflag)) + ") in " + extractFilename)
		}
	}
	for extractFilename, destination := range seenHardlinks {
		err := os.Link(path.Join(rootDir, destination), extractFilename)
		if err != nil {
			return err
		}

		if verbose {
			fmt.Println("Created Hard Link: " + extractFilename + " -> " + path.Join(rootDir, destination))
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
		err = executePackageScript(filename, rootDir, verbose, "post_install.sh")
		if err != nil {
			log.Printf("Warning: %s\n", err)
		}
	} else {
		err = executePackageScript(filename, rootDir, verbose, "post_update.sh")
		if err != nil {
			log.Printf("Warning: %s\n", err)
		}
	}

	// Ensure local package information has been initialized for rootDir
	err = initializeLocalPackageInformation(rootDir)
	if err != nil {
		return err
	}

	// Add or update package information for rootDir
	localPackageInformation[rootDir][bpmpkg.PkgInfo.Name] = bpmpkg

	return nil
}

func removePackage(pkg string, verbose bool, rootDir string) error {
	pkgDir := path.Join("/var/lib/bpm/installed/", pkg)
	pkgInfo := GetPackageInfo(pkg, rootDir)
	if pkgInfo == nil {
		return errors.New("could not get package info")
	}

	// Executing pre_remove script
	err := executePackageScript(pkg, rootDir, verbose, "pre_remove.sh")
	if err != nil {
		log.Printf("Warning: %s\n", err)
	}

	// Get BPM package
	bpmpkg := GetPackage(pkg, rootDir)

	// Fetching and reversing package file entry list
	fileEntries := bpmpkg.PkgFiles
	sort.Slice(fileEntries, func(i, j int) bool {
		return fileEntries[i].Path < fileEntries[j].Path
	})
	slices.Reverse(fileEntries)
	files, err := GetAllPackageFiles(rootDir, pkg)
	if err != nil {
		return err
	}

	bar := createProgressBar(int64(bpmpkg.GetInstalledSize()), "Removing "+bpmpkg.PkgInfo.Name, verbose)
	defer bar.Close()

	// Removing package files
	for _, entry := range fileEntries {
		bar.Add64(int64(entry.SizeInBytes))
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
	err = executePackageScript(pkg, rootDir, verbose, "post_remove.sh")
	if err != nil {
		log.Printf("Warning: %s\n", err)
	}

	// Removing package directory
	if verbose {
		fmt.Println("Removing: " + path.Join(rootDir, pkgDir))
	}
	err = os.RemoveAll(path.Join(rootDir, pkgDir))
	if err != nil {
		return err
	}

	// Ensure local package information has been initialized for rootDir
	err = initializeLocalPackageInformation(rootDir)
	if err != nil {
		return err
	}

	// Add or update package information for rootDir
	delete(localPackageInformation[rootDir], pkgInfo.Name)

	return nil
}
