// Package cmd implements the deadcode CLI using Cobra.
//
// Each subcommand is built by a NewXxxCmd constructor and attached to
// the root in NewRootCmd. No init() side effects — everything is wired
// explicitly so the command tree is testable and obvious.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/adapters/javascript"
	"github.com/sglyon/deadcode/internal/adapters/python"
)

const Version = "0.3.0"

// builtinAdapters returns every adapter compiled into this binary.
// Adding a language means adding it here.
func builtinAdapters() []adapter.Adapter {
	return []adapter.Adapter{
		python.New(),
		javascript.New(),
	}
}

// NewRootCmd builds the full command tree. Exported so tests can build
// fresh trees and so main is a one-liner.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "deadcode [path]",
		Short: "Multi-language dead-code detection orchestrator",
		Long: `deadcode runs the right per-language analyzer for every file in a
repo, normalizes the output to a unified Finding schema, and emits an
agent-friendly report.

With no subcommand, deadcode runs 'scan' on the given path (default ".").`,
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Accept arbitrary positional args so `deadcode <path>` works.
		// Cobra still dispatches to a known subcommand first; only
		// non-subcommand args reach RunE.
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScanFromRoot(cmd, args)
		},
	}

	// Make `deadcode --version` print just the version (Cobra default
	// includes the binary name, which is fine but slightly noisy).
	root.SetVersionTemplate("deadcode {{.Version}}\n")

	// Scan flags live on the root so `deadcode .` and
	// `deadcode scan .` accept the same set.
	addScanFlags(root)

	// Subcommands.
	root.AddCommand(NewScanCmd())
	root.AddCommand(NewDoctorCmd())
	root.AddCommand(NewAdaptersCmd())
	root.AddCommand(NewIgnoreCmd())

	return root
}

// Execute runs the root command and exits with the appropriate code.
// main.go calls this.
func Execute() {
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "deadcode:", err)
		os.Exit(2)
	}
}
