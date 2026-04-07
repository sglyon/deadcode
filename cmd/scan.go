package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sglyon/deadcode/internal/finding"
	"github.com/sglyon/deadcode/internal/ignore"
	"github.com/sglyon/deadcode/internal/report"
	"github.com/sglyon/deadcode/internal/runner"
)

// scanFlags holds the parsed flag values shared between `deadcode scan`
// and `deadcode <path>`. Both commands attach the same flag set, so the
// values land on the same struct via the package-level variables below.
type scanFlags struct {
	jsonOut             bool
	outputPath          string
	langStr             string
	kindStr             string
	minConfidence       float64
	excludeTests        bool
	ignoreDecoratorsStr string
	noDefaultDecorators bool
	ignoreFilePath      string
	noIgnoreFile        bool
	showIgnored         bool
	threshold           int
	exitCode            int
	verbose             bool
}

// scanFlagValues is shared between root + scan because Cobra doesn't
// support inheriting flag *values* via PersistentFlags when only the
// child reads them. Both commands bind to this single instance.
var scanFlagValues scanFlags

func addScanFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.BoolVar(&scanFlagValues.jsonOut, "json", false, "Emit JSON instead of console output")
	f.StringVarP(&scanFlagValues.outputPath, "output", "o", "", "Write report to this file (default stdout)")
	f.StringVar(&scanFlagValues.langStr, "lang", "", "Restrict to languages (comma-separated, e.g. python,typescript)")
	f.StringVar(&scanFlagValues.kindStr, "kind", "", "Restrict to finding kinds (comma-separated)")
	f.Float64Var(&scanFlagValues.minConfidence, "min-confidence", 0, "Drop findings below this confidence (0.0-1.0)")
	f.BoolVar(&scanFlagValues.excludeTests, "exclude-tests", false, "Drop findings whose only callers are tests")
	f.StringVar(&scanFlagValues.ignoreDecoratorsStr, "ignore-decorators", "",
		"Add Python decorator glob patterns to the framework ignore list (e.g. \"@my.task,@app.event\")")
	f.BoolVar(&scanFlagValues.noDefaultDecorators, "no-default-decorators", false,
		"Disable the built-in framework decorator list")
	f.StringVar(&scanFlagValues.ignoreFilePath, "ignore-file", "",
		"Use this .deadcode-ignore.toml (default: discover upward from the scan root)")
	f.BoolVar(&scanFlagValues.noIgnoreFile, "no-ignore-file", false, "Disable ignore-file loading entirely")
	f.BoolVar(&scanFlagValues.showIgnored, "show-ignored", false, "Print suppressed findings (with reasons)")
	f.IntVar(&scanFlagValues.threshold, "threshold", -1, "Fail if total findings exceeds this (-1 disables)")
	f.IntVar(&scanFlagValues.exitCode, "exit-code", 0, "Exit code to use when threshold is exceeded")
	f.BoolVarP(&scanFlagValues.verbose, "verbose", "v", false, "Verbose console output (preserves tool raw lines)")
}

// NewScanCmd builds the explicit `deadcode scan` subcommand.
func NewScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan [path...]",
		Short: "Run dead-code analyzers and emit findings",
		Long: `Run every applicable adapter against the given paths (default "."),
normalize the output, apply the ignore filter, and emit findings to
stdout (or --output) as console text or JSON.`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doScan(cmd, args)
		},
	}
	addScanFlags(cmd)
	return cmd
}

// runScanFromRoot is invoked when the user types `deadcode <path>` with
// no subcommand. It dispatches to the same scan logic as `deadcode scan`.
func runScanFromRoot(cmd *cobra.Command, args []string) error {
	return doScan(cmd, args)
}

func doScan(cmd *cobra.Command, args []string) error {
	paths := args
	if len(paths) == 0 {
		paths = []string{"."}
	}

	kinds := make([]finding.Kind, 0)
	for _, k := range runner.SplitCSV(scanFlagValues.kindStr) {
		kinds = append(kinds, finding.Kind(k))
	}

	rules, err := loadIgnoreRules(scanFlagValues.ignoreFilePath, scanFlagValues.noIgnoreFile, paths)
	if err != nil {
		return err
	}
	if rules != nil {
		if errs := rules.Validate(); len(errs) > 0 {
			fmt.Fprintln(os.Stderr, "deadcode: invalid ignore file:")
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, "  -", e)
			}
			return fmt.Errorf("ignore file failed validation")
		}
	}

	opts := runner.Options{
		Paths:               paths,
		Languages:           runner.SplitCSV(scanFlagValues.langStr),
		Kinds:               kinds,
		MinConfidence:       scanFlagValues.minConfidence,
		ExcludeTests:        scanFlagValues.excludeTests,
		IgnoreDecorators:    runner.SplitCSV(scanFlagValues.ignoreDecoratorsStr),
		NoDefaultDecorators: scanFlagValues.noDefaultDecorators,
		IgnoreRules:         rules,
		Verbose:             scanFlagValues.verbose,
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	result, err := runner.Run(ctx, builtinAdapters(), opts)
	if err != nil {
		return err
	}

	out := os.Stdout
	if scanFlagValues.outputPath != "" {
		f, err := os.Create(scanFlagValues.outputPath)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}

	if scanFlagValues.jsonOut {
		if err := report.JSON(out, result, scanFlagValues.showIgnored); err != nil {
			return err
		}
	} else {
		if err := report.Console(out, result, scanFlagValues.verbose, scanFlagValues.showIgnored); err != nil {
			return err
		}
	}

	if scanFlagValues.threshold >= 0 && len(result.Findings) > scanFlagValues.threshold {
		os.Exit(scanFlagValues.exitCode)
	}
	return nil
}

// loadIgnoreRules resolves the ignore file based on the user's flags.
//   - --no-ignore-file: never load anything (returns nil ruleset)
//   - --ignore-file <path>: load that exact path; missing is an error
//   - default: discover .deadcode-ignore.toml upward from the first
//     scan path; missing is fine
func loadIgnoreRules(explicitPath string, noFile bool, paths []string) (*ignore.Ruleset, error) {
	if noFile {
		return nil, nil
	}
	if explicitPath != "" {
		return ignore.Load(explicitPath, true)
	}
	start := "."
	if len(paths) > 0 {
		start = paths[0]
	}
	rs, err := ignore.Discover(start)
	if err != nil {
		return nil, err
	}
	if rs.Empty() {
		return nil, nil
	}
	return rs, nil
}
