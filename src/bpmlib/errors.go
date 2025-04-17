package bpmlib

import (
	"fmt"
	"strings"
)

type PackageNotFoundErr struct {
	packages []string
}

func (e PackageNotFoundErr) Error() string {
	return "The following packages were not found in any repositories: " + strings.Join(e.packages, ", ")
}

type DependencyNotFoundErr struct {
	dependencies []string
}

func (e DependencyNotFoundErr) Error() string {
	return "The following dependencies were not found in any repositories: " + strings.Join(e.dependencies, ", ")
}

type PackageConflictErr struct {
	pkg       string
	conflicts []string
}

func (e PackageConflictErr) Error() string {
	return fmt.Sprintf("Package (%s) is in conflict with the following packages: %s", e.pkg, strings.Join(e.conflicts, ", "))
}
