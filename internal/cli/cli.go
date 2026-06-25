// Package cli dispatches Tanaka subcommands.
package cli

import (
	"fmt"
	"io"
)

const version = "0.0.1"

// Run dispatches a subcommand and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: tanaka <command> [args]")
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "tanaka %s\n", version)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		return 2
	}
}
