package elixir

import (
	"path/filepath"
	"strings"

	"github.com/sglyon/deadcode/internal/finding"
)

// DefaultIgnoreSymbols is the built-in list of fully-qualified
// function patterns that look unused but are dispatched by Phoenix,
// OTP, or Plug at runtime. The Elixir compiler can't see these
// callers because they go through behaviour callbacks, the Phoenix
// router DSL, or PubSub channels.
//
// Patterns use doublestar glob syntax. Symbol form is
// "Module.function/arity" — see parser.go.
//
// This list is intentionally conservative. It's better to miss
// suppressing a false positive than to silently swallow a real bug.
// Users can extend it via .deadcode-ignore.toml.
var DefaultIgnoreSymbols = []string{
	// Phoenix controller default actions
	"*Controller.index/2",
	"*Controller.show/2",
	"*Controller.new/2",
	"*Controller.create/2",
	"*Controller.edit/2",
	"*Controller.update/2",
	"*Controller.delete/2",
	"*Controller.action/2",
	"*Controller.call/2",

	// GenServer / Agent / Task callbacks (also covered by @impl
	// annotations on most projects, but kept here as belt + braces).
	"*.handle_call/3",
	"*.handle_cast/2",
	"*.handle_info/2",
	"*.handle_continue/2",
	"*.init/1",
	"*.terminate/2",
	"*.code_change/3",
	"*.format_status/1",
	"*.format_status/2",

	// Phoenix LiveView callbacks
	"*Live.mount/3",
	"*Live.handle_event/3",
	"*Live.handle_info/2",
	"*Live.handle_params/3",
	"*Live.handle_async/3",
	"*Live.render/1",
	"*LiveView.mount/3",
	"*LiveView.render/1",
	"*Component.render/1",
	"*Component.update/2",
	"*Component.handle_event/3",

	// Phoenix Channel callbacks
	"*Channel.join/3",
	"*Channel.handle_in/3",
	"*Channel.handle_out/3",
	"*Channel.terminate/2",

	// Plug behaviour
	"*.call/2",

	// Ecto Repo and changeset hooks
	"*.changeset/2",
	"*.changeset/1",
}

// applyDefaultIgnores filters out findings whose symbol matches one of
// the built-in Phoenix/OTP patterns. This runs before the unified
// .deadcode-ignore.toml filter so user rules take precedence over the
// defaults (the runner sees the kept findings, applies its filter, and
// only then renders).
//
// Why this is hardcoded with no flag in v0.4: the built-in tools
// (mix compile + xref) currently don't catch unused public functions
// across modules, which is precisely what these patterns target. The
// filter is essentially dormant until v0.4.x adds mix_unused. We're
// pre-installing the safety net.
func applyDefaultIgnores(findings []finding.Finding) []finding.Finding {
	if len(findings) == 0 {
		return findings
	}
	out := findings[:0]
	for _, f := range findings {
		if matchesAnyDefault(f.Symbol) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// matchesAnyDefault tests a symbol against the built-in pattern set.
// Doublestar treats `/` as a separator, which is fine here because
// our symbol form is `Module.func/arity` — exactly two segments.
func matchesAnyDefault(symbol string) bool {
	if symbol == "" {
		return false
	}
	for _, pattern := range DefaultIgnoreSymbols {
		ok, err := filepath.Match(pattern, symbol)
		if err == nil && ok {
			return true
		}
		// Fall back to a manual prefix match for "*Controller.X/N"
		// shapes where filepath.Match's strict semantics don't quite
		// fit. We split the pattern at the first '*' and check the
		// suffix matches the symbol's tail.
		if strings.HasPrefix(pattern, "*") {
			suffix := strings.TrimPrefix(pattern, "*")
			if strings.HasSuffix(symbol, suffix) {
				return true
			}
		}
	}
	return false
}
