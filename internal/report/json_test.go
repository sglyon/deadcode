package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sglyon/deadcode/internal/runner"
)

// TestJSONEmptyFindingsAreArray pins the v0.3.1 fix: an empty findings
// slice MUST marshal to `[]`, not `null`, so downstream jq consumers
// can iterate without nil checks.
func TestJSONEmptyFindingsAreArray(t *testing.T) {
	r := &runner.Result{
		// Findings, LanguagesPresent, ToolsRun all nil — the wire to
		// avoid is `null` for any of them.
		FilesScanned: 0,
		DurationMs:   1,
	}
	var buf bytes.Buffer
	if err := JSON(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if strings.Contains(out, "null") {
		t.Errorf("expected no `null` in JSON output, got:\n%s", out)
	}

	// Round-trip parse and verify slice fields are present and empty
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"findings"} {
		v, ok := doc[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}
		arr, ok := v.([]any)
		if !ok || arr == nil {
			t.Errorf("expected %q to be an array, got %T (%v)", key, v, v)
		}
	}
	summary, ok := doc["summary"].(map[string]any)
	if !ok {
		t.Fatal("missing summary")
	}
	for _, key := range []string{"languages", "tools_run"} {
		v, ok := summary[key]
		if !ok {
			t.Errorf("missing summary.%s", key)
			continue
		}
		if _, ok := v.([]any); !ok || v == nil {
			t.Errorf("expected summary.%s to be an array, got %T", key, v)
		}
	}
}

// TestJSONShowIgnoredEmpty pins that the ignored array is also `[]`
// when --show-ignored is set on a result with no suppressions.
func TestJSONShowIgnoredEmpty(t *testing.T) {
	r := &runner.Result{}
	var buf bytes.Buffer
	if err := JSON(&buf, r, true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"ignored": null`) {
		t.Errorf("expected ignored to be [], got:\n%s", buf.String())
	}
}
