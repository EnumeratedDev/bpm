package utils

type BPMOperation struct {
	Actions []OperationAction
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

func (operation *BPMOperation) GetFinalActionSize(rootDir string) uint64 {
	var ret uint64 = 0
	for _, action := range operation.Actions {
		if action.GetActionType() == "install" {
			ret += action.(*InstallPackageAction).BpmPackage.GetInstalledSize()
			if IsPackageInstalled(action.(*InstallPackageAction).BpmPackage.PkgInfo.Name, rootDir) {
				ret -= GetPackage(action.(*InstallPackageAction).BpmPackage.PkgInfo.Name, rootDir).GetInstalledSize()
			}
		} else if action.GetActionType() == "fetch" {
			ret += action.(*FetchPackageAction).RepositoryEntry.InstalledSize
		} else if action.GetActionType() == "remove" {
			ret -= action.(*RemovePackageAction).BpmPackage.GetInstalledSize()
		}
	}
	return ret
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
