// Package ignore implements the unified .deadcode-ignore.toml mechanism.
//
// One file, one syntax, works across every adapter. Rules are matched
// against normalized findings AFTER each adapter runs, so this layer is
// completely tool-agnostic — it never reaches into vulture's whitelist
// format, knip's config, or anything else native.
//
// See docs/SPEC.md for the design rationale.
package ignore

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/sglyon/deadcode/internal/finding"
)

// Rule is one entry in a .deadcode-ignore.toml file.
//
// Field semantics:
//   - id        : exact match against finding.ID (most precise)
//   - file      : doublestar glob; matched against the absolute path AND
//                 the path relative to the ignore-file's directory
//   - symbol    : doublestar glob against finding.Symbol
//   - kinds     : finding.Kind must be in this list
//   - languages : finding.Language must be in this list
//   - tools     : finding.Tool must be in this list
//   - reason    : required for documentation; not used for matching
//
// All specified fields AND together. A rule with NO matchers matches
// nothing — that's a config bug, surfaced by Validate().
type Rule struct {
	ID        string   `toml:"id"`
	File      string   `toml:"file"`
	Symbol    string   `toml:"symbol"`
	Kinds     []string `toml:"kinds"`
	Languages []string `toml:"languages"`
	Tools     []string `toml:"tools"`
	Reason    string   `toml:"reason"`
}

// File is the on-disk shape of .deadcode-ignore.toml.
type File struct {
	Ignore []Rule `toml:"ignore"`
}

// Ruleset is a loaded, validated set of ignore rules with the directory
// the file lived in (used to resolve relative file globs).
type Ruleset struct {
	Path string // absolute path to the .deadcode-ignore.toml file
	Dir  string // directory containing the ignore file (= primary glob root)
	// MatchRoots are additional candidate roots used to resolve relative
	// file globs. The runner populates these with the absolute scan
	// paths so a glob like "src/models/**.py" still matches when the
	// ignore file lives outside the scanned tree.
	MatchRoots []string
	Rules      []Rule
}

// Empty returns true if there are no rules to apply.
func (rs *Ruleset) Empty() bool {
	return rs == nil || len(rs.Rules) == 0
}

// Match returns the index of the first rule that matches f, or -1 if
// none do. Rules are evaluated in order; the first match wins so users
// can order specific rules above general ones.
func (rs *Ruleset) Match(f finding.Finding) int {
	if rs == nil {
		return -1
	}
	roots := rs.allRoots()
	for i := range rs.Rules {
		if rs.Rules[i].matches(f, roots) {
			return i
		}
	}
	return -1
}

func (rs *Ruleset) allRoots() []string {
	roots := make([]string, 0, 1+len(rs.MatchRoots))
	if rs.Dir != "" {
		roots = append(roots, rs.Dir)
	}
	roots = append(roots, rs.MatchRoots...)
	return roots
}

func (r *Rule) matches(f finding.Finding, roots []string) bool {
	if !r.hasAnyMatcher() {
		return false
	}
	if r.ID != "" && r.ID != f.ID {
		return false
	}
	if r.File != "" && !matchFile(r.File, f.File, roots) {
		return false
	}
	if r.Symbol != "" && !matchGlob(r.Symbol, f.Symbol) {
		return false
	}
	if len(r.Kinds) > 0 && !contains(r.Kinds, string(f.Kind)) {
		return false
	}
	if len(r.Languages) > 0 && !contains(r.Languages, f.Language) {
		return false
	}
	if len(r.Tools) > 0 && !contains(r.Tools, f.Tool) {
		return false
	}
	return true
}

func (r *Rule) hasAnyMatcher() bool {
	return r.ID != "" || r.File != "" || r.Symbol != "" ||
		len(r.Kinds) > 0 || len(r.Languages) > 0 || len(r.Tools) > 0
}

// Validate checks each rule has at least one matcher and a reason.
// Returns a slice of human-readable error messages keyed by rule index;
// empty if all rules are valid.
func (rs *Ruleset) Validate() []string {
	var errs []string
	for i, r := range rs.Rules {
		if !r.hasAnyMatcher() {
			errs = append(errs, fmt.Sprintf("rule %d: at least one matcher required (id/file/symbol/kinds/languages/tools)", i))
		}
		if strings.TrimSpace(r.Reason) == "" {
			errs = append(errs, fmt.Sprintf("rule %d: missing 'reason' (required for documentation)", i))
		}
		for _, k := range r.Kinds {
			if !knownKind(k) {
				errs = append(errs, fmt.Sprintf("rule %d: unknown kind %q", i, k))
			}
		}
	}
	return errs
}

// matchFile applies a doublestar glob to a finding's file path. It tries:
//  1. the absolute path itself (so absolute globs work)
//  2. the path relative to each candidate root (Dir + MatchRoots), so
//     portable globs like "src/models/**.py" work whether the ignore
//     file lives in the scanned repo or out-of-tree
//  3. the basename, so a bare "**.py" still matches anywhere
func matchFile(pattern, path string, roots []string) bool {
	pattern = filepath.ToSlash(pattern)
	abs := filepath.ToSlash(path)
	if matchGlob(pattern, abs) {
		return true
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			if matchGlob(pattern, filepath.ToSlash(rel)) {
				return true
			}
		}
	}
	return matchGlob(pattern, filepath.Base(path))
}

func matchGlob(pattern, s string) bool {
	if pattern == "" {
		return false
	}
	ok, err := doublestar.Match(pattern, s)
	return err == nil && ok
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func knownKind(k string) bool {
	switch finding.Kind(k) {
	case finding.KindUnusedFunction,
		finding.KindUnusedMethod,
		finding.KindUnusedClass,
		finding.KindUnusedVariable,
		finding.KindUnusedConstant,
		finding.KindUnusedImport,
		finding.KindUnusedExport,
		finding.KindUnusedParam,
		finding.KindUnusedField,
		finding.KindUnusedFile,
		finding.KindUnreachable,
		finding.KindUnusedDependency:
		return true
	}
	return false
}
