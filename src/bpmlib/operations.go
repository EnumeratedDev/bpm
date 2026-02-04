package bpmlib

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"slices"
	"strings"
	"text/tabwriter"
)

type BPMOperation struct {
	Actions           []OperationAction
	UnresolvedDepends []string
	Changes           map[string]string
	CompilationJobs   int
	RunChecks         bool
	RootDir           string

	compiledPackages   map[string]string
	hasFetchedPackages bool
}

func (operation *BPMOperation) ActionsContainPackage(pkg string) bool {
	for _, action := range operation.Actions {
		if action.GetActionType() == "install" {
			if action.(*InstallPackageAction).BpmPackage.PkgInfo.Name == pkg {
				return true
			}
		} else if action.GetActionType() == "fetch" {
			if action.(*FetchPackageAction).DatabaseEntry.Info.Name == pkg {
				return true
			}
		} else if action.GetActionType() == "remove" {
			if action.(*RemovePackageAction).BpmPackage.PkgInfo.Name == pkg {
				return true
			}
		}
	}
	return false
}

func (operation *BPMOperation) AppendAction(action OperationAction) {
	operation.InsertActionAt(len(operation.Actions), action)
}

func (operation *BPMOperation) InsertActionAt(index int, action OperationAction) {
	if len(operation.Actions) == index { // nil or empty slice or after last element
		operation.Actions = append(operation.Actions, action)
	} else {
		operation.Actions = append(operation.Actions[:index+1], operation.Actions[index:]...) // index < len(a)
		operation.Actions[index] = action
	}

	if action.GetActionType() == "install" {
		pkgInfo := action.(*InstallPackageAction).BpmPackage.PkgInfo
		if !IsPackageInstalled(pkgInfo.Name, operation.RootDir) {
			operation.Changes[pkgInfo.Name] = "install"
		} else {
			operation.Changes[pkgInfo.Name] = "upgrade"
		}
	} else if action.GetActionType() == "fetch" {
		pkgInfo := action.(*FetchPackageAction).DatabaseEntry.Info
		if !IsPackageInstalled(pkgInfo.Name, operation.RootDir) {
			operation.Changes[pkgInfo.Name] = "install"
		} else {
			operation.Changes[pkgInfo.Name] = "upgrade"
		}
	} else if action.GetActionType() == "remove" {
		operation.Changes[action.(*RemovePackageAction).BpmPackage.PkgInfo.Name] = "remove"
	}
}

func (operation *BPMOperation) RemoveAction(pkg, actionType string) {
	operation.Actions = slices.DeleteFunc(operation.Actions, func(a OperationAction) bool {
		if a.GetActionType() != actionType {
			return false
		}
		if a.GetActionType() == "install" {
			return a.(*InstallPackageAction).BpmPackage.PkgInfo.Name == pkg
		} else if a.GetActionType() == "fetch" {
			return a.(*FetchPackageAction).DatabaseEntry.Info.Name == pkg
		} else if a.GetActionType() == "remove" {
			return a.(*RemovePackageAction).BpmPackage.PkgInfo.Name == pkg
		}
		return false
	})
}

func (operation *BPMOperation) GetTotalDownloadSize() int64 {
	var ret int64 = 0
	for _, action := range operation.Actions {
		if action.GetActionType() == "fetch" {
			ret += action.(*FetchPackageAction).DatabaseEntry.DownloadSize
		}
	}
	return ret
}

func (operation *BPMOperation) GetTotalInstalledSize() int64 {
	var ret int64 = 0
	for _, action := range operation.Actions {
		if action.GetActionType() == "install" {
			ret += action.(*InstallPackageAction).BpmPackage.GetInstalledSize()
		} else if action.GetActionType() == "fetch" {
			ret += action.(*FetchPackageAction).DatabaseEntry.InstalledSize
		}
	}
	return ret
}

