package elixir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/finding"
)

// TestParseFixtureRegression pins the parser against the exact output
// captured from `mix compile` running against testdata/elixir-sample
// on Elixir 1.19.5 / OTP 28. If a future Elixir release changes the
// warning format we'll see a clear failure here and know to update.
func TestParseFixtureRegression(t *testing.T) {
	// Walk up to the repo root from this test file's directory so the
	// fixture path resolves regardless of `go test` invocation cwd.
	root := repoRoot(t)
	fixturePath := filepath.Join(root, "testdata", "elixir-sample", "expected-mix-compile.txt")
	out, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	findings := parseMixCompileOutput(out, "/proj", "", adapter.RunOptions{})
	if got, want := len(findings), 3; got != want {
		t.Fatalf("expected %d findings from fixture, got %d:\n%+v", want, got, findings)
	}

	// Finding 1: private unused function in lib/orphan.ex:16
	got1 := findings[0]
	if got1.Kind != finding.KindUnusedFunction {
		t.Errorf("[0] kind = %s, want unused_function", got1.Kind)
	}
	if got1.Symbol != "Sample.Orphan.private_dead/0" {
		t.Errorf("[0] symbol = %q, want Sample.Orphan.private_dead/0", got1.Symbol)
	}
	if got1.Line != 16 {
		t.Errorf("[0] line = %d, want 16", got1.Line)
	}
	if !strings.HasSuffix(got1.File, "lib/orphan.ex") {
		t.Errorf("[0] file = %q, expected suffix lib/orphan.ex", got1.File)
	}
	if got1.Confidence != 0.90 {
		t.Errorf("[0] confidence = %f, want 0.90", got1.Confidence)
	}

	// Finding 2: type-system unreachable clause in main/0
	got2 := findings[1]
	if got2.Kind != finding.KindUnreachable {
		t.Errorf("[1] kind = %s, want unreachable", got2.Kind)
	}
	if got2.Symbol != "Sample.main/0" {
		t.Errorf("[1] symbol = %q, want Sample.main/0", got2.Symbol)
	}
	if got2.Confidence != 0.95 {
		t.Errorf("[1] confidence = %f, want 0.95", got2.Confidence)
	}
	if got2.Evidence["elixir_phase"] != "type_checker" {
		t.Errorf("[1] phase = %q, want type_checker", got2.Evidence["elixir_phase"])
	}

	// Finding 3: unreachable defp clause
	got3 := findings[2]
	if got3.Kind != finding.KindUnreachable {
		t.Errorf("[2] kind = %s, want unreachable", got3.Kind)
	}
	if got3.Symbol != "Sample.classify/1" {
		t.Errorf("[2] symbol = %q, want Sample.classify/1", got3.Symbol)
	}

	// Every finding must carry the original mix output for audit.
	for i, f := range findings {
		if f.Tool != "mix_compile" {
			t.Errorf("[%d] tool = %q, want mix_compile", i, f.Tool)
		}
		if f.Language != "elixir" {
			t.Errorf("[%d] language = %q, want elixir", i, f.Language)
		}
		if f.Evidence["tool_raw"] == "" {
			t.Errorf("[%d] missing tool_raw evidence", i)
		}
	}
}

func TestParseHandlesEmptyOutput(t *testing.T) {
	findings := parseMixCompileOutput([]byte(""), "/proj", "", adapter.RunOptions{})
	if len(findings) != 0 {
		t.Errorf("expected zero findings on empty input, got %d", len(findings))
	}
}

func TestParseSkipsTrailingWarningWithNoLocation(t *testing.T) {
	out := []byte("    warning: something with no location\n")
	if got := parseMixCompileOutput(out, "/proj", "", adapter.RunOptions{}); len(got) != 0 {
		t.Errorf("expected zero findings, got %d", len(got))
	}
}

