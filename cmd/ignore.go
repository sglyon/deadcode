package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sglyon/deadcode/internal/ignore"
)

// NewIgnoreCmd builds the `deadcode ignore` parent command and its
// list/validate children.
func NewIgnoreCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:           "ignore",
		Short:         "Manage the unified .deadcode-ignore.toml file",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	parent.AddCommand(newIgnoreListCmd())
	parent.AddCommand(newIgnoreValidateCmd())
	return parent
}

func newIgnoreListCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:           "list",
		Short:         "Print all rules in the discovered (or specified) ignore file",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rs, err := loadForIgnoreCmd(path)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if rs == nil || rs.Empty() {
				fmt.Fprintln(out, "no .deadcode-ignore.toml found")
				return nil
			}
			fmt.Fprintf(out, "%s  (%d rule(s))\n\n", rs.Path, len(rs.Rules))
			for i, r := range rs.Rules {
				fmt.Fprintf(out, "[%d] reason: %s\n", i, r.Reason)
				if r.ID != "" {
					fmt.Fprintf(out, "    id:        %s\n", r.ID)
				}
				if r.File != "" {
					fmt.Fprintf(out, "    file:      %s\n", r.File)
				}
				if r.Symbol != "" {
					fmt.Fprintf(out, "    symbol:    %s\n", r.Symbol)
				}
				if len(r.Kinds) > 0 {
					fmt.Fprintf(out, "    kinds:     %s\n", strings.Join(r.Kinds, ","))
				}
				if len(r.Languages) > 0 {
					fmt.Fprintf(out, "    languages: %s\n", strings.Join(r.Languages, ","))
				}
				if len(r.Tools) > 0 {
					fmt.Fprintf(out, "    tools:     %s\n", strings.Join(r.Tools, ","))
				}
				fmt.Fprintln(out)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "file", "", "explicit ignore file path")
	return cmd
}

func newIgnoreValidateCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:           "validate",
		Short:         "Parse and validate the ignore file (exits non-zero on any error)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rs, err := loadForIgnoreCmd(path)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if rs == nil || rs.Empty() {
				fmt.Fprintln(out, "no .deadcode-ignore.toml found (nothing to validate)")
				return nil
			}
			errs := rs.Validate()
			if len(errs) == 0 {
				fmt.Fprintf(out, "%s: %d rule(s), all valid\n", rs.Path, len(rs.Rules))
				return nil
			}
			fmt.Fprintf(os.Stderr, "%s: %d error(s):\n", rs.Path, len(errs))
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, "  -", e)
			}
			os.Exit(1)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "file", "", "explicit ignore file path")
	return cmd
}

func loadForIgnoreCmd(explicit string) (*ignore.Ruleset, error) {
	if explicit != "" {
		return ignore.Load(explicit, true)
	}
	return ignore.Discover(".")
}