func (operation *BPMOperation) GetFinalActionSize(rootDir string) int64 {
	var ret int64 = 0
	for _, action := range operation.Actions {
		if action.GetActionType() == "install" {
			ret += action.(*InstallPackageAction).BpmPackage.GetInstalledSize()
			if IsPackageInstalled(action.(*InstallPackageAction).BpmPackage.PkgInfo.Name, rootDir) {
				ret -= GetPackage(action.(*InstallPackageAction).BpmPackage.PkgInfo.Name, rootDir).GetInstalledSize()
			}
		} else if action.GetActionType() == "fetch" {
			ret += action.(*FetchPackageAction).DatabaseEntry.InstalledSize
			if IsPackageInstalled(action.(*FetchPackageAction).DatabaseEntry.Info.Name, rootDir) {
				ret -= action.(*FetchPackageAction).DatabaseEntry.InstalledSize
			}
		} else if action.GetActionType() == "remove" {
			ret -= action.(*RemovePackageAction).BpmPackage.GetInstalledSize()
		}
	}
	return ret
}

func (operation *BPMOperation) ResolveDependencies(reinstallDependencies, installRuntimeDependencies, installOptionalDependencies, verbose bool) error {
	pos := 0
	resolvedVirtualPkgs := make(map[string]string, 0)
	for _, value := range slices.Clone(operation.Actions) {
		var pkgInfo *PackageInfo
		if value.GetActionType() == "install" {
			action := value.(*InstallPackageAction)
			pkgInfo = action.BpmPackage.PkgInfo
		} else if value.GetActionType() == "fetch" {
			action := value.(*FetchPackageAction)
			pkgInfo = action.DatabaseEntry.Info
		} else {
			pos++
			continue
		}

		resolved, unresolved := ResolveAllPackageDependenciesFromDatabases(pkgInfo, resolvedVirtualPkgs, pkgInfo.Type == "source", pkgInfo.Type == "source" && operation.RunChecks, installRuntimeDependencies, installOptionalDependencies, !reinstallDependencies, verbose, operation.RootDir)

		operation.UnresolvedDepends = append(operation.UnresolvedDepends, unresolved...)

		for _, resolvedPkg := range resolved {
			if !operation.ActionsContainPackage(resolvedPkg.PkgName) && resolvedPkg.PkgName != pkgInfo.Name {
				if !reinstallDependencies && IsPackageInstalled(resolvedPkg.PkgName, operation.RootDir) {
					continue
				}
				entry, _, err := GetDatabaseEntry(resolvedPkg.PkgName)
				if err != nil {
					return errors.New("could not get database entry for package (" + resolvedPkg.PkgName + ")")
				}
				operation.InsertActionAt(pos, &FetchPackageAction{
					InstallationReason: resolvedPkg.InstallationReason,
					DatabaseEntry:      entry,
				})
				pos++
			}
		}
		pos++
	}

	return nil
}

func (operation *BPMOperation) RemoveNeededPackages() error {
	removeActions := make(map[string]*RemovePackageAction)
	for _, action := range slices.Clone(operation.Actions) {
		if action.GetActionType() == "remove" {
			removeActions[action.(*RemovePackageAction).BpmPackage.PkgInfo.Name] = action.(*RemovePackageAction)
		}
	}

	for pkg, action := range removeActions {
		dependants := action.BpmPackage.PkgInfo.GetPackageDependants(operation.RootDir)
		dependants = slices.DeleteFunc(dependants, func(d string) bool {
			if _, ok := removeActions[d]; ok {
				return true
			}
			return false
		})
		if len(dependants) != 0 {
			operation.RemoveAction(pkg, action.GetActionType())
		}
	}

	return nil
}

