package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func runDoctor(args []string) int {
	ctx := context.Background()
	adapters := builtinAdapters()

	fmt.Println("deadcode doctor")
	fmt.Println(strings.Repeat("=", 40))
	fmt.Printf("Adapters: %d built in\n\n", len(adapters))

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
		fmt.Printf("  %-12s  [%s]  langs=%s\n", a.Name(), status, strings.Join(a.Languages(), ","))
		if hint != "" {
			fmt.Printf("                hint: %s\n", hint)
		}
	}

	fmt.Println()
	if missing == 0 {
		fmt.Println("All adapters available.")
		return 0
	}
	fmt.Fprintf(os.Stderr, "%d adapter(s) unavailable. Install the missing tools above and re-run.\n", missing)
	return 1
}

func runAdapters() int {
	adapters := builtinAdapters()
	fmt.Println("Built-in adapters:")
	for _, a := range adapters {
		fmt.Printf("  - %-12s langs=%s\n", a.Name(), strings.Join(a.Languages(), ","))
	}
	return 0
}
