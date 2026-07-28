package registry

import (
	"context"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/restitch/restitch-gateway/internal/composition"
	"github.com/restitch/restitch-gateway/internal/gwconfig"
)

// lineRe and columnRe extract "line N" / "column N" substrings from yaml.v3
// error messages (format: "yaml: line N: <reason>" or
// "yaml: line N: column N: <reason>").
var (
	lineRe   = regexp.MustCompile(`line (\d+)`)
	columnRe = regexp.MustCompile(`column (\d+)`)
)

// fieldPathRe matches "composition <name>[:,] step <name>" fragments in
// composition package error messages (e.g. "composition test: step s1: ..."
// or "composition test, step s1: ...") for extraction into a dotted field
// path like "compositions.test.steps.s1".
var fieldPathRe = regexp.MustCompile(`composition[s]?\s+([^\s:,]+)[:,]\s*step\s+([^\s:,]+)`)

// Validate runs a three-stage validation pipeline against raw YAML config
// content: (1) YAML syntax, (2) env-var expansion and structural parsing via
// composition.ParseConfig, (3) DAG construction and expression compilation
// via composition.CompileConfig. It never panics; all errors from any stage
// are captured as ValidationErrors. Empty input is a valid (empty) config.
func Validate(yamlContent []byte) *ValidationResult {
	result := &ValidationResult{Valid: true, Errors: []ValidationError{}}

	// Stage 1: YAML syntax.
	var node yaml.Node
	if err := yaml.Unmarshal(yamlContent, &node); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, newSyntaxError(err))
		return result
	}

	// Stage 2: env-var expansion, then structural parsing.
	expanded, err := gwconfig.ExpandEnvStrict(string(yamlContent))
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Message: err.Error(),
		})
		return result
	}

	cfg, err := composition.ParseConfig([]byte(expanded))
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Message: err.Error(),
			Field:   extractFieldPath(err.Error()),
		})
		return result
	}

	// Stage 3: DAG construction and expression compilation.
	if _, err := composition.CompileConfig(context.Background(), cfg, composition.CompileOptions{SkipAuthInit: true}); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Message: err.Error(),
			Field:   extractFieldPath(err.Error()),
		})
		return result
	}

	return result
}

// newSyntaxError builds a ValidationError from a yaml.v3 syntax error,
// extracting line/column position when present in the error message.
func newSyntaxError(err error) ValidationError {
	ve := ValidationError{Message: err.Error()}

	if m := lineRe.FindStringSubmatch(err.Error()); len(m) == 2 {
		if line, parseErr := strconv.Atoi(m[1]); parseErr == nil {
			ve.Line = line
		}
	}
	if m := columnRe.FindStringSubmatch(err.Error()); len(m) == 2 {
		if column, parseErr := strconv.Atoi(m[1]); parseErr == nil {
			ve.Column = column
		}
	}

	return ve
}

// extractFieldPath parses composition-package error messages for
// "composition X[:,] step Y" fragments and converts them into a dotted
// field path "compositions.X.steps.Y". Returns "" if no such fragment is
// found.
func extractFieldPath(msg string) string {
	m := fieldPathRe.FindStringSubmatch(msg)
	if len(m) != 3 {
		return ""
	}
	return "compositions." + m[1] + ".steps." + m[2]
}
