package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/restitch/restitch-gateway/internal/composition"
)

func checkCmd(args []string) int {
	flags := flag.NewFlagSet("check", flag.ExitOnError)
	configFile := flags.String("config", "restitch.yaml", "path to config file")
	quiet := flags.Bool("q", false, "quiet mode (errors only)")
	flags.Parse(args)

	cfg, err := composition.LoadConfigFile(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	compiledCfg, err := composition.CompileConfig(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if !*quiet {
		for name, comp := range compiledCfg.Compositions {
			plan := comp.ExecutionPlan
			fmt.Printf("  %s: %d steps, %d waves", name, countAllSteps(plan.Waves), len(plan.Waves))
			for i, wave := range plan.Waves {
				if i == 0 {
					fmt.Print(" [")
				} else {
					fmt.Print(" → [")
				}
				for j, s := range wave {
					if j > 0 {
						fmt.Print(" ")
					}
					fmt.Print(s)
				}
				fmt.Print("]")
			}
			fmt.Println()
		}
	}

	fmt.Println("Syntax OK")
	return 0
}

func countAllSteps(waves [][]string) int {
	n := 0
	for _, w := range waves {
		n += len(w)
	}
	return n
}
