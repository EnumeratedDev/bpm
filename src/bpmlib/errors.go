package bpmlib

import (
	"fmt"
	"slices"
	"strings"
)

type PackageNotFoundErr struct {
	packages []string
}

func (e PackageNotFoundErr) Error() string {
	slices.Sort(e.packages)
	return "The following packages were not found in any databases: " + strings.Join(e.packages, ", ")
}

type DependencyNotFoundErr struct {
	dependencies []string
}

func (e DependencyNotFoundErr) Error() string {
	slices.Sort(e.dependencies)
	return "The following dependencies were not found in any databases: " + strings.Join(e.dependencies, ", ")
}

type PackageConflictErr struct {
	pkg       string
	conflicts []string
}

func (e PackageConflictErr) Error() string {
	slices.Sort(e.conflicts)
	return fmt.Sprintf("Package (%s) is in conflict with the following packages: %s", e.pkg, strings.Join(e.conflicts, ", "))

}

type PackageScriptErr struct {
	err           error
	packageName   string
	packageScript string
}

func (e PackageScriptErr) Error() string {
	return fmt.Sprintf("could not execute package script (%s) for package (%s): %s", e.packageScript, e.packageName, e.err)
}

type PackageRemovalDependencyErr struct {
	RequiredPackages map[string][]string
}

func (e PackageRemovalDependencyErr) Error() string {
	return "removing these package would break other installed packages"
}
