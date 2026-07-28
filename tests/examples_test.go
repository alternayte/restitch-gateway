//go:build e2e

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExampleConfigs(t *testing.T) {
	matches, err := filepath.Glob("../examples/**/restitch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// Also check nested dirs
	nested, _ := filepath.Glob("../examples/*/**/restitch.yaml")
	matches = append(matches, nested...)

	if len(matches) == 0 {
		t.Fatal("no example restitch.yaml files found")
	}

	// Set dummy env vars for configs that reference secrets
	envVars := map[string]string{
		"API_KEY":        "test-key",
		"GATEWAY_KEY":    "test-gateway-key",
		"ADMIN_KEY":      "test-admin-key",
		"USERNAME":       "testuser",
		"PASSWORD":       "testpass",
		"OAUTH_TOKEN_URL": "https://example.com/token",
		"OAUTH_CLIENT_ID": "test-client",
		"OAUTH_SECRET":   "test-secret",
	}
	for k, v := range envVars {
		t.Setenv(k, v)
	}

	seen := make(map[string]bool)
	for _, match := range matches {
		abs, _ := filepath.Abs(match)
		if seen[abs] {
			continue
		}
		seen[abs] = true

		rel, _ := filepath.Rel("..", match)
		t.Run(rel, func(t *testing.T) {
			data, err := os.ReadFile(match)
			if err != nil {
				t.Fatal(err)
			}

			// Expand ${VAR} patterns with env values
			content := string(data)
			content = expandSimple(content)

			var out interface{}
			if err := yaml.Unmarshal([]byte(content), &out); err != nil {
				t.Errorf("invalid YAML in %s: %v", match, err)
			}

			m, ok := out.(map[string]interface{})
			if !ok {
				t.Fatalf("expected map, got %T", out)
			}

			if _, hasUpstreams := m["upstreams"]; !hasUpstreams {
				t.Error("config missing 'upstreams' key")
			}
			if _, hasCompositions := m["compositions"]; !hasCompositions {
				t.Error("config missing 'compositions' key")
			}
		})
	}
}

func expandSimple(s string) string {
	// Replace ${VAR} with env values, ${VAR:default} with value or default
	result := s
	for {
		idx := strings.Index(result, "${")
		if idx < 0 {
			break
		}
		end := strings.Index(result[idx:], "}")
		if end < 0 {
			break
		}
		end += idx
		varExpr := result[idx+2 : end]
		varName := varExpr
		defaultVal := ""
		if colonIdx := strings.Index(varExpr, ":"); colonIdx >= 0 {
			varName = varExpr[:colonIdx]
			defaultVal = varExpr[colonIdx+1:]
		}
		val := os.Getenv(varName)
		if val == "" {
			val = defaultVal
		}
		if val == "" {
			val = "placeholder"
		}
		result = result[:idx] + val + result[end+1:]
	}
	// Handle $$ → $
	result = strings.ReplaceAll(result, "$$", "$")
	return result
}
