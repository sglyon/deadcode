// Package javascript contains adapters for JavaScript and TypeScript
// dead-code analyzers.
package javascript

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/finding"
)

// Knip wraps the `knip` JavaScript/TypeScript analyzer.
//
// Unlike vulture, knip is project-aware: it walks from a package.json,
// uses tsconfig.json, and understands the entry points of the project.
// Our integration honors that by discovering the nearest package.json
// for each scan path and running knip from THAT directory.
//
// knip exits 0 when clean, 1 when issues are found (the happy path for
// us), and 2 on actual error.
type Knip struct{}

func New() *Knip { return &Knip{} }

func (k *Knip) Name() string        { return "knip" }
func (k *Knip) Languages() []string { return []string{"javascript", "typescript"} }

func (k *Knip) Check(ctx context.Context) error {
	cmd, _, err := resolveKnipCommand()
	if err != nil {
		return err
	}
	// Verify the resolved command can run --version. We pass --version
	// because knip with no args would actually scan the current dir.
	c := exec.CommandContext(ctx, cmd, k.versionArgs(cmd)...)
	if err := c.Run(); err != nil {
		return fmt.Errorf("knip is reachable via %s but `--version` failed: %w", cmd, err)
	}
	return nil
}

// versionArgs returns the args needed to invoke `knip --version` for
// the given resolved command (knip vs npx).
func (k *Knip) versionArgs(cmd string) []string {
	if cmd == "npx" {
		return []string{"--yes", "knip", "--version"}
	}
	return []string{"--version"}
}

func (k *Knip) Run(ctx context.Context, paths []string, opts adapter.RunOptions) ([]finding.Finding, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	cmd, baseArgs, err := resolveKnipCommand()
	if err != nil {
		return nil, err
	}

	roots := projectRoots(paths)
	if len(roots) == 0 {
		// No package.json found anywhere — nothing to do, but not an error.
		return nil, nil
	}

	var allFindings []finding.Finding
	for _, root := range roots {
		args := append([]string{}, baseArgs...)
		args = append(args, "--reporter", "json", "--no-progress", "--no-config-hints")
		c := exec.CommandContext(ctx, cmd, args...)
		c.Dir = root
		out, runErr := c.Output()
		if runErr != nil {
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				// Exit 1 = issues found (success for us). Exit 2 = real error.
				if exitErr.ExitCode() != 1 && exitErr.ExitCode() != 0 {
					return nil, knipExecError(root, exitErr.ExitCode(), exitErr.Stderr)
				}
			} else {
				return nil, fmt.Errorf("knip failed in %s: %w", root, runErr)
			}
		}
		findings, err := parseKnipOutput(out, root, opts)
		if err != nil {
			return nil, fmt.Errorf("parsing knip output for %s: %w", root, err)
		}
		allFindings = append(allFindings, findings...)
	}
	return allFindings, nil
}

// knipExecError translates a knip non-zero exit into an actionable
// message. The most common real-world failure is "Cannot find module"
// when the project's node_modules aren't installed — knip dynamically
// loads config files (vite.config.ts, etc.) and they import their own
// deps. The fix is always the same: install the project's deps.
func knipExecError(root string, exitCode int, stderr []byte) error {
	stderrStr := strings.TrimSpace(string(stderr))
	if strings.Contains(stderrStr, "Cannot find module") {
		return fmt.Errorf(
			"knip failed in %s (exit %d): the project's node_modules are missing or incomplete. "+
				"Run `npm install` (or `npm ci` / `pnpm install` / `yarn install`) in %s and re-scan. "+
				"Original error: %s",
			root, exitCode, root, stderrStr,
		)
	}
	return fmt.Errorf("knip failed in %s (exit %d): %s", root, exitCode, stderrStr)
}

// resolveKnipCommand picks the best available way to invoke knip:
// prefer a globally-installed `knip` for speed, fall back to `npx -y knip`
// when only Node is present. Returns the command name and the leading
// args (so callers can append their own).
func resolveKnipCommand() (string, []string, error) {
	if _, err := exec.LookPath("knip"); err == nil {
		return "knip", nil, nil
	}
	if _, err := exec.LookPath("npx"); err == nil {
		return "npx", []string{"--yes", "knip"}, nil
	}
	return "", nil, fmt.Errorf("knip not found in PATH and npx unavailable (install with: npm install -g knip, or install Node.js for the npx fallback)")
}

