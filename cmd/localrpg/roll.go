package main

import (
	"fmt"
	"io"
	"os"

	"github.com/darkliquid/localrpg/pkg/rules"
)

func handleRollCommand(args []string) {
	code := runRollCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

func runRollCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "Usage: localrpg roll <notation> (e.g. 1d20+5, 4d6kh3, 3dF)")
		return 1
	}

	notation := args[0]
	res, err := rules.EvaluateRoll(notation)
	if err != nil {
		fmt.Fprintf(stderr, "Error evaluating roll %q: %v\n", notation, err)
		return 1
	}

	fmt.Fprintf(stdout, "Roll: %s\n", res.Notation)
	fmt.Fprintf(stdout, "Total: %d\n", res.Total)
	if res.Successes > 0 {
		fmt.Fprintf(stdout, "Successes: %d\n", res.Successes)
	}
	fmt.Fprintf(stdout, "Dice rolled: %d\n", res.RollCount)
	return 0
}
