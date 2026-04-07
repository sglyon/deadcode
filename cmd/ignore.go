package cmd

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sglyon/deadcode/internal/ignore"
)

func runIgnore(args []string) int {
	if len(args) == 0 {
		printIgnoreHelp()
		return 0
	}
	switch args[0] {
	case "list":
		return runIgnoreList(args[1:])
	case "validate":
		return runIgnoreValidate(args[1:])
	case "help", "--help", "-h":
		printIgnoreHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "deadcode ignore: unknown subcommand %q\n", args[0])
		printIgnoreHelp()
		return 2
	}
}

func printIgnoreHelp() {
	fmt.Print(`deadcode ignore - manage the unified .deadcode-ignore.toml file

USAGE:
  deadcode ignore list [--file <path>]
      Print all rules in the discovered (or specified) ignore file.

  deadcode ignore validate [--file <path>]
      Parse and validate the ignore file. Exits non-zero on any error.
`)
}

func runIgnoreList(args []string) int {
	fs := flag.NewFlagSet("ignore list", flag.ContinueOnError)
	var path string
	fs.StringVar(&path, "file", "", "explicit ignore file path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	rs, err := loadForIgnoreCmd(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deadcode ignore:", err)
		return 2
	}
	if rs == nil || rs.Empty() {
		fmt.Println("no .deadcode-ignore.toml found")
		return 0
	}
	fmt.Printf("%s  (%d rule(s))\n\n", rs.Path, len(rs.Rules))
	for i, r := range rs.Rules {
		fmt.Printf("[%d] reason: %s\n", i, r.Reason)
		if r.ID != "" {
			fmt.Printf("    id:        %s\n", r.ID)
		}
		if r.File != "" {
			fmt.Printf("    file:      %s\n", r.File)
		}
		if r.Symbol != "" {
			fmt.Printf("    symbol:    %s\n", r.Symbol)
		}
		if len(r.Kinds) > 0 {
			fmt.Printf("    kinds:     %s\n", strings.Join(r.Kinds, ","))
		}
		if len(r.Languages) > 0 {
			fmt.Printf("    languages: %s\n", strings.Join(r.Languages, ","))
		}
		if len(r.Tools) > 0 {
			fmt.Printf("    tools:     %s\n", strings.Join(r.Tools, ","))
		}
		fmt.Println()
	}
	return 0
}

func runIgnoreValidate(args []string) int {
	fs := flag.NewFlagSet("ignore validate", flag.ContinueOnError)
	var path string
	fs.StringVar(&path, "file", "", "explicit ignore file path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	rs, err := loadForIgnoreCmd(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deadcode ignore:", err)
		return 2
	}
	if rs == nil || rs.Empty() {
		fmt.Println("no .deadcode-ignore.toml found (nothing to validate)")
		return 0
	}
	errs := rs.Validate()
	if len(errs) == 0 {
		fmt.Printf("%s: %d rule(s), all valid\n", rs.Path, len(rs.Rules))
		return 0
	}
	fmt.Fprintf(os.Stderr, "%s: %d error(s):\n", rs.Path, len(errs))
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "  -", e)
	}
	return 1
}

func loadForIgnoreCmd(explicit string) (*ignore.Ruleset, error) {
	if explicit != "" {
		return ignore.Load(explicit, true)
	}
	return ignore.Discover(".")
}
