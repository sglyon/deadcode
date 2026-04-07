package golang

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/finding"
)

// TestParseFixtureRegression pins the parser against NDJSON captured
// from `staticcheck -checks=U1000 -f json ./...` running against
// testdata/go-sample on staticcheck 2026.1. If a future release
// changes the JSON schema we'll see a clean failure here.
func TestParseFixtureRegression(t *testing.T) {
	root := repoRoot(t)
	fixturePath := filepath.Join(root, "testdata", "go-sample", "expected-staticcheck.json")
	out, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	findings := parseStaticcheckOutput(out, "/proj", adapter.RunOptions{})
	if got, want := len(findings), 6; got != want {
		t.Fatalf("expected %d findings from fixture, got %d:\n%+v", want, got, findings)
	}

	// Count by kind.
	byKind := map[finding.Kind]int{}
	for _, f := range findings {
		byKind[f.Kind]++
		if f.Tool != "staticcheck" {
			t.Errorf("wrong tool: %+v", f)
		}
		if f.Language != "go" {
			t.Errorf("wrong language: %+v", f)
		}
		if f.Confidence != 0.95 {
			t.Errorf("wrong confidence: %+v", f)
		}
		if f.Evidence["tool_code"] != "U1000" {
			t.Errorf("missing U1000 evidence code: %+v", f)
		}
	}
	want := map[finding.Kind]int{
		finding.KindUnusedType:     2, // unusedThing, orphanType
		finding.KindUnusedConstant: 2, // unusedConst, orphanConst
		finding.KindUnusedFunction: 2, // unusedHelper, orphanFunc
	}
	for k, count := range want {
		if byKind[k] != count {
			t.Errorf("kind %s: got %d, want %d", k, byKind[k], count)
		}
	}

	// Spot-check specific findings exist with expected symbols.
	expectedSymbols := map[string]bool{
		"unusedThing":   false,
		"orphanType":    false,
		"unusedConst":   false,
		"orphanConst":   false,
		"unusedHelper":  false,
		"orphanFunc":    false,
	}
	for _, f := range findings {
		if _, expected := expectedSymbols[f.Symbol]; expected {
			expectedSymbols[f.Symbol] = true
		}
	}
	for sym, seen := range expectedSymbols {
		if !seen {
			t.Errorf("expected symbol %q not present", sym)
		}
	}
}

func TestClassifyMessage(t *testing.T) {
	cases := []struct {
		msg    string
		kind   finding.Kind
		symbol string
	}{
		{"func MyFunc is unused", finding.KindUnusedFunction, "MyFunc"},
		{"method (T).DoThing is unused", finding.KindUnusedMethod, "(T).DoThing"},
		{"type MyType is unused", finding.KindUnusedType, "MyType"},
		{"const MyConst is unused", finding.KindUnusedConstant, "MyConst"},
		{"var myVar is unused", finding.KindUnusedVariable, "myVar"},
		{"field Name is unused", finding.KindUnusedField, "Name"},
	}
	for _, c := range cases {
		kind, symbol := classifyMessage(c.msg)
		if kind != c.kind {
			t.Errorf("classifyMessage(%q) kind = %s, want %s", c.msg, kind, c.kind)
		}
		if symbol != c.symbol {
			t.Errorf("classifyMessage(%q) symbol = %q, want %q", c.msg, symbol, c.symbol)
		}
	}
}

func TestClassifyMessageUnknownShape(t *testing.T) {
	// Defensive: if staticcheck ever emits a message we don't
	// recognize, we still produce a finding rather than silently
	// dropping it.
	kind, symbol := classifyMessage("something weird happened")
	if kind != finding.KindUnusedFunction {
		t.Errorf("unknown shape should default to unused_function, got %s", kind)
	}
	if symbol == "" {
		t.Errorf("unknown shape should use whole message as symbol, got empty")
	}
}

func TestParseSkipsNonU1000(t *testing.T) {
	// Defensive: even if staticcheck ever leaks non-U1000 findings
	// (compile errors it decides to surface, e.g.), we skip them.
	out := []byte(`{"code":"SA4006","location":{"file":"a.go","line":1,"column":1},"message":"this value is never used"}
{"code":"U1000","location":{"file":"a.go","line":5,"column":6},"message":"func unused is unused"}
`)
	findings := parseStaticcheckOutput(out, "/proj", adapter.RunOptions{})
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (U1000 only), got %d", len(findings))
	}
	if findings[0].Symbol != "unused" {
		t.Errorf("wrong symbol: %s", findings[0].Symbol)
	}
}

