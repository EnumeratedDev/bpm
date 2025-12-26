package bpmlib

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type BPMDatabase struct {
	DatabaseVersion int                          `yaml:"database_version"`
	Entries         map[string]*BPMDatabaseEntry `yaml:"entries"`
	VirtualPackages map[string][]string
	Name            string
	Source          string
}

type BPMDatabaseEntry struct {
	Info          *PackageInfo `yaml:"info"`
	Filepath      string       `yaml:"filepath"`
	DownloadSize  int64        `yaml:"download_size"`
	InstalledSize int64        `yaml:"installed_size"`
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
	database.Name = db.Name
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
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create progress bar
	bar := createProgressBar(resp.ContentLength, "Syncing "+db.Name, false)

	// Copy data
	var buffer bytes.Buffer
	_, err = io.Copy(io.MultiWriter(&buffer, bar), resp.Body)
	if err != nil {
		return err
	}

	// Unmarshal data to ensure it is a valid BPM database
	err = yaml.Unmarshal(buffer.Bytes(), &BPMDatabase{})
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

	_, err = out.Write(buffer.Bytes())

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
			slices.Sort(v)
			return db.Entries[v[0]]
		}
	}

	return nil
}

func (db *BPMDatabase) FetchPackage(pkg string) (string, error) {
	// Check if package exists in database
	if !db.ContainsPackage(pkg) {
		return "", errors.New("could not fetch package '" + pkg + "'")
	}

	// Get package url from database
	entry := db.Entries[pkg]
	u, err := url.JoinPath(db.Source, entry.Filepath)
	if err != nil {
		return "", err
	}

	// Download package from url
	err = downloadFile("Downloading "+entry.Info.Name, u, path.Join("/var/cache/bpm/fetched/", path.Base(entry.Filepath)), 0644)
	if err != nil {
		return "", err
	}

	return path.Join("/var/cache/bpm/fetched/", path.Base(entry.Filepath)), nil
}

func (entry *BPMDatabaseEntry) GetEntryDependants() (dependants []string) {
	dependantsMap := make(map[string][]string)
	for _, db := range BPMDatabases {
		for _, e := range db.Entries {
			if slices.Contains(e.Info.Depends, entry.Info.Name) {
				dependantsMap[e.Info.Name] = append(dependantsMap[e.Info.Name], e.Database.Name)
			}
		}
	}

	// Get keys
	keySlice := slices.Collect(maps.Keys(dependantsMap))
	slices.Sort(keySlice)

	// Add all dependant entries to slice in alphabetical order
	for _, entryName := range keySlice {
		dbs := dependantsMap[entryName]
		if len(dbs) > 1 {
			for _, db := range dbs {
				dependants = append(dependants, db+"/"+entryName)
			}
		} else {
			dependants = append(dependants, entryName)
		}
	}

	return dependants
}

func (entry *BPMDatabaseEntry) GetEntryOptionalDependants() (dependants []string) {
	dependantsMap := make(map[string][]string)
	for _, db := range BPMDatabases {
		for _, e := range db.Entries {
			if slices.Contains(e.Info.OptionalDepends, entry.Info.Name) {
				dependantsMap[e.Info.Name] = append(dependantsMap[e.Info.Name], e.Database.Name)
			}
		}
	}

	// Get keys
	keySlice := slices.Collect(maps.Keys(dependantsMap))
	slices.Sort(keySlice)

	// Add all dependant entries to slice in alphabetical order
	for _, entryName := range keySlice {
		dbs := dependantsMap[entryName]
		if len(dbs) > 1 {
			for _, db := range dbs {
				dependants = append(dependants, db+"/"+entryName)
			}
		} else {
			dependants = append(dependants, entryName)
		}
	}

	return dependants
}

func (entry *BPMDatabaseEntry) CreateReadableInfo(rootDir string, humanReadableSize bool) string {
	ret := make([]string, 0)
	appendArray := func(label string, array []string, sort bool) {
		if len(array) == 0 {
			return
		}

		if sort {
			// Sort array
			slices.Sort(array)
		}

		ret = append(ret, fmt.Sprintf("%s: %s", label, strings.Join(array, ", ")))
	}

	ret = append(ret, "Name: "+entry.Info.Name)
	ret = append(ret, "Database: "+entry.Database.Name)
	ret = append(ret, "Description: "+entry.Info.Description)
	ret = append(ret, "Version: "+entry.Info.GetFullVersion())
	if entry.Info.Url != "" {
		ret = append(ret, "URL: "+entry.Info.Url)
	}
	if entry.Info.License != "" {
		ret = append(ret, "License: "+entry.Info.License)
	}
	ret = append(ret, "Architecture: "+entry.Info.Arch)
	if entry.Info.Type == "source" && entry.Info.OutputArch != "" && entry.Info.OutputArch != GetArch() {
		ret = append(ret, "Output architecture: "+entry.Info.OutputArch)
	}
	ret = append(ret, "Type: "+entry.Info.Type)
	appendArray("Dependencies", entry.Info.Depends, true)
	if entry.Info.Type == "source" {
		appendArray("Make Dependencies", entry.Info.MakeDepends, true)
	}
	appendArray("Optional dependencies", entry.Info.OptionalDepends, true)
	dependants := entry.GetEntryDependants()
	if len(dependants) > 0 {
		appendArray("Dependant packages", dependants, false)
	}
	optionalDependants := entry.GetEntryOptionalDependants()
	if len(optionalDependants) > 0 {
		appendArray("Optionally dependant packages", optionalDependants, false)
	}
	appendArray("Conflicting packages", entry.Info.Conflicts, true)
	appendArray("Provided packages", entry.Info.Provides, true)
	appendArray("Replaces packages", entry.Info.Replaces, true)

	if entry.Info.Type == "source" && len(entry.Info.SplitPackages) != 0 {
		splitPkgs := make([]string, len(entry.Info.SplitPackages))
		for i, splitPkgInfo := range entry.Info.SplitPackages {
			splitPkgs[i] = splitPkgInfo.Name
		}
		appendArray("Split Packages", splitPkgs, true)
	}

	if rootDir != "" && IsPackageInstalled(entry.Info.Name, rootDir) {
		installationReason := GetInstallationReason(entry.Info.Name, rootDir)
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
	if entry.Info.Type == "binary" {
		installedSize := entry.InstalledSize
		var installedSizeStr string
		if humanReadableSize {
			installedSizeStr = BytesToHumanReadable(installedSize)
		} else {
			installedSizeStr = strconv.FormatInt(installedSize, 10)
		}
		ret = append(ret, "Installed size: "+installedSizeStr)
	}
	return strings.Join(ret, "\n")
}
