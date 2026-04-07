package elixir

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/finding"
)

// parseMixCompileOutput converts the captured stderr+stdout from
// `mix compile` into normalized Findings.
//
// Elixir 1.18+ emits warnings as multi-line box-drawn diagnostic
// blocks like:
//
//	    warning: function private_dead/0 is unused
//	    │
//	 16 │   defp private_dead do
//	    │        ~
//	    │
//	    └─ lib/orphan.ex:16:8: Sample.Orphan (module)
//
// A single warning block can have MULTIPLE `└─` location lines when
// the same issue applies to multiple call sites (e.g. a deprecated
// function called from three places). We emit one Finding per
// location line.
//
// Mix also groups compilation by application, delimited by lines like
// `==> app_name`. When scanning a user's project, we want warnings
// from THEIR app only — not from the dozens of hex deps that mix
// transitively re-compiles. The caller passes the user's app name via
// keepApp; any warnings encountered outside that section are skipped.
// If keepApp is empty, all sections are kept (useful for tests and as
// a safety fallback when we can't determine the app name).
//
// The body of each warning (everything between `warning:` and the
// closing `└─`) is preserved in evidence.tool_raw so users can see
// exactly what mix said.
func parseMixCompileOutput(out []byte, projectRoot, keepApp string, opts adapter.RunOptions) []finding.Finding {
	var findings []finding.Finding
	var current *warningBlock
	currentSection := ""

	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

		if sec, ok := matchSectionMarker(line); ok {
			// Section transition — drop any in-progress block (a
			// warning straddling a section marker is malformed).
			current = nil
			currentSection = sec
			continue
		}

		if isWarningStart(line) {
			// Flush any in-progress block (shouldn't normally
			// happen — warnings always terminate with a └─ line).
			current = newWarningBlock(line, currentSection)
			continue
		}

		if current == nil {
			continue
		}
		current.lines = append(current.lines, line)

		if loc := matchLocationLine(line); loc != nil {
			// Multiple └─ lines can follow a single warning body.
			// Append and keep the block open so the next └─ also
			// gets picked up. Blocks are flushed on the next
			// `warning:`, `==>`, or EOF.
			current.locations = append(current.locations, loc)

			// Match rule:
			//   - keepApp == ""            → no filter, keep all
			//   - current.section == ""    → standalone no-deps project;
			//                                mix didn't emit any ==> markers
			//                                so this is implicitly the project
			//   - current.section == keepApp → explicit section match
			if keepApp == "" || current.section == "" || current.section == keepApp {
				if f, ok := current.toFinding(projectRoot, loc); ok {
					findings = append(findings, f)
				}
			}
		}
	}
	return findings
}

// sectionMarkerRe matches `==> app_name` lines that delimit the
// per-application chunks in mix compile output.
var sectionMarkerRe = regexp.MustCompile(`^==>\s+(\S+)\s*$`)

func matchSectionMarker(line string) (string, bool) {
	m := sectionMarkerRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// warningBlock accumulates the lines of one box-drawn warning as
// they stream in. A block may be emitted multiple times if it has
// multiple closing └─ locations.
type warningBlock struct {
	header    string // the "warning: ..." line stripped of indent
	section   string // the ==> app_name section this warning belongs to
	lines     []string
	locations []*parsedLocation
}

func newWarningBlock(line, section string) *warningBlock {
	return &warningBlock{
		header:  strings.TrimSpace(line),
		section: section,
		lines:   []string{line},
	}
}

// parsedLocation is the structured form of `└─ file:line[:col]: ctx`.
type parsedLocation struct {
	file    string // relative to project root, as mix emitted it
	line    int
	col     int    // 0 if not present
	context string // e.g., "Sample.Orphan (module)" or "Sample.classify/1"
}

// warningStartRe matches a line that begins a warning block. The
// indent is variable (mix uses 4 spaces, but we don't depend on it).
var warningStartRe = regexp.MustCompile(`^\s*warning:\s*(.*)$`)

// locationLineRe matches the └─ closing line of a warning block:
//
//	└─ lib/orphan.ex:16:8: Sample.Orphan (module)
//	└─ lib/sample.ex:16: Sample.main/0
//
// The leading box character is U+2514 U+2500 followed by a space.
var locationLineRe = regexp.MustCompile(`^\s*└─\s+(.+?):(\d+)(?::(\d+))?:\s*(.+)$`)

func isWarningStart(line string) bool {
	return warningStartRe.MatchString(line)
}

func matchLocationLine(line string) *parsedLocation {
	m := locationLineRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	lineNum, err := strconv.Atoi(m[2])
	if err != nil {
		return nil
	}
	col := 0
	if m[3] != "" {
		col, _ = strconv.Atoi(m[3])
	}
	return &parsedLocation{
		file:    m[1],
		line:    lineNum,
		col:     col,
		context: strings.TrimSpace(m[4]),
	}
}

// toFinding converts the block into a Finding anchored at the given
// location. Called once per `└─` line so a multi-location block
// produces N findings sharing the same message but with distinct
// file:line identities.
func (b *warningBlock) toFinding(projectRoot string, loc *parsedLocation) (finding.Finding, bool) {
	if loc == nil {
		return finding.Finding{}, false
	}
	headerMsg := strings.TrimPrefix(b.header, "warning:")
	headerMsg = strings.TrimSpace(headerMsg)

	kind := classifyKind(headerMsg, b.lines)
	symbol := extractSymbol(headerMsg, loc.context, kind)

	abs := loc.file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(projectRoot, abs)
	}

	return finding.Finding{
		ID:         fmt.Sprintf("ex:%s:%d:%s:%s", loc.file, loc.line, kind, symbol),
		File:       abs,
		Line:       loc.line,
		Symbol:     symbol,
		Kind:       kind,
		Language:   "elixir",
		Tool:       "mix_compile",
		Confidence: confidenceFor(kind),
		Message:    headerMsg,
		Evidence: map[string]string{
			"tool_raw":     strings.Join(b.lines, "\n"),
			"context":      loc.context,
			"elixir_phase": classifyPhase(headerMsg, b.lines),
			"mix_section":  b.section,
		},
		FixHint: finding.FixDelete,
	}, true
}

