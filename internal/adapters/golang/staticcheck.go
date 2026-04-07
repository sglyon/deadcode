// Package golang contains the adapter for Go dead-code detection via
// staticcheck.
//
// Strategy
//
// staticcheck's U1000 check catches unused Go functions, types,
// constants, variables, fields, and methods with high precision. It
// understands interface satisfaction, init functions, exported API
// surfaces, and build tags — we don't need a built-in ignore list
// the way we do for Python/Elixir frameworks.
//
// Invocation: `staticcheck -checks=U1000 -f json ./...` from the
// project root. Output is NDJSON — one JSON object per line.
//
// Compatibility
//
// Tested on staticcheck 2026.1 (v0.7.0). The JSON schema has been
// stable since the 2017.2 release, so the parser should hold across
// future versions. Pinned with expected-staticcheck.json regression
// fixtures.
package golang

import (
	"bufio"
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

// Staticcheck is the Go adapter. Named after the underlying tool
// because one day we might also add a `deadcode` (golang.org/x/tools)
// adapter as a second source.
type Staticcheck struct{}

func New() *Staticcheck { return &Staticcheck{} }

func (s *Staticcheck) Name() string        { return "staticcheck" }
func (s *Staticcheck) Languages() []string { return []string{"go"} }

func (s *Staticcheck) Check(ctx context.Context) error {
	if _, err := exec.LookPath("staticcheck"); err != nil {
		return fmt.Errorf(
			"staticcheck not found in PATH (install with: " +
				"`go install honnef.co/go/tools/cmd/staticcheck@latest` " +
				"and ensure $GOPATH/bin or $GOBIN is on your PATH)")
	}
	c := exec.CommandContext(ctx, "staticcheck", "-version")
	if err := c.Run(); err != nil {
		return fmt.Errorf("staticcheck is installed but `staticcheck -version` failed: %w", err)
	}
	return nil
}

func (s *Staticcheck) Run(ctx context.Context, paths []string, opts adapter.RunOptions) ([]finding.Finding, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	roots := projectRoots(paths)
	if len(roots) == 0 {
		return nil, nil
	}

	var allFindings []finding.Finding
	for _, root := range roots {
		findings, err := runStaticcheck(ctx, root, opts)
		if err != nil {
			return nil, err
		}
		allFindings = append(allFindings, findings...)
	}
	return allFindings, nil
}

// runStaticcheck executes staticcheck in the given module root and
// parses its NDJSON output.
//
// staticcheck exits non-zero when it finds issues (this varies by
// version); we treat exit codes 0 and 1 as "ran successfully". Any
// higher code or a non-exec error is a real build failure and gets
// surfaced with an actionable message.
func runStaticcheck(ctx context.Context, root string, opts adapter.RunOptions) ([]finding.Finding, error) {
	cmd := exec.CommandContext(ctx, "staticcheck",
		"-checks=U1000",
		"-f", "json",
		"./...",
	)
	cmd.Dir = root
	out, runErr := cmd.Output()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			if exitErr.ExitCode() > 1 {
				return nil, staticcheckError(root, exitErr.ExitCode(), exitErr.Stderr, out)
			}
		} else {
			return nil, fmt.Errorf("staticcheck failed in %s: %w", root, runErr)
		}
	}
	return parseStaticcheckOutput(out, root, opts), nil
}

// staticcheckError translates a staticcheck exit code > 1 into an
// actionable error. The most common real-world failure is "missing
// dependencies" when the module hasn't been `go mod download`-ed yet.
func staticcheckError(root string, exitCode int, stderr, stdout []byte) error {
	combined := strings.TrimSpace(string(stderr))
	if combined == "" {
		combined = strings.TrimSpace(string(stdout))
	}
	if strings.Contains(combined, "missing go.sum") ||
		strings.Contains(combined, "no required module provides") ||
		strings.Contains(combined, "cannot find package") {
		return fmt.Errorf(
			"staticcheck failed in %s (exit %d): the module's dependencies are missing or stale. "+
				"Run `go mod download` (or `go mod tidy`) in %s and re-scan. "+
				"Original error:\n%s",
			root, exitCode, root, combined,
		)
	}
	return fmt.Errorf("staticcheck failed in %s (exit %d):\n%s", root, exitCode, combined)
}

