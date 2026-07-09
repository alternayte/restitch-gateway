package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

func importCmd(args []string) int {
	if len(args) < 2 || args[0] != "openapi" {
		fmt.Fprintf(os.Stderr, "usage: restitch import openapi <spec.yaml|spec.json> [flags]\n")
		return 2
	}

	specFile := args[1]
	flags := flag.NewFlagSet("import openapi", flag.ExitOnError)
	upstreamName := flags.String("upstream", "api", "upstream name")
	baseURL := flags.String("base-url", "", "base URL (default: from spec servers[0])")
	ops := flags.String("ops", "", "comma-separated operationIds to include (default: all)")
	outFile := flags.String("o", "", "output file (default: stdout)")
	flags.Parse(args[2:])

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading spec: %v\n", err)
		return 1
	}

	base := *baseURL
	if base == "" && len(doc.Servers) > 0 {
		base = doc.Servers[0].URL
	}
	if base == "" {
		base = "http://localhost:8080"
	}

	opsFilter := map[string]bool{}
	if *ops != "" {
		for _, op := range strings.Split(*ops, ",") {
			opsFilter[strings.TrimSpace(op)] = true
		}
	}

	type stepDef struct {
		Name     string `yaml:"name"`
		Upstream string `yaml:"upstream"`
		Path     string `yaml:"path"`
		Method   string `yaml:"method"`
	}
	type responseDef struct {
		Status int            `yaml:"status"`
		Body   map[string]any `yaml:"body"`
	}
	type compDef struct {
		Path     string      `yaml:"path"`
		Method   string      `yaml:"method,omitempty"`
		Steps    []stepDef   `yaml:"steps"`
		Response responseDef `yaml:"response"`
	}

	compositions := map[string]compDef{}

	paramPattern := regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

	for path, pathItem := range doc.Paths.Map() {
		for method, op := range pathItem.Operations() {
			if op.OperationID == "" {
				continue
			}
			if len(opsFilter) > 0 && !opsFilter[op.OperationID] {
				continue
			}

			name := toKebab(op.OperationID)

			stepPath := paramPattern.ReplaceAllStringFunc(path, func(m string) string {
				param := m[1 : len(m)-1]
				return "{{ req.params." + param + " }}"
			})

			var queryParts []string
			if op.Parameters != nil {
				for _, p := range op.Parameters {
					if p.Value != nil && p.Value.In == "query" && p.Value.Required {
						queryParts = append(queryParts, p.Value.Name+"={{ req.query."+p.Value.Name+" }}")
					}
				}
			}
			if len(queryParts) > 0 {
				stepPath += "?" + strings.Join(queryParts, "&")
			}

			compositions[name] = compDef{
				Path:   path,
				Method: strings.ToUpper(method),
				Steps: []stepDef{{
					Name:     "main",
					Upstream: *upstreamName,
					Path:     stepPath,
					Method:   strings.ToUpper(method),
				}},
				Response: responseDef{
					Status: 200,
					Body:   map[string]any{"result": "{{ steps.main.body }}"},
				},
			}
		}
	}

	output := map[string]any{
		"upstreams": map[string]any{
			*upstreamName: map[string]any{"url": base},
		},
		"compositions": compositions,
	}

	yamlBytes, err := yaml.Marshal(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error generating YAML: %v\n", err)
		return 1
	}

	if *outFile != "" {
		if err := os.WriteFile(*outFile, yamlBytes, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing file: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "scaffold written to %s — review auth, timeouts, and response shaping\n", *outFile)
	} else {
		fmt.Print(string(yamlBytes))
		fmt.Fprintln(os.Stderr, "scaffold only — review auth, timeouts, and response shaping")
	}

	return 0
}

func toKebab(s string) string {
	var result []rune
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result = append(result, '-')
			}
			result = append(result, r+32)
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}
