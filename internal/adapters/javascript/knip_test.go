package javascript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/finding"
)

// sampleKnipJSON mirrors the actual shape emitted by knip 5.x as
// captured by running it against testdata/ts-sample. Notable quirks:
//   - `files` is per-issue (not top-level) and shaped as [{"name":...}]
//   - `enumMembers` and `classMembers` are flat arrays with optional
//     `namespace` instead of maps keyed by container name
//   - extra fields (catalog, namespaceMembers, etc.) exist and we drop
const sampleKnipJSON = `{
  "issues": [
    {
      "file": "src/orphan.ts",
      "binaries": [],
      "catalog": [],
      "dependencies": [],
      "devDependencies": [],
      "duplicates": [],
      "enumMembers": [],
      "exports": [],
      "files": [{"name": "src/orphan.ts"}],
      "namespaceMembers": [],
      "optionalPeerDependencies": [],
      "types": [],
      "unlisted": [],
      "unresolved": []
    },
    {
      "file": "package.json",
      "dependencies": [{"name": "left-pad", "line": 5, "col": 6, "pos": 71}],
      "devDependencies": [{"name": "old-jest", "line": 9, "col": 6, "pos": 99}],
      "binaries": [{"name": "old-cli", "line": 12, "col": 6, "pos": 130}],
      "files": []
    },
    {
      "file": "src/Registration.tsx",
      "exports": [{"name": "unusedExport", "line": 1, "col": 14, "pos": 13}],
      "types": [{"name": "UnusedType", "line": 8, "col": 14, "pos": 145}],
      "enumMembers": [
        {"namespace": "MyEnum", "name": "OBSOLETE", "line": 13, "col": 3, "pos": 167}
      ],
      "classMembers": [
        {"namespace": "MyClass", "name": "deadHelper", "line": 40, "col": 3, "pos": 687}
      ],
      "duplicates": ["Registration"],
      "unlisted": [{"name": "react"}],
      "unresolved": [{"name": "./missing", "line": 8, "col": 23, "pos": 407}],
      "files": []
    }
  ]
}`

func TestParseKnipOutput(t *testing.T) {
	findings, err := parseKnipOutput([]byte(sampleKnipJSON), "/repo/myapp", adapter.RunOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Expected: 1 unused file (orphan.ts) + 1 dep + 1 devDep + 1 binary
	// + 1 export + 1 type + 1 enumMember + 1 classMember = 8
	if got, want := len(findings), 8; got != want {
		t.Fatalf("expected %d findings, got %d", want, got)
	}

	byKind := map[finding.Kind]int{}
	for _, f := range findings {
		byKind[f.Kind]++
		if f.Tool != "knip" {
			t.Errorf("wrong tool on %+v", f)
		}
		if f.Confidence != 0.95 {
			t.Errorf("expected 0.95 confidence on %+v", f)
		}
	}
	wantKinds := map[finding.Kind]int{
		finding.KindUnusedFile:       1,
		finding.KindUnusedDependency: 3, // dep + devDep + binary
		finding.KindUnusedExport:     3, // export + type + enumMember
		finding.KindUnusedField:      1, // classMember
	}
	for k, want := range wantKinds {
		if got := byKind[k]; got != want {
			t.Errorf("kind %s: got %d, want %d", k, got, want)
		}
	}

	// Spot checks on specific findings
	var fileFinding, enumFinding, classFinding *finding.Finding
	for i := range findings {
		f := findings[i]
		switch {
		case f.Kind == finding.KindUnusedFile && f.Symbol == "orphan.ts":
			fileFinding = &findings[i]
		case f.Kind == finding.KindUnusedExport && strings.HasPrefix(f.Symbol, "MyEnum."):
			enumFinding = &findings[i]
		case f.Kind == finding.KindUnusedField && strings.HasPrefix(f.Symbol, "MyClass."):
			classFinding = &findings[i]
		}
	}
	if fileFinding == nil {
		t.Fatal("missing file finding for orphan.ts")
	}
	if !strings.HasSuffix(fileFinding.File, "src/orphan.ts") {
		t.Errorf("file finding has wrong abs path: %s", fileFinding.File)
	}
	if fileFinding.Language != "typescript" {
		t.Errorf("expected typescript, got %s", fileFinding.Language)
	}
	if enumFinding == nil || enumFinding.Symbol != "MyEnum.OBSOLETE" {
		t.Errorf("enum member symbol should be qualified: %+v", enumFinding)
	}
	if classFinding == nil || classFinding.Symbol != "MyClass.deadHelper" {
		t.Errorf("class member symbol should be qualified: %+v", classFinding)
	}
}

func TestParseKnipOutputExcludeTests(t *testing.T) {
	out := []byte(`{
  "issues": [
    {"file": "src/main.ts", "files": [{"name": "src/main.ts"}]},
    {"file": "tests/old.test.ts", "files": [{"name": "tests/old.test.ts"}]},
    {"file": "src/__tests__/dead.ts", "files": [{"name": "src/__tests__/dead.ts"}]},
    {"file": "src/main.spec.ts", "exports": [{"name": "x", "line": 1, "col": 1, "pos": 0}]}
  ]
}`)
	findings, err := parseKnipOutput(out, "/repo", adapter.RunOptions{ExcludeTests: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (only src/main.ts), got %d: %+v", len(findings), findings)
	}
	if findings[0].Symbol != "main.ts" {
		t.Errorf("wrong finding survived: %+v", findings[0])
	}
}

func TestLanguageForExt(t *testing.T) {
	cases := map[string]string{
		"src/foo.ts":      "typescript",
		"src/Foo.tsx":     "typescript",
		"types.mts":       "typescript",
		"src/foo.js":      "javascript",
		"src/Foo.jsx":     "javascript",
		"build.cjs":       "javascript",
		"package.json":    "javascript",
	}
	for path, want := range cases {
		if got := languageForExt(path); got != want {
			t.Errorf("languageForExt(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestLooksLikeTestFile(t *testing.T) {
	cases := map[string]bool{
		"/r/src/foo.ts":               false,
		"/r/src/foo.test.ts":          true,
		"/r/src/foo.spec.tsx":         true,
		"/r/tests/foo.ts":             true,
		"/r/test/foo.ts":              true,
		"/r/__tests__/foo.ts":         true,
		"/r/src/__tests__/inner.ts":   true,
		"/r/spec/foo.ts":              true,
		"/r/contest.ts":               false,
	}
	for path, want := range cases {
		if got := looksLikeTestFile(path); got != want {
			t.Errorf("looksLikeTestFile(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestProjectRootsDedupe(t *testing.T) {
	dir := t.TempDir()
	subA := filepath.Join(dir, "src")
	subB := filepath.Join(dir, "test")
	for _, d := range []string{subA, subB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	roots := projectRoots([]string{subA, subB})
	if len(roots) != 1 || roots[0] != dir {
		t.Errorf("expected single deduped root %s, got %v", dir, roots)
	}
}

func TestProjectRootsNoPackageJSON(t *testing.T) {
	dir := t.TempDir()
	roots := projectRoots([]string{dir})
	if len(roots) != 0 {
		t.Errorf("expected zero roots when no package.json, got %v", roots)
	}
}
