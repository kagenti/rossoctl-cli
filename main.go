// Package main is the entry point for the rossoctl CLI.
//
// It intentionally contains almost no logic: all command wiring lives in the
// cmd package, and all business logic lives under internal/. This keeps main
// trivial and makes the command tree easy to test in isolation.
package main

import "github.com/kagenti/rossoctl-cli/cmd"

func main() {
	cmd.Execute()
}
