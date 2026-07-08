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
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	if err := validateAndApplyDefaults(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadConfigFile reads a YAML configuration file and parses it.
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
type CompiledConfig struct {
	Config       *Config
	Compositions map[string]*CompiledComposition
	Upstreams    map[string]*CompiledUpstream
}

// DefaultStepTimeout is the default timeout for step execution when not configured.
const DefaultStepTimeout = 30 * time.Second

// CompiledUpstream holds an upstream definition plus its compiled auth strategy.
type CompiledUpstream struct {
	Upstream         *Upstream
	Auth             auth.Strategy
	Timeout          time.Duration
	MaxResponseBytes int64
}

// CompiledComposition holds compiled expressions for a single composition.
type CompiledComposition struct {
	Steps         map[string]*CompiledStep
	Response      *CompiledResponse
	ExecutionPlan *ExecutionPlan
}

// CompiledStep holds a step definition plus its compiled templates.
type CompiledStep struct {
	Step       *Step
	PathPart   *Template
	QueryPart  *Template
	BodyTmpl   *Template
	Headers    map[string]*Template
	Deps       []string
	Optional   bool
	ErrorRules []ErrorRule
}

// CompiledResponse holds compiled templates for response.
type CompiledResponse struct {
	StatusTmpl  *Template
	Body        *CompiledBodyNode
	ContentType string
}

// CompiledBodyNode represents a compiled node in the response body tree.
type CompiledBodyNode struct {
	Tmpl    *Template
	Literal any
	Map     map[string]*CompiledBodyNode
	List    []*CompiledBodyNode
}

// CompileConfig takes a parsed config and compiles all expressions and auth strategies.
func CompileConfig(ctx context.Context, cfg *Config) (*CompiledConfig, error) {
	compiled := &CompiledConfig{
		Config:       cfg,
		Compositions: make(map[string]*CompiledComposition),
		Upstreams:    make(map[string]*CompiledUpstream),
	}

	for name, upstream := range cfg.Upstreams {
		var strategy auth.Strategy
		if upstream.Auth != nil {
			if err := upstream.Auth.Validate(); err != nil {
				return nil, fmt.Errorf("upstream %s: %w", name, err)
			}
			var err error
			strategy, err = upstream.Auth.Build(ctx)
			if err != nil {
				return nil, fmt.Errorf("upstream %s auth: %w", name, err)
			}
		}
		upstreamCopy := upstream
		compiled.Upstreams[name] = &CompiledUpstream{
			Upstream:         &upstreamCopy,
			Auth:             strategy,
			Timeout:          upstream.Timeout,
			MaxResponseBytes: upstream.MaxResponseBytes,
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

func compileComposition(comp *Composition) (*CompiledComposition, error) {
	stepNames := make([]string, len(comp.Steps))
	for i, step := range comp.Steps {
		stepNames[i] = step.Name
	}

	env := BuildBaseEnvironment(stepNames)

	compiledComp := &CompiledComposition{
		Steps: make(map[string]*CompiledStep),
	}

	for i := range comp.Steps {
		step := &comp.Steps[i]
		compiledStep, err := compileStep(step, env)
		if err != nil {
			return nil, fmt.Errorf("step %s: %w", step.Name, err)
		}
		compiledComp.Steps[step.Name] = compiledStep
	}

	compiledResp, err := compileResponse(&comp.Response, env)
	if err != nil {
		return nil, fmt.Errorf("response: %w", err)
	}
	compiledComp.Response = compiledResp

	executionPlan, err := BuildDAG(compiledComp)
	if err != nil {
		return nil, fmt.Errorf("invalid composition structure: %w", err)
	}
	compiledComp.ExecutionPlan = executionPlan

	return compiledComp, nil
}

func compileStep(step *Step, env map[string]any) (*CompiledStep, error) {
	compiled := &CompiledStep{
		Step:       step,
		Headers:    make(map[string]*Template),
		Optional:   step.Optional,
		ErrorRules: step.ErrorRules,
	}

	pathRaw := step.Path
	var queryRaw string
	if idx := strings.Index(pathRaw, "?"); idx >= 0 {
		queryRaw = pathRaw[idx+1:]
		pathRaw = pathRaw[:idx]
	}

	pathTmpl, err := CompileTemplate(pathRaw, env)
	if err != nil {
		return nil, fmt.Errorf("path: %w", err)
	}
	compiled.PathPart = pathTmpl

	if queryRaw != "" {
		queryTmpl, err := CompileTemplate(queryRaw, env)
		if err != nil {
			return nil, fmt.Errorf("query: %w", err)
		}
		compiled.QueryPart = queryTmpl
	}

	if step.Body != "" {
		bodyTmpl, err := CompileTemplate(step.Body, env)
		if err != nil {
			return nil, fmt.Errorf("body: %w", err)
		}
		compiled.BodyTmpl = bodyTmpl
	}

	for key, value := range step.Headers {
		headerTmpl, err := CompileTemplate(value, env)
		if err != nil {
			return nil, fmt.Errorf("header %s: %w", key, err)
		}
		compiled.Headers[key] = headerTmpl
	}

	var allDeps []string
	allDeps = append(allDeps, compiled.PathPart.Deps...)
	if compiled.QueryPart != nil {
		allDeps = append(allDeps, compiled.QueryPart.Deps...)
	}
	if compiled.BodyTmpl != nil {
		allDeps = append(allDeps, compiled.BodyTmpl.Deps...)
	}
	for _, h := range compiled.Headers {
		allDeps = append(allDeps, h.Deps...)
	}
	compiled.Deps = MergeDependencies(step.DependsOn, allDeps)

	return compiled, nil
}

func compileResponse(resp *ResponseTemplate, env map[string]any) (*CompiledResponse, error) {
	compiled := &CompiledResponse{
		ContentType: resp.ContentType,
	}

	if statusStr, ok := resp.Status.(string); ok {
		if IsExpression(statusStr) {
			statusTmpl, err := CompileTemplate(statusStr, env)
			if err != nil {
				return nil, fmt.Errorf("status: %w", err)
			}
			compiled.StatusTmpl = statusTmpl
		}
	}

	bodyNode, err := compileBodyNode(resp.Body, env)
	if err != nil {
		return nil, fmt.Errorf("body: %w", err)
	}
	compiled.Body = bodyNode

	return compiled, nil
}

func compileBodyNode(body any, env map[string]any) (*CompiledBodyNode, error) {
	switch v := body.(type) {
	case string:
		if IsExpression(v) {
			tmpl, err := CompileTemplate(v, env)
			if err != nil {
				return nil, err
			}
			return &CompiledBodyNode{Tmpl: tmpl}, nil
		}
		return &CompiledBodyNode{Literal: v}, nil

	case map[string]any:
		m := make(map[string]*CompiledBodyNode, len(v))
		for key, value := range v {
			node, err := compileBodyNode(value, env)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", key, err)
			}
			m[key] = node
		}
		return &CompiledBodyNode{Map: m}, nil

	case []any:
		list := make([]*CompiledBodyNode, len(v))
		for i, item := range v {
			node, err := compileBodyNode(item, env)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", i, err)
			}
			list[i] = node
		}
		return &CompiledBodyNode{List: list}, nil

	default:
		return &CompiledBodyNode{Literal: v}, nil
	}
}

func validateAndApplyDefaults(cfg *Config) error {
	for name, up := range cfg.Upstreams {
		if up.MaxResponseBytes <= 0 {
			up.MaxResponseBytes = 10 * 1024 * 1024 // 10 MiB
		}
		cfg.Upstreams[name] = up
	}

	for compName, comp := range cfg.Compositions {
		if comp.Method == "" {
			comp.Method = "GET"
		}

		if comp.Response.ContentType == "" {
			comp.Response.ContentType = "application/json"
		}

		stepNames := make(map[string]bool)

		for i := range comp.Steps {
			step := &comp.Steps[i]

			if step.Name == "" {
				return fmt.Errorf("composition %s: step %d has no name", compName, i)
			}
			if stepNames[step.Name] {
				return fmt.Errorf("composition %s: duplicate step name %q", compName, step.Name)
			}
			stepNames[step.Name] = true

			if step.Upstream == "" {
				return fmt.Errorf("composition %s, step %s: upstream is required", compName, step.Name)
			}
			if _, exists := cfg.Upstreams[step.Upstream]; !exists {
				return fmt.Errorf("composition %s, step %s: upstream %q not found", compName, step.Name, step.Upstream)
			}

			if step.Method == "" {
				step.Method = "GET"
			}
			if step.Headers == nil {
				step.Headers = make(map[string]string)
			}
		}

		cfg.Compositions[compName] = comp
	}

	return nil
}
