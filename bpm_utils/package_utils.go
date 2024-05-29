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
	Keep        []string
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

func ReadPackageScripts(filename string) (map[string]string, error) {
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
	ret := make(map[string]string)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Name == "pre_install.sh" {
			bs, _ := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			ret[header.Name] = string(bs)
		} else if header.Name == "post_install.sh" {
			bs, _ := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			ret[header.Name] = string(bs)
		} else if header.Name == "pre_update.sh" {
			bs, _ := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			ret[header.Name] = string(bs)
		} else if header.Name == "post_update.sh" {
			bs, _ := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			ret[header.Name] = string(bs)
		} else if header.Name == "post_remove.sh" {
			bs, _ := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			ret[header.Name] = string(bs)
		}
	}
	err = archive.Close()
	if err != nil {
		return nil, err
	}
	err = file.Close()
	if err != nil {
		return nil, err
	}
	return ret, nil
}

type Operation uint8

const (
	Install Operation = 0
	Update            = 1
	Remove            = 2
)

func ExecutePackageScripts(filename, rootDir string, operation Operation, postOperation bool) error {
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
		if !BPMConfig.SilentCompilation {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.Dir = rootDir
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_ROOT=%s", rootDir))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_NAME=%s", pkgInfo.Name))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DESC=%s", pkgInfo.Description))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_VERSION=%s", pkgInfo.Version))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_URL=%s", pkgInfo.Url))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_ARCH=%s", pkgInfo.Arch))
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
		return nil
	}

	if operation == Install {
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
	} else if operation == Update {
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
	} else if operation == Remove {
		if val, ok := scripts["post_remove.sh"]; postOperation && ok {
			err := run("post_remove.sh", val)
			if err != nil {
				return err
			}
		}
	}
	return nil
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
		Keep:        nil,
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
		case "keep":
			pkgInfo.Keep = strings.Split(strings.Replace(split[1], " ", "", -1), ",")
			pkgInfo.Keep = stringSliceRemoveEmpty(pkgInfo.Keep)
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

func CreateInfoFile(pkgInfo PackageInfo, keepSourceFields bool) string {
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
	if len(pkgInfo.Keep) > 0 {
		ret = ret + "keep (" + strconv.Itoa(len(pkgInfo.Keep)) + "): " + strings.Join(pkgInfo.Keep, ",") + "\n"
	}
	if len(pkgInfo.Depends) > 0 {
		ret = ret + "depends (" + strconv.Itoa(len(pkgInfo.Depends)) + "): " + strings.Join(pkgInfo.Depends, ",") + "\n"
	}
	if len(pkgInfo.MakeDepends) > 0 && keepSourceFields {
		ret = ret + "make_depends (" + strconv.Itoa(len(pkgInfo.MakeDepends)) + "): " + strings.Join(pkgInfo.MakeDepends, ",") + "\n"
	}
	if len(pkgInfo.Provides) > 0 {
		ret = ret + "provides (" + strconv.Itoa(len(pkgInfo.Provides)) + "): " + strings.Join(pkgInfo.Provides, ",") + "\n"
	}
	return ret
}

