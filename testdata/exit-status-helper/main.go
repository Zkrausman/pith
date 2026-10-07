// A deterministic child executable for CLI exit-status tests. It uses no shell,
// model, network, or external command and is never part of the shipped binary.
package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) != 3 {
		os.Exit(99)
	}
	code, err := strconv.Atoi(os.Args[1])
	if err != nil {
		os.Exit(99)
	}
	switch os.Args[2] {
	case "silent":
	case "success-text":
		fmt.Fprintln(os.Stdout, "All checks passed")
	case "stderr-only":
		fmt.Fprintln(os.Stderr, "diagnostic marker")
	case "streams":
		fmt.Fprintln(os.Stdout, "All checks passed")
		fmt.Fprintln(os.Stderr, "diagnostic marker")
	case "kill":
		process, err := os.FindProcess(os.Getpid())
		if err != nil || process.Kill() != nil {
			os.Exit(99)
		}
	default:
		os.Exit(99)
	}
	os.Exit(code)
}
