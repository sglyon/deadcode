package report

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/sglyon/deadcode/internal/finding"
	"github.com/sglyon/deadcode/internal/runner"
)

// Pretty renders a Lipgloss-styled report. Layout works regardless of
// color support; useColor=false uses the same boxes and tables but
// strips ANSI escapes (so piped output stays tidy).
//
// Findings are grouped by file so the visual noise of repeated paths
// disappears. The summary header is boxed; high-confidence findings
// (>=0.85) get a marker so the eye lands on them first.
func Pretty(w io.Writer, r *runner.Result, showIgnored, useColor bool) error {
	r2 := lipgloss.NewRenderer(w)
	if !useColor {
		r2.SetColorProfile(termenv.Ascii)
	}
	s := newPrettyStyles(r2)

	if err := writeHeader(w, r, s); err != nil {
		return err
	}
	if len(r.Findings) > 0 {
		writeFindings(w, r.Findings, s)
	} else {
		fmt.Fprintln(w, s.dim.Render("  no findings"))
		fmt.Fprintln(w)
	}
	if showIgnored && len(r.Ignored) > 0 {
		writeIgnored(w, r.Ignored, s)
	}
	if len(r.ToolsUnavailable) > 0 {
		writeUnavailable(w, r.ToolsUnavailable, s)
	}
	return nil
}

// prettyStyles bundles every styled element so call sites stay tidy
// and we never accidentally use a default-renderer style.
type prettyStyles struct {
	headerBox     lipgloss.Style
	headerTitle   lipgloss.Style
	statBold      lipgloss.Style
	statLabel     lipgloss.Style
	statSep       lipgloss.Style
	ignoreLine    lipgloss.Style
	fileHeader    lipgloss.Style
	lineNum       lipgloss.Style
	kindTag       lipgloss.Style
	confHigh      lipgloss.Style
	confMed       lipgloss.Style
	confLow       lipgloss.Style
	symbol        lipgloss.Style
	highlightMark lipgloss.Style
	dim           lipgloss.Style
	ignoredHeader lipgloss.Style
	ignoredItem   lipgloss.Style
	ignoredReason lipgloss.Style
	warnHeader    lipgloss.Style
	warnItem      lipgloss.Style
}

func newPrettyStyles(r *lipgloss.Renderer) prettyStyles {
	mk := r.NewStyle
	// Palette: cyan/blue accents, yellow/red severity, dim greys for chrome.
	cyan := lipgloss.Color("14")
	cyanDim := lipgloss.Color("37")
	green := lipgloss.Color("10")
	yellow := lipgloss.Color("11")
	red := lipgloss.Color("9")
	magenta := lipgloss.Color("13")
	grey := lipgloss.Color("241")

	return prettyStyles{
		headerBox: mk().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cyanDim).
			Padding(0, 1),
		headerTitle:   mk().Bold(true).Foreground(cyan),
		statBold:      mk().Bold(true),
		statLabel:     mk().Foreground(grey),
		statSep:       mk().Foreground(grey),
		ignoreLine:    mk().Foreground(grey).Italic(true),
		fileHeader:    mk().Bold(true).Foreground(cyan),
		lineNum:       mk().Foreground(grey),
		kindTag:       mk().Foreground(magenta),
		confHigh:      mk().Bold(true).Foreground(green),
		confMed:       mk().Foreground(yellow),
		confLow:       mk().Foreground(grey),
		symbol:        mk().Bold(true),
		highlightMark: mk().Foreground(red).Bold(true),
		dim:           mk().Foreground(grey),
		ignoredHeader: mk().Bold(true).Foreground(grey),
		ignoredItem:   mk().Foreground(grey),
		ignoredReason: mk().Foreground(grey).Italic(true),
		warnHeader:    mk().Bold(true).Foreground(yellow),
		warnItem:      mk().Foreground(yellow),
	}
}

