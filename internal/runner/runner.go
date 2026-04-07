// Package runner walks the target paths, detects which languages are
// present, and dispatches the matching adapters in parallel.
package runner

import (
	"context"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/finding"
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
	Verbose             bool
}

// Result is what Run returns. Findings are sorted (file, line) for
// deterministic output.
type Result struct {
	Findings         []finding.Finding
	LanguagesPresent []string
	FilesScanned     int
	ToolsRun         []string
	ToolsUnavailable []string
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

	totalFiles := 0
	for _, n := range filesByLang {
		totalFiles += n
	}

	return &Result{
		Findings:         allFindings,
		LanguagesPresent: languages,
		FilesScanned:     totalFiles,
		ToolsRun:         toolsRun,
		ToolsUnavailable: toolsUnavailable,
		DurationMs:       time.Since(start).Milliseconds(),
	}, nil
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
		".venv":        true,
		"venv":         true,
		"__pycache__":  true,
		"vendor":       true,
		".tox":         true,
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
	switch filepath.Ext(filename) {
	case ".py":
		return "python"
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

// uniqueLanguages is a small helper used by callers building summaries.
func UniqueLanguages(findings []finding.Finding) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range findings {
		if !seen[f.Language] {
			seen[f.Language] = true
			out = append(out, f.Language)
		}
	}
	sort.Strings(out)
	return out
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
