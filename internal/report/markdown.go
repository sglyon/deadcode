package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/sglyon/deadcode/internal/finding"
	"github.com/sglyon/deadcode/internal/runner"
)

// Markdown writes a CommonMark-compatible report suitable for pasting
// into PR descriptions, GitHub issues, or Slack messages. The output
// is readable both as rendered markdown AND as raw text in a
// terminal — no fancy syntax that would obscure the content.
//
// Layout:
//
//	# deadcode report
//	**N findings** · M files · langs · time
//
//	> N suppressed by ignore file
//
//	## <language>
//
//	| File | Line | Kind | Confidence | Symbol |
//	(one row per finding)
//
//	## tools unavailable
//	(if any)
//
//	<details><summary>N ignored</summary>
//	(folded ignored block when --show-ignored is set)
//	</details>
//
// File paths are stripped of the scan-root prefix (same as the
// pretty reporter) so the table stays narrow. Multi-tool findings
// add a "Tools" column ONLY when at least one finding has more
// than one tool — single-tool reports stay compact.
func Markdown(w io.Writer, r *runner.Result, showIgnored bool) error {
	writeMarkdownHeader(w, r)

	if len(r.Findings) == 0 {
		fmt.Fprintln(w, "_no findings_")
		fmt.Fprintln(w)
	} else {
		writeMarkdownFindings(w, r.Findings, r.ScanRoots, shouldHighlightContrast(r.Findings))
	}

	if len(r.ToolsUnavailable) > 0 {
		writeMarkdownUnavailable(w, r.ToolsUnavailable)
	}

	if showIgnored && len(r.Ignored) > 0 {
		writeMarkdownIgnored(w, r.Ignored, r.ScanRoots)
	}

	return nil
}

func writeMarkdownHeader(w io.Writer, r *runner.Result) {
	fmt.Fprintln(w, "# deadcode report")
	fmt.Fprintln(w)

	parts := []string{
		fmt.Sprintf("**%d findings**", len(r.Findings)),
		fmt.Sprintf("%d files", r.FilesScanned),
		joinOrNone(r.LanguagesPresent),
		fmt.Sprintf("%dms", r.DurationMs),
	}
	fmt.Fprintln(w, strings.Join(parts, " · "))
	fmt.Fprintln(w)

	if len(r.Ignored) > 0 {
		ignoreFile := r.IgnoreFile
		if ignoreFile != "" {
			ignoreFile = " by `" + shortenPath(ignoreFile, r.ScanRoots) + "`"
		}
		fmt.Fprintf(w, "> %d findings suppressed%s\n\n", len(r.Ignored), ignoreFile)
	}
}

func writeMarkdownFindings(w io.Writer, findings []finding.Finding, scanRoots []string, highlightContrast bool) {
	// Group by language so multi-language scans get distinct sections.
	groups := groupByLanguage(findings)

	// Decide whether to include a Tools column. Only show it when at
	// least one finding in the entire set has multi-tool agreement,
	// otherwise the column is dead weight on the common case.
	includeToolsCol := false
	for _, f := range findings {
		if len(f.Tools) > 1 {
			includeToolsCol = true
			break
		}
	}

	for _, lang := range groups.order {
		fmt.Fprintf(w, "## %s\n\n", lang)
		writeMarkdownTable(w, groups.byLang[lang], scanRoots, includeToolsCol, highlightContrast)
		fmt.Fprintln(w)
	}
}

func writeMarkdownTable(w io.Writer, findings []finding.Finding, scanRoots []string, includeTools, highlightContrast bool) {
	if includeTools {
		fmt.Fprintln(w, "| File | Line | Kind | Confidence | Symbol | Tools |")
		fmt.Fprintln(w, "|---|---|---|---|---|---|")
	} else {
		fmt.Fprintln(w, "| File | Line | Kind | Confidence | Symbol |")
		fmt.Fprintln(w, "|---|---|---|---|---|")
	}
	for _, f := range findings {
		file := mdEscape(shortenPath(f.File, scanRoots))
		conf := fmt.Sprintf("%.0f%%", f.Confidence*100)
		// Bold high-confidence rows so they jump off the page in
		// rendered markdown — but ONLY when there's contrast in
		// the report. When every row is uniformly high-conf,
		// bolding everything is noise (the % column already says
		// so). Same conditional-on-contrast logic as the pretty
		// reporter.
		confCell := conf
		if highlightContrast && f.Confidence >= 0.85 {
			confCell = "**" + conf + "**"
		}
		symbol := "`" + mdEscape(f.Symbol) + "`"
		if f.Symbol == "" {
			symbol = "_(none)_"
		}
		if includeTools {
			tools := strings.Join(f.Tools, ", ")
			fmt.Fprintf(w, "| `%s` | %d | %s | %s | %s | %s |\n",
				file, f.Line, f.Kind, confCell, symbol, tools)
		} else {
			fmt.Fprintf(w, "| `%s` | %d | %s | %s | %s |\n",
				file, f.Line, f.Kind, confCell, symbol)
		}
	}
}

func writeMarkdownUnavailable(w io.Writer, items []string) {
	fmt.Fprintln(w, "## tools unavailable")
	fmt.Fprintln(w)
	for _, t := range items {
		fmt.Fprintf(w, "- %s\n", t)
	}
	fmt.Fprintln(w)
}

func writeMarkdownIgnored(w io.Writer, ignored []finding.IgnoredFinding, scanRoots []string) {
	fmt.Fprintf(w, "<details><summary>%d ignored findings</summary>\n\n", len(ignored))
	// Group by matched rule so users see WHY findings were
	// suppressed, not just THAT they were.
	byRule := map[int][]finding.IgnoredFinding{}
	var ruleOrder []int
	for _, f := range ignored {
		if _, seen := byRule[f.MatchedRule]; !seen {
			ruleOrder = append(ruleOrder, f.MatchedRule)
		}
		byRule[f.MatchedRule] = append(byRule[f.MatchedRule], f)
	}
	sort.Ints(ruleOrder)
	for _, idx := range ruleOrder {
		group := byRule[idx]
		fmt.Fprintf(w, "**rule #%d** — %s _(%d finding(s))_\n\n",
			idx, mdEscape(group[0].IgnoreReason), len(group))
		fmt.Fprintln(w, "| File | Line | Kind | Symbol |")
		fmt.Fprintln(w, "|---|---|---|---|")
		for _, f := range group {
			file := mdEscape(shortenPath(f.File, scanRoots))
			symbol := "`" + mdEscape(f.Symbol) + "`"
			fmt.Fprintf(w, "| `%s` | %d | %s | %s |\n", file, f.Line, f.Kind, symbol)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "</details>")
	fmt.Fprintln(w)
}

// langGroups preserves first-seen order so the output is stable.
type langGroups struct {
	order  []string
	byLang map[string][]finding.Finding
}

func groupByLanguage(findings []finding.Finding) langGroups {
	g := langGroups{byLang: map[string][]finding.Finding{}}
	for _, f := range findings {
		if _, seen := g.byLang[f.Language]; !seen {
			g.order = append(g.order, f.Language)
		}
		g.byLang[f.Language] = append(g.byLang[f.Language], f)
	}
	return g
}

// mdEscape escapes the small set of markdown characters that would
// break a table cell. Pipes are the most common offender (paths
// rarely contain them, but better safe). Backticks would close a
// code span; we just remove them since file paths and symbols don't
// legitimately contain them.
func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "`", "")
	return s
}