func extractPackage(pkgInfo *PackageInfo, filename, rootDir string) (error, []string) {
	var files []string
	if !IsPackageInstalled(pkgInfo.Name, rootDir) {
		err := ExecutePackageScripts(filename, rootDir, Install, false)
		if err != nil {
			return err, nil
		}
	} else {
		err := ExecutePackageScripts(filename, rootDir, Update, false)
		if err != nil {
			return err, nil
		}
	}
	seenHardlinks := make(map[string]string)
	file, err := os.Open(filename)
	if err != nil {
		return err, nil
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return err, nil
	}
	tr := tar.NewReader(archive)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err, nil
		}
		if strings.HasPrefix(header.Name, "files/") && header.Name != "files/" {
			trimmedName := strings.TrimPrefix(header.Name, "files/")
			extractFilename := path.Join(rootDir, trimmedName)
			switch header.Typeflag {
			case tar.TypeDir:
				files = append(files, strings.TrimPrefix(header.Name, "files/"))
				if err := os.Mkdir(extractFilename, 0755); err != nil {
					if !os.IsExist(err) {
						return err, nil
					}
				} else {
					fmt.Println("Creating Directory: " + extractFilename)
				}
			case tar.TypeReg:
				if _, err := os.Stat(extractFilename); err == nil {
					if slices.Contains(pkgInfo.Keep, trimmedName) {
						fmt.Println("Skipping File: " + extractFilename + "(File is configured to be kept during installs/updates)")
						files = append(files, trimmedName)
						continue
					}
				}
				err := os.Remove(extractFilename)
				if err != nil && !os.IsNotExist(err) {
					return err, nil
				}
				outFile, err := os.Create(extractFilename)
				fmt.Println("Creating File: " + extractFilename)
				files = append(files, strings.TrimPrefix(header.Name, "files/"))
				if err != nil {
					return err, nil
				}
				if _, err := io.Copy(outFile, tr); err != nil {
					return err, nil
				}
				if err := os.Chmod(extractFilename, header.FileInfo().Mode()); err != nil {
					return err, nil
				}
				err = outFile.Close()
				if err != nil {
					return err, nil
				}
			case tar.TypeSymlink:
				fmt.Println("Creating Symlink: " + extractFilename + " -> " + header.Linkname)
				files = append(files, strings.TrimPrefix(header.Name, "files/"))
				err := os.Remove(extractFilename)
				if err != nil && !os.IsNotExist(err) {
					return err, nil
				}
				err = os.Symlink(header.Linkname, extractFilename)
				if err != nil {
					return err, nil
				}
			case tar.TypeLink:
				fmt.Println("Detected Hard Link: " + extractFilename + " -> " + path.Join(rootDir, strings.TrimPrefix(header.Linkname, "files/")))
				files = append(files, strings.TrimPrefix(header.Name, "files/"))
				seenHardlinks[extractFilename] = path.Join(strings.TrimPrefix(header.Linkname, "files/"))
				err := os.Remove(extractFilename)
				if err != nil && !os.IsNotExist(err) {
					return err, nil
				}
			default:
				return errors.New("ExtractTarGz: unknown type: " + strconv.Itoa(int(header.Typeflag)) + " in " + extractFilename), nil
			}
		}
	}
	for extractFilename, destination := range seenHardlinks {
		fmt.Println("Creating Hard Link: " + extractFilename + " -> " + path.Join(rootDir, destination))
		err := os.Link(path.Join(rootDir, destination), extractFilename)
		if err != nil {
			return err, nil
		}
	}
	defer archive.Close()
	defer file.Close()
	return nil, files
}

