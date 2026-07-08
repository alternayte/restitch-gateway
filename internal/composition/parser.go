package composition

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/restitch/restitch-gateway/internal/auth"
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

// CompiledConfig holds the parsed configuration plus all compiled expressions.
// All expressions are compiled at parse time to fail fast on syntax errors.
type CompiledConfig struct {
	Config       *Config
	Compositions map[string]*CompiledComposition
	Upstreams    map[string]*CompiledUpstream // Compiled upstreams with auth strategies
}

// DefaultStepTimeout is the default timeout for step execution when not configured.
const DefaultStepTimeout = 30 * time.Second

// CompiledUpstream holds an upstream definition plus its compiled auth strategy.
type CompiledUpstream struct {
	Upstream *Upstream
	Auth     auth.Strategy // nil for no auth
	Timeout  time.Duration // Upstream default timeout (0 means use 30s default)
}

// CompiledComposition holds compiled expressions for a single composition.
type CompiledComposition struct {
	Steps         map[string]*CompiledStep
	Response      *CompiledResponse
	ExecutionPlan *ExecutionPlan // Pre-built DAG execution plan (validated at parse time)
}

// CompiledStep holds a step definition plus its compiled expressions.
type CompiledStep struct {
	Step        *Step
	PathExpr    *CompiledExpr            // Compiled path expression
	BodyExpr    *CompiledExpr            // Compiled body expression (may be nil)
	HeaderExprs map[string]*CompiledExpr // Compiled header expressions
	Optional    bool                     // Whether step failure allows composition to continue
	Timeout     time.Duration            // Resolved timeout value (step > upstream > 30s default)
	ErrorRules  []ErrorRule              // Error matching rules
}

// CompiledResponse holds compiled expressions for response template.
type CompiledResponse struct {
	StatusExpr  *CompiledExpr            // nil if status is static int
	BodyExprs   map[string]*CompiledExpr // Compiled body template expressions
	BodyTemplate interface{}             // Original body template structure for evaluation
	ContentType string                   // Content-Type header value
}

// CompileConfig takes a parsed config and compiles all expressions and auth strategies.
// This MUST be called before using the configuration to serve requests.
//
// All expressions are compiled at parse time (not request time) to fail fast
// on syntax errors per CONTEXT.md decisions.
//
// Auth strategies are built at compile time to:
//   - Fail fast on missing/invalid environment variables (credentials)
//   - Fail fast on invalid OAuth2 credentials (initial token fetch)
//   - Validate mutual exclusivity of auth strategies per upstream
//
// Returns an error if any expression has invalid syntax or auth config is invalid.
func CompileConfig(ctx context.Context, cfg *Config) (*CompiledConfig, error) {
	compiled := &CompiledConfig{
		Config:       cfg,
		Compositions: make(map[string]*CompiledComposition),
		Upstreams:    make(map[string]*CompiledUpstream),
	}

	// Build auth strategies for each upstream (fail-fast on invalid config)
	for name, upstream := range cfg.Upstreams {
		var strategy auth.Strategy = nil
		if upstream.Auth != nil {
			// Validate mutual exclusivity
			if err := upstream.Auth.Validate(); err != nil {
				return nil, fmt.Errorf("upstream %s: %w", name, err)
			}
			// Build strategy (expands env vars, validates credentials)
			var err error
			strategy, err = upstream.Auth.Build(ctx)
			if err != nil {
				return nil, fmt.Errorf("upstream %s auth: %w", name, err)
			}
		}
		// Keep reference to original upstream
		upstreamCopy := upstream
		compiled.Upstreams[name] = &CompiledUpstream{
			Upstream: &upstreamCopy,
			Auth:     strategy,
			Timeout:  upstream.Timeout,
		}
	}

	for compName, comp := range cfg.Compositions {
		compiledComp, err := compileComposition(&comp)
		if err != nil {
			return nil, fmt.Errorf("composition %s: %w", compName, err)
		}
		compiled.Compositions[compName] = compiledComp
	}

	return compiled, nil
}

// compileComposition compiles all expressions in a composition.
func compileComposition(comp *Composition) (*CompiledComposition, error) {
	// Build list of step names for environment
	stepNames := make([]string, len(comp.Steps))
	for i, step := range comp.Steps {
		stepNames[i] = step.Name
	}

	// Build environment for expression compilation
	env := BuildBaseEnvironment(stepNames)

	compiledComp := &CompiledComposition{
		Steps: make(map[string]*CompiledStep),
	}

	// Compile each step's expressions
	for i := range comp.Steps {
		step := &comp.Steps[i]
		compiledStep, err := compileStep(step, env)
		if err != nil {
			return nil, fmt.Errorf("step %s: %w", step.Name, err)
		}
		compiledComp.Steps[step.Name] = compiledStep
	}

	// Compile response expressions
	compiledResp, err := compileResponse(&comp.Response, env)
	if err != nil {
		return nil, fmt.Errorf("response: %w", err)
	}
	compiledComp.Response = compiledResp

	// Build and validate DAG execution plan at parse time
	executionPlan, err := BuildDAG(compiledComp)
	if err != nil {
		return nil, fmt.Errorf("invalid composition structure: %w", err)
	}
	compiledComp.ExecutionPlan = executionPlan

	return compiledComp, nil
}

