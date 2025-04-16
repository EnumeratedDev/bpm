package bpmlib

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"slices"
	"strings"
)

type BPMOperation struct {
	Actions                 []OperationAction
	UnresolvedDepends       []string
	Changes                 map[string]string
	RootDir                 string
	ForceInstallationReason InstallationReason
}

func (operation *BPMOperation) ActionsContainPackage(pkg string) bool {
	for _, action := range operation.Actions {
		if action.GetActionType() == "install" {
			if action.(*InstallPackageAction).BpmPackage.PkgInfo.Name == pkg {
				return true
			}
		} else if action.GetActionType() == "fetch" {
			if action.(*FetchPackageAction).RepositoryEntry.Info.Name == pkg {
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
		pkgInfo := action.(*FetchPackageAction).RepositoryEntry.Info
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
			return a.(*FetchPackageAction).RepositoryEntry.Info.Name == pkg
		} else if a.GetActionType() == "remove" {
			return a.(*RemovePackageAction).BpmPackage.PkgInfo.Name == pkg
		}
		return false
	})
}

func (operation *BPMOperation) GetTotalDownloadSize() uint64 {
	var ret uint64 = 0
	for _, action := range operation.Actions {
		if action.GetActionType() == "fetch" {
			ret += action.(*FetchPackageAction).RepositoryEntry.DownloadSize
		}
	}
	return ret
}

func (operation *BPMOperation) GetTotalInstalledSize() uint64 {
	var ret uint64 = 0
	for _, action := range operation.Actions {
		if action.GetActionType() == "install" {
			ret += action.(*InstallPackageAction).BpmPackage.GetInstalledSize()
		} else if action.GetActionType() == "fetch" {
			ret += action.(*FetchPackageAction).RepositoryEntry.InstalledSize
		}
	}
	return ret
}

func (operation *BPMOperation) GetFinalActionSize(rootDir string) int64 {
	var ret int64 = 0
	for _, action := range operation.Actions {
		if action.GetActionType() == "install" {
			ret += int64(action.(*InstallPackageAction).BpmPackage.GetInstalledSize())
			if IsPackageInstalled(action.(*InstallPackageAction).BpmPackage.PkgInfo.Name, rootDir) {
				ret -= int64(GetPackage(action.(*InstallPackageAction).BpmPackage.PkgInfo.Name, rootDir).GetInstalledSize())
			}
		} else if action.GetActionType() == "fetch" {
			ret += int64(action.(*FetchPackageAction).RepositoryEntry.InstalledSize)
		} else if action.GetActionType() == "remove" {
			ret -= int64(action.(*RemovePackageAction).BpmPackage.GetInstalledSize())
		}
	}
	return ret
}

