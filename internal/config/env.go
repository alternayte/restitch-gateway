// Package config provides configuration utilities including environment variable
// expansion with fail-fast validation.
package config

import (
	"fmt"
	"os"
	"regexp"
)

// envVarPattern matches ${VAR_NAME} syntax for environment variable references.
// Supports alphanumeric characters and underscores in variable names.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// ExpandEnvWithValidation expands environment variable references in the given
// string using ${VAR_NAME} syntax. Unlike os.ExpandEnv, this function validates
// that all referenced variables exist and are non-empty before expanding.
//
// This enables fail-fast behavior: configuration errors are caught at startup
// rather than at runtime when a request needs the credential.
//
// Returns an error if any referenced variable is not set or is empty.
func ExpandEnvWithValidation(value string) (string, error) {
	// Find all ${VAR} references
	matches := envVarPattern.FindAllStringSubmatch(value, -1)

	// Validate each variable exists and is non-empty
	for _, match := range matches {
		varName := match[1] // Capture group 1 is the variable name

		val, exists := os.LookupEnv(varName)
		if !exists {
			return "", fmt.Errorf("environment variable %s is not set", varName)
		}
		if val == "" {
			return "", fmt.Errorf("environment variable %s is empty", varName)
		}
	}

	// Safe to expand now that we've validated all variables
	return os.ExpandEnv(value), nil
}
