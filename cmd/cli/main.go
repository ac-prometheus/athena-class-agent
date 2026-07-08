package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/tools"
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

Arguments may be positional (first positional → "query", second → "target")
or key=value pairs (e.g. query=hello target=world).

Run 'athena <command> --help' for command-specific help.

Environment:
  RUNTIME_DIR   directory containing harness.sock (default: ./run)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(helpText)
		os.Exit(0)
	}

	switch os.Args[1] {
	case "--help", "-h", "help":
		fmt.Print(helpText)
		os.Exit(0)
	}

	runtimeDir := os.Getenv("RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = "./run"
	}
	socketPath := strings.TrimRight(runtimeDir, "/") + "/harness.sock"

	method := os.Args[1]
	args := parseArgs(os.Args[2:])

	req := tools.SocketRequest{
		Method: method,
		Args:   args,
	}
	payload, err := json.Marshal(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "athena: marshal request: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "athena: connect to %s: %v\n", socketPath, err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := tools.WriteFrame(conn, payload); err != nil {
		fmt.Fprintf(os.Stderr, "athena: send request: %v\n", err)
		os.Exit(1)
	}

	respPayload, err := tools.ReadFrame(conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "athena: read response: %v\n", err)
		os.Exit(1)
	}

	var resp tools.SocketResponse
	if err := json.Unmarshal(respPayload, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "athena: parse response: %v\n", err)
		os.Exit(1)
	}

	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "athena: %s\n", resp.Error)
		os.Exit(1)
	}
	fmt.Println(resp.Result)
}

// parseArgs converts positional and key=value args to a map.
// First bare positional → "query"; second → "target".
func parseArgs(argv []string) map[string]any {
	result := make(map[string]any)
	positional := 0
	for _, arg := range argv {
		if idx := strings.Index(arg, "="); idx > 0 {
			key := arg[:idx]
			val := arg[idx+1:]
			result[key] = val
		} else {
			switch positional {
			case 0:
				result["query"] = arg
			case 1:
				result["target"] = arg
			}
			positional++
		}
	}
	return result
}