// TestParsePhoenixRealWorldFixture pins the parser against a 384-line
// capture of `mix compile` running against ~/src/arete/arilearn-phx
// (a real Phoenix app with ~100 hex deps). Every warning in the
// fixture comes from a dependency, not from arilearn's own code.
//
// When scoped to the "arilearn" app section, we expect zero findings
// (the team keeps the app warning-clean). This is the load-bearing
// test for the section-filtering behavior.
func TestParsePhoenixRealWorldFixture(t *testing.T) {
	root := repoRoot(t)
	fixturePath := filepath.Join(root, "testdata", "elixir-sample", "phoenix-real-world.txt")
	out, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// With no app scoping we should see every dep warning plus the
	// boruta multi-location finding (3 locations on one warning).
	unscoped := parseMixCompileOutput(out, "/proj", "", adapter.RunOptions{})
	if len(unscoped) < 6 {
		t.Errorf("unscoped parse should see all dep warnings (>= 6), got %d", len(unscoped))
	}

	// Multi-location verification: boruta's List.zip/1 deprecation
	// has three call sites. Count findings whose file is the boruta
	// tasks file.
	borutaHits := 0
	for _, f := range unscoped {
		if strings.Contains(f.File, "boruta.gen.controllers.ex") {
			borutaHits++
		}
	}
	if borutaHits != 3 {
		t.Errorf("expected 3 findings for boruta multi-location warning, got %d", borutaHits)
	}

	// Scoped to "arilearn" — the user's own app is warning-clean,
	// so we expect zero findings. This is the whole point of the
	// section filter: dep noise should not leak into the report.
	scoped := parseMixCompileOutput(out, "/proj", "arilearn", adapter.RunOptions{})
	if len(scoped) != 0 {
		t.Errorf("scoped parse to 'arilearn' should yield 0 findings (team keeps app clean), got %d:\n%+v",
			len(scoped), scoped)
	}

	// A fictional scope that matches none of the sections should
	// also yield zero findings without crashing.
	empty := parseMixCompileOutput(out, "/proj", "nonexistent_app", adapter.RunOptions{})
	if len(empty) != 0 {
		t.Errorf("scoped parse to missing section should yield 0 findings, got %d", len(empty))
	}
}

func TestParseMultiLocationSynthetic(t *testing.T) {
	// Minimal reproduction of the multi-location case: one warning
	// body followed by three └─ lines. Each should become its own
	// finding with distinct (file, line).
	out := []byte(`==> myapp
Compiling 1 file (.ex)
     warning: Foo.bar/1 is deprecated
     │
 100 │     Foo.bar(1)
     │
     └─ lib/a.ex:10:5: MyApp.A.caller_one/0
     └─ lib/b.ex:20:5: MyApp.B.caller_two/0
     └─ lib/c.ex:30:5: MyApp.C.caller_three/1

Generated myapp app
`)
	findings := parseMixCompileOutput(out, "/proj", "myapp", adapter.RunOptions{})
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings from multi-location warning, got %d: %+v", len(findings), findings)
	}
	expectedLines := []int{10, 20, 30}
	for i, f := range findings {
		if f.Line != expectedLines[i] {
			t.Errorf("finding %d line = %d, want %d", i, f.Line, expectedLines[i])
		}
		if !strings.Contains(f.Message, "Foo.bar/1 is deprecated") {
			t.Errorf("finding %d missing shared message", i)
		}
	}
}

func TestExtractSymbolFromFunctionHeader(t *testing.T) {
	cases := []struct {
		header  string
		context string
		want    string
	}{
		{"function private_dead/0 is unused", "Sample.Orphan (module)", "Sample.Orphan.private_dead/0"},
		{"defp helper/1 is unused", "MyApp.Worker (module)", "MyApp.Worker.helper/1"},
		{"this clause of defp classify/1 is never used", "Sample.classify/1", "Sample.classify/1"},
		{"the following clause will never match", "Sample.main/0", "Sample.main/0"},
	}
	for _, c := range cases {
		got := extractSymbol(c.header, c.context, finding.KindUnusedFunction)
		if got != c.want {
			t.Errorf("extractSymbol(%q, %q) = %q, want %q", c.header, c.context, got, c.want)
		}
	}
}

func TestClassifyKind(t *testing.T) {
	cases := []struct {
		header string
		want   finding.Kind
	}{
		{"function private_dead/0 is unused", finding.KindUnusedFunction},
		{"this clause of defp classify/1 is never used", finding.KindUnreachable},
		{"the following clause will never match", finding.KindUnreachable},
		{"unused alias Foo", finding.KindUnusedImport},
		{"unused import Bar", finding.KindUnusedImport},
		{"unused variable x", finding.KindUnusedVariable},
	}
	for _, c := range cases {
		if got := classifyKind(c.header, nil); got != c.want {
			t.Errorf("classifyKind(%q) = %s, want %s", c.header, got, c.want)
		}
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
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod walking upward from " + wd)
		}
		dir = parent
	}
}