// classifyKind maps an Elixir warning message to one of our finding
// kinds. The mapping is intentionally pattern-based, not regex-deep,
// so it's easy to extend as new warning shapes appear in future
// Elixir releases.
func classifyKind(header string, body []string) finding.Kind {
	switch {
	case strings.Contains(header, "is unused"):
		// "function X/N is unused" -- private function, never called
		return finding.KindUnusedFunction
	case strings.Contains(header, "is never used"):
		// "this clause of defp X/N is never used"
		return finding.KindUnreachable
	case strings.Contains(header, "will never match"):
		// "the following clause will never match"
		return finding.KindUnreachable
	case strings.Contains(header, "unused alias"):
		return finding.KindUnusedImport
	case strings.Contains(header, "unused import"):
		return finding.KindUnusedImport
	case strings.Contains(header, "unused variable"):
		return finding.KindUnusedVariable
	}
	// Type-system findings often have multi-line bodies; check the
	// body for telltale phrases.
	joined := strings.Join(body, "\n")
	if strings.Contains(joined, "typing violation") {
		return finding.KindUnreachable
	}
	// Default for an unknown shape: treat it as an unreachable hint
	// rather than dropping it. The user still sees the message via
	// evidence.tool_raw.
	return finding.KindUnreachable
}

// classifyPhase distinguishes type-system findings from the older
// xref-style findings the compiler still emits. Useful for users
// who want to filter by source.
func classifyPhase(header string, body []string) string {
	joined := strings.Join(body, "\n")
	if strings.Contains(joined, "typing violation") || strings.Contains(joined, "which has type:") {
		return "type_checker"
	}
	if strings.Contains(header, "is unused") || strings.Contains(header, "unused ") {
		return "compiler_xref"
	}
	if strings.Contains(header, "will never match") || strings.Contains(header, "is never used") {
		return "type_checker"
	}
	return "compiler"
}

// confidenceFor maps kind to confidence. The Elixir compiler is
// authoritative within its scope: when it says something is unused,
// it's right. When the type system says a clause is unreachable,
// it's also right (sound reasoning).
//
// We rate the type-system findings slightly higher than the older
// "is unused" findings out of an abundance of caution — the type
// system is provably sound, while the unused-private-function check
// has historically had occasional false positives around macros.
func confidenceFor(kind finding.Kind) float64 {
	switch kind {
	case finding.KindUnreachable:
		return 0.95
	case finding.KindUnusedFunction:
		return 0.90
	default:
		return 0.85
	}
}

// extractSymbol pulls the best identifier we can out of the warning
// header and the location context.
//
// The location context is one of:
//   - "Sample.Orphan (module)"            -> bare module, header names the function
//   - "Sample.main/0"                     -> already qualified, header is descriptive
//   - "Sample.classify/1"                 -> already qualified, header is descriptive
//
// Rule: if the context is already a qualified Module.func/arity, use
// it directly — that's the most precise identifier we have. Only when
// the context is a bare module do we combine it with a function name
// pulled from the header.
func extractSymbol(header, context string, kind finding.Kind) string {
	if mod, ok := bareModuleFromContext(context); ok {
		if m := headerFuncRe.FindStringSubmatch(header); m != nil {
			return mod + "." + m[1]
		}
		return mod
	}
	// Context is already a qualified function — this is the common
	// case for type-system findings on a clause inside an enclosing
	// function.
	return context
}

// headerFuncRe matches `function name/N` and `defp name/N` patterns.
//
// Captured group 1 is the bare `name/N` form.
var headerFuncRe = regexp.MustCompile(`(?:function|defp|def)\s+([\w!?]+/\d+)`)

// bareModuleFromContext extracts a module name from a context like
// "Sample.Orphan (module)". Returns ("", false) if the context names
// a function instead.
func bareModuleFromContext(context string) (string, bool) {
	if strings.HasSuffix(context, "(module)") {
		return strings.TrimSpace(strings.TrimSuffix(context, "(module)")), true
	}
	return "", false
}
