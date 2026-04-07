// Package cmd implements the deadcode CLI subcommands.
//
// v0.1 uses stdlib `flag` for hermetic builds. If subcommand sprawl warrants
// it in v0.2, migrate to Cobra.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/sglyon/deadcode/internal/adapter"
	"github.com/sglyon/deadcode/internal/adapters/python"
)

const Version = "0.1.0"

// builtinAdapters returns every adapter compiled into this binary.
// New adapters get added here. v0.1 ships only the Python/vulture adapter.
func builtinAdapters() []adapter.Adapter {
	return []adapter.Adapter{
		python.New(),
	}
}

// Main is the CLI entry point. main.go calls this.
func Main(args []string) int {
	if len(args) < 2 {
		// Default: `deadcode` with no args == `deadcode scan .`
		return runScan([]string{"."})
	}

	switch args[1] {
	case "scan":
		return runScan(args[2:])
	case "doctor":
		return runDoctor(args[2:])
	case "adapters":
		return runAdapters()
	case "ignore":
		return runIgnore(args[2:])
	case "version", "--version", "-v":
		fmt.Println("deadcode", Version)
		return 0
	case "help", "--help", "-h":
		printRootHelp()
		return 0
	default:
		// Bare path argument — treat as `deadcode scan <path>`
		if !looksLikeFlag(args[1]) {
			return runScan(args[1:])
		}
		fmt.Fprintf(os.Stderr, "deadcode: unknown command %q\n", args[1])
		printRootHelp()
		return 2
	}
}

func looksLikeFlag(s string) bool {
	return len(s) > 0 && s[0] == '-'
}

func printRootHelp() {
	fmt.Print(`deadcode - multi-language dead code detection

USAGE:
  deadcode [path]                    scan path (default: .)
  deadcode scan [path] [flags]       explicit scan
  deadcode doctor                    check tool availability
  deadcode adapters                  list supported languages and tools
  deadcode ignore <subcommand>       manage .deadcode-ignore.toml
  deadcode version                   print version

Run 'deadcode scan --help' or 'deadcode ignore help' for details.
`)
}

// withCancelOnInterrupt is a helper used by long-running subcommands.
func withCancelOnInterrupt(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(ctx)
}
