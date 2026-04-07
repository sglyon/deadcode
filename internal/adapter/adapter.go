// Package adapter defines the contract every per-language analyzer
// integration implements. Adding a language is implementing this interface.
package adapter

import (
	"context"

	"github.com/sglyon/deadcode/internal/finding"
)

// RunOptions controls a single adapter invocation. Add fields here as
// new flags are introduced — adapters can ignore unknown options.
type RunOptions struct {
	// ExcludeTests drops findings whose only callers are test files.
	// Adapters that can't distinguish should ignore this.
	ExcludeTests bool

	// IgnoreDecorators is a list of decorator/annotation glob patterns
	// (e.g. "@app.*", "@router.get") whose decorated symbols should be
	// treated as reachable. Adapters that don't understand decorators
	// should ignore this.
	IgnoreDecorators []string

	// NoDefaultDecorators disables the adapter's built-in default
	// decorator ignore list. Use only when the defaults conflict with
	// the project.
	NoDefaultDecorators bool

	// Verbose preserves chatty tool output for debugging.
	Verbose bool
}

// Adapter is the seam between deadcode core and a specific analyzer.
type Adapter interface {
	// Name returns the analyzer's name (e.g., "vulture", "knip").
	Name() string

	// Languages returns the languages this adapter handles
	// (e.g., []string{"python"}).
	Languages() []string

	// Check verifies the underlying tool is installed and usable.
	// On failure, the returned error MUST contain a human-readable
	// install hint — `doctor` surfaces it directly to the user.
	Check(ctx context.Context) error

	// Run executes the analyzer over the given paths and parses
	// its output into normalized Findings.
	Run(ctx context.Context, paths []string, opts RunOptions) ([]finding.Finding, error)
}
