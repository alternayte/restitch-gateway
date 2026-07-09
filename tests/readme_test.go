//go:build e2e

package tests

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var yamlFencePattern = regexp.MustCompile("(?m)^```yaml\\s*(.*?)\\n([\\s\\S]*?)^```")

func TestReadmeYAMLBlocks(t *testing.T) {
	readme, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}

	matches := yamlFencePattern.FindAllSubmatch(readme, -1)
	if len(matches) == 0 {
		t.Skip("no YAML fenced blocks found in README.md")
	}

	for i, m := range matches {
		info := string(m[1])
		block := string(m[2])

		if strings.Contains(info, "fragment") {
			continue
		}

		t.Run(fmt.Sprintf("block_%d", i), func(t *testing.T) {
			var out interface{}
			if err := yaml.Unmarshal([]byte(block), &out); err != nil {
				preview := block
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				t.Errorf("YAML block %d is not valid YAML:\n%s\nerror: %v", i, preview, err)
			}
		})
	}
}
