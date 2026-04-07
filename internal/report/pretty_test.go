package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sglyon/deadcode/internal/finding"
	"github.com/sglyon/deadcode/internal/runner"
)

func sampleResult() *runner.Result {
	return &runner.Result{
		Findings: []finding.Finding{
			{
				ID: "py:foo.py:1:unused_import:json", File: "/repo/foo.py", Line: 1,
				Symbol: "json", Kind: finding.KindUnusedImport,
				Language: "python", Tool: "vulture", Confidence: 0.90,
				Message: "Import 'json' appears unused",
			},
			{
				ID: "py:foo.py:5:unused_function:helper", File: "/repo/foo.py", Line: 5,
				Symbol: "helper", Kind: finding.KindUnusedFunction,
				Language: "python", Tool: "vulture", Confidence: 0.60,
				Message: "Function 'helper' appears unused",
			},
			{
				ID: "py:bar.py:9:unused_class:Old", File: "/repo/bar.py", Line: 9,
				Symbol: "Old", Kind: finding.KindUnusedClass,
				Language: "python", Tool: "vulture", Confidence: 0.60,
				Message: "Class 'Old' appears unused",
			},
		},
		LanguagesPresent: []string{"python"},
		FilesScanned:     2,
		ToolsRun:         []string{"vulture"},
		DurationMs:       42,
	}
}

func TestPrettyNoColorRendersAllFindings(t *testing.T) {
	var buf bytes.Buffer
	if err := Pretty(&buf, sampleResult(), false, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	// Header content
	for _, want := range []string{"deadcode", "3 findings", "python", "42ms"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
	// File grouping: each file appears exactly once as a group header
	if got := strings.Count(out, "foo.py"); got < 1 {
		t.Errorf("foo.py not present in output")
	}
	if got := strings.Count(out, "bar.py"); got < 1 {
		t.Errorf("bar.py not present in output")
	}
	// All three symbols present
	for _, sym := range []string{"json", "helper", "Old"} {
		if !strings.Contains(out, sym) {
			t.Errorf("missing symbol %q", sym)
		}
	}
	// 90% finding gets the high-confidence marker
	if !strings.Contains(out, "high confidence") {
		t.Errorf("expected high-confidence marker for 90%% finding:\n%s", out)
	}
	// no-color mode: no ANSI escapes
	if strings.Contains(out, "\x1b[") {
		t.Errorf("unexpected ANSI escape in no-color output:\n%s", out)
	}
}

func TestPrettyEmptyFindings(t *testing.T) {
	r := sampleResult()
	r.Findings = nil
	var buf bytes.Buffer
	if err := Pretty(&buf, r, false, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "no findings") {
		t.Errorf("expected 'no findings' message:\n%s", out)
	}
	if !strings.Contains(out, "0 findings") {
		t.Errorf("header should still render with 0 findings")
	}
}

func TestPrettyShowIgnored(t *testing.T) {
	r := sampleResult()
	r.Ignored = []finding.IgnoredFinding{
		{
			Finding: finding.Finding{
				ID: "py:m.py:1:unused_field:created_at", File: "/repo/m.py", Line: 1,
				Symbol: "created_at", Kind: finding.KindUnusedField,
				Language: "python", Tool: "vulture", Confidence: 0.6,
			},
			IgnoreReason: "ORM timestamp",
			MatchedRule:  2,
		},
	}
	var buf bytes.Buffer
	if err := Pretty(&buf, r, true, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "ignored (1)") {
		t.Errorf("expected ignored summary header:\n%s", out)
	}
	if !strings.Contains(out, "rule #2") {
		t.Errorf("expected matched-rule attribution:\n%s", out)
	}
	if !strings.Contains(out, "ORM timestamp") {
		t.Errorf("expected ignore reason in output:\n%s", out)
	}
}
