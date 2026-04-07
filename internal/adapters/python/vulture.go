// Package python contains adapters for Python analyzers.
package python

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/finding"
)

// Vulture wraps the `vulture` Python static analyzer.
//
// Vulture's output format is:
//
//	<file>:<line>: unused <kind> '<symbol>' (<n>% confidence)
//	<file>:<line>: unreachable code after '<token>' (<n>% confidence)
//
// Vulture exits with code 3 when it finds unused code — that is "success"
// for our purposes, not an error.
type Vulture struct{}

func New() *Vulture { return &Vulture{} }

func (v *Vulture) Name() string        { return "vulture" }
func (v *Vulture) Languages() []string { return []string{"python"} }

func (v *Vulture) Check(ctx context.Context) error {
	if _, err := exec.LookPath("vulture"); err != nil {
		return fmt.Errorf("vulture not found in PATH (install with: pip install vulture)")
	}
	cmd := exec.CommandContext(ctx, "vulture", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vulture is installed but `vulture --version` failed: %w", err)
	}
	return nil
}

func (v *Vulture) Run(ctx context.Context, paths []string, opts adapter.RunOptions) ([]finding.Finding, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	var args []string
	if flag := BuildIgnoreDecoratorsFlag(opts.IgnoreDecorators, !opts.NoDefaultDecorators); flag != "" {
		args = append(args, "--ignore-decorators", flag)
	}
	args = append(args, paths...)
	cmd := exec.CommandContext(ctx, "vulture", args...)
	out, err := cmd.Output()
	if err != nil {
		// Vulture exits 3 when findings exist. That is the happy path for us.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() != 3 {
				return nil, fmt.Errorf("vulture failed (exit %d): %s", exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
			}
		} else {
			return nil, fmt.Errorf("vulture failed: %w", err)
		}
	}
	// Resolve a stable project root for ID-path normalization. We
	// walk up from the first scan path looking for a Python project
	// marker (pyproject.toml, setup.py, setup.cfg) or .git. Falls
	// back to the scan path itself if nothing matches. The result
	// is used ONLY for ID building — the user-visible File field
	// stays absolute so editors/agents can jump to it.
	projectRoot := pythonProjectRoot(paths[0])
	return parseVultureOutput(out, projectRoot, opts), nil
}

// pythonProjectRoot walks upward from start looking for the nearest
// directory containing a Python project marker. Returns the start
// directory itself if no marker is found, so IDs are still stable
// (just rooted at the scan path instead of a project root).
func pythonProjectRoot(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return start
	}
	dir := abs
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	startDir := dir
	markers := []string{"pyproject.toml", "setup.py", "setup.cfg", ".git"}
	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return startDir
		}
		dir = parent
	}
}

// vultureLine matches both unused-symbol and unreachable-code lines.
//
// Captured groups:
//  1. file
//  2. line number
//  3. message body
//  4. confidence (digits)
var vultureLine = regexp.MustCompile(`^(.+?):(\d+):\s+(.+?)\s+\((\d+)% confidence\)\s*$`)

// unusedSymbol matches the body of an "unused X 'name'" message.
//
// Captured groups:
//  1. raw kind word (function/method/class/variable/import/attribute/property)
//  2. symbol name
var unusedSymbol = regexp.MustCompile(`^unused\s+(\w+)\s+'([^']+)'$`)

func parseVultureOutput(out []byte, projectRoot string, opts adapter.RunOptions) []finding.Finding {
	var findings []finding.Finding
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		m := vultureLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		file, lineStr, body, confStr := m[1], m[2], m[3], m[4]

		lineNum, err := strconv.Atoi(lineStr)
		if err != nil {
			continue
		}
		confInt, err := strconv.Atoi(confStr)
		if err != nil {
			continue
		}
		confidence := float64(confInt) / 100.0

		var kind finding.Kind
		var symbol string
		var message string

		if sm := unusedSymbol.FindStringSubmatch(body); sm != nil {
			kind = mapVultureKind(sm[1])
			symbol = sm[2]
			message = fmt.Sprintf("%s '%s' appears unused", titleCase(sm[1]), symbol)
		} else if strings.HasPrefix(body, "unreachable code") {
			kind = finding.KindUnreachable
			message = "Unreachable code"
		} else {
			// Unknown vulture message shape — skip rather than misclassify.
			continue
		}

		absFile, err := filepath.Abs(file)
		if err != nil {
			absFile = file
		}

		if opts.ExcludeTests && looksLikeTestFile(absFile) {
			continue
		}

		findings = append(findings, finding.Finding{
			ID:         buildID("py", absFile, projectRoot, lineNum, kind, symbol),
			File:       absFile,
			Line:       lineNum,
			Symbol:     symbol,
			Kind:       kind,
			Language:   "python",
			Tool:       "vulture",
			Confidence: confidence,
			Message:    message,
			Evidence: map[string]string{
				"tool_raw":        line,
				"tool_confidence": confStr + "%",
			},
			FixHint: finding.FixDelete,
		})
	}
	return findings
}

func mapVultureKind(raw string) finding.Kind {
	switch raw {
	case "function":
		return finding.KindUnusedFunction
	case "method":
		return finding.KindUnusedMethod
	case "class":
		return finding.KindUnusedClass
	case "variable":
		return finding.KindUnusedVariable
	case "import":
		return finding.KindUnusedImport
	case "attribute", "property":
		return finding.KindUnusedField
	default:
		// Unknown — surface it as a generic unused variable so it isn't lost,
		// but this should be rare and worth investigating.
		return finding.KindUnusedVariable
	}
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// buildID constructs a stable Finding ID with a path relative to
// the project root. This makes IDs portable across machines and
// shells — two team members running deadcode on the same project
// from different working directories produce identical IDs, so a
// committed .deadcode-ignore.toml with `id =` rules works for
// everyone. We never use cwd-relative paths or absolute paths in
// IDs (which was the v0.6 bug surfaced during dogfood).
//
// Falls back to the absolute path if it can't be made relative
// (e.g., across drives on Windows). Better a stable absolute than
// a cwd-dependent relative.
func buildID(langPrefix, absFile, projectRoot string, line int, kind finding.Kind, symbol string) string {
	rel := absFile
	if r, err := filepath.Rel(projectRoot, absFile); err == nil && !strings.HasPrefix(r, "..") {
		rel = filepath.ToSlash(r)
	}
	return fmt.Sprintf("%s:%s:%d:%s:%s", langPrefix, rel, line, kind, symbol)
}

func looksLikeTestFile(path string) bool {
	base := filepath.Base(path)
	dir := filepath.ToSlash(filepath.Dir(path))
	if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
		return true
	}
	for _, seg := range strings.Split(dir, "/") {
		if seg == "tests" || seg == "test" {
			return true
		}
	}
	return false
}
