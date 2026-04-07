package golang

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/finding"
)

// XDeadcode is the second Go adapter. It wraps the Go team's official
// `golang.org/x/tools/cmd/deadcode` binary, which performs whole-
// program Rapid Type Analysis (RTA) from main functions to build a
// call graph and reports any function not reachable from main.
//
// Why a second Go adapter
//
// staticcheck's U1000 check has a documented blindspot: it refuses
// to flag exported identifiers in non-main packages, even under
// internal/, on the theory that external consumers might use them.
// x/tools/cmd/deadcode has no such blindspot — if a function isn't
// in the call graph starting from main, it's dead, regardless of
// whether it's exported. This is how we caught runner.UniqueLanguages
// during v0.5 dogfood (by grep); now we catch it with the tool.
//
// Scope
//
// This adapter ONLY reports unreachable functions/methods. Types,
// constants, variables, and fields are outside its remit. Run the
// staticcheck adapter alongside it for full coverage — both adapters
// emit distinct Finding.Tool values so the reporter shows which tool
// found what, and the user can see when both tools agree (high
// confidence signal) vs when only one catches something.
//
// Requirements
//
// The tool requires at least one main package in the module. Pure
// library modules produce "no main packages" (exit 1) — we treat
// that as a clean no-op, not an error.
//
// Compatibility
//
// Tested with the latest x/tools release. JSON schema is stable
// enough that the parser should hold; regression fixture pins it.
type XDeadcode struct{}

func NewXDeadcode() *XDeadcode { return &XDeadcode{} }

func (x *XDeadcode) Name() string        { return "xdeadcode" }
func (x *XDeadcode) Languages() []string { return []string{"go"} }

func (x *XDeadcode) Check(ctx context.Context) error {
	if _, err := exec.LookPath("deadcode"); err != nil {
		return fmt.Errorf(
			"deadcode (x/tools/cmd/deadcode) not found in PATH " +
				"(install with: `go install golang.org/x/tools/cmd/deadcode@latest` " +
				"and ensure $GOPATH/bin or $GOBIN is on your PATH)")
	}
	// The tool has no --version; running it with no args prints usage
	// and exits non-zero. Instead check that -h works.
	c := exec.CommandContext(ctx, "deadcode", "-h")
	if err := c.Run(); err != nil {
		// -h exits 2 on some versions (flag parsing prints help then
		// exits). That's fine — if the binary ran at all, it's usable.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return fmt.Errorf("deadcode tool is installed but `-h` failed: %w", err)
		}
	}
	return nil
}

func (x *XDeadcode) Run(ctx context.Context, paths []string, opts adapter.RunOptions) ([]finding.Finding, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	// Reuse the Go project-root discovery from the staticcheck adapter.
	roots := projectRoots(paths)
	if len(roots) == 0 {
		return nil, nil
	}

	var allFindings []finding.Finding
	for _, root := range roots {
		findings, err := runXDeadcode(ctx, root, opts)
		if err != nil {
			return nil, err
		}
		allFindings = append(allFindings, findings...)
	}
	return allFindings, nil
}

// runXDeadcode invokes the tool in the given module root and parses
// its JSON output into Findings.
//
// Exit code handling:
//   - 0: success, may have findings in stdout
//   - 1: "no main packages" OR a fatal error; distinguish by checking
//     stderr. "no main packages" → return nil (library-only module,
//     nothing to analyze, not an error).
//   - other: treat as a real failure.
func runXDeadcode(ctx context.Context, root string, opts adapter.RunOptions) ([]finding.Finding, error) {
	cmd := exec.CommandContext(ctx, "deadcode", "-json", "./...")
	cmd.Dir = root
	out, runErr := cmd.Output()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if strings.Contains(stderr, "no main packages") {
				// Pure library module — nothing for RTA to analyze
				// from. Silent skip is correct.
				return nil, nil
			}
			return nil, xdeadcodeError(root, exitErr.ExitCode(), exitErr.Stderr, out)
		}
		return nil, fmt.Errorf("deadcode tool failed in %s: %w", root, runErr)
	}
	return parseXDeadcodeOutput(out, root, opts), nil
}

