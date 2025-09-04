package bpmlib

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

type BPMDatabase struct {
	DatabaseVersion int                          `yaml:"database_version"`
	Entries         map[string]*BPMDatabaseEntry `yaml:"entries"`
	VirtualPackages map[string][]string
	Source          string
}

type BPMDatabaseEntry struct {
	Info          *PackageInfo `yaml:"info"`
	Filepath      string       `yaml:"filepath"`
	DownloadSize  uint64       `yaml:"download_size"`
	InstalledSize uint64       `yaml:"installed_size"`
	Database      *BPMDatabase
}

var BPMDatabases = make(map[string]*BPMDatabase)

func (db *BPMDatabase) ContainsPackage(pkg string) bool {
	_, ok := db.Entries[pkg]
	return ok
}

func (db *configDatabase) ReadLocalDatabase() error {
	dbFile := "/var/lib/bpm/databases/" + db.Name + ".bpmdb"
	if _, err := os.Stat(dbFile); err != nil {
		return nil
	}

	bytes, err := os.ReadFile(dbFile)
	if err != nil {
		return err
	}

	// Unmarshal yaml
	database := &BPMDatabase{}
	err = yaml.Unmarshal(bytes, database)
	if err != nil {
		return err
	}

	// Initialize struct values
	database.VirtualPackages = make(map[string][]string)
	database.Source = db.Source

	entriesToRemove := make([]string, 0)
	for entryName, entry := range database.Entries {
		entry.Database = database

		if entry.Info.IsSplitPackage() {
			// Handle split packages
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
				database.Entries[splitPkg.Name] = &BPMDatabaseEntry{
					Info:          &splitPkgClone,
					Filepath:      entry.Filepath,
					DownloadSize:  entry.DownloadSize,
					InstalledSize: 0,
					Database:      database,
				}

				// Add virtual packages to database
				for _, p := range splitPkg.Provides {
					database.VirtualPackages[p] = append(database.VirtualPackages[p], splitPkg.Name)
				}

				// Add current entry to list for removal
				entriesToRemove = append(entriesToRemove, entryName)
			}
		} else {
			// Add virtual packages to database
			for _, p := range entry.Info.Provides {
				database.VirtualPackages[p] = append(database.VirtualPackages[p], entry.Info.Name)
			}
		}
	}

	// Remove entries
	for _, entryName := range entriesToRemove {
		delete(database.Entries, entryName)
	}

	BPMDatabases[db.Name] = database

	return nil
}

func (db *configDatabase) SyncLocalDatabaseFile() error {
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
	for _, db := range MainBPMConfig.Databases {
		// Read database
		err = db.ReadLocalDatabase()
		if err != nil {
			return err
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
		for _, db := range BPMDatabases {
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
		db := BPMDatabases[dbName]
		if db == nil || !db.ContainsPackage(pkgName) {
			return nil, nil, errors.New("could not find database entry for this package")
		}
		return db.Entries[pkgName], db, nil
	} else {
		return nil, nil, errors.New("could not find database entry for this package")
	}
}

func FindReplacement(pkg string) *BPMDatabaseEntry {
	for _, db := range BPMDatabases {
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
	for _, db := range BPMDatabases {
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
	URL, err := url.JoinPath(db.Source, entry.Filepath)
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
	out, err := os.Create("/var/cache/bpm/fetched/" + path.Base(entry.Filepath))
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return "/var/cache/bpm/fetched/" + path.Base(entry.Filepath), nil
}