// compileStep compiles all expressions in a step.
func compileStep(step *Step, env map[string]interface{}) (*CompiledStep, error) {
	compiled := &CompiledStep{
		Step:        step,
		HeaderExprs: make(map[string]*CompiledExpr),
		Optional:    step.Optional,
		ErrorRules:  step.ErrorRules,
		// Timeout is resolved at execution time when we have upstream available
		Timeout:     0,
	}

	// Compile path expression
	pathExpr, err := compileTemplateString(step.Path, env)
	if err != nil {
		return nil, fmt.Errorf("path: %w", err)
	}
	compiled.PathExpr = pathExpr

	// Compile body expression if present
	if step.Body != "" {
		bodyExpr, err := compileTemplateString(step.Body, env)
		if err != nil {
			return nil, fmt.Errorf("body: %w", err)
		}
		compiled.BodyExpr = bodyExpr
	}

	// Compile header expressions
	for key, value := range step.Headers {
		if IsExpression(value) {
			headerExpr, err := compileTemplateString(value, env)
			if err != nil {
				return nil, fmt.Errorf("header %s: %w", key, err)
			}
			compiled.HeaderExprs[key] = headerExpr
		}
	}

	return compiled, nil
}

// compileResponse compiles all expressions in a response template.
func compileResponse(resp *ResponseTemplate, env map[string]interface{}) (*CompiledResponse, error) {
	compiled := &CompiledResponse{
		BodyExprs:    make(map[string]*CompiledExpr),
		BodyTemplate: resp.Body,
		ContentType:  resp.ContentType,
	}

	// Compile status if it's an expression string
	if statusStr, ok := resp.Status.(string); ok {
		if IsExpression(statusStr) {
			statusExpr, err := compileTemplateString(statusStr, env)
			if err != nil {
				return nil, fmt.Errorf("status: %w", err)
			}
			compiled.StatusExpr = statusExpr
		}
	}

	// Compile body expressions recursively
	if err := compileBodyExpressions(resp.Body, env, "", compiled.BodyExprs); err != nil {
		return nil, fmt.Errorf("body: %w", err)
	}

	return compiled, nil
}

// compileBodyExpressions recursively compiles expressions in response body template.
func compileBodyExpressions(body interface{}, env map[string]interface{}, path string, result map[string]*CompiledExpr) error {
	switch v := body.(type) {
	case string:
		if IsExpression(v) {
			compiled, err := compileTemplateString(v, env)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			result[path] = compiled
		}
	case map[string]interface{}:
		for key, value := range v {
			keyPath := path
			if keyPath != "" {
				keyPath += "."
			}
			keyPath += key
			if err := compileBodyExpressions(value, env, keyPath, result); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, item := range v {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			if err := compileBodyExpressions(item, env, itemPath, result); err != nil {
				return err
			}
		}
	}

	return nil
}

// compileTemplateString compiles a template string that may contain {{ expr }} patterns.
// For simple expressions like "{{ req.query.id }}", it compiles the expression directly.
// For template strings like "/users/{{ req.query.id }}", it extracts and compiles
// embedded expressions.
func compileTemplateString(template string, env map[string]interface{}) (*CompiledExpr, error) {
	// Check if the entire string is a single expression
	template = strings.TrimSpace(template)
	if len(template) > 4 && template[:2] == "{{" && template[len(template)-2:] == "}}" {
		// Single expression - compile directly
		exprContent := strings.TrimSpace(template[2 : len(template)-2])
		return CompileExpression(exprContent, env)
	}

	// Template string with embedded expressions - compile each expression
	exprs := ExtractExpressions(template)
	if len(exprs) == 0 {
		// No expressions - treat as literal string
		return &CompiledExpr{Raw: template}, nil
	}

	// For now, compile the first expression for validation
	// TODO: Full template interpolation support in later phases
	for _, expr := range exprs {
		if _, err := CompileExpression(expr, env); err != nil {
			return nil, err
		}
	}

	// Store the template for later interpolation
	return &CompiledExpr{Raw: template}, nil
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