// projectRoots walks upward from each scan path looking for the nearest
// package.json. Dedupes the result so a `deadcode scan src/ test/` in
// the same project only runs knip once.
func projectRoots(paths []string) []string {
	seen := map[string]bool{}
	var roots []string
	for _, p := range paths {
		root := findPackageJSON(p)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

func findPackageJSON(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	dir := abs
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// knipReport is the parsed shape of `knip --reporter json` as actually
// emitted by knip 5.x — which differs from the public docs in three
// ways: (1) `files` is per-issue, not top-level; (2) `enumMembers` and
// `classMembers` are flat arrays with an optional `namespace` field,
// not maps keyed by container name; (3) extra fields exist (catalog,
// namespaceMembers, optionalPeerDependencies) which we ignore.
//
// Decode only the fields we care about; the json package silently
// drops the rest.
type knipReport struct {
	Issues []knipIssue `json:"issues"`
}

type knipIssue struct {
	File            string         `json:"file"`
	Files           []knipFileMark `json:"files"`
	Dependencies    []knipSymbol   `json:"dependencies"`
	DevDependencies []knipSymbol   `json:"devDependencies"`
	Binaries        []knipSymbol   `json:"binaries"`
	Exports         []knipSymbol   `json:"exports"`
	Types           []knipSymbol   `json:"types"`
	EnumMembers     []knipSymbol   `json:"enumMembers"`
	ClassMembers    []knipSymbol   `json:"classMembers"`
}

// knipFileMark is the per-issue marker that says "this file is itself
// unused." A non-empty `files` array on an issue means the file should
// be reported as unused_file regardless of whether it has other issues.
type knipFileMark struct {
	Name string `json:"name"`
}

// knipSymbol represents a single unused symbol. The `namespace` field
// is populated for enum members ("Status") and class members
// ("Registry"); empty for everything else.
type knipSymbol struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Line      int    `json:"line"`
	Col       int    `json:"col"`
	Pos       int    `json:"pos"`
}

// parseKnipOutput converts a knip JSON document (rooted at projectRoot)
// into normalized Findings. The mapping is documented in docs/SPEC.md
// under "Adapter: knip".
func parseKnipOutput(out []byte, projectRoot string, opts adapter.RunOptions) ([]finding.Finding, error) {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, nil
	}
	var report knipReport
	if err := json.Unmarshal(out, &report); err != nil {
		return nil, err
	}

	var findings []finding.Finding

	for _, issue := range report.Issues {
		abs := absInProject(projectRoot, issue.File)
		if opts.ExcludeTests && looksLikeTestFile(abs) {
			continue
		}
		lang := languageForExt(issue.File)

		// A non-empty `files` array on an issue marks the file ITSELF
		// as unused. Emit one unused_file finding per entry (almost
		// always exactly one — the issue's own file).
		for _, mark := range issue.Files {
			markAbs := absInProject(projectRoot, mark.Name)
			if opts.ExcludeTests && looksLikeTestFile(markAbs) {
				continue
			}
			findings = append(findings, finding.Finding{
				ID:         buildID(mark.Name, 1, finding.KindUnusedFile, filepath.Base(mark.Name)),
				File:       markAbs,
				Line:       1,
				Symbol:     filepath.Base(mark.Name),
				Kind:       finding.KindUnusedFile,
				Language:   languageForExt(mark.Name),
				Tool:       "knip",
				Confidence: 0.95,
				Message:    fmt.Sprintf("File '%s' has no consumers", mark.Name),
				Evidence: map[string]string{
					"tool_category": "files",
					"tool_raw":      mark.Name,
				},
				FixHint: finding.FixDelete,
			})
		}

		emit := func(kind finding.Kind, category, msgTpl string, sym knipSymbol, qualifyWithNamespace bool) {
			displayName := sym.Name
			if qualifyWithNamespace && sym.Namespace != "" {
				displayName = sym.Namespace + "." + sym.Name
			}
			findings = append(findings, finding.Finding{
				ID:         buildID(issue.File, sym.Line, kind, displayName),
				File:       abs,
				Line:       sym.Line,
				Symbol:     displayName,
				Kind:       kind,
				Language:   lang,
				Tool:       "knip",
				Confidence: 0.95,
				Message:    fmt.Sprintf(msgTpl, displayName),
				Evidence: map[string]string{
					"tool_category": category,
				},
				FixHint: finding.FixDelete,
			})
		}

		for _, d := range issue.Dependencies {
			emit(finding.KindUnusedDependency, "dependencies", "Dependency '%s' declared but unused", d, false)
		}
		for _, d := range issue.DevDependencies {
			emit(finding.KindUnusedDependency, "devDependencies", "Dev dependency '%s' declared but unused", d, false)
		}
		for _, b := range issue.Binaries {
			emit(finding.KindUnusedDependency, "binaries", "Binary '%s' declared but unused", b, false)
		}
		for _, e := range issue.Exports {
			emit(finding.KindUnusedExport, "exports", "Export '%s' has no consumers", e, false)
		}
		for _, t := range issue.Types {
			emit(finding.KindUnusedExport, "types", "Type '%s' has no consumers", t, false)
		}
		for _, m := range issue.EnumMembers {
			emit(finding.KindUnusedExport, "enumMembers", "Enum member '%s' has no consumers", m, true)
		}
		for _, m := range issue.ClassMembers {
			emit(finding.KindUnusedField, "classMembers", "Class member '%s' has no consumers", m, true)
		}
		// Skipped on purpose: unlisted, unresolved, duplicates,
		// catalog, namespaceMembers, optionalPeerDependencies —
		// these are not dead code.
	}

	return findings, nil
}

func absInProject(root, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(root, rel)
}

func languageForExt(file string) string {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	}
	// package.json + others — call it javascript by convention.
	return "javascript"
}

func buildID(file string, line int, kind finding.Kind, symbol string) string {
	return fmt.Sprintf("js:%s:%d:%s:%s", file, line, kind, symbol)
}

func looksLikeTestFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	dir := filepath.ToSlash(filepath.Dir(path))
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}
	for _, seg := range strings.Split(dir, "/") {
		switch seg {
		case "tests", "test", "__tests__", "__test__", "spec":
			return true
		}
	}
	return false
}
