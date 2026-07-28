package composition

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

type goldenPlan struct {
	Waves [][]string            `json:"waves"`
	Deps  map[string][]string   `json:"deps"`
}

func TestDAGGolden(t *testing.T) {
	fixtures, err := filepath.Glob("testdata/dags/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no YAML fixtures found in testdata/dags/")
	}

	for _, fixture := range fixtures {
		name := strings.TrimSuffix(filepath.Base(fixture), ".yaml")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}

			cfg, err := ParseConfig(data)
			if err != nil {
				t.Fatalf("ParseConfig: %v", err)
			}

			compiled, err := CompileConfig(context.Background(), cfg, CompileOptions{SkipAuthInit: true})
			if err != nil {
				t.Fatalf("CompileConfig: %v", err)
			}

			comp := compiled.Compositions["test"]
			if comp == nil {
				t.Fatal("composition 'test' not found")
			}

			plan := comp.ExecutionPlan

			// Sort waves for determinism
			gp := goldenPlan{
				Waves: make([][]string, len(plan.Waves)),
				Deps:  make(map[string][]string),
			}
			for i, wave := range plan.Waves {
				w := make([]string, len(wave))
				copy(w, wave)
				sort.Strings(w)
				gp.Waves[i] = w
			}
			for step, deps := range plan.Deps {
				sorted := make([]string, len(deps))
				copy(sorted, deps)
				sort.Strings(sorted)
				gp.Deps[step] = sorted
			}

			got, err := json.MarshalIndent(gp, "", "  ")
			if err != nil {
				t.Fatal(err)
			}

			goldenPath := strings.TrimSuffix(fixture, ".yaml") + ".golden.json"

			if *update {
				if err := os.WriteFile(goldenPath, got, 0644); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s", goldenPath)
				return
			}

			expected, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden file (run with -update to create): %v", err)
			}

			if string(got) != string(expected) {
				t.Errorf("golden mismatch for %s\ngot:\n%s\nwant:\n%s", name, string(got), string(expected))
			}
		})
	}
}
