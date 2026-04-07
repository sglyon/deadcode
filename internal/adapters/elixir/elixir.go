// Package elixir contains the adapter for Elixir dead-code detection.
//
// Strategy
//
// As of Elixir 1.19, ALL dead-code signal that the standard toolchain
// produces comes from `mix compile`. The set-theoretic type system
// (introduced in 1.18) catches unreachable clauses, impossible pattern
// matches, and unused private functions; `mix xref unreachable` was
// deprecated in 1.19 with the message "The unreachable check has been
// moved to the compiler and has no effect now". So this adapter shells
// out to `mix compile` once per project root and parses its warning
// stream.
//
// Compatibility
//
// Tested on Elixir 1.19 / OTP 28. The warning format is the box-drawn
// multi-line diagnostic introduced in 1.18. The parser should work on
// any Elixir >= 1.18 but is not guaranteed against future format
// changes — pin via expected-mix-compile.txt regression fixtures.
//
// Known v0.4 limitation
//
// Built-in tools do NOT catch unused PUBLIC functions across modules
// (the famous "mix xref unreachable" gap). Users who need this should
// add the optional `mix_unused` dev dependency to their project; its
// warnings flow through `mix compile` and our parser will pick them
// up automatically. The Phoenix/OTP default ignore list (see
// defaults.go) is in place for the day mix_unused lights it up.
package elixir

import (
	"context"
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

type Elixir struct{}

func New() *Elixir { return &Elixir{} }

func (e *Elixir) Name() string        { return "elixir" }
func (e *Elixir) Languages() []string { return []string{"elixir"} }

func (e *Elixir) Check(ctx context.Context) error {
	if _, err := exec.LookPath("mix"); err != nil {
		return fmt.Errorf("mix not found in PATH (install Elixir from https://elixir-lang.org/install.html)")
	}
	if _, err := exec.LookPath("elixir"); err != nil {
		return fmt.Errorf("elixir not found in PATH (install Elixir from https://elixir-lang.org/install.html)")
	}
	c := exec.CommandContext(ctx, "elixir", "--version")
	if err := c.Run(); err != nil {
		return fmt.Errorf("elixir is reachable but `elixir --version` failed: %w", err)
	}
	return nil
}

func (e *Elixir) Run(ctx context.Context, paths []string, opts adapter.RunOptions) ([]finding.Finding, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	roots := projectRoots(paths)
	if len(roots) == 0 {
		return nil, nil // no mix.exs anywhere — nothing to do, not an error
	}

	var allFindings []finding.Finding
	for _, root := range roots {
		findings, err := compileAndParse(ctx, root, opts)
		if err != nil {
			return nil, err
		}
		allFindings = append(allFindings, findings...)
	}
	return applyDefaultIgnores(allFindings), nil
}

// compileAndParse runs `mix compile` once in the given project root and
// turns the captured warning stream into Findings.
//
// We pass --force to ensure warnings are always re-emitted; mix's
// incremental compiler suppresses warnings on unchanged files which
// would silently hide findings on a second run.
func compileAndParse(ctx context.Context, root string, opts adapter.RunOptions) ([]finding.Finding, error) {
	if err := requireBuildArtifacts(root); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "mix", "compile", "--force")
	cmd.Dir = root
	// mix writes warnings to STDERR via the standard logger; capture
	// the merged stream so we get everything in order.
	combined, runErr := cmd.CombinedOutput()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			// mix compile exits 0 on warning; non-zero means a real
			// build failure (syntax error, missing dep, etc.). Return
			// an actionable message in that case.
			return nil, mixExecError(root, exitErr.ExitCode(), combined)
		}
		return nil, fmt.Errorf("mix compile failed in %s: %w", root, runErr)
	}
	// Scope the parser to this project's own app section so we don't
	// surface warnings from the hex deps mix re-compiles.
	appName := projectAppName(root)
	return parseMixCompileOutput(combined, root, appName, opts), nil
}

// requireBuildArtifacts mirrors knip's "node_modules missing" friendly
// error: if the project hasn't compiled at least once, `mix compile`
// can run for minutes while it pulls hex packages. Catch it early.
//
// We only check _build/ — `mix compile` itself handles missing deps
// gracefully and will refuse to run with a clear error if deps are
// missing. We don't pre-check deps/ because projects with no
// dependencies (like our test fixture) don't have a deps/ directory
// at all.
func requireBuildArtifacts(root string) error {
	build := filepath.Join(root, "_build")
	if _, err := os.Stat(build); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf(
			"mix project at %s has no _build/ directory. "+
				"Run `mix deps.get && mix compile` in %s and re-scan "+
				"(first compile may take a minute)",
			root, root,
		)
	}
	return nil
}

// mixExecError translates a mix non-zero exit into an actionable
// message, distinguishing the common "missing deps" failure from a
// genuine compilation error.
func mixExecError(root string, exitCode int, combined []byte) error {
	out := strings.TrimSpace(string(combined))
	if strings.Contains(out, "could not find") || strings.Contains(out, "Unchecked dependencies") {
		return fmt.Errorf(
			"mix compile failed in %s (exit %d): the project's dependencies are missing or stale. "+
				"Run `mix deps.get && mix compile` in %s and re-scan. "+
				"Original error:\n%s",
			root, exitCode, root, out,
		)
	}
	return fmt.Errorf("mix compile failed in %s (exit %d):\n%s", root, exitCode, out)
}

// projectRoots walks upward from each scan path looking for the
// nearest mix.exs. Dedupes so a multi-path scan inside one project
// only runs mix once.
func projectRoots(paths []string) []string {
	seen := map[string]bool{}
	var roots []string
	for _, p := range paths {
		root := findMixExs(p)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

func findMixExs(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	dir := abs
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "mix.exs")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
