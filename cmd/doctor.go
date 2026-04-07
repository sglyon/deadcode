package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// NewDoctorCmd builds the `deadcode doctor` subcommand: checks each
// adapter's underlying tool and prints install hints for missing ones.
func NewDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "doctor",
		Short:         "Check adapter availability and print install hints",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			adapters := builtinAdapters()

			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "deadcode doctor")
			fmt.Fprintln(out, strings.Repeat("=", 40))
			fmt.Fprintf(out, "Adapters: %d built in\n\n", len(adapters))

			missing := 0
			for _, a := range adapters {
				err := a.Check(ctx)
				status := "OK"
				hint := ""
				if err != nil {
					status = "MISSING"
					hint = err.Error()
					missing++
				}
				fmt.Fprintf(out, "  %-12s  [%s]  langs=%s\n", a.Name(), status, strings.Join(a.Languages(), ","))
				if hint != "" {
					fmt.Fprintf(out, "                hint: %s\n", hint)
				}
			}
			fmt.Fprintln(out)

			if missing == 0 {
				fmt.Fprintln(out, "All adapters available.")
				return nil
			}
			fmt.Fprintf(os.Stderr, "%d adapter(s) unavailable. Install the missing tools above and re-run.\n", missing)
			os.Exit(1)
			return nil
		},
	}
}

// NewAdaptersCmd builds the `deadcode adapters` subcommand: a static
// list of every adapter compiled into this binary.
func NewAdaptersCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "adapters",
		Short:         "List built-in adapters and the languages they handle",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Built-in adapters:")
			for _, a := range builtinAdapters() {
				fmt.Fprintf(out, "  - %-12s langs=%s\n", a.Name(), strings.Join(a.Languages(), ","))
			}
			return nil
		},
	}
}