// xdeadcodeError translates non-zero exits into actionable messages.
// Most failure modes are the same class as staticcheck: missing
// module deps, or a genuine compile error.
func xdeadcodeError(root string, exitCode int, stderr, stdout []byte) error {
	combined := strings.TrimSpace(string(stderr))
	if combined == "" {
		combined = strings.TrimSpace(string(stdout))
	}
	if strings.Contains(combined, "missing go.sum") ||
		strings.Contains(combined, "no required module provides") ||
		strings.Contains(combined, "cannot find package") {
		return fmt.Errorf(
			"deadcode tool failed in %s (exit %d): the module's dependencies are missing or stale. "+
				"Run `go mod download` (or `go mod tidy`) in %s and re-scan. "+
				"Original error:\n%s",
			root, exitCode, root, combined,
		)
	}
	return fmt.Errorf("deadcode tool failed in %s (exit %d):\n%s", root, exitCode, combined)
}

// xDeadcodeReport is the parsed shape of `deadcode -json ./...`:
// a top-level array of package objects, each containing a list of
// unreachable funcs with source positions.
type xDeadcodePackage struct {
	Name  string              `json:"Name"`
	Path  string              `json:"Path"`
	Funcs []xDeadcodeFunc     `json:"Funcs"`
}

type xDeadcodeFunc struct {
	Name      string             `json:"Name"`
	Position  xDeadcodePosition  `json:"Position"`
	Generated bool               `json:"Generated"`
	Marker    bool               `json:"Marker"`
}

type xDeadcodePosition struct {
	File string `json:"File"`
	Line int    `json:"Line"`
	Col  int    `json:"Col"`
}

// parseXDeadcodeOutput decodes the JSON array and converts it to
// Findings. The tool only reports functions/methods, so every finding
// has kind unused_function.
//
// The tool already skips generated files and marker interface methods
// by default (see `deadcode -h`), so we don't need to re-filter those.
// We DO honor ExcludeTests here even though the tool has its own
// -test flag; our semantics are "drop findings whose only callers
// are tests" which the upstream tool doesn't model.
func parseXDeadcodeOutput(out []byte, projectRoot string, opts adapter.RunOptions) []finding.Finding {
	out = trimJSON(out)
	if len(out) == 0 || string(out) == "null" {
		return nil
	}
	var packages []xDeadcodePackage
	if err := json.Unmarshal(out, &packages); err != nil {
		return nil
	}
	var findings []finding.Finding
	for _, pkg := range packages {
		for _, fn := range pkg.Funcs {
			if fn.Generated || fn.Marker {
				// Defensive: should already be filtered by the tool,
				// but honor the flags if they ever appear.
				continue
			}
			absFile := fn.Position.File
			if !filepath.IsAbs(absFile) {
				absFile = filepath.Join(projectRoot, absFile)
			}
			if opts.ExcludeTests && looksLikeTestFile(absFile) {
				continue
			}
			// Qualified symbol: pkg.Name.fn.Name for readability.
			// The user's mental model is "which function in which
			// package" — matches how Go developers reference symbols.
			symbol := fmt.Sprintf("%s.%s", pkg.Name, fn.Name)
			idPath := relPathForID(fn.Position.File, projectRoot)
			findings = append(findings, finding.Finding{
				ID:         fmt.Sprintf("go:%s:%d:%s:%s", idPath, fn.Position.Line, finding.KindUnusedFunction, symbol),
				File:       absFile,
				Line:       fn.Position.Line,
				Symbol:     symbol,
				Kind:       finding.KindUnusedFunction,
				Language:   "go",
				Tool:       "xdeadcode",
				Confidence: 0.95,
				Message:    fmt.Sprintf("function %s is unreachable from main (RTA call graph)", symbol),
				Evidence: map[string]string{
					"package_path": pkg.Path,
					"tool_source":  "golang.org/x/tools/cmd/deadcode",
				},
				FixHint: finding.FixDelete,
			})
		}
	}
	return findings
}

// trimJSON strips leading/trailing whitespace that can come from the
// deadcode tool printing a diagnostic line before the JSON payload.
func trimJSON(out []byte) []byte {
	// Find the first '[' or '{' and trim everything before it.
	for i, b := range out {
		if b == '[' || b == '{' {
			out = out[i:]
			break
		}
	}
	// Trim trailing whitespace.
	for len(out) > 0 {
		last := out[len(out)-1]
		if last == ' ' || last == '\n' || last == '\r' || last == '\t' {
			out = out[:len(out)-1]
			continue
		}
		break
	}
	return out
}