func (operation *BPMOperation) Cleanup(cleanupMakeDepends bool) error {
	// Get all installed packages
	installedPackageNames, err := GetInstalledPackages(operation.RootDir)
	if err != nil {
		return fmt.Errorf("could not get installed packages: %s", err)
	}
	installedPackages := make([]*PackageInfo, len(installedPackageNames))
	for i, value := range installedPackageNames {
		bpmpkg := GetPackage(value, operation.RootDir)
		if bpmpkg == nil {
			return errors.New("could not find installed package (" + value + ")")
		}
		installedPackages[i] = bpmpkg.PkgInfo
	}

	// Get packages to remove
	removeActions := make(map[string]*RemovePackageAction)
	for _, action := range slices.Clone(operation.Actions) {
		if action.GetActionType() == "remove" {
			removeActions[action.(*RemovePackageAction).BpmPackage.PkgInfo.Name] = action.(*RemovePackageAction)
		}
	}

	// Get manually installed packages, resolve all their dependencies and add them to the keepPackages slice
	keepPackages := make([]string, 0)
	for _, pkg := range slices.Clone(installedPackages) {
		if getPackageLocalInfo(pkg.Name, operation.RootDir).GetInstallationReason() != InstallationReasonManual {
			continue
		}

		// Do not resolve dependencies or add package to keepPackages slice if package removal action exists for it
		if _, ok := removeActions[pkg.Name]; ok {
			continue
		}

		keepPackages = append(keepPackages, pkg.Name)
		resolved := pkg.GetDependenciesRecursive(true, !cleanupMakeDepends, !cleanupMakeDepends, operation.RootDir)
		for _, value := range resolved {
			if !slices.Contains(keepPackages, value) && !slices.Contains(MainBPMConfig.IgnorePackages, value) {
				keepPackages = append(keepPackages, value)
			}
		}
	}

	// Get all installed packages that are not in the keepPackages slice and add them to the BPM operation
	for _, pkg := range installedPackageNames {
		// Do not add package removal action if there already is one
		if _, ok := removeActions[pkg]; ok {
			continue
		}
		if !slices.Contains(keepPackages, pkg) {
			bpmpkg := GetPackage(pkg, operation.RootDir)
			if bpmpkg == nil {
				return errors.New("Error: could not find installed package (" + pkg + ")")
			}
			operation.Actions = append(operation.Actions, &RemovePackageAction{BpmPackage: bpmpkg})
		}
	}

	return nil
}

func (operation *BPMOperation) ReplaceObsoletePackages() {
	for _, value := range slices.Clone(operation.Actions) {
		var pkgInfo *PackageInfo
		if value.GetActionType() == "install" {
			action := value.(*InstallPackageAction)
			pkgInfo = action.BpmPackage.PkgInfo

		} else if value.GetActionType() == "fetch" {
			action := value.(*FetchPackageAction)
			pkgInfo = action.DatabaseEntry.Info
		} else {
			continue
		}

		for _, r := range pkgInfo.Replaces {
			if bpmpkg := GetPackage(r, operation.RootDir); bpmpkg != nil && !operation.ActionsContainPackage(bpmpkg.PkgInfo.Name) {
				operation.InsertActionAt(0, &RemovePackageAction{
					BpmPackage: bpmpkg,
				})
			}
		}
	}
}