func compilePackage(pkgInfo *PackageInfo, filename, rootDir string, binaryPkgFromSrc, keepTempDir bool) (error, []string) {
	var files []string
	if !IsPackageInstalled(pkgInfo.Name, rootDir) {
		err := ExecutePackageScripts(filename, rootDir, Install, false)
		if err != nil {
			return err, nil
		}
	} else {
		err := ExecutePackageScripts(filename, rootDir, Update, false)
		if err != nil {
			return err, nil
		}
	}
	//seenHardlinks := make(map[string]string)
	file, err := os.Open(filename)
	if err != nil {
		return err, nil
	}
	archive, err := gzip.NewReader(file)
	if err != nil {
		return err, nil
	}
	tr := tar.NewReader(archive)

	temp := "/var/tmp/bpm_source-" + pkgInfo.Name
	err = os.RemoveAll(temp)
	if err != nil {
		return err, nil
	}
	err = os.Mkdir(temp, 0755)
	fmt.Println("Creating temp directory at: " + temp)
	if err != nil {
		return err, nil
	}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err, nil
		}
		if strings.HasPrefix(header.Name, "source-files/") && header.Name != "source-files/" {
			extractFilename := path.Join(temp, strings.TrimPrefix(header.Name, "source-files/"))
			switch header.Typeflag {
			case tar.TypeDir:
				if err := os.Mkdir(extractFilename, 0755); err != nil {
					if !os.IsExist(err) {
						return err, nil
					}
				} else {
					fmt.Println("Creating Directory: " + extractFilename)
				}
			case tar.TypeReg:
				err := os.Remove(extractFilename)
				if err != nil && !os.IsNotExist(err) {
					return err, nil
				}
				outFile, err := os.Create(extractFilename)
				fmt.Println("Creating File: " + extractFilename)
				if err != nil {
					return err, nil
				}
				if _, err := io.Copy(outFile, tr); err != nil {
					return err, nil
				}
				if err := os.Chmod(extractFilename, header.FileInfo().Mode()); err != nil {
					return err, nil
				}
				err = outFile.Close()
				if err != nil {
					return err, nil
				}
			case tar.TypeSymlink:
				fmt.Println("Skipping symlink (Bundling symlinks in source packages is not supported)")
			case tar.TypeLink:
				fmt.Println("Skipping hard link (Bundling hard links in source packages is not supported)")
			default:
				return errors.New("ExtractTarGz: unknown type: " + strconv.Itoa(int(header.Typeflag)) + " in " + extractFilename), nil
			}
		}
		if header.Name == "source.sh" {
			bs, err := io.ReadAll(tr)
			if err != nil {
				return err, nil
			}
			err = os.WriteFile(path.Join(temp, "source.sh"), bs, 0644)
			if err != nil {
				return err, nil
			}
		}
	}
	if _, err := os.Stat(path.Join(temp, "source.sh")); os.IsNotExist(err) {
		return errors.New("source.sh file could not be found in the temporary build directory"), nil
	}
	if err != nil {
		return err, nil
	}
	fmt.Println("Running source.sh file...")
	if !IsPackageInstalled(pkgInfo.Name, rootDir) {
		err = ExecutePackageScripts(filename, rootDir, Install, false)
		if err != nil {
			return err, nil
		}
	} else {
		err = ExecutePackageScripts(filename, rootDir, Update, false)
		if err != nil {
			return err, nil
		}
	}
	bs, err := os.ReadFile(path.Join(temp, "source.sh"))
	if err != nil {
		return err, nil
	}
	hasPackageFunc := false
	compatMode := false
	if strings.Contains(string(bs), "package()") {
		hasPackageFunc = true
	}
	if !hasPackageFunc {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("This package does not seem to have the required 'package' function\nThe source.sh file may have been created for an older BPM version\nWould you like to run the script in compatibility mode (Not Recommended)? [Y\\n] ")
		text, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(text)) != "y" && strings.TrimSpace(strings.ToLower(text)) != "yes" {
			return errors.New("could not find required 'package' function in source.sh"), nil
		}
		compatMode = true
	}

	runScript := `
cd "$BPM_WORKDIR"
source "source.sh"
if [[ $(type -t prepare) == function ]]; then
  echo "Running prepare() function..."
  export -f prepare
  bash -e -c prepare
  if [ $? -ne 0 ]; then
    echo "Failed to run prepare() function in source.sh"
  fi
fi
cd "$BPM_SOURCE"
if [[ $(type -t build) == function ]]; then
  echo "Running build() function..."
  export -f build
  bash -e -c build
  if [ $? -ne 0 ]; then
    echo "Failed to run build() function in source.sh"
  fi
fi
cd "$BPM_SOURCE"
if [[ $(type -t check) == function ]]; then
  echo "Running check() function..."
  export -f check
  bash -e -c check
  if [ $? -ne 0 ]; then
    echo "Failed to run check() function in source.sh"
  fi
fi
if ! [[ $(type -t package) == function ]]; then
  echo "Failed to locate package() function in source.sh"
  exit 1
fi
echo "Running package() function..."
export -f package
bash -e -c package
if [ $? -ne 0 ]; then
  echo "Failed to run package() function in source.sh"
fi
`

	err = os.WriteFile(path.Join(temp, "run.sh"), []byte(runScript), 0644)
	if err != nil {
		return err, nil
	}

	cmd := exec.Command("/bin/bash", "-e", "run.sh")
	cmd.Dir = temp
	cmd.Env = os.Environ()

	if compatMode {
		cmd = exec.Command("/bin/bash", "-e", "source.sh")
		cmd.Dir = temp
		cmd.Env = os.Environ()
	} else {
		err := os.Mkdir(path.Join(temp, "source"), 755)
		if err != nil {
			return err, nil
		}
		err = os.Mkdir(path.Join(temp, "output"), 755)
		if err != nil {
			return err, nil
		}
		cmd.Env = append(cmd.Env, "BPM_WORKDIR="+temp)
		cmd.Env = append(cmd.Env, "BPM_SOURCE="+path.Join(temp, "source"))
		cmd.Env = append(cmd.Env, "BPM_OUTPUT="+path.Join(temp, "output"))
	}

	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_ROOT=%s", rootDir))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_NAME=%s", pkgInfo.Name))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DESC=%s", pkgInfo.Description))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_VERSION=%s", pkgInfo.Version))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_URL=%s", pkgInfo.Url))
	cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_ARCH=%s", pkgInfo.Arch))
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
	for _, value := range BPMConfig.CompilationEnv {
		cmd.Env = append(cmd.Env, value)
	}
	cmd.Env = append(cmd.Env, "BPM_PKG_TYPE=source")

	if !BPMConfig.SilentCompilation {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	err = cmd.Run()
	if err != nil {
		return err, nil
	}
	if _, err := os.Stat(path.Join(temp, "output/")); err != nil {
		if os.IsNotExist(err) {
			return errors.New("output directory not be found at " + path.Join(temp, "output/")), nil
		}
		return err, nil
	}
	if dir, _ := os.ReadDir(path.Join(temp, "output/")); len(dir) == 0 {
		return errors.New("output directory is empty"), nil
	}
	fmt.Println("Copying all files...")
	err = filepath.WalkDir(path.Join(temp, "/output/"), func(fullpath string, d fs.DirEntry, err error) error {
		relFilename, err := filepath.Rel(path.Join(temp, "/output/"), fullpath)
		if relFilename == "." {
			return nil
		}
		extractFilename := path.Join(rootDir, relFilename)
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
			if _, err := os.Stat(extractFilename); err == nil {
				if slices.Contains(pkgInfo.Keep, relFilename) {
					fmt.Println("Skipping File: " + extractFilename + "(File is configured to be kept during installs/updates)")
					files = append(files, relFilename)
					return nil
				}
			}
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
		return err, nil
	}
	if binaryPkgFromSrc {
		compiledDir := path.Join(rootDir, "var/lib/bpm/compiled/")
		err = os.MkdirAll(compiledDir, 755)
		compiledInfo := PackageInfo{}
		compiledInfo = *pkgInfo
		compiledInfo.Type = "binary"
		compiledInfo.Arch = GetArch()
		err = os.WriteFile(path.Join(compiledDir, "pkg.info"), []byte(CreateInfoFile(compiledInfo, false)), 0644)
		if err != nil {
			return err, nil
		}
		scripts, err := ReadPackageScripts(filename)
		for key, val := range scripts {
			err = os.WriteFile(path.Join(compiledDir, key), []byte(val), 0644)
			if err != nil {
				return err, nil
			}
		}
		sed := fmt.Sprintf("s/%s/files/", strings.Replace(strings.TrimPrefix(path.Join(temp, "/output/"), "/"), "/", `\/`, -1))
		cmd := exec.Command("/usr/bin/tar", "-czvf", compiledInfo.Name+"-"+compiledInfo.Version+".bpm", "pkg.info", path.Join(temp, "/output/"), "--transform", sed)
		if !BPMConfig.SilentCompilation {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.Dir = compiledDir
		fmt.Printf("running command: %s %s\n", cmd.Path, strings.Join(cmd.Args, " "))
		err = cmd.Run()
		if err != nil {
			return err, nil
		}
		err = os.Remove(path.Join(compiledDir, "pkg.info"))
		for key := range scripts {
			err = os.Remove(path.Join(compiledDir, key))
			if err != nil {
				return err, nil
			}
		}
		if err != nil {
			return err, nil
		}
	}
	if !keepTempDir {
		err := os.RemoveAll(temp)
		if err != nil {
			return err, nil
		}
	}
	if len(files) == 0 {
		return errors.New("no output files for source package. Cancelling package installation"), nil
	}

	defer archive.Close()
	defer file.Close()
	return nil, files
}

func InstallPackage(filename, rootDir string, force, binaryPkgFromSrc, keepTempDir bool) error {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return err
	}
	var oldFiles []string
	var files []string
	pkgInfo, err := ReadPackage(filename)
	if err != nil {
		return err
	}
	packageInstalled := IsPackageInstalled(pkgInfo.Name, rootDir)
	if packageInstalled {
		oldFiles = GetPackageFiles(pkgInfo.Name, rootDir)
	}
	if !force {
		if pkgInfo.Arch != "any" && pkgInfo.Arch != GetArch() {
			return errors.New("cannot install a package with a different architecture")
		}
		if unresolved := CheckDependencies(pkgInfo, rootDir); len(unresolved) != 0 {
			return errors.New("Could not resolve all dependencies. Missing " + strings.Join(unresolved, ", "))
		}
	}
	if pkgInfo.Type == "binary" {
		err, i := extractPackage(pkgInfo, filename, rootDir)
		if err != nil {
			return err
		}
		files = i
	} else if pkgInfo.Type == "source" {
		err, i := compilePackage(pkgInfo, filename, rootDir, binaryPkgFromSrc, keepTempDir)
		if err != nil {
			return err
		}
		files = i
	} else {
		return errors.New("Unknown package type: " + pkgInfo.Type)
	}
	slices.Sort(files)
	slices.Reverse(files)

	filesDiff := slices.DeleteFunc(oldFiles, func(f string) bool {
		return slices.Contains(files, f)
	})

	installedDir := path.Join(rootDir, "var/lib/bpm/installed/")
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

	scripts, err := ReadPackageScripts(filename)
	if err != nil {
		return err
	}
	if val, ok := scripts["post_remove.sh"]; ok {
		f, err = os.Create(path.Join(pkgDir, "post_remove.sh"))
		if err != nil {
			return err
		}
		_, err = f.WriteString(val)
		if err != nil {
			return err
		}
		err = f.Close()
		if err != nil {
			return err
		}
	}

	if len(filesDiff) != 0 {
		fmt.Println("Removing obsolete files")
		var symlinks []string
		for _, f := range filesDiff {
			f = path.Join(rootDir, f)
			lstat, err := os.Lstat(f)
			if os.IsNotExist(err) {
				continue
			} else if err != nil {
				return err
			}
			if lstat.Mode()&os.ModeSymlink != 0 {
				symlinks = append(symlinks, f)
				continue
			}
			stat, err := os.Stat(f)
			if os.IsNotExist(err) {
				continue
			} else if err != nil {
				return err
			}
			if stat.IsDir() {
				dir, err := os.ReadDir(f)
				if err != nil {
					return err
				}
				if len(dir) == 0 {
					fmt.Println("Removing: " + f)
					err := os.Remove(f)
					if err != nil {
						return err
					}
				}
			} else {
				fmt.Println("Removing: " + f)
				err := os.Remove(f)
				if err != nil {
					return err
				}
			}
		}
		removals := -1
		for len(symlinks) > 0 && removals != 0 {
			removals = 0
			for i := len(symlinks) - 1; i >= 0; i-- {
				f := symlinks[i]
				f = path.Join(rootDir, f)
				_, err := os.Lstat(f)
				if os.IsNotExist(err) {
					continue
				} else if err != nil {
					return err
				}
				_, err = filepath.EvalSymlinks(f)
				if os.IsNotExist(err) {
					err := os.Remove(f)
					if err != nil {
						return err
					}
					removals++
					fmt.Println("Removing: " + f)
				} else if err != nil {
					return err
				}
			}
		}
	}
	if !packageInstalled {
		err = ExecutePackageScripts(filename, rootDir, Install, true)
		if err != nil {
			return err
		}
	} else {
		err = ExecutePackageScripts(filename, rootDir, Update, true)
		if err != nil {
			return err
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
	pkgInfo := GetPackageInfo(pkg, rootDir, false)
	if pkgInfo == nil {
		return errors.New("could not get package info")
	}
	files := GetPackageFiles(pkg, rootDir)
	var symlinks []string
	for _, file := range files {
		file = path.Join(rootDir, file)
		lstat, err := os.Lstat(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if lstat.Mode()&os.ModeSymlink != 0 {
			symlinks = append(symlinks, file)
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
	removals := -1
	for len(symlinks) > 0 && removals != 0 {
		removals = 0
		for i := len(symlinks) - 1; i >= 0; i-- {
			file := symlinks[i]
			file = path.Join(rootDir, file)
			_, err := os.Lstat(file)
			if os.IsNotExist(err) {
				continue
			} else if err != nil {
				return err
			}
			_, err = filepath.EvalSymlinks(file)
			if os.IsNotExist(err) {
				err := os.Remove(file)
				if err != nil {
					return err
				}
				removals++
				fmt.Println("Removing: " + file)
			} else if err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(path.Join(pkgDir, "post_remove.sh")); err == nil {
		cmd := exec.Command("/bin/bash", path.Join(pkgDir, "post_remove.sh"))
		if !BPMConfig.SilentCompilation {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.Dir = rootDir
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_ROOT=%s", rootDir))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_NAME=%s", pkgInfo.Name))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_DESC=%s", pkgInfo.Description))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_VERSION=%s", pkgInfo.Version))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_URL=%s", pkgInfo.Url))
		cmd.Env = append(cmd.Env, fmt.Sprintf("BPM_PKG_ARCH=%s", pkgInfo.Arch))
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
	}
	err := os.RemoveAll(pkgDir)
	if err != nil {
		return err
	}
	fmt.Println("Removing: " + pkgDir)
	return nil
}
