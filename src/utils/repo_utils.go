package utils

import (
	"errors"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

type Repository struct {
	Name            string `yaml:"name"`
	Source          string `yaml:"source"`
	Disabled        *bool  `yaml:"disabled"`
	Entries         map[string]*RepositoryEntry
	VirtualPackages map[string][]string
}

type RepositoryEntry struct {
	Info          *PackageInfo `yaml:"info"`
	Download      string       `yaml:"download"`
	DownloadSize  uint64       `yaml:"download_size"`
	InstalledSize uint64       `yaml:"installed_size"`
	Repository    *Repository
}

func (repo *Repository) ContainsPackage(pkg string) bool {
	_, ok := repo.Entries[pkg]
	return ok
}

func (repo *Repository) ReadLocalDatabase() error {
	repoFile := "/var/lib/bpm/repositories/" + repo.Name + ".bpmdb"
	if _, err := os.Stat(repoFile); err != nil {
		return nil
	}

	bytes, err := os.ReadFile(repoFile)
	if err != nil {
		return err
	}

	data := string(bytes)
	for _, b := range strings.Split(data, "---") {
		entry := RepositoryEntry{
			Info: &PackageInfo{
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
				Provides:        make([]string, 0),
			},
			Download:      "",
			DownloadSize:  0,
			InstalledSize: 0,
			Repository:    repo,
		}
		err := yaml.Unmarshal([]byte(b), &entry)
		if err != nil {
			return err
		}

		for _, p := range entry.Info.Provides {
			repo.VirtualPackages[p] = append(repo.VirtualPackages[p], entry.Info.Name)
		}
		repo.Entries[entry.Info.Name] = &entry
	}

	return nil
}

func (repo *Repository) SyncLocalDatabase() error {
	repoFile := "/var/lib/bpm/repositories/" + repo.Name + ".bpmdb"
	err := os.MkdirAll(path.Dir(repoFile), 0755)
	if err != nil {
		return err
	}

	u, err := url.JoinPath(repo.Source, "database.bpmdb")
	if err != nil {
		return err
	}

	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(repoFile)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)

	return nil
}

func GetRepository(name string) *Repository {
	for _, repo := range BPMConfig.Repositories {
		if repo.Name == name {
			return repo
		}
	}
	return nil
}

func GetRepositoryEntry(str string) (*RepositoryEntry, *Repository, error) {
	split := strings.Split(str, "/")
	if len(split) == 1 {
		pkgName := strings.TrimSpace(split[0])
		if pkgName == "" {
			return nil, nil, errors.New("could not find repository entry for this package")
		}
		for _, repo := range BPMConfig.Repositories {
			if repo.ContainsPackage(pkgName) {
				return repo.Entries[pkgName], repo, nil
			}
		}
		return nil, nil, errors.New("could not find repository entry for this package")
	} else if len(split) == 2 {
		repoName := strings.TrimSpace(split[0])
		pkgName := strings.TrimSpace(split[1])
		if repoName == "" || pkgName == "" {
			return nil, nil, errors.New("could not find repository entry for this package")
		}
		repo := GetRepository(repoName)
		if repo == nil || !repo.ContainsPackage(pkgName) {
			return nil, nil, errors.New("could not find repository entry for this package")
		}
		return repo.Entries[pkgName], repo, nil
	} else {
		return nil, nil, errors.New("could not find repository entry for this package")
	}
}

func FindReplacement(pkg string) *RepositoryEntry {
	for _, repo := range BPMConfig.Repositories {
		for _, entry := range repo.Entries {
			for _, replaced := range entry.Info.Replaces {
				if replaced == pkg {
					return entry
				}
			}
		}
	}

	return nil
}

func ResolveVirtualPackage(vpkg string) *RepositoryEntry {
	for _, repo := range BPMConfig.Repositories {
		if v, ok := repo.VirtualPackages[vpkg]; ok {
			for _, pkg := range v {
				return repo.Entries[pkg]
			}
		}
	}

	return nil
}

func (repo *Repository) FetchPackage(pkg string) (string, error) {
	if !repo.ContainsPackage(pkg) {
		return "", errors.New("could not fetch package '" + pkg + "'")
	}
	entry := repo.Entries[pkg]
	URL, err := url.JoinPath(repo.Source, entry.Download)
	if err != nil {
		return "", err
	}
	resp, err := http.Get(URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	err = os.MkdirAll("/var/cache/bpm/packages/", 0755)
	if err != nil {
		return "", err
	}
	out, err := os.Create("/var/cache/bpm/packages/" + path.Base(entry.Download))
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return "/var/cache/bpm/packages/" + path.Base(entry.Download), nil
}