// projectRoots walks upward from each scan path looking for go.mod.
// Dedupes so a multi-path scan inside one module runs staticcheck
// exactly once.
func projectRoots(paths []string) []string {
	seen := map[string]bool{}
	var roots []string
	for _, p := range paths {
		root := findGoMod(p)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

func findGoMod(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	dir := abs
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// staticcheckDiagnostic mirrors staticcheck's JSON formatter shape.
// We only decode the fields we use; staticcheck ships more (severity,
// end, related) that the json package silently ignores.
type staticcheckDiagnostic struct {
	Code     string             `json:"code"`
	Severity string             `json:"severity"`
	Location staticcheckLoc     `json:"location"`
	End      staticcheckLoc     `json:"end"`
	Message  string             `json:"message"`
	Related  []staticcheckRelat `json:"related,omitempty"`
}

type staticcheckLoc struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type staticcheckRelat struct {
	Location staticcheckLoc `json:"location"`
	Message  string         `json:"message"`
}

// parseStaticcheckOutput converts a stream of NDJSON diagnostics
// from `staticcheck -f json` into normalized Findings. projectRoot
// is used to build IDs with paths relative to the go.mod directory
// so they're stable across machines and shells (the user-visible
// File field stays absolute for editor jump-to support).
func parseStaticcheckOutput(out []byte, projectRoot string, opts adapter.RunOptions) []finding.Finding {
	var findings []finding.Finding
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var diag staticcheckDiagnostic
		if err := json.Unmarshal(line, &diag); err != nil {
			// Non-JSON line (can happen if staticcheck prints a
			// build diagnostic to stdout). Skip quietly.
			continue
		}
		if diag.Code != "U1000" {
			// We only ask for U1000 via -checks, but be defensive
			// in case staticcheck ever emits other codes anyway
			// (e.g. compile errors it decides to surface).
			continue
		}
		if opts.ExcludeTests && looksLikeTestFile(diag.Location.File) {
			continue
		}
		kind, symbol := classifyMessage(diag.Message)
		idPath := relPathForID(diag.Location.File, projectRoot)
		findings = append(findings, finding.Finding{
			ID:         fmt.Sprintf("go:%s:%d:%s:%s", idPath, diag.Location.Line, kind, symbol),
			File:       diag.Location.File,
			Line:       diag.Location.Line,
			Symbol:     symbol,
			Kind:       kind,
			Language:   "go",
			Tool:       "staticcheck",
			Confidence: 0.95,
			Message:    diag.Message,
			Evidence: map[string]string{
				"tool_code": diag.Code,
				"severity":  diag.Severity,
			},
			FixHint: finding.FixDelete,
		})
	}
	return findings
}

// classifyMessage maps a staticcheck U1000 message to a kind and a
// symbol. The message shapes are stable across staticcheck versions:
//
//	func X is unused
//	method (T).X is unused
//	type X is unused
//	const X is unused
//	var X is unused
//	field X is unused
//
// We return the bare symbol name (without the "is unused" suffix)
// and the kind enum value. On an unknown shape we fall back to
// KindUnusedFunction with the whole message as the symbol, so nothing
// is silently dropped.
func classifyMessage(msg string) (finding.Kind, string) {
	for _, p := range patterns {
		if strings.HasPrefix(msg, p.prefix) && strings.HasSuffix(msg, " is unused") {
			symbol := strings.TrimSuffix(strings.TrimPrefix(msg, p.prefix), " is unused")
			return p.kind, symbol
		}
	}
	return finding.KindUnusedFunction, msg
}

// patterns lists the U1000 message prefixes in priority order. The
// "method " prefix must be checked before "func " because methods
// come before functions alphabetically in the current staticcheck
// output but we want the precise kind.
var patterns = []struct {
	prefix string
	kind   finding.Kind
}{
	{"func ", finding.KindUnusedFunction},
	{"method ", finding.KindUnusedMethod},
	{"type ", finding.KindUnusedType},
	{"const ", finding.KindUnusedConstant},
	{"var ", finding.KindUnusedVariable},
	{"field ", finding.KindUnusedField},
}

// relPathForID normalizes a file path to a project-relative form
// suitable for use in a Finding.ID. Both Go adapters share this so
// IDs are consistent regardless of which tool found the dead code.
//
// We accept absolute or already-relative paths. If the input is
// absolute and inside projectRoot, we strip the prefix. If the input
// is already relative, we return it as-is. If neither works (across
// drives, weird paths), we return the input unchanged — better
// stable-but-ugly than cwd-dependent.
func relPathForID(file, projectRoot string) string {
	if !filepath.IsAbs(file) {
		// Already relative — assume it's project-relative (this is
		// how x/tools/cmd/deadcode emits paths).
		return filepath.ToSlash(file)
	}
	if r, err := filepath.Rel(projectRoot, file); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return file
}

func looksLikeTestFile(path string) bool {
	base := filepath.Base(path)
	// Go convention: test files always end in _test.go.
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	// testdata/ is the standard directory name Go's own tooling
	// excludes; any file under it is a fixture.
	for _, seg := range strings.Split(filepath.ToSlash(filepath.Dir(path)), "/") {
		if seg == "testdata" {
			return true
		}
	}
	return false
}
