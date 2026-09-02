// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/alternayte/restitch-gateway/internal/composition"
)

func checkCmd(args []string) int {
	flags := flag.NewFlagSet("check", flag.ExitOnError)
	configFile := flags.String("config", "restitch.yaml", "path to config file")
	quiet := flags.Bool("q", false, "quiet mode (errors only)")
	_ = flags.Parse(args)

	cfg, err := composition.LoadConfigFile(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	compiledCfg, err := composition.CompileConfig(context.Background(), cfg, composition.CompileOptions{SkipAuthInit: true})
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
