// Command deadcode is a multi-language dead-code detection orchestrator.
// See docs/SPEC.md for the architecture.
package main

import (
	"os"

	"github.com/sglyon/deadcode/cmd"
)

func main() {
	os.Exit(cmd.Main(os.Args))
}
