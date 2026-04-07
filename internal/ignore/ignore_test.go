package ignore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sglyon/deadcode/internal/finding"
)

func mkFinding(file, symbol string, kind finding.Kind, lang, tool string) finding.Finding {
	return finding.Finding{
		ID:       "py:" + file + ":1:" + string(kind) + ":" + symbol,
		File:     file,
		Line:     1,
		Symbol:   symbol,
		Kind:     kind,
		Language: lang,
		Tool:     tool,
	}
}

func TestRuleMatchByID(t *testing.T) {
	f := mkFinding("/repo/src/foo.py", "bar", finding.KindUnusedFunction, "python", "vulture")
	rs := &Ruleset{Rules: []Rule{
		{ID: f.ID, Reason: "x"},
	}}
	if rs.Match(f) != 0 {
		t.Errorf("expected ID match")
	}
	rs.Rules[0].ID = "no-match"
	if rs.Match(f) != -1 {
		t.Errorf("expected no match")
	}
}

func TestRuleMatchByFileGlob(t *testing.T) {
	f := mkFinding("/repo/src/models/user.py", "name", finding.KindUnusedField, "python", "vulture")
	rs := &Ruleset{
		Dir: "/repo",
		Rules: []Rule{
			{File: "src/models/**.py", Reason: "schema fields"},
		},
	}
	if rs.Match(f) != 0 {
		t.Errorf("expected file glob to match relative path")
	}
}

func TestRuleMatchByFileBasename(t *testing.T) {
	f := mkFinding("/some/random/abs/path/parser.py", "x", finding.KindUnusedVariable, "python", "vulture")
	rs := &Ruleset{Rules: []Rule{{File: "parser.py", Reason: "noisy"}}}
	if rs.Match(f) != 0 {
		t.Errorf("expected basename match")
	}
}

func TestRuleMatchBySymbolGlob(t *testing.T) {
	rs := &Ruleset{Rules: []Rule{
		{Symbol: "*_at", Kinds: []string{"unused_variable"}, Reason: "ORM timestamps"},
	}}
	hit := mkFinding("/r/m.py", "created_at", finding.KindUnusedVariable, "python", "vulture")
	miss1 := mkFinding("/r/m.py", "created_at", finding.KindUnusedFunction, "python", "vulture") // wrong kind
	miss2 := mkFinding("/r/m.py", "created", finding.KindUnusedVariable, "python", "vulture")    // wrong symbol
	if rs.Match(hit) != 0 {
		t.Errorf("expected hit")
	}
	if rs.Match(miss1) != -1 {
		t.Errorf("expected miss on wrong kind")
	}
	if rs.Match(miss2) != -1 {
		t.Errorf("expected miss on wrong symbol")
	}
}

func TestRuleMatchByLanguageAndTool(t *testing.T) {
	rs := &Ruleset{Rules: []Rule{
		{Languages: []string{"python"}, Tools: []string{"vulture"}, Reason: "all py vulture"},
	}}
	hit := mkFinding("/r/m.py", "x", finding.KindUnusedVariable, "python", "vulture")
	miss := mkFinding("/r/m.go", "x", finding.KindUnusedVariable, "go", "staticcheck")
	if rs.Match(hit) != 0 || rs.Match(miss) != -1 {
		t.Errorf("language/tool matcher broken")
	}
}

func TestFirstRuleWins(t *testing.T) {
	rs := &Ruleset{Rules: []Rule{
		{Symbol: "specific_name", Reason: "first"},
		{Languages: []string{"python"}, Reason: "second (general)"},
	}}
	f := mkFinding("/r/m.py", "specific_name", finding.KindUnusedVariable, "python", "vulture")
	if got := rs.Match(f); got != 0 {
		t.Errorf("expected first rule (0), got %d", got)
	}
}

func TestEmptyRuleMatchesNothing(t *testing.T) {
	rs := &Ruleset{Rules: []Rule{{Reason: "oops, no matchers"}}}
	f := mkFinding("/r/m.py", "x", finding.KindUnusedVariable, "python", "vulture")
	if rs.Match(f) != -1 {
		t.Errorf("a rule with no matchers must not match anything")
	}
}

func TestValidate(t *testing.T) {
	rs := &Ruleset{Rules: []Rule{
		{Reason: "no matchers here"},
		{Symbol: "*", Reason: ""},
		{Symbol: "*", Kinds: []string{"bogus_kind"}, Reason: "bad kind"},
		{Symbol: "*", Kinds: []string{"unused_function"}, Reason: "fine"},
	}}
	errs := rs.Validate()
	if len(errs) < 3 {
		t.Errorf("expected >=3 errors, got %d: %v", len(errs), errs)
	}
}

func TestLoadFromTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	body := `
[[ignore]]
id = "py:src/foo.py:42:unused_function:legacy"
reason = "Called via reflection"

[[ignore]]
file = "src/models/**.py"
kinds = ["unused_field", "unused_variable"]
reason = "Pydantic/SQLAlchemy fields"

[[ignore]]
symbol = "*_at"
languages = ["python"]
kinds = ["unused_variable"]
reason = "ORM timestamps"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	rs, err := Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(rs.Rules), 3; got != want {
		t.Fatalf("expected %d rules, got %d", want, got)
	}
	if rs.Rules[0].ID == "" || rs.Rules[1].File == "" || rs.Rules[2].Symbol == "" {
		t.Errorf("rules did not parse correctly: %+v", rs.Rules)
	}
	if errs := rs.Validate(); len(errs) != 0 {
		t.Errorf("expected zero validation errors, got %v", errs)
	}
}

func TestLoadMissingNotRequired(t *testing.T) {
	rs, err := Load(filepath.Join(t.TempDir(), "nope.toml"), false)
	if err != nil {
		t.Fatal(err)
	}
	if !rs.Empty() {
		t.Errorf("expected empty ruleset")
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "src", "models")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[[ignore]]\nid = \"abc\"\nreason = \"r\"\n"
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	rs, err := Discover(deep)
	if err != nil {
		t.Fatal(err)
	}
	if rs.Empty() {
		t.Fatalf("expected to discover ignore file walking up from %s", deep)
	}
	if len(rs.Rules) != 1 || rs.Rules[0].ID != "abc" {
		t.Errorf("wrong rule loaded: %+v", rs.Rules)
	}
}

func TestDiscoverNotFound(t *testing.T) {
	rs, err := Discover(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !rs.Empty() {
		t.Errorf("expected empty ruleset, got %+v", rs)
	}
}
