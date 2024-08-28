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
	Name     string `yaml:"name"`
	Source   string `yaml:"source"`
	Disabled *bool  `yaml:"disabled"`
	Entries  map[string]*RepositoryEntry
}

type RepositoryEntry struct {
	Info     PackageInfo `yaml:"info"`
	Download string      `yaml:"download"`
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
			Info: PackageInfo{
				Name:                   "",
				Description:            "",
				Version:                "",
				Url:                    "",
				License:                "",
				Arch:                   "",
				Type:                   "",
				Keep:                   make([]string, 0),
				Depends:                make([]string, 0),
				ConditionalDepends:     make(map[string][]string),
				MakeDepends:            make([]string, 0),
				ConditionalMakeDepends: make(map[string][]string),
				Conflicts:              make([]string, 0),
				ConditionalConflicts:   make(map[string][]string),
				Optional:               make([]string, 0),
				ConditionalOptional:    make(map[string][]string),
				Provides:               make([]string, 0),
			},
			Download: "",
		}
		err := yaml.Unmarshal([]byte(b), &entry)
		if err != nil {
			return err
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

func GetRepositoryEntry(str string) (*RepositoryEntry, error) {
	split := strings.Split(str, "/")
	if len(split) == 1 {
		pkgName := strings.TrimSpace(split[0])
		if pkgName == "" {
			return nil, errors.New("could not find repository entry for this package")
		}
		for _, repo := range BPMConfig.Repositories {
			if repo.ContainsPackage(pkgName) {
				return repo.Entries[pkgName], nil
			}
		}
		return nil, errors.New("could not find repository entry for this package")
	} else if len(split) == 2 {
		repoName := strings.TrimSpace(split[0])
		pkgName := strings.TrimSpace(split[1])
		if repoName == "" || pkgName == "" {
			return nil, errors.New("could not find repository entry for this package")
		}
		repo := GetRepository(repoName)
		if repo == nil || !repo.ContainsPackage(pkgName) {
			return nil, errors.New("could not find repository entry for this package")
		}
		return repo.Entries[pkgName], nil
	} else {
		return nil, errors.New("could not find repository entry for this package")
	}
}

func (repo *Repository) FetchPackage(pkg, savePath string) (string, error) {
	if !repo.ContainsPackage(pkg) {
		return "", errors.New("Could not fetch package '" + pkg + "'")
	}
	entry := repo.Entries[pkg]
	resp, err := http.Get(entry.Download)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	out, err := os.Create(savePath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return savePath, nil
}
