package utils

import (
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
)

type BPMOperation struct {
	Actions           []OperationAction
	UnresolvedDepends []string
	RootDir           string
}

func (operation *BPMOperation) ActionsContainPackage(pkg string) bool {
	for _, action := range operation.Actions {
		if action.GetActionType() == "install" {
			return action.(*InstallPackageAction).BpmPackage.PkgInfo.Name == pkg
		} else if action.GetActionType() == "fetch" {
			return action.(*FetchPackageAction).RepositoryEntry.Info.Name == pkg
		} else if action.GetActionType() == "remove" {
			return action.(*RemovePackageAction).BpmPackage.PkgInfo.Name == pkg
		}
	}
	return false
}

func (operation *BPMOperation) InsertActionAt(index int, action OperationAction) {
	if len(operation.Actions) == index { // nil or empty slice or after last element
		operation.Actions = append(operation.Actions, action)
	}
	operation.Actions = append(operation.Actions[:index+1], operation.Actions[index:]...) // index < len(a)
	operation.Actions[index] = action
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

func (operation *BPMOperation) ShowOperationSummary() {
	if len(operation.Actions) == 0 {
		fmt.Println("All packages are up to date!")
		os.Exit(0)
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
			if operation.RootDir != "/" {
				log.Fatalf("cannot compile and install source packages to a different root directory")
			}
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
		fmt.Printf("%s will be downloaded to complete this operation\n", UnsignedBytesToHumanReadable(operation.GetTotalDownloadSize()))
	}
	if operation.GetFinalActionSize(operation.RootDir) > 0 {
		fmt.Printf("A total of %s will be installed after the operation finishes\n", BytesToHumanReadable(operation.GetFinalActionSize(operation.RootDir)))
	} else if operation.GetFinalActionSize(operation.RootDir) < 0 {
		fmt.Printf("A total of %s will be freed after the operation finishes\n", strings.TrimPrefix(BytesToHumanReadable(operation.GetFinalActionSize(operation.RootDir)), "-"))
	}
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
			err := RemovePackage(pkgInfo.Name, verbose, operation.RootDir)
			if err != nil {
				return errors.New(fmt.Sprintf("could not remove package (%s): %s\n", pkgInfo.Name, err))
			}
		} else if action.GetActionType() == "install" {
			value := action.(*InstallPackageAction)
			bpmpkg := value.BpmPackage
			var err error
			if value.IsDependency {
				err = InstallPackage(value.File, operation.RootDir, verbose, true, false, false, false)
			} else {
				err = InstallPackage(value.File, operation.RootDir, verbose, force, false, false, false)
			}
			if err != nil {
				return errors.New(fmt.Sprintf("could not install package (%s): %s\n", bpmpkg.PkgInfo.Name, err))
			}
			fmt.Printf("Package (%s) was successfully installed\n", bpmpkg.PkgInfo.Name)
			if value.IsDependency {
				err := SetInstallationReason(bpmpkg.PkgInfo.Name, Dependency, operation.RootDir)
				if err != nil {
					return errors.New(fmt.Sprintf("could not set installation reason for package (%s): %s\n", value.BpmPackage.PkgInfo.Name, err))
				}
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
