package main

import (
	"fmt"
	"os"

	"github.com/darkliquid/localrpg/pkg/rules"
)

func handleRollCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: localrpg roll <notation> (e.g. 1d20+5, 4d6kh3, 3dF)")
		os.Exit(1)
	}

	notation := args[0]
	res, err := rules.EvaluateRoll(notation)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error evaluating roll %q: %v\n", notation, err)
		os.Exit(1)
	}

	fmt.Printf("Roll: %s\n", res.Notation)
	fmt.Printf("Total: %d\n", res.Total)
	if res.Successes > 0 {
		fmt.Printf("Successes: %d\n", res.Successes)
	}
	fmt.Printf("Dice rolled: %d\n", res.RollCount)
}
