package runner

import (
	"reflect"
	"testing"

	"github.com/sglyon/deadcode/internal/finding"
)

func mk(file string, line int, kind finding.Kind, symbol, tool string, conf float64) finding.Finding {
	return finding.Finding{
		ID:         "x:" + file + ":" + symbol,
		File:       file,
		Line:       line,
		Symbol:     symbol,
		Kind:       kind,
		Language:   "go",
		Tool:       tool,
		Confidence: conf,
		Message:    tool + " says " + symbol + " is unused",
		Evidence:   map[string]string{"src": tool + "_evidence"},
	}
}

func TestDedupeNoDuplicates(t *testing.T) {
	in := []finding.Finding{
		mk("a.go", 1, finding.KindUnusedFunction, "Foo", "staticcheck", 0.95),
		mk("b.go", 2, finding.KindUnusedFunction, "Bar", "staticcheck", 0.95),
	}
	out := dedupeFindings(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 findings unchanged, got %d", len(out))
	}
	// Single-tool findings get Tools = [Tool] populated for schema
	// consistency.
	for _, f := range out {
		if len(f.Tools) != 1 || f.Tools[0] != f.Tool {
			t.Errorf("single-tool finding should have Tools = [Tool], got %+v", f.Tools)
		}
	}
}

func TestDedupeMergesAcrossTools(t *testing.T) {
	// Two findings at the same (file, line, kind) from different
	// adapters with different symbol formats — exactly the
	// staticcheck/xdeadcode case.
	in := []finding.Finding{
		mk("main.go", 33, finding.KindUnusedFunction, "unusedHelper", "staticcheck", 0.95),
		mk("main.go", 33, finding.KindUnusedFunction, "main.unusedHelper", "xdeadcode", 0.95),
	}
	out := dedupeFindings(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 merged finding, got %d:\n%+v", len(out), out)
	}
	merged := out[0]

	// Longest symbol wins.
	if merged.Symbol != "main.unusedHelper" {
		t.Errorf("merged symbol = %q, want main.unusedHelper", merged.Symbol)
	}

	// Tools is sorted union.
	wantTools := []string{"staticcheck", "xdeadcode"}
	if !reflect.DeepEqual(merged.Tools, wantTools) {
		t.Errorf("merged Tools = %v, want %v", merged.Tools, wantTools)
	}

	// Tool stays as the primary (the one with the longest symbol).
	if merged.Tool != "xdeadcode" {
		t.Errorf("merged Tool = %q, want xdeadcode (provided longest symbol)", merged.Tool)
	}

	// Evidence preserves both tools' data, prefixed.
	if merged.Evidence["staticcheck.src"] != "staticcheck_evidence" {
		t.Errorf("missing staticcheck evidence: %+v", merged.Evidence)
	}
	if merged.Evidence["xdeadcode.src"] != "xdeadcode_evidence" {
		t.Errorf("missing xdeadcode evidence: %+v", merged.Evidence)
	}
	if merged.Evidence["dedupe.tools_count"] != "2" {
		t.Errorf("missing dedupe.tools_count = 2: %+v", merged.Evidence)
	}
}

func TestDedupeKeepsDifferentKinds(t *testing.T) {
	// Same file:line but different kinds — should NOT merge.
	in := []finding.Finding{
		mk("a.go", 5, finding.KindUnusedFunction, "Foo", "staticcheck", 0.95),
		mk("a.go", 5, finding.KindUnusedType, "Foo", "staticcheck", 0.95),
	}
	out := dedupeFindings(in)
	if len(out) != 2 {
		t.Errorf("different kinds at same line should NOT merge, got %d", len(out))
	}
}

func TestDedupeKeepsDifferentLanguages(t *testing.T) {
	// Conceptually impossible (one file is one language) but test
	// the language part of the key anyway.
	in := []finding.Finding{
		mk("a", 1, finding.KindUnusedFunction, "Foo", "tool1", 0.95),
		mk("a", 1, finding.KindUnusedFunction, "Foo", "tool2", 0.95),
	}
	in[1].Language = "python"
	out := dedupeFindings(in)
	if len(out) != 2 {
		t.Errorf("different languages should NOT merge, got %d", len(out))
	}
}

func TestDedupeMaxConfidence(t *testing.T) {
	in := []finding.Finding{
		mk("a.go", 1, finding.KindUnusedFunction, "Foo", "tool1", 0.60),
		mk("a.go", 1, finding.KindUnusedFunction, "pkg.Foo", "tool2", 0.95),
	}
	out := dedupeFindings(in)
	if len(out) != 1 {
		t.Fatalf("expected merge, got %d", len(out))
	}
	if out[0].Confidence != 0.95 {
		t.Errorf("merged confidence = %f, want 0.95 (max)", out[0].Confidence)
	}
}

func TestDedupeLongestMessage(t *testing.T) {
	in := []finding.Finding{
		mk("a.go", 1, finding.KindUnusedFunction, "F", "t1", 0.9),
		mk("a.go", 1, finding.KindUnusedFunction, "pkg.F", "t2", 0.9),
	}
	in[0].Message = "short"
	in[1].Message = "much longer message with more detail"
	out := dedupeFindings(in)
	if len(out) != 1 {
		t.Fatal("expected merge")
	}
	if out[0].Message != "much longer message with more detail" {
		t.Errorf("merged message = %q, want longest", out[0].Message)
	}
}

func TestDedupeStableOrder(t *testing.T) {
	// First-key-seen order is preserved across the merge.
	in := []finding.Finding{
		mk("z.go", 1, finding.KindUnusedFunction, "Z", "t1", 0.9),
		mk("a.go", 1, finding.KindUnusedFunction, "A", "t1", 0.9),
		mk("a.go", 1, finding.KindUnusedFunction, "pkg.A", "t2", 0.9),
		mk("m.go", 1, finding.KindUnusedFunction, "M", "t1", 0.9),
	}
	out := dedupeFindings(in)
	if len(out) != 3 {
		t.Fatalf("expected 3 (z, merged-a, m), got %d", len(out))
	}
	wantOrder := []string{"z.go", "a.go", "m.go"}
	for i, f := range out {
		if f.File != wantOrder[i] {
			t.Errorf("position %d file = %s, want %s", i, f.File, wantOrder[i])
		}
	}
}

func TestDedupeEmpty(t *testing.T) {
	if got := dedupeFindings(nil); len(got) != 0 {
		t.Errorf("nil input should return empty, got %v", got)
	}
	if got := dedupeFindings([]finding.Finding{}); len(got) != 0 {
		t.Errorf("empty input should return empty, got %v", got)
	}
}
