package python

import (
	"testing"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/finding"
)

func TestParseVultureOutput(t *testing.T) {
	out := []byte(`src/foo.py:42: unused function 'compute_total' (60% confidence)
src/foo.py:10: unused import 'json' (90% confidence)
src/bar.py:5: unused class 'MyClass' (60% confidence)
src/bar.py:25: unused variable 'x' (60% confidence)
src/bar.py:30: unused method 'helper' (60% confidence)
src/bar.py:55: unused attribute 'name' (60% confidence)
src/bar.py:80: unreachable code after 'return' (100% confidence)
junk line that should be ignored
`)

	findings := parseVultureOutput(out, adapter.RunOptions{})
	if got, want := len(findings), 7; got != want {
		t.Fatalf("expected %d findings, got %d: %+v", want, got, findings)
	}

	cases := []struct {
		idx    int
		kind   finding.Kind
		symbol string
		conf   float64
	}{
		{0, finding.KindUnusedFunction, "compute_total", 0.60},
		{1, finding.KindUnusedImport, "json", 0.90},
		{2, finding.KindUnusedClass, "MyClass", 0.60},
		{3, finding.KindUnusedVariable, "x", 0.60},
		{4, finding.KindUnusedMethod, "helper", 0.60},
		{5, finding.KindUnusedField, "name", 0.60},
		{6, finding.KindUnreachable, "", 1.00},
	}
	for _, c := range cases {
		f := findings[c.idx]
		if f.Kind != c.kind {
			t.Errorf("[%d] kind = %s, want %s", c.idx, f.Kind, c.kind)
		}
		if f.Symbol != c.symbol {
			t.Errorf("[%d] symbol = %q, want %q", c.idx, f.Symbol, c.symbol)
		}
		if f.Confidence != c.conf {
			t.Errorf("[%d] confidence = %f, want %f", c.idx, f.Confidence, c.conf)
		}
		if f.Tool != "vulture" || f.Language != "python" {
			t.Errorf("[%d] wrong tool/lang: %+v", c.idx, f)
		}
		if f.Evidence["tool_raw"] == "" {
			t.Errorf("[%d] missing tool_raw evidence", c.idx)
		}
	}
}

func TestParseVultureOutputExcludeTests(t *testing.T) {
	out := []byte(`src/foo.py:1: unused function 'real' (60% confidence)
tests/test_foo.py:1: unused function 'helper' (60% confidence)
src/test_helpers.py:1: unused function 'h' (60% confidence)
`)

	findings := parseVultureOutput(out, adapter.RunOptions{ExcludeTests: true})
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding after excluding tests, got %d: %+v", len(findings), findings)
	}
	if findings[0].Symbol != "real" {
		t.Errorf("wrong finding survived: %+v", findings[0])
	}
}

func TestLooksLikeTestFile(t *testing.T) {
	cases := map[string]bool{
		"/repo/src/foo.py":             false,
		"/repo/tests/test_foo.py":      true,
		"/repo/test/foo.py":            true,
		"/repo/src/test_helpers.py":    true,
		"/repo/src/foo_test.py":        true,
		"/repo/src/contest.py":         false,
		"/repo/src/testimonial.py":     false,
	}
	for path, want := range cases {
		if got := looksLikeTestFile(path); got != want {
			t.Errorf("looksLikeTestFile(%q) = %v, want %v", path, got, want)
		}
	}
}
