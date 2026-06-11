package main

import (
	"fmt"
	"os"
)

const helpText = `athena — Athena-class agent CLI

Usage:
  athena <command> [args]

Commands:
  memory    search, recall, link memory entries
  people    manage relational profiles
  channel   list, read, reply across channels
  reflect   author and retrieve reflections
  knowledge query and update the world model
  settings  view and adjust agent settings
  advisor   request a second opinion from an external model
  discover  discover available tool groups

Run 'athena <command> --help' for command-specific help.

Note: Full CLI dispatch lands in Phase 4. This binary is a stub.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(helpText)
		os.Exit(0)
	}

	switch os.Args[1] {
	case "--help", "-h", "help":
		fmt.Print(helpText)
	default:
		fmt.Fprintf(os.Stderr, "athena: unknown command %q (Phase 4 CLI dispatch not yet implemented)\n", os.Args[1])
		os.Exit(1)
	}
}
