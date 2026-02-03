package composition

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseConfig parses and validates composition configuration from YAML bytes.
// It applies default values and validates that all referenced upstreams exist.
//
// Returns an error if:
//   - YAML syntax is invalid
//   - Step names are not unique within a composition
//   - Steps reference non-existent upstreams
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	// Validate and apply defaults
	if err := validateAndApplyDefaults(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadConfigFile reads a YAML configuration file and parses it.
// This is the primary entry point for loading composition configurations.
func LoadConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg, err := ParseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	return cfg, nil
}

// validateAndApplyDefaults validates the configuration and applies default values.
func validateAndApplyDefaults(cfg *Config) error {
	// Validate compositions
	for compName, comp := range cfg.Compositions {
		// Apply composition defaults
		if comp.Method == "" {
			comp.Method = "GET"
		}

		// Validate response template defaults
		if comp.Response.ContentType == "" {
			comp.Response.ContentType = "application/json"
		}

		// Track step names for uniqueness validation
		stepNames := make(map[string]bool)

		// Validate and apply defaults for each step
		for i := range comp.Steps {
			step := &comp.Steps[i]

			// Check step name uniqueness
			if step.Name == "" {
				return fmt.Errorf("composition %s: step %d has no name", compName, i)
			}
			if stepNames[step.Name] {
				return fmt.Errorf("composition %s: duplicate step name %q", compName, step.Name)
			}
			stepNames[step.Name] = true

			// Validate upstream reference
			if step.Upstream == "" {
				return fmt.Errorf("composition %s, step %s: upstream is required", compName, step.Name)
			}
			if _, exists := cfg.Upstreams[step.Upstream]; !exists {
				return fmt.Errorf("composition %s, step %s: upstream %q not found", compName, step.Name, step.Upstream)
			}

			// Apply step defaults
			if step.Method == "" {
				step.Method = "GET"
			}
			if step.Headers == nil {
				step.Headers = make(map[string]string)
			}
		}

		// Update composition with defaults applied
		cfg.Compositions[compName] = comp
	}

	return nil
}
