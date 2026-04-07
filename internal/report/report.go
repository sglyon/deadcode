// Package report renders runner Results into the formats deadcode supports.
// v0.1: JSON (the canonical format) and console (human-readable).
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/sglyon/deadcode/internal/finding"
	"github.com/sglyon/deadcode/internal/runner"
)

// JSON writes the report as the canonical schema document.
func JSON(w io.Writer, r *runner.Result) error {
	doc := finding.Report{
		SchemaVersion: finding.SchemaVersion,
		Summary: finding.Summary{
			Languages:        r.LanguagesPresent,
			FilesScanned:     r.FilesScanned,
			FindingsTotal:    len(r.Findings),
			ToolsRun:         dedupSorted(r.ToolsRun),
			ToolsUnavailable: r.ToolsUnavailable,
			DurationMs:       r.DurationMs,
		},
		Findings: r.Findings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// Console writes a compact human-readable report. Use Verbose for the
// full evidence dump.
func Console(w io.Writer, r *runner.Result, verbose bool) error {
	if len(r.Findings) == 0 {
		fmt.Fprintf(w, "deadcode: no findings (%d files, %s, %dms)\n",
			r.FilesScanned, joinOrNone(r.LanguagesPresent), r.DurationMs)
		printUnavailable(w, r)
		return nil
	}

	fmt.Fprintf(w, "deadcode: %d finding(s) across %d file(s) (%s, %dms)\n\n",
		len(r.Findings), countFiles(r.Findings), joinOrNone(r.LanguagesPresent), r.DurationMs)

	for _, f := range r.Findings {
		fmt.Fprintf(w, "  %s:%d  [%s %s %.0f%%]  %s\n",
			f.File, f.Line, f.Tool, f.Kind, f.Confidence*100, f.Message)
		if verbose && f.Evidence["tool_raw"] != "" {
			fmt.Fprintf(w, "    raw: %s\n", f.Evidence["tool_raw"])
		}
	}
	fmt.Fprintln(w)
	printUnavailable(w, r)
	return nil
}

func printUnavailable(w io.Writer, r *runner.Result) {
	if len(r.ToolsUnavailable) == 0 {
		return
	}
	fmt.Fprintln(w, "tools unavailable:")
	for _, t := range r.ToolsUnavailable {
		fmt.Fprintf(w, "  - %s\n", t)
	}
}

func countFiles(findings []finding.Finding) int {
	seen := map[string]bool{}
	for _, f := range findings {
		seen[f.File] = true
	}
	return len(seen)
}

func joinOrNone(s []string) string {
	if len(s) == 0 {
		return "no recognized languages"
	}
	return strings.Join(s, ",")
}

func dedupSorted(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