func (operation *BPMOperation) CheckForConflicts() map[string][]string {
	conflicts := make(map[string][]string)

	// Get installed packages
	installedPackages := localPackageInformation[operation.RootDir]

	// Get packages to be removed
	removedPackages := make([]string, 0)
	for _, value := range slices.Clone(operation.Actions) {
		if value.GetActionType() != "remove" {
			continue
		}

		removedPackages = append(removedPackages, value.(*RemovePackageAction).BpmPackage.PkgInfo.Name)
	}

	// Check for conflicts
	for _, value := range slices.Clone(operation.Actions) {
		var pkgInfo *PackageInfo
		if value.GetActionType() == "install" {
			pkgInfo = value.(*InstallPackageAction).BpmPackage.PkgInfo
		} else if value.GetActionType() == "fetch" {
			pkgInfo = value.(*FetchPackageAction).DatabaseEntry.Info
		} else {
			continue
		}

		// Check for conflicts with installed packages
		for _, installedPkg := range installedPackages {
			// Skip if package is to be removed
			if slices.Contains(removedPackages, installedPkg.Name) {
				continue
			}

			// Skip if same package
			if pkgInfo.Name == installedPkg.Name {
				continue
			}

			// Check for new package conflicts
			if slices.Contains(pkgInfo.Conflicts, installedPkg.Name) {
				conflicts[pkgInfo.Name] = append(conflicts[pkgInfo.Name], installedPkg.Name)
			}
			for _, vpkg := range installedPkg.Provides {
				if slices.Contains(pkgInfo.Conflicts, vpkg) {
					conflicts[pkgInfo.Name] = append(conflicts[pkgInfo.Name], vpkg+" ("+installedPkg.Name+")")
				}
			}

			// Check for installed package conflicts
			for _, vpkg := range pkgInfo.Provides {
				if slices.Contains(installedPkg.Conflicts, vpkg) {
					conflicts[installedPkg.Name] = append(conflicts[installedPkg.Name], vpkg+" ("+pkgInfo.Name+")")
				}
			}
		}

		// Check for conflicts with other new packages
		for _, value := range slices.Clone(operation.Actions) {
			var pkgInfo2 *PackageInfo
			if value.GetActionType() == "install" {
				pkgInfo2 = value.(*InstallPackageAction).BpmPackage.PkgInfo
			} else if value.GetActionType() == "fetch" {
				pkgInfo2 = value.(*FetchPackageAction).DatabaseEntry.Info
			} else {
				continue
			}

			// Skip if same package
			if pkgInfo.Name == pkgInfo2.Name {
				continue
			}

			// Check for other package conflicts
			if slices.Contains(pkgInfo.Conflicts, pkgInfo2.Name) {
				conflicts[pkgInfo.Name] = append(conflicts[pkgInfo.Name], pkgInfo2.Name)
			}
			for _, vpkg := range pkgInfo2.Provides {
				if slices.Contains(pkgInfo.Conflicts, vpkg) {
					conflicts[pkgInfo.Name] = append(conflicts[pkgInfo.Name], vpkg+" ("+pkgInfo2.Name+")")
				}
			}
		}
	}

	return conflicts
}

