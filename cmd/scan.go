package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/sglyon/deadcode/internal/finding"
	"github.com/sglyon/deadcode/internal/report"
	"github.com/sglyon/deadcode/internal/runner"
)

func runScan(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `deadcode scan - run dead-code analyzers and emit findings

USAGE:
  deadcode scan [flags] [path...]

FLAGS:
  --json                Emit JSON instead of console output
  -o, --output <path>   Write report to file (default stdout)
  --lang <list>         Restrict to languages (comma-separated)
  --kind <list>         Restrict to finding kinds (comma-separated)
  --min-confidence <f>  Drop findings below this confidence (0.0-1.0)
  --exclude-tests       Drop findings whose only callers are tests
  --ignore-decorators <list>
                        Add decorator glob patterns to the framework
                        ignore list (e.g. "@my.task,@app.event")
  --no-default-decorators
                        Disable the built-in framework decorator list
  --threshold <n>       Fail if total findings exceeds this
  --exit-code <n>       Exit code when threshold exceeded (default 0)
  -v, --verbose         Verbose console output (preserves tool raw lines)
`)
	}

	var (
		jsonOut             bool
		outputPath          string
		langStr             string
		kindStr             string
		minConfidence       float64
		excludeTests        bool
		ignoreDecoratorsStr string
		noDefaultDecorators bool
		threshold           int
		exitCode            int
		verbose             bool
	)
	fs.BoolVar(&jsonOut, "json", false, "")
	fs.StringVar(&outputPath, "output", "", "")
	fs.StringVar(&outputPath, "o", "", "")
	fs.StringVar(&langStr, "lang", "", "")
	fs.StringVar(&kindStr, "kind", "", "")
	fs.Float64Var(&minConfidence, "min-confidence", 0, "")
	fs.BoolVar(&excludeTests, "exclude-tests", false, "")
	fs.StringVar(&ignoreDecoratorsStr, "ignore-decorators", "", "")
	fs.BoolVar(&noDefaultDecorators, "no-default-decorators", false, "")
	fs.IntVar(&threshold, "threshold", -1, "")
	fs.IntVar(&exitCode, "exit-code", 0, "")
	fs.BoolVar(&verbose, "verbose", false, "")
	fs.BoolVar(&verbose, "v", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	kinds := make([]finding.Kind, 0)
	for _, k := range runner.SplitCSV(kindStr) {
		kinds = append(kinds, finding.Kind(k))
	}

	opts := runner.Options{
		Paths:               paths,
		Languages:           runner.SplitCSV(langStr),
		Kinds:               kinds,
		MinConfidence:       minConfidence,
		ExcludeTests:        excludeTests,
		IgnoreDecorators:    runner.SplitCSV(ignoreDecoratorsStr),
		NoDefaultDecorators: noDefaultDecorators,
		Verbose:             verbose,
	}

	ctx, cancel := withCancelOnInterrupt(context.Background())
	defer cancel()

	result, err := runner.Run(ctx, builtinAdapters(), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deadcode:", err)
		return 2
	}

	out := os.Stdout
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "deadcode:", err)
			return 2
		}
		defer f.Close()
		out = f
	}

	if jsonOut {
		if err := report.JSON(out, result); err != nil {
			fmt.Fprintln(os.Stderr, "deadcode:", err)
			return 2
		}
	} else {
		if err := report.Console(out, result, verbose); err != nil {
			fmt.Fprintln(os.Stderr, "deadcode:", err)
			return 2
		}
	}

	if threshold >= 0 && len(result.Findings) > threshold {
		return exitCode
	}
	return 0
}
