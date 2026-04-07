// Package lib is a library sub-package deliberately constructed to
// expose the staticcheck blindspot: exported symbols in non-main
// packages that staticcheck's U1000 refuses to flag even when they
// have zero callers. x/tools/cmd/deadcode catches them via
// whole-program reachability analysis from main.
package lib

// UsedByMain is called from the root main package — reachable.
func UsedByMain() string {
	return "reachable from main"
}

// DeadExport is exported but never called from anywhere. staticcheck
// silently skips it (exported in non-main package). deadcode catches
// it (unreachable from main via RTA call graph).
func DeadExport() string {
	return "this should be caught by x/tools/cmd/deadcode"
}