func (operation *BPMOperation) ShowOperationSummary() {
	if len(operation.Actions) == 0 {
		fmt.Println("No action needs to be taken")
		return
	}

	writer := tabwriter.NewWriter(os.Stdout, 6, 4, 6, ' ', 0)
	fmt.Fprintln(writer, "Name\tVersion\tAction\tInstallation Reason\tFrom Source")

	for _, value := range operation.Actions {
		var pkgInfo *PackageInfo
		var installationReason = InstallationReasonUnknown
		if value.GetActionType() == "install" {
			installationReason = value.(*InstallPackageAction).InstallationReason
			pkgInfo = value.(*InstallPackageAction).BpmPackage.PkgInfo
			if value.(*InstallPackageAction).SplitPackageToInstall != "" {
				pkgInfo = pkgInfo.GetSplitPackageInfo(value.(*InstallPackageAction).SplitPackageToInstall)
			}
		} else if value.GetActionType() == "fetch" {
			installationReason = value.(*FetchPackageAction).InstallationReason
			pkgInfo = value.(*FetchPackageAction).DatabaseEntry.Info
		} else {
			pkgInfo = value.(*RemovePackageAction).BpmPackage.PkgInfo
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", pkgInfo.Name, pkgInfo.GetFullVersion(), "Remove", "-", "-")
			continue
		}

		installationReasonStr := ""
		switch installationReason {
		case InstallationReasonManual:
			installationReasonStr = "Manual"
		case InstallationReasonDependency:
			installationReasonStr = "Dependency"
		case InstallationReasonMakeDependency:
			installationReasonStr = "Make dependency"
		default:
			installationReasonStr = "Unknown"
		}

		installedInfo := GetPackageInfo(pkgInfo.Name, operation.RootDir)
		if installedInfo == nil {
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%t\n", pkgInfo.Name, pkgInfo.GetFullVersion(), "Install", installationReasonStr, pkgInfo.Type == "source")
		} else {
			comparison := CompareVersions(pkgInfo.GetFullVersion(), installedInfo.GetFullVersion())
			if comparison < 0 {
				fmt.Fprintf(writer, "%s\t%s -> %s\t%s\t%s\t%t\n", pkgInfo.Name, installedInfo.GetFullVersion(), pkgInfo.GetFullVersion(), "Downgrade", installationReasonStr, pkgInfo.Type == "source")
			} else if comparison > 0 {
				fmt.Fprintf(writer, "%s\t%s -> %s\t%s\t%s\t%t\n", pkgInfo.Name, installedInfo.GetFullVersion(), pkgInfo.GetFullVersion(), "Upgrade", installationReasonStr, pkgInfo.Type == "source")
			} else {
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%t\n", pkgInfo.Name, pkgInfo.GetFullVersion(), "Reinstall", installationReasonStr, pkgInfo.Type == "source")
			}
		}
	}

	writer.Flush()
	fmt.Println()

	if operation.RootDir != "/" {
		fmt.Println("Warning: Operating in " + operation.RootDir)
	}
	if operation.GetTotalDownloadSize() > 0 {
		fmt.Printf("%s will be downloaded to complete this operation\n", BytesToHumanReadable(operation.GetTotalDownloadSize()))
	}
	if operation.GetFinalActionSize(operation.RootDir) > 0 {
		fmt.Printf("A total of %s will be installed after the operation finishes\n", BytesToHumanReadable(operation.GetFinalActionSize(operation.RootDir)))
	} else if operation.GetFinalActionSize(operation.RootDir) < 0 {
		fmt.Printf("A total of %s will be freed after the operation finishes\n", strings.TrimPrefix(BytesToHumanReadable(operation.GetFinalActionSize(operation.RootDir)), "-"))
	}
}

func (operation *BPMOperation) ShowSourcePackageContent() (sourcePackagesShown int, err error) {
	// Fetch packages
	if !operation.hasFetchedPackages {
		err = operation.FetchPackages()
		if err != nil {
			return 0, err
		}
	}

	for _, action := range operation.Actions {
		if action.GetActionType() != "install" {
			continue
		}
		if action.(*InstallPackageAction).BpmPackage.PkgInfo.Type != "source" {
			continue
		}

		err = showPackageFiles(action.(*InstallPackageAction).File)
		if err != nil {
			return 0, err
		}

		sourcePackagesShown++
	}

	return sourcePackagesShown, nil
}

func (operation *BPMOperation) GetOptionalDependencies() (optionalDepends map[string][]string) {
	optionalDepends = make(map[string][]string)

	// Find all optional dependencies
	for _, value := range slices.Clone(operation.Actions) {
		var pkgInfo *PackageInfo
		if value.GetActionType() == "install" {
			action := value.(*InstallPackageAction)
			pkgInfo = action.BpmPackage.PkgInfo
		} else if value.GetActionType() == "fetch" {
			action := value.(*FetchPackageAction)
			pkgInfo = action.DatabaseEntry.Info
		} else {
			continue
		}

		for _, depend := range pkgInfo.OptionalDepends {
			dependSplit := strings.SplitN(depend, ":", 2)

			// Skip if dependency is already installed
			if IsPackageInstalled(dependSplit[0], operation.RootDir) {
				continue
			}

			// Skip if not a new dependency of the package
			if installedPkg := GetPackage(pkgInfo.Name, operation.RootDir); installedPkg != nil && slices.ContainsFunc(installedPkg.PkgInfo.OptionalDepends, func(n string) bool {
				return strings.SplitN(n, ":", 2)[0] == dependSplit[0]
			}) {
				continue
			}

			if len(dependSplit) == 2 {
				optionalDepends[pkgInfo.Name] = append(optionalDepends[pkgInfo.Name], fmt.Sprintf("%s (%s)", dependSplit[0], dependSplit[1]))
			} else {
				optionalDepends[pkgInfo.Name] = append(optionalDepends[pkgInfo.Name], dependSplit[0])
			}
		}
	}

	return
}

