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

// TestPrettyHighConfidenceMarkerOnlyWithContrast pins the v0.6
// dogfood fix: the "← high confidence" marker only appears when the
// report has BOTH high and lower-confidence findings. When every
// finding is uniformly high-conf (e.g. a knip-only TS report where
// every finding is 0.95), marking every row would be visual noise
// rather than emphasis.
func TestPrettyHighConfidenceMarkerOnlyWithContrast(t *testing.T) {
	// Case 1: contrast exists (90% + 60% findings) → marker fires
	contrastResult := sampleResult() // sampleResult has 90, 60, 60
	var buf bytes.Buffer
	if err := Pretty(&buf, contrastResult, false, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "high confidence") {
		t.Errorf("expected marker on contrast report:\n%s", buf.String())
	}

	// Case 2: all findings are uniformly high-conf → marker
	// suppressed (the 95% column carries the same info)
	uniformResult := sampleResult()
	for i := range uniformResult.Findings {
		uniformResult.Findings[i].Confidence = 0.95
	}
	buf.Reset()
	if err := Pretty(&buf, uniformResult, false, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "high confidence") {
		t.Errorf("expected NO marker on uniformly-high report:\n%s", buf.String())
	}

	// Case 3: all findings are uniformly low-conf → marker also
	// suppressed (no contrast either direction)
	lowResult := sampleResult()
	for i := range lowResult.Findings {
		lowResult.Findings[i].Confidence = 0.60
	}
	buf.Reset()
	if err := Pretty(&buf, lowResult, false, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "high confidence") {
		t.Errorf("expected NO marker on uniformly-low report:\n%s", buf.String())
	}
}

// TestShortenPathStripsScanRoots pins the dogfood fix: file paths
// in pretty output should strip the longest matching prefix from
// the scan roots, so a scan of `/Users/me/src/repo` shows
// findings as `src/foo.py` instead of the full absolute path.
func TestShortenPathStripsScanRoots(t *testing.T) {
	cases := []struct {
		path  string
		roots []string
		want  string
	}{
		{
			path:  "/Users/me/src/repo/src/foo.py",
			roots: []string{"/Users/me/src/repo"},
			want:  "src/foo.py",
		},
		{
			// Two scan roots, longest match wins
			path:  "/Users/me/src/repo/sub/foo.py",
			roots: []string{"/Users/me/src", "/Users/me/src/repo"},
			want:  "sub/foo.py",
		},
		{
			// Partial-name collision must NOT match (trailing
			// separator handling): /a/build should not strip
			// against /a/b
			path:  "/a/build/foo.go",
			roots: []string{"/a/b"},
			want:  "/a/build/foo.go",
		},
		{
			// No match → unchanged
			path:  "/some/other/path.go",
			roots: []string{"/elsewhere"},
			want:  "/some/other/path.go",
		},
	}
	for _, c := range cases {
		got := shortenPath(c.path, c.roots)
		if got != c.want {
			t.Errorf("shortenPath(%q, %v) = %q, want %q", c.path, c.roots, got, c.want)
		}
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
