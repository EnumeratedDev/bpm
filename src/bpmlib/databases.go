package bpmlib

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

type BPMDatabase struct {
	Name            string `yaml:"name"`
	Source          string `yaml:"source"`
	Disabled        *bool  `yaml:"disabled"`
	Entries         map[string]*BPMDatabaseEntry
	VirtualPackages map[string][]string
}

type BPMDatabaseEntry struct {
	Info          *PackageInfo `yaml:"info"`
	Download      string       `yaml:"download"`
	DownloadSize  uint64       `yaml:"download_size"`
	InstalledSize uint64       `yaml:"installed_size"`
	Database      *BPMDatabase
}

func (db *BPMDatabase) ContainsPackage(pkg string) bool {
	_, ok := db.Entries[pkg]
	return ok
}

func (db *BPMDatabase) ReadLocalDatabase() error {
	dbFile := "/var/lib/bpm/databases/" + db.Name + ".bpmdb"
	if _, err := os.Stat(dbFile); err != nil {
		return nil
	}

	bytes, err := os.ReadFile(dbFile)
	if err != nil {
		return err
	}

	data := string(bytes)
	for _, b := range strings.Split(data, "---") {
		entry := BPMDatabaseEntry{
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
			Database:      db,
		}
		err := yaml.Unmarshal([]byte(b), &entry)
		if err != nil {
			return err
		}

		// Create database entries
		if entry.Info.IsSplitPackage() {
			for _, splitPkg := range entry.Info.SplitPackages {
				// Turn split package into json data
				splitPkgJson, err := yaml.Marshal(splitPkg)
				if err != nil {
					return err
				}

				// Clone all main package fields onto split package
				splitPkgClone := *entry.Info

				// Set split package field of split package to nil
				splitPkgClone.SplitPackages = nil

				// Unmarshal json data back to struct
				err = yaml.Unmarshal(splitPkgJson, &splitPkgClone)
				if err != nil {
					return err
				}

				// Force set split package version, revision and URL
				splitPkgClone.Version = entry.Info.Version
				splitPkgClone.Revision = entry.Info.Revision
				splitPkgClone.Url = entry.Info.Url

				// Create entry for split package
				db.Entries[splitPkg.Name] = &BPMDatabaseEntry{
					Info:          &splitPkgClone,
					Download:      entry.Download,
					DownloadSize:  entry.DownloadSize,
					InstalledSize: 0,
					Database:      db,
				}

				// Add virtual packages to database
				for _, p := range splitPkg.Provides {
					db.VirtualPackages[p] = append(db.VirtualPackages[p], splitPkg.Name)
				}
			}
		} else {
			// Create entry for package
			db.Entries[entry.Info.Name] = &entry

			// Add virtual packages to database
			for _, p := range entry.Info.Provides {
				db.VirtualPackages[p] = append(db.VirtualPackages[p], entry.Info.Name)
			}
		}

	}

	return nil
}

func (db *BPMDatabase) SyncLocalDatabaseFile() error {
	dbFile := "/var/lib/bpm/databases/" + db.Name + ".bpmdb"

	// Get URL to database
	u, err := url.JoinPath(db.Source, "database.bpmdb")
	if err != nil {
		return err
	}

	// Retrieve data from URL
	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Load data into byte buffer
	buffer, err := io.ReadAll(resp.Body)

	// Unmarshal data to ensure it is a valid BPM database
	err = yaml.Unmarshal(buffer, &BPMDatabase{})
	if err != nil {
		return fmt.Errorf("could not decode database: %s", err)
	}

	// Create parent directories to database file
	err = os.MkdirAll(path.Dir(dbFile), 0755)
	if err != nil {
		return err
	}

	// Create file and save database data
	out, err := os.Create(dbFile)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = out.Write(buffer)

	return nil
}

func ReadLocalDatabaseFiles() (err error) {
	for _, db := range BPMConfig.Databases {
		// Initialize struct values
		db.Entries = make(map[string]*BPMDatabaseEntry)
		db.VirtualPackages = make(map[string][]string)

		// Read database
		err = db.ReadLocalDatabase()
		if err != nil {
			return err
		}
	}

	return nil
}

func GetDatabase(name string) *BPMDatabase {
	for _, db := range BPMConfig.Databases {
		if db.Name == name {
			return db
		}
	}
	return nil
}

func GetDatabaseEntry(str string) (*BPMDatabaseEntry, *BPMDatabase, error) {
	split := strings.Split(str, "/")
	if len(split) == 1 {
		pkgName := strings.TrimSpace(split[0])
		if pkgName == "" {
			return nil, nil, errors.New("could not find database entry for this package")
		}
		for _, db := range BPMConfig.Databases {
			if db.ContainsPackage(pkgName) {
				return db.Entries[pkgName], db, nil
			}
		}
		return nil, nil, errors.New("could not find database entry for this package")
	} else if len(split) == 2 {
		dbName := strings.TrimSpace(split[0])
		pkgName := strings.TrimSpace(split[1])
		if dbName == "" || pkgName == "" {
			return nil, nil, errors.New("could not find database entry for this package")
		}
		db := GetDatabase(dbName)
		if db == nil || !db.ContainsPackage(pkgName) {
			return nil, nil, errors.New("could not find database entry for this package")
		}
		return db.Entries[pkgName], db, nil
	} else {
		return nil, nil, errors.New("could not find database entry for this package")
	}
}

func FindReplacement(pkg string) *BPMDatabaseEntry {
	for _, db := range BPMConfig.Databases {
		for _, entry := range db.Entries {
			for _, replaced := range entry.Info.Replaces {
				if replaced == pkg {
					return entry
				}
			}
		}
	}

	return nil
}

func ResolveVirtualPackage(vpkg string) *BPMDatabaseEntry {
	for _, db := range BPMConfig.Databases {
		if v, ok := db.VirtualPackages[vpkg]; ok {
			for _, pkg := range v {
				return db.Entries[pkg]
			}
		}
	}

	return nil
}

func (db *BPMDatabase) FetchPackage(pkg string) (string, error) {
	if !db.ContainsPackage(pkg) {
		return "", errors.New("could not fetch package '" + pkg + "'")
	}
	entry := db.Entries[pkg]
	URL, err := url.JoinPath(db.Source, entry.Download)
	if err != nil {
		return "", err
	}
	resp, err := http.Get(URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	err = os.MkdirAll("/var/cache/bpm/fetched/", 0755)
	if err != nil {
		return "", err
	}
	out, err := os.Create("/var/cache/bpm/fetched/" + path.Base(entry.Download))
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return "/var/cache/bpm/fetched/" + path.Base(entry.Download), nil
}