func (operation *BPMOperation) RunHooks(verbose bool) error {
	// Return if hooks directory does not exist
	if stat, err := os.Stat(path.Join(operation.RootDir, "var/lib/bpm/hooks")); err != nil || !stat.IsDir() {
		return nil
	}

	// Get directory entries in hooks directory
	dirEntries, err := os.ReadDir(path.Join(operation.RootDir, "var/lib/bpm/hooks"))
	if err != nil {
		return err
	}

	// Find all hooks, validate and execute them
	for _, entry := range dirEntries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".bpmhook") {
			hook, err := createHook(path.Join(operation.RootDir, "var/lib/bpm/hooks", entry.Name()))
			if err != nil {
				log.Printf("Error while reading hook (%s): %s", entry.Name(), err)
			}

			err = hook.Execute(operation.Changes, verbose, operation.RootDir)
			if err != nil {
				log.Printf("Warning: could not execute hook (%s): %s\n", entry.Name(), err)
				continue
			}
		}
	}

	return nil
}

func (operation *BPMOperation) FetchPackages() (err error) {
	// Fetch packages from databases
	if slices.ContainsFunc(operation.Actions, func(action OperationAction) bool {
		return action.GetActionType() == "fetch"
	}) {
		fmt.Println("Fetching packages from available databases...")

		// Create map for fetched packages
		fetchedPackages := make(map[string]string)

		for i, action := range operation.Actions {
			if action.GetActionType() != "fetch" {
				continue
			}

			// Get database entry
			entry := action.(*FetchPackageAction).DatabaseEntry

			// Create bpmpkg variable
			var bpmpkg *BPMPackage

			// Check if package has already been fetched from download link
			if fetchedFilepath, ok := fetchedPackages[entry.Filepath]; !ok {
				// Fetch package from database
				fetchedPackage, err := entry.Database.FetchPackage(entry.Info.Name)
				if err != nil {
					return fmt.Errorf("could not fetch package (%s): %s\n", entry.Info.Name, err)
				}

				// Read fetched package
				bpmpkg, err = ReadPackage(fetchedPackage)
				if err != nil {
					return fmt.Errorf("could not fetch package (%s): %s\n", entry.Info.Name, err)
				}

				// Add fetched package to map
				fetchedPackages[entry.Filepath] = fetchedPackage
			} else {
				// Read fetched package
				bpmpkg, err = ReadPackage(fetchedFilepath)
				if err != nil {
					return fmt.Errorf("could not read package (%s): %s\n", entry.Info.Name, err)
				}

				// Get size of fetched archive
				stat, err := os.Stat(fetchedFilepath)
				if err != nil {
					return err
				}

				bar := createProgressBar(stat.Size(), "Downloading "+entry.Info.Name, false)
				bar.Set64(stat.Size())
				bar.Close()
			}

			if bpmpkg.PkgInfo.IsSplitPackage() {
				operation.Actions[i] = &InstallPackageAction{
					File:                  fetchedPackages[entry.Filepath],
					InstallationReason:    action.(*FetchPackageAction).InstallationReason,
					BpmPackage:            bpmpkg,
					SplitPackageToInstall: entry.Info.Name,
				}
			} else {
				operation.Actions[i] = &InstallPackageAction{
					File:               fetchedPackages[entry.Filepath],
					InstallationReason: action.(*FetchPackageAction).InstallationReason,
					BpmPackage:         bpmpkg,
				}
			}
		}
	}

	operation.hasFetchedPackages = true
	return nil
}

