package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sglyon/deadcode/internal/finding"
)

func TestMarkdownBasic(t *testing.T) {
	r := sampleResult() // 90% + 60% + 60% — has contrast
	var buf bytes.Buffer
	if err := Markdown(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	// Header
	if !strings.Contains(out, "# deadcode report") {
		t.Errorf("missing top-level heading:\n%s", out)
	}
	if !strings.Contains(out, "**3 findings**") {
		t.Errorf("missing finding count in header:\n%s", out)
	}
	if !strings.Contains(out, "python") {
		t.Errorf("missing language in header:\n%s", out)
	}

	// Per-language section
	if !strings.Contains(out, "## python") {
		t.Errorf("missing per-language section:\n%s", out)
	}

	// Table header
	if !strings.Contains(out, "| File | Line | Kind | Confidence | Symbol |") {
		t.Errorf("missing table header:\n%s", out)
	}

	// Bolding fires on the 90% row (contrast exists)
	if !strings.Contains(out, "**90%**") {
		t.Errorf("expected bolded 90%% row in contrast report:\n%s", out)
	}
	if !strings.Contains(out, "| 60% |") {
		t.Errorf("60%% rows should NOT be bolded:\n%s", out)
	}

	// Each finding is present by symbol
	for _, sym := range []string{"json", "helper", "Old"} {
		if !strings.Contains(out, "`"+sym+"`") {
			t.Errorf("missing symbol %q in code span:\n%s", sym, out)
		}
	}
}

func TestMarkdownNoBoldingWhenUniform(t *testing.T) {
	// Force every finding to 95% so there's no contrast — bolding
	// should be suppressed.
	r := sampleResult()
	for i := range r.Findings {
		r.Findings[i].Confidence = 0.95
	}
	var buf bytes.Buffer
	if err := Markdown(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "**95%**") {
		t.Errorf("expected no bolding on uniform-high report:\n%s", out)
	}
	// The percentage itself should still appear, just unbolded.
	if !strings.Contains(out, "| 95% |") {
		t.Errorf("expected unbolded 95%% rows:\n%s", out)
	}
}

func TestMarkdownToolsColumnOnlyWhenMultiTool(t *testing.T) {
	// Single-tool report → no Tools column.
	r := sampleResult()
	var buf bytes.Buffer
	if err := Markdown(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "| File | Line | Kind | Confidence | Symbol | Tools |") {
		t.Errorf("Tools column should be hidden on single-tool report")
	}

	// Multi-tool report → Tools column appears.
	r2 := sampleResult()
	r2.Findings[0].Tools = []string{"toolA", "toolB"}
	buf.Reset()
	if err := Markdown(&buf, r2, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "| File | Line | Kind | Confidence | Symbol | Tools |") {
		t.Errorf("Tools column should appear on multi-tool report:\n%s", out)
	}
	if !strings.Contains(out, "toolA, toolB") {
		t.Errorf("expected joined tools list:\n%s", out)
	}
}

func TestMarkdownEmptyFindings(t *testing.T) {
	r := sampleResult()
	r.Findings = nil
	var buf bytes.Buffer
	if err := Markdown(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "_no findings_") {
		t.Errorf("expected 'no findings' marker:\n%s", out)
	}
	if !strings.Contains(out, "**0 findings**") {
		t.Errorf("header should still show count:\n%s", out)
	}
}

func TestMarkdownIgnoredFolded(t *testing.T) {
	r := sampleResult()
	r.Ignored = []finding.IgnoredFinding{
		{
			Finding: finding.Finding{
				File: "/r/m.py", Line: 1, Symbol: "x",
				Kind: finding.KindUnusedField, Language: "python", Tool: "vulture",
			},
			IgnoreReason: "ORM column",
			MatchedRule:  2,
		},
	}
	var buf bytes.Buffer
	if err := Markdown(&buf, r, true); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "<details><summary>1 ignored findings</summary>") {
		t.Errorf("expected folded details block:\n%s", out)
	}
	if !strings.Contains(out, "**rule #2**") {
		t.Errorf("expected rule attribution:\n%s", out)
	}
	if !strings.Contains(out, "ORM column") {
		t.Errorf("expected reason in output:\n%s", out)
	}
	if !strings.Contains(out, "</details>") {
		t.Errorf("expected closing details tag:\n%s", out)
	}
}

func TestMarkdownIgnoredHiddenWithoutFlag(t *testing.T) {
	r := sampleResult()
	r.Ignored = []finding.IgnoredFinding{
		{
			Finding:      finding.Finding{File: "/r/m.py", Line: 1, Symbol: "x"},
			IgnoreReason: "x",
		},
	}
	var buf bytes.Buffer
	if err := Markdown(&buf, r, false); err != nil { // showIgnored=false
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "<details>") {
		t.Errorf("ignored block should be hidden when showIgnored=false")
	}
	// But the suppression note in the header should still appear.
	if !strings.Contains(buf.String(), "1 findings suppressed") {
		t.Errorf("header should still note the suppression count")
	}
}

func TestMarkdownPipeEscaping(t *testing.T) {
	// Defensive: a symbol or path containing a pipe character would
	// break the table. Escape it.
	r := sampleResult()
	r.Findings[0].Symbol = "weird|name"
	r.Findings[0].File = "/r/has|pipe.py"
	var buf bytes.Buffer
	if err := Markdown(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "weird\\|name") {
		t.Errorf("expected escaped pipe in symbol:\n%s", out)
	}
	if !strings.Contains(out, "has\\|pipe.py") {
		t.Errorf("expected escaped pipe in file path:\n%s", out)
	}
}