func writeHeader(w io.Writer, r *runner.Result, s prettyStyles) error {
	title := s.headerTitle.Render(fmt.Sprintf("deadcode  %d findings", len(r.Findings)))

	stats := []string{
		formatStat(s, fmt.Sprintf("%d", len(r.Ignored)), "ignored"),
		formatStat(s, fmt.Sprintf("%d", r.FilesScanned), "files"),
		formatStat(s, joinOrNone(r.LanguagesPresent), ""),
		formatStat(s, fmt.Sprintf("%dms", r.DurationMs), ""),
	}
	statsLine := strings.Join(stats, s.statSep.Render("  ·  "))

	body := title + "\n" + statsLine
	if r.IgnoreFile != "" {
		body += "\n" + s.ignoreLine.Render("ignore: "+shortenPath(r.IgnoreFile))
	}
	fmt.Fprintln(w, s.headerBox.Render(body))
	fmt.Fprintln(w)
	return nil
}

func formatStat(s prettyStyles, value, label string) string {
	v := s.statBold.Render(value)
	if label == "" {
		return v
	}
	return v + " " + s.statLabel.Render(label)
}

func writeFindings(w io.Writer, findings []finding.Finding, s prettyStyles) {
	groups := groupByFile(findings)
	maxLineWidth, maxKindWidth := columnWidths(findings)

	for i, file := range groups.order {
		fmt.Fprintln(w, "  "+s.fileHeader.Render(shortenPath(file)))
		for _, f := range groups.byFile[file] {
			line := fmt.Sprintf("%*d", maxLineWidth, f.Line)
			kind := fmt.Sprintf("%-*s", maxKindWidth, string(f.Kind))
			conf := fmt.Sprintf("%3.0f%%", f.Confidence*100)

			confStyled := pickConfStyle(s, f.Confidence).Render(conf)
			highlight := ""
			if f.Confidence >= 0.85 {
				highlight = "  " + s.highlightMark.Render("← high confidence")
			}
			fmt.Fprintf(w, "    %s  %s  %s  %s%s\n",
				s.lineNum.Render(line),
				s.kindTag.Render(kind),
				confStyled,
				s.symbol.Render(f.Symbol),
				highlight,
			)
		}
		if i < len(groups.order)-1 {
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintln(w)
}

func writeIgnored(w io.Writer, ignored []finding.IgnoredFinding, s prettyStyles) {
	fmt.Fprintln(w, s.ignoredHeader.Render(fmt.Sprintf("ignored (%d)", len(ignored))))
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
		fmt.Fprintf(w, "  %s  %s\n",
			s.ignoredHeader.Render(fmt.Sprintf("rule #%d", idx)),
			s.ignoredReason.Render(fmt.Sprintf("(%d) %s", len(group), group[0].IgnoreReason)),
		)
	}
	fmt.Fprintln(w)
}

func writeUnavailable(w io.Writer, items []string, s prettyStyles) {
	fmt.Fprintln(w, s.warnHeader.Render("tools unavailable:"))
	for _, t := range items {
		fmt.Fprintln(w, "  "+s.warnItem.Render("- "+t))
	}
	fmt.Fprintln(w)
}

// fileGroups preserves the order findings were encountered (which the
// runner has already sorted by file,line) so the report stays stable.
type fileGroups struct {
	order  []string
	byFile map[string][]finding.Finding
}

func groupByFile(findings []finding.Finding) fileGroups {
	g := fileGroups{byFile: map[string][]finding.Finding{}}
	for _, f := range findings {
		if _, seen := g.byFile[f.File]; !seen {
			g.order = append(g.order, f.File)
		}
		g.byFile[f.File] = append(g.byFile[f.File], f)
	}
	return g
}

func columnWidths(findings []finding.Finding) (lineW, kindW int) {
	for _, f := range findings {
		if w := len(fmt.Sprintf("%d", f.Line)); w > lineW {
			lineW = w
		}
		if w := len(string(f.Kind)); w > kindW {
			kindW = w
		}
	}
	if lineW < 4 {
		lineW = 4
	}
	return
}

func pickConfStyle(s prettyStyles, c float64) lipgloss.Style {
	switch {
	case c >= 0.85:
		return s.confHigh
	case c >= 0.60:
		return s.confMed
	default:
		return s.confLow
	}
}

// shortenPath strips the cwd prefix when present so paths are readable
// without losing the absolute reference. Falls back to the input on any
// error.
func shortenPath(p string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return p
	}
	rel := strings.TrimPrefix(p, cwd)
	if rel == p {
		return p
	}
	return strings.TrimPrefix(rel, "/")
}