func TestParseSkipsInvalidJSON(t *testing.T) {
	out := []byte(`this is not json
{"code":"U1000","location":{"file":"a.go","line":1,"column":1},"message":"func x is unused"}
also not json
`)
	findings := parseStaticcheckOutput(out, "/proj", adapter.RunOptions{})
	if len(findings) != 1 {
		t.Errorf("expected 1 finding (invalid lines skipped), got %d", len(findings))
	}
}

func TestParseExcludeTests(t *testing.T) {
	out := []byte(`{"code":"U1000","location":{"file":"/proj/main.go","line":1,"column":1},"message":"func real is unused"}
{"code":"U1000","location":{"file":"/proj/foo_test.go","line":1,"column":1},"message":"func helperT is unused"}
{"code":"U1000","location":{"file":"/proj/testdata/dead.go","line":1,"column":1},"message":"func fixture is unused"}
`)
	findings := parseStaticcheckOutput(out, "/proj", adapter.RunOptions{ExcludeTests: true})
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding after excluding tests, got %d: %+v", len(findings), findings)
	}
	if findings[0].Symbol != "real" {
		t.Errorf("wrong finding survived: %+v", findings[0])
	}
}

// TestStaticcheckIDsAreProjectRelative pins the v0.6.1 dogfood fix:
// the ID field uses paths relative to the go.mod root. Without this,
// staticcheck's absolute file paths would make IDs machine-specific.
func TestStaticcheckIDsAreProjectRelative(t *testing.T) {
	out := []byte(`{"code":"U1000","location":{"file":"/abs/proj/internal/runner/dead.go","line":42,"column":6},"message":"func helper is unused"}
{"code":"U1000","location":{"file":"/abs/proj/main.go","line":5,"column":7},"message":"const X is unused"}
`)
	findings := parseStaticcheckOutput(out, "/abs/proj", adapter.RunOptions{})
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
	wantIDs := []string{
		"go:internal/runner/dead.go:42:unused_function:helper",
		"go:main.go:5:unused_constant:X",
	}
	for i, f := range findings {
		if f.ID != wantIDs[i] {
			t.Errorf("finding %d ID = %q, want %q", i, f.ID, wantIDs[i])
		}
	}
}

func TestRelPathForID(t *testing.T) {
	cases := []struct {
		file    string
		root    string
		want    string
	}{
		{"/abs/proj/lib/foo.go", "/abs/proj", "lib/foo.go"},
		{"lib/foo.go", "/abs/proj", "lib/foo.go"}, // already relative
		{"/abs/elsewhere/foo.go", "/abs/proj", "/abs/elsewhere/foo.go"}, // outside
	}
	for _, c := range cases {
		if got := relPathForID(c.file, c.root); got != c.want {
			t.Errorf("relPathForID(%q, %q) = %q, want %q", c.file, c.root, got, c.want)
		}
	}
}

func TestLooksLikeTestFile(t *testing.T) {
	// Go's convention: ANY file ending in "_test.go" is a test file,
	// regardless of the prefix. So "not_a_test.go" is treated as a
	// test file — the convention is strict.
	cases := map[string]bool{
		"/repo/main.go":              false,
		"/repo/main_test.go":         true,
		"/repo/internal/foo.go":      false,
		"/repo/internal/foo_test.go": true,
		"/repo/testdata/fixture.go":  true,
		"/repo/pkg/testdata/f.go":    true,
		"/repo/helper.go":            false,
		"/repo/anything.go":          false,
	}
	for path, want := range cases {
		if got := looksLikeTestFile(path); got != want {
			t.Errorf("looksLikeTestFile(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestProjectRootsDedupe(t *testing.T) {
	dir := t.TempDir()
	subA := filepath.Join(dir, "cmd", "foo")
	subB := filepath.Join(dir, "pkg", "bar")
	for _, d := range []string{subA, subB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	roots := projectRoots([]string{subA, subB})
	if len(roots) != 1 || roots[0] != dir {
		t.Errorf("expected single deduped root %s, got %v", dir, roots)
	}
}

// repoRoot walks upward from this test file looking for go.mod so the
// fixture path is stable regardless of where `go test` is run from.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		// Skip the fixture's go.mod — we want the repo's.
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if !strings.HasSuffix(dir, "go-sample") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root go.mod walking upward")
		}
		dir = parent
	}
}
