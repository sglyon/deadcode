package elixir

import (
	"os"
	"path/filepath"
	"regexp"
)

// projectAppName extracts the user's application name from the root
// mix.exs so the parser can scope warnings to only THAT section of
// mix compile output (skipping the dozens of hex deps that mix
// re-compiles on --force).
//
// Parses a line like:
//
//	app: :my_app,
//
// Returns "" if we can't determine the name — callers should treat
// that as "keep all sections" (noisier but safe).
//
// This is deliberately a cheap regex, not a full Elixir AST parser.
// The `app: :atom,` form is in the generated mix.exs template and
// is overwhelmingly stable across projects. Umbrella projects (which
// have multiple child apps under apps/*/mix.exs) are not handled in
// v0.4 — we return "" and accept the dep noise as a v0.4 known
// limitation.
func projectAppName(projectRoot string) string {
	data, err := os.ReadFile(filepath.Join(projectRoot, "mix.exs"))
	if err != nil {
		return ""
	}
	// Bail out early on umbrella projects: their root mix.exs has
	// `apps_path: "apps"` and no top-level `app:`. Returning ""
	// keeps all sections (noisy but we won't silently drop the
	// user's child-app warnings).
	if appsPathRe.Match(data) {
		return ""
	}
	m := appNameRe.FindSubmatch(data)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// appNameRe matches `app: :my_app_name` allowing for whitespace and
// the name to contain lowercase letters, digits, and underscores.
var appNameRe = regexp.MustCompile(`app:\s*:([a-z][a-z0-9_]*)`)

// appsPathRe signals an umbrella root project; we handle those as a
// v0.4 known limitation by returning "" (all-sections kept).
var appsPathRe = regexp.MustCompile(`apps_path:\s*"`)