func (operation *BPMOperation) ResolveDependencies(reinstallDependencies, installOptionalDependencies, verbose bool) error {
	pos := 0
	for _, value := range slices.Clone(operation.Actions) {
		var pkgInfo *PackageInfo
		if value.GetActionType() == "install" {
			action := value.(*InstallPackageAction)
			pkgInfo = action.BpmPackage.PkgInfo
		} else if value.GetActionType() == "fetch" {
			action := value.(*FetchPackageAction)
			pkgInfo = action.RepositoryEntry.Info
		} else {
			pos++
			continue
		}

		resolved, unresolved := pkgInfo.ResolveDependencies(&[]string{}, &[]string{}, pkgInfo.Type == "source", installOptionalDependencies, !reinstallDependencies, verbose, operation.RootDir)

		operation.UnresolvedDepends = append(operation.UnresolvedDepends, unresolved...)

		for _, depend := range resolved {
			if !operation.ActionsContainPackage(depend) && depend != pkgInfo.Name {
				if !reinstallDependencies && IsPackageInstalled(depend, operation.RootDir) {
					continue
				}
				entry, _, err := GetRepositoryEntry(depend)
				if err != nil {
					return errors.New("could not get repository entry for package (" + depend + ")")
				}
				operation.InsertActionAt(pos, &FetchPackageAction{
					IsDependency:    true,
					RepositoryEntry: entry,
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
		dependants, err := action.BpmPackage.PkgInfo.GetDependants(operation.RootDir)
		if err != nil {
			return errors.New("could not get dependant packages for package (" + pkg + ")")
		}
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

func (operation *BPMOperation) Cleanup(verbose bool) error {
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
		if GetInstallationReason(pkg.Name, operation.RootDir) != InstallationReasonManual {
			continue
		}

		// Do not resolve dependencies or add package to keepPackages slice if package removal action exists for it
		if _, ok := removeActions[pkg.Name]; ok {
			continue
		}

		keepPackages = append(keepPackages, pkg.Name)
		resolved, _ := pkg.ResolveDependencies(&[]string{}, &[]string{}, false, true, false, verbose, operation.RootDir)
		for _, value := range resolved {
			if !slices.Contains(keepPackages, value) && slices.Contains(installedPackageNames, value) {
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
			pkgInfo = action.RepositoryEntry.Info
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

func (operation *BPMOperation) CheckForConflicts() (map[string][]string, error) {
	conflicts := make(map[string][]string)
	installedPackages, err := GetInstalledPackages(operation.RootDir)
	if err != nil {
		return nil, err
	}
	allPackages := make([]*PackageInfo, len(installedPackages))
	for i, value := range installedPackages {
		bpmpkg := GetPackage(value, operation.RootDir)
		if bpmpkg == nil {
			return nil, errors.New(fmt.Sprintf("could not find installed package (%s)", value))
		}
		allPackages[i] = bpmpkg.PkgInfo
	}

	// Add all new packages to the allPackages slice
	for _, value := range slices.Clone(operation.Actions) {
		if value.GetActionType() == "install" {
			action := value.(*InstallPackageAction)
			pkgInfo := action.BpmPackage.PkgInfo
			allPackages = append(allPackages, pkgInfo)
		} else if value.GetActionType() == "fetch" {
			action := value.(*FetchPackageAction)
			pkgInfo := action.RepositoryEntry.Info
			allPackages = append(allPackages, pkgInfo)
		} else if value.GetActionType() == "remove" {
			action := value.(*RemovePackageAction)
			pkgInfo := action.BpmPackage.PkgInfo
			for i := len(allPackages) - 1; i >= 0; i-- {
				info := allPackages[i]
				if info.Name == pkgInfo.Name {
					allPackages = append(allPackages[:i], allPackages[i+1:]...)
				}
			}
		}
	}

	for _, value := range allPackages {
		for _, conflict := range value.Conflicts {
			if slices.ContainsFunc(allPackages, func(info *PackageInfo) bool {
				return info.Name == conflict
			}) {
				conflicts[value.Name] = append(conflicts[value.Name], conflict)
			}
		}
	}

	return conflicts, nil
}

func (operation *BPMOperation) ShowOperationSummary() {
	if len(operation.Actions) == 0 {
		fmt.Println("No action needs to be taken")
		return
	}

	for _, value := range operation.Actions {
		var pkgInfo *PackageInfo
		if value.GetActionType() == "install" {
			pkgInfo = value.(*InstallPackageAction).BpmPackage.PkgInfo
		} else if value.GetActionType() == "fetch" {
			pkgInfo = value.(*FetchPackageAction).RepositoryEntry.Info
		} else {
			pkgInfo = value.(*RemovePackageAction).BpmPackage.PkgInfo
			fmt.Printf("%s: %s (Remove)\n", pkgInfo.Name, pkgInfo.GetFullVersion())
			continue
		}

		installedInfo := GetPackageInfo(pkgInfo.Name, operation.RootDir)
		sourceInfo := ""
		if pkgInfo.Type == "source" {
			sourceInfo = "(From Source)"
		}

		if installedInfo == nil {
			fmt.Printf("%s: %s (Install) %s\n", pkgInfo.Name, pkgInfo.GetFullVersion(), sourceInfo)
		} else {
			comparison := ComparePackageVersions(*pkgInfo, *installedInfo)
			if comparison < 0 {
				fmt.Printf("%s: %s -> %s (Downgrade) %s\n", pkgInfo.Name, installedInfo.GetFullVersion(), pkgInfo.GetFullVersion(), sourceInfo)
			} else if comparison > 0 {
				fmt.Printf("%s: %s -> %s (Upgrade) %s\n", pkgInfo.Name, installedInfo.GetFullVersion(), pkgInfo.GetFullVersion(), sourceInfo)
			} else {
				fmt.Printf("%s: %s (Reinstall) %s\n", pkgInfo.Name, pkgInfo.GetFullVersion(), sourceInfo)
			}
		}
	}

	if operation.RootDir != "/" {
		fmt.Println("Warning: Operating in " + operation.RootDir)
	}
	if operation.GetTotalDownloadSize() > 0 {
		fmt.Printf("%s will be downloaded to complete this operation\n", unsignedBytesToHumanReadable(operation.GetTotalDownloadSize()))
	}
	if operation.GetFinalActionSize(operation.RootDir) > 0 {
		fmt.Printf("A total of %s will be installed after the operation finishes\n", bytesToHumanReadable(operation.GetFinalActionSize(operation.RootDir)))
	} else if operation.GetFinalActionSize(operation.RootDir) < 0 {
		fmt.Printf("A total of %s will be freed after the operation finishes\n", strings.TrimPrefix(bytesToHumanReadable(operation.GetFinalActionSize(operation.RootDir)), "-"))
	}
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

func (operation *BPMOperation) Execute(verbose, force bool) error {
	// Fetch packages from repositories
	if slices.ContainsFunc(operation.Actions, func(action OperationAction) bool {
		return action.GetActionType() == "fetch"
	}) {
		fmt.Println("Fetching packages from available repositories...")
		for i, action := range operation.Actions {
			if action.GetActionType() != "fetch" {
				continue
			}
			entry := action.(*FetchPackageAction).RepositoryEntry
			fetchedPackage, err := entry.Repository.FetchPackage(entry.Info.Name)
			if err != nil {
				return errors.New(fmt.Sprintf("could not fetch package (%s): %s\n", entry.Info.Name, err))
			}
			bpmpkg, err := ReadPackage(fetchedPackage)
			if err != nil {
				return errors.New(fmt.Sprintf("could not fetch package (%s): %s\n", entry.Info.Name, err))
			}
			fmt.Printf("Package (%s) was successfully fetched!\n", bpmpkg.PkgInfo.Name)
			operation.Actions[i] = &InstallPackageAction{
				File:         fetchedPackage,
				IsDependency: action.(*FetchPackageAction).IsDependency,
				BpmPackage:   bpmpkg,
			}
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
				return errors.New(fmt.Sprintf("could not remove package (%s): %s\n", pkgInfo.Name, err))
			}
		} else if action.GetActionType() == "install" {
			value := action.(*InstallPackageAction)
			bpmpkg := value.BpmPackage
			isReinstall := IsPackageInstalled(bpmpkg.PkgInfo.Name, operation.RootDir)
			var err error
			if value.IsDependency {
				err = installPackage(value.File, operation.RootDir, verbose, true)
			} else {
				err = installPackage(value.File, operation.RootDir, verbose, force)
			}
			if err != nil {
				return errors.New(fmt.Sprintf("could not install package (%s): %s\n", bpmpkg.PkgInfo.Name, err))
			}
			if operation.ForceInstallationReason != InstallationReasonUnknown && !value.IsDependency {
				err := SetInstallationReason(bpmpkg.PkgInfo.Name, operation.ForceInstallationReason, operation.RootDir)
				if err != nil {
					return errors.New(fmt.Sprintf("could not set installation reason for package (%s): %s\n", value.BpmPackage.PkgInfo.Name, err))
				}
			} else if value.IsDependency && !isReinstall {
				err := SetInstallationReason(bpmpkg.PkgInfo.Name, InstallationReasonDependency, operation.RootDir)
				if err != nil {
					return errors.New(fmt.Sprintf("could not set installation reason for package (%s): %s\n", value.BpmPackage.PkgInfo.Name, err))
				}
			}
			fmt.Printf("Package (%s) was successfully installed\n", bpmpkg.PkgInfo.Name)
		}
	}
	fmt.Println("Operation complete!")

	return nil
}

type OperationAction interface {
	GetActionType() string
}

type InstallPackageAction struct {
	File         string
	IsDependency bool
	BpmPackage   *BPMPackage
}

func (action *InstallPackageAction) GetActionType() string {
	return "install"
}

type FetchPackageAction struct {
	IsDependency    bool
	RepositoryEntry *RepositoryEntry
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
