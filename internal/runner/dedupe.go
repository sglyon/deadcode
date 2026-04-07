package runner

import (
	"sort"

	"github.com/sglyon/deadcode/internal/finding"
)

// dedupeFindings collapses findings produced by multiple adapters that
// independently flagged the same dead code. Two findings are
// considered the same when they share (Language, File, Line, Kind).
// Symbol is intentionally excluded because adapters use different
// formats (e.g., staticcheck emits "unusedHelper" where xdeadcode
// emits "main.unusedHelper" for the same function); kind is included
// to prevent collapsing distinct findings that happen to share a line.
//
// On collapse:
//
//   - Symbol picks the longest one (proxy for "most qualified" — a
//     fully-qualified `pkg.Name` beats a bare `Name`)
//   - Tools is the sorted union of every contributing adapter
//   - Tool stays as the primary (the adapter that supplied the
//     winning symbol; ties broken by the first one seen)
//   - Confidence is the max of contributing confidences
//   - Message picks the longest one
//   - Evidence is merged with first-key-wins on collisions; entries
//     get a `dedupe.tools_count` count for diagnostics
//   - FixHint stays as the primary's
//
// Findings without duplicates pass through unchanged but get
// Tools = [Tool] populated for schema consistency.
//
// Stable for tests: input order is preserved (the runner sorts
// findings by file/line/symbol BEFORE calling this).
func dedupeFindings(findings []finding.Finding) []finding.Finding {
	if len(findings) == 0 {
		return findings
	}

	type key struct {
		language string
		file     string
		line     int
		kind     finding.Kind
	}

	// Bucket findings by key, preserving the order each key first
	// appeared so the output is stable.
	buckets := map[key][]finding.Finding{}
	var order []key
	for _, f := range findings {
		k := key{f.Language, f.File, f.Line, f.Kind}
		if _, seen := buckets[k]; !seen {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], f)
	}

	out := make([]finding.Finding, 0, len(order))
	for _, k := range order {
		group := buckets[k]
		out = append(out, mergeGroup(group))
	}
	return out
}

// mergeGroup collapses N findings sharing the dedupe key into one.
func mergeGroup(group []finding.Finding) finding.Finding {
	if len(group) == 1 {
		// Common case: ensure Tools is populated for schema
		// consistency.
		f := group[0]
		if len(f.Tools) == 0 {
			f.Tools = []string{f.Tool}
		}
		return f
	}

	// Pick the finding with the longest symbol as the "primary" so
	// its tool, message, fix_hint, and ID format become the merged
	// finding's. Ties broken by the first occurrence (stable).
	primary := 0
	for i := 1; i < len(group); i++ {
		if len(group[i].Symbol) > len(group[primary].Symbol) {
			primary = i
		}
	}
	merged := group[primary]

	// Collect every contributing tool, sorted and deduped.
	toolSet := map[string]bool{}
	for _, f := range group {
		toolSet[f.Tool] = true
	}
	tools := make([]string, 0, len(toolSet))
	for t := range toolSet {
		tools = append(tools, t)
	}
	sort.Strings(tools)
	merged.Tools = tools

	// Pick the longest message (most informative).
	for _, f := range group {
		if len(f.Message) > len(merged.Message) {
			merged.Message = f.Message
		}
	}

	// Take the max confidence.
	for _, f := range group {
		if f.Confidence > merged.Confidence {
			merged.Confidence = f.Confidence
		}
	}

	// Merge evidence: each tool's keys get prefixed with the tool
	// name so distinct values from different tools both survive.
	// First-write wins on prefix-key collisions (stable).
	evidence := map[string]string{}
	for _, f := range group {
		for k, v := range f.Evidence {
			prefixed := f.Tool + "." + k
			if _, exists := evidence[prefixed]; !exists {
				evidence[prefixed] = v
			}
		}
	}
	// Add a marker so consumers can tell at a glance how many tools
	// agreed without parsing Tools.
	evidence["dedupe.tools_count"] = sprintInt(len(tools))
	merged.Evidence = evidence

	return merged
}

// sprintInt is a tiny helper to avoid pulling in fmt for one int.
func sprintInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
