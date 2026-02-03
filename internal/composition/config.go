// Package composition provides YAML configuration parsing and expression compilation
// for the composition engine.
//
// The composition engine allows users to define multi-step API compositions that
// fetch data from multiple upstreams in parallel and merge responses using expressions.
//
// Key concepts:
//   - Upstreams: Named upstream services referenced in steps
//   - Compositions: Multi-step workflows with response templates
//   - Steps: Individual upstream requests that can depend on each other
//   - Expressions: Dynamic values using {{ expr }} template syntax
//
// All expressions are compiled at parse time (not request time) to fail fast on
// syntax errors.
package composition

// Config represents the complete composition configuration loaded from YAML.
// It defines all upstreams and compositions available to the gateway.
type Config struct {
	Upstreams    map[string]Upstream    `yaml:"upstreams"`
	Compositions map[string]Composition `yaml:"compositions"`
}

// Upstream represents a named backend service that steps can call.
type Upstream struct {
	URL string `yaml:"url"`
}

// Composition represents a multi-step API composition with a response template.
// Compositions are matched to incoming requests by path and method.
type Composition struct {
	Path     string           `yaml:"path"`     // Route path pattern
	Method   string           `yaml:"method"`   // HTTP method (defaults to GET)
	Steps    []Step           `yaml:"steps"`    // Execution steps
	Response ResponseTemplate `yaml:"response"` // Response template
}

// Step represents a single upstream request in a composition.
// Steps can depend on other steps either implicitly (via expression usage)
// or explicitly (via DependsOn).
type Step struct {
	Name      string            `yaml:"name"`       // Unique step name within composition
	Upstream  string            `yaml:"upstream"`   // Reference to named upstream
	Path      string            `yaml:"path"`       // Path template with expressions
	Method    string            `yaml:"method"`     // HTTP method (defaults to GET)
	Headers   map[string]string `yaml:"headers"`    // Header templates (defaults to empty)
	Body      string            `yaml:"body"`       // Request body template for POST/PUT
	DependsOn []string          `yaml:"depends_on"` // Explicit dependencies (optional)
}

// ResponseTemplate defines the structure of the composition response.
// The body can contain nested maps with expression templates that reference
// step results.
type ResponseTemplate struct {
	Status      interface{} `yaml:"status"`       // int or expression string
	ContentType string      `yaml:"content_type"` // MIME type (defaults to application/json)
	Body        interface{} `yaml:"body"`         // Nested map with expression templates
}
