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

// JSON writes the report as the canonical schema document. When
// showIgnored is true, the suppressed findings appear in the `ignored`
// array; otherwise only the count appears in `summary.findings_ignored`.
//
// Empty arrays are emitted as `[]`, never `null`, so downstream jq
// consumers can iterate without nil checks.
func JSON(w io.Writer, r *runner.Result, showIgnored bool) error {
	findings := r.Findings
	if findings == nil {
		findings = []finding.Finding{}
	}
	languages := r.LanguagesPresent
	if languages == nil {
		languages = []string{}
	}
	tools := dedupSorted(r.ToolsRun)
	if tools == nil {
		tools = []string{}
	}
	doc := finding.Report{
		SchemaVersion: finding.SchemaVersion,
		Summary: finding.Summary{
			Languages:        languages,
			FilesScanned:     r.FilesScanned,
			FindingsTotal:    len(findings),
			FindingsIgnored:  len(r.Ignored),
			ToolsRun:         tools,
			ToolsUnavailable: r.ToolsUnavailable,
			IgnoreFile:       r.IgnoreFile,
			DurationMs:       r.DurationMs,
		},
		Findings: findings,
	}
	if showIgnored {
		ignored := r.Ignored
		if ignored == nil {
			ignored = []finding.IgnoredFinding{}
		}
		doc.Ignored = ignored
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// Console writes a compact human-readable report. Use verbose for the
// full evidence dump and showIgnored to print suppressed findings too.
func Console(w io.Writer, r *runner.Result, verbose, showIgnored bool) error {
	ignoredNote := ""
	if len(r.Ignored) > 0 {
		ignoredNote = fmt.Sprintf(" [%d ignored]", len(r.Ignored))
	}

	if len(r.Findings) == 0 {
		fmt.Fprintf(w, "deadcode: no findings (%d files, %s, %dms)%s\n",
			r.FilesScanned, joinOrNone(r.LanguagesPresent), r.DurationMs, ignoredNote)
		printIgnored(w, r, showIgnored)
		printUnavailable(w, r)
		return nil
	}

	fmt.Fprintf(w, "deadcode: %d finding(s) across %d file(s) (%s, %dms)%s\n\n",
		len(r.Findings), countFiles(r.Findings), joinOrNone(r.LanguagesPresent), r.DurationMs, ignoredNote)

	for _, f := range r.Findings {
		fmt.Fprintf(w, "  %s:%d  [%s %s %.0f%%]  %s\n",
			f.File, f.Line, f.Tool, f.Kind, f.Confidence*100, f.Message)
		if verbose && f.Evidence["tool_raw"] != "" {
			fmt.Fprintf(w, "    raw: %s\n", f.Evidence["tool_raw"])
		}
	}
	fmt.Fprintln(w)
	printIgnored(w, r, showIgnored)
	printUnavailable(w, r)
	return nil
}

func printIgnored(w io.Writer, r *runner.Result, showIgnored bool) {
	if !showIgnored || len(r.Ignored) == 0 {
		return
	}
	fmt.Fprintf(w, "ignored (%d):\n", len(r.Ignored))
	for _, f := range r.Ignored {
		fmt.Fprintf(w, "  %s:%d  [%s %s]  %s\n", f.File, f.Line, f.Tool, f.Kind, f.Message)
		fmt.Fprintf(w, "    rule #%d: %s\n", f.MatchedRule, f.IgnoreReason)
	}
	fmt.Fprintln(w)
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
