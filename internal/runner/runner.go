// Package runner walks the target paths, detects which languages are
// present, and dispatches the matching adapters in parallel.
package runner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/finding"
	"github.com/sglyon/deadcode/internal/ignore"
)

// Options controls a Run invocation.
type Options struct {
	Paths               []string
	Languages           []string // restrict to these languages; empty = all
	Kinds               []finding.Kind
	MinConfidence       float64
	ExcludeTests        bool
	IgnoreDecorators    []string
	NoDefaultDecorators bool
	IgnoreRules         *ignore.Ruleset // nil means no ignore filtering
	Verbose             bool
}

// Result is what Run returns. Findings are sorted (file, line) for
// deterministic output.
type Result struct {
	Findings         []finding.Finding
	Ignored          []finding.IgnoredFinding
	LanguagesPresent []string
	FilesScanned     int
	ToolsRun         []string
	ToolsUnavailable []string
	IgnoreFile       string
	DurationMs       int64
}

// Run dispatches adapters and returns merged, filtered, sorted findings.
func Run(ctx context.Context, adapters []adapter.Adapter, opts Options) (*Result, error) {
	start := time.Now()

	languages, filesByLang, err := scanPaths(opts.Paths)
	if err != nil {
		return nil, err
	}

	wantLang := make(map[string]bool, len(opts.Languages))
	for _, l := range opts.Languages {
		wantLang[l] = true
	}

	var (
		wg               sync.WaitGroup
		mu               sync.Mutex
		allFindings      []finding.Finding
		toolsRun         []string
		toolsUnavailable []string
	)

	for _, a := range adapters {
		// Skip adapters whose languages aren't present in the scan.
		if !adapterRelevant(a, languages) {
			continue
		}
		// Honor --lang restriction.
		if len(wantLang) > 0 && !adapterMatchesFilter(a, wantLang) {
			continue
		}

		if err := a.Check(ctx); err != nil {
			mu.Lock()
			toolsUnavailable = append(toolsUnavailable, a.Name()+": "+err.Error())
			mu.Unlock()
			continue
		}

		wg.Add(1)
		go func(a adapter.Adapter) {
			defer wg.Done()
			fs, err := a.Run(ctx, opts.Paths, adapter.RunOptions{
				ExcludeTests:        opts.ExcludeTests,
				IgnoreDecorators:    opts.IgnoreDecorators,
				NoDefaultDecorators: opts.NoDefaultDecorators,
				Verbose:             opts.Verbose,
			})
			mu.Lock()
			defer mu.Unlock()
			toolsRun = append(toolsRun, a.Name())
			if err != nil {
				toolsUnavailable = append(toolsUnavailable, a.Name()+": run error: "+err.Error())
				return
			}
			allFindings = append(allFindings, fs...)
		}(a)
	}
	wg.Wait()

	allFindings = filterFindings(allFindings, opts)
	sortFindings(allFindings)

	if opts.IgnoreRules != nil {
		opts.IgnoreRules.MatchRoots = absPaths(opts.Paths)
	}
	kept, ignored := applyIgnoreRules(allFindings, opts.IgnoreRules)

	totalFiles := 0
	for _, n := range filesByLang {
		totalFiles += n
	}

	ignoreFilePath := ""
	if opts.IgnoreRules != nil {
		ignoreFilePath = opts.IgnoreRules.Path
	}

	return &Result{
		Findings:         kept,
		Ignored:          ignored,
		LanguagesPresent: languages,
		FilesScanned:     totalFiles,
		ToolsRun:         toolsRun,
		ToolsUnavailable: toolsUnavailable,
		IgnoreFile:       ignoreFilePath,
		DurationMs:       time.Since(start).Milliseconds(),
	}, nil
}

// absPaths converts each path to its absolute form, dropping any that
// fail, then enriches the result with each path's discovered project
// root (so a glob like "src/models/**.py" works whether the user scans
// the whole repo or just src/models). The matcher tries each candidate
// in turn — false positives are extremely unlikely because all matchers
// on a rule must AND together.
func absPaths(paths []string) []string {
	seen := make(map[string]bool, 2*len(paths))
	out := make([]string, 0, 2*len(paths))
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		add(abs)
		if root := projectRoot(abs); root != "" {
			add(root)
		}
	}
	return out
}

// projectRoot walks upward from start looking for the nearest ancestor
// containing one of the well-known project markers. Returns "" if none
// found before hitting the filesystem root.
func projectRoot(start string) string {
	markers := []string{".git", "pyproject.toml", "package.json", "Cargo.toml", "go.mod"}
	dir := start
	if info, err := os.Stat(start); err == nil && !info.IsDir() {
		dir = filepath.Dir(start)
	}
	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// applyIgnoreRules splits findings into (kept, ignored) using the ruleset.
// If rules is nil or empty, all findings are kept.
func applyIgnoreRules(findings []finding.Finding, rules *ignore.Ruleset) ([]finding.Finding, []finding.IgnoredFinding) {
	if rules == nil || rules.Empty() {
		return findings, nil
	}
	kept := findings[:0]
	var ignored []finding.IgnoredFinding
	for _, f := range findings {
		idx := rules.Match(f)
		if idx < 0 {
			kept = append(kept, f)
			continue
		}
		ignored = append(ignored, finding.IgnoredFinding{
			Finding:      f,
			IgnoreReason: rules.Rules[idx].Reason,
			MatchedRule:  idx,
		})
	}
	return kept, ignored
}

// scanPaths walks the given roots and returns the set of languages present
// and a per-language file count. v0.1 only knows Python — extend as adapters
// grow.
func scanPaths(roots []string) ([]string, map[string]int, error) {
	counts := make(map[string]int)
	skipDirs := map[string]bool{
		".git":         true,
		"node_modules": true,
		"dist":         true,
		"build":        true,
		".next":        true,
		".nuxt":        true,
		".turbo":       true,
		".venv":        true,
		"venv":         true,
		"__pycache__":  true,
		"vendor":       true,
		".tox":         true,
		"_build":       true,
		".elixir_ls":   true,
		"testdata":     true,
	}

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // best-effort: skip unreadable entries
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			lang := languageFor(d.Name())
			if lang != "" {
				counts[lang]++
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}

	langs := make([]string, 0, len(counts))
	for l := range counts {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	return langs, counts, nil
}

func languageFor(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".py":
		return "python"
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ex", ".exs":
		return "elixir"
	case ".go":
		return "go"
	}
	return ""
}

func adapterRelevant(a adapter.Adapter, present []string) bool {
	set := make(map[string]bool, len(present))
	for _, p := range present {
		set[p] = true
	}
	for _, l := range a.Languages() {
		if set[l] {
			return true
		}
	}
	return false
}

func adapterMatchesFilter(a adapter.Adapter, filter map[string]bool) bool {
	for _, l := range a.Languages() {
		if filter[l] {
			return true
		}
	}
	return false
}

func filterFindings(findings []finding.Finding, opts Options) []finding.Finding {
	wantKind := make(map[finding.Kind]bool, len(opts.Kinds))
	for _, k := range opts.Kinds {
		wantKind[k] = true
	}
	out := findings[:0]
	for _, f := range findings {
		if opts.MinConfidence > 0 && f.Confidence < opts.MinConfidence {
			continue
		}
		if len(wantKind) > 0 && !wantKind[f.Kind] {
			continue
		}
		out = append(out, f)
	}
	return out
}

func sortFindings(findings []finding.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Symbol < findings[j].Symbol
	})
}

// SplitCSV is a tiny helper used by the CLI flag parsing layer to split
// comma-separated lists like `python,typescript` into clean entries.
func SplitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