func (operation *BPMOperation) Execute(verbose, force bool) (err error) {
	// Fetch packages
	if !operation.hasFetchedPackages {
		err = operation.FetchPackages()
		if err != nil {
			return err
		}
	}

	// Determine words to be used for the following message
	words := make([]string, 0)
	if slices.ContainsFunc(operation.Actions, func(action OperationAction) bool {
		return action.GetActionType() == "install"
	}) {
		words = append(words, "Installing")
	}

	if slices.ContainsFunc(operation.Actions, func(action OperationAction) bool {
		return action.GetActionType() == "remove"
	}) {
		words = append(words, "Removing")
	}

	if len(words) == 0 {
		return nil
	}
	fmt.Printf("%s packages...\n", strings.Join(words, "/"))

	// Installing/Removing packages from system
	for _, action := range operation.Actions {
		if action.GetActionType() == "remove" {
			pkgInfo := action.(*RemovePackageAction).BpmPackage.PkgInfo
			err := removePackage(pkgInfo.Name, verbose, operation.RootDir)
			if err != nil {
				return fmt.Errorf("could not remove package (%s): %s\n", pkgInfo.Name, err)
			}
		} else if action.GetActionType() == "install" {
			value := action.(*InstallPackageAction)
			fileToInstall := value.File
			bpmpkg := value.BpmPackage
			var err error

			// Compile package if type is 'source'
			if bpmpkg.PkgInfo.Type == "source" {
				// Get path to compiled package directory
				compiledDir := path.Join(operation.RootDir, "/var/cache/bpm/compiled/")

				// Create compiled package directory if not exists
				if _, err := os.Stat(compiledDir); err != nil {
					err := os.MkdirAll(compiledDir, 0755)
					if err != nil {
						return err
					}
				}

				// Get package name to install
				pkgNameToInstall := bpmpkg.PkgInfo.Name
				if bpmpkg.PkgInfo.IsSplitPackage() {
					pkgNameToInstall = value.SplitPackageToInstall
				}

				// Compile source package if not compiled already
				if _, ok := operation.compiledPackages[pkgNameToInstall]; !ok {
					outputBpmPackages, err := CompileSourcePackage(value.File, compiledDir, operation.CompilationJobs, !operation.RunChecks, false, verbose)
					if err != nil {
						return fmt.Errorf("could not compile source package (%s): %s\n", value.File, err)
					}

					// Add compiled packages to slice
					for pkgName, pkgFile := range outputBpmPackages {
						operation.compiledPackages[pkgName] = pkgFile
					}
				}

				// Set values
				fileToInstall = operation.compiledPackages[pkgNameToInstall]
				bpmpkg, err = ReadPackage(fileToInstall)
				if err != nil {
					return fmt.Errorf("could not read package (%s): %s\n", fileToInstall, err)
				}
			}

			if value.InstallationReason != InstallationReasonManual {
				err = installPackage(fileToInstall, value.InstallationReason, operation.RootDir, verbose, true)
			} else {
				err = installPackage(fileToInstall, value.InstallationReason, operation.RootDir, verbose, force)
			}
			if err != nil {
				return fmt.Errorf("could not install package (%s): %s\n", bpmpkg.PkgInfo.Name, err)
			}
		}
	}
	fmt.Println("Operation complete!")

	return nil
}

type OperationAction interface {
	GetActionType() string
}

type InstallPackageAction struct {
	File                  string
	InstallationReason    InstallationReason
	SplitPackageToInstall string
	BpmPackage            *BPMPackage
}

func (action *InstallPackageAction) GetActionType() string {
	return "install"
}

type FetchPackageAction struct {
	InstallationReason InstallationReason
	DatabaseEntry      *BPMDatabaseEntry
}

func (action *FetchPackageAction) GetActionType() string {
	return "fetch"
}

type RemovePackageAction struct {
	BpmPackage *BPMPackage
}

func (action *RemovePackageAction) GetActionType() string {
	return "remove"
}
