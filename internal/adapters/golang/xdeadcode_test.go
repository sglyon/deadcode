package golang

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/finding"
)

// TestParseXDeadcodeFixture pins the parser against JSON captured
// from `deadcode -json ./...` running against testdata/go-sample.
// The fixture deliberately includes a library sub-package (./lib)
// with an unused exported function — the exact case staticcheck's
// U1000 misses but x/tools/cmd/deadcode catches.
func TestParseXDeadcodeFixture(t *testing.T) {
	root := repoRoot(t)
	fixturePath := filepath.Join(root, "testdata", "go-sample", "expected-xdeadcode.json")
	out, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	findings := parseXDeadcodeOutput(out, "/proj", adapter.RunOptions{})
	if got, want := len(findings), 3; got != want {
		t.Fatalf("expected %d findings, got %d:\n%+v", want, got, findings)
	}

	// Every finding is a function (the tool only reports funcs).
	for _, f := range findings {
		if f.Kind != finding.KindUnusedFunction {
			t.Errorf("kind = %s, want unused_function", f.Kind)
		}
		if f.Tool != "xdeadcode" {
			t.Errorf("tool = %s, want xdeadcode", f.Tool)
		}
		if f.Language != "go" {
			t.Errorf("language = %s, want go", f.Language)
		}
		if f.Confidence != 0.95 {
			t.Errorf("confidence = %f, want 0.95", f.Confidence)
		}
		if f.Evidence["package_path"] == "" {
			t.Errorf("missing package_path evidence: %+v", f)
		}
	}

	// Spot-check the key finding: lib.DeadExport. This is the one
	// staticcheck silently skipped (exported symbol in non-main
	// package). Its presence in this fixture proves the adapter
	// closes the blindspot.
	var deadExportFinding *finding.Finding
	for i := range findings {
		if findings[i].Symbol == "lib.DeadExport" {
			deadExportFinding = &findings[i]
			break
		}
	}
	if deadExportFinding == nil {
		t.Fatal("lib.DeadExport not present in findings — adapter doesn't close the staticcheck blindspot")
	}
	if !strings.HasSuffix(deadExportFinding.File, "lib/lib.go") {
		t.Errorf("lib.DeadExport file = %s, expected suffix lib/lib.go", deadExportFinding.File)
	}
	if deadExportFinding.Evidence["package_path"] != "example.com/gosample/lib" {
		t.Errorf("lib.DeadExport package_path = %q, want example.com/gosample/lib",
			deadExportFinding.Evidence["package_path"])
	}

	// Symbol format is pkg.Name — check all three.
	expected := map[string]bool{
		"main.unusedHelper": false,
		"main.orphanFunc":   false,
		"lib.DeadExport":    false,
	}
	for _, f := range findings {
		if _, ok := expected[f.Symbol]; ok {
			expected[f.Symbol] = true
		}
	}
	for sym, seen := range expected {
		if !seen {
			t.Errorf("expected symbol %q not found", sym)
		}
	}
}

func TestParseXDeadcodeNullOutput(t *testing.T) {
	// `deadcode -json ./...` prints `null` when there are no
	// findings. We must treat that as zero findings, not an error.
	findings := parseXDeadcodeOutput([]byte("null\n"), "/proj", adapter.RunOptions{})
	if len(findings) != 0 {
		t.Errorf("expected 0 findings on null output, got %d", len(findings))
	}
}

func TestParseXDeadcodeEmptyArray(t *testing.T) {
	findings := parseXDeadcodeOutput([]byte("[]\n"), "/proj", adapter.RunOptions{})
	if len(findings) != 0 {
		t.Errorf("expected 0 findings on empty array, got %d", len(findings))
	}
}

func TestParseXDeadcodeSkipsGeneratedAndMarker(t *testing.T) {
	// Defensive: the upstream tool already excludes generated files
	// and marker interface methods by default, but if a future
	// version ever leaks them we still filter.
	out := []byte(`[
		{
			"Name": "pkg",
			"Path": "example.com/pkg",
			"Funcs": [
				{"Name": "Real", "Position": {"File": "a.go", "Line": 1, "Col": 1}, "Generated": false, "Marker": false},
				{"Name": "FromGenFile", "Position": {"File": "gen.go", "Line": 1, "Col": 1}, "Generated": true, "Marker": false},
				{"Name": "Marker", "Position": {"File": "iface.go", "Line": 1, "Col": 1}, "Generated": false, "Marker": true}
			]
		}
	]`)
	findings := parseXDeadcodeOutput(out, "/proj", adapter.RunOptions{})
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (generated + marker filtered), got %d", len(findings))
	}
	if findings[0].Symbol != "pkg.Real" {
		t.Errorf("wrong survivor: %+v", findings[0])
	}
}

func TestParseXDeadcodeExcludeTests(t *testing.T) {
	out := []byte(`[
		{
			"Name": "pkg",
			"Path": "example.com/pkg",
			"Funcs": [
				{"Name": "Prod", "Position": {"File": "main.go", "Line": 1, "Col": 1}},
				{"Name": "Helper", "Position": {"File": "main_test.go", "Line": 1, "Col": 1}}
			]
		}
	]`)
	findings := parseXDeadcodeOutput(out, "/proj", adapter.RunOptions{ExcludeTests: true})
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding after excluding tests, got %d", len(findings))
	}
	if findings[0].Symbol != "pkg.Prod" {
		t.Errorf("wrong survivor: %+v", findings[0])
	}
}

func TestTrimJSONLeadingDiagnostic(t *testing.T) {
	// If the tool ever prints a diagnostic to stdout before the JSON
	// payload, we should still recover.
	in := []byte(`some leading noise
[{"Name": "x", "Path": "p", "Funcs": []}]
`)
	out := trimJSON(in)
	if len(out) == 0 || out[0] != '[' {
		t.Errorf("trimJSON failed to recover, got: %q", string(out))
	}
}
