// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package composition

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Template is a parsed "{{ expr }}"-bearing string, compiled once.
type Template struct {
	Raw      string
	Segments []Segment
	Deps     []string
	IsSingle bool
}

// Segment is either a literal string or a compiled expression.
type Segment struct {
	Literal string
	Program *vm.Program
	Expr    string
}

// EscapeMode controls how evaluated expression values are escaped.
type EscapeMode int

const (
	EscapeNone  EscapeMode = iota // headers, response strings
	EscapePath                    // url.PathEscape per interpolated value
	EscapeQuery                   // url.QueryEscape per interpolated value
	EscapeJSON                    // JSON-encode the value (bodies)
)

// stepRefPattern matches steps.<name> references in expressions.
var stepRefPattern = regexp.MustCompile(`\bsteps\.([A-Za-z_][A-Za-z0-9_]*)`)

// CompileTemplate parses raw, compiles every {{...}} against env.
func CompileTemplate(raw string, env map[string]any) (*Template, error) {
	segments, err := parseSegments(raw, env)
	if err != nil {
		return nil, err
	}

	var deps []string
	seen := map[string]bool{}
	for _, seg := range segments {
		if seg.Program != nil {
			for _, d := range extractStepDeps(seg.Expr) {
				if !seen[d] {
					deps = append(deps, d)
					seen[d] = true
				}
			}
		}
	}

	trimmed := strings.TrimSpace(raw)
	isSingle := false
	if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") {
		exprCount := 0
		hasNonEmptyLiteral := false
		for _, seg := range segments {
			if seg.Program != nil {
				exprCount++
			} else if seg.Literal != "" {
				hasNonEmptyLiteral = true
			}
		}
		isSingle = exprCount == 1 && !hasNonEmptyLiteral
	}

	return &Template{
		Raw:      raw,
		Segments: segments,
		Deps:     deps,
		IsSingle: isSingle,
	}, nil
}

// extractStepDeps returns step names referenced in an expression string.
func extractStepDeps(exprStr string) []string {
	matches := stepRefPattern.FindAllStringSubmatch(exprStr, -1)
	if matches == nil {
		return nil
	}

	seen := map[string]bool{}
	var deps []string
	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			deps = append(deps, m[1])
			seen[m[1]] = true
		}
	}
	return deps
}

// parseSegments splits raw into alternating literal and expression segments.
func parseSegments(raw string, env map[string]any) ([]Segment, error) {
	var segments []Segment
	rest := raw

	for {
		idx := strings.Index(rest, "{{")
		if idx < 0 {
			if rest != "" {
				segments = append(segments, Segment{Literal: rest})
			}
			break
		}

		if idx > 0 {
			segments = append(segments, Segment{Literal: rest[:idx]})
		}

		rest = rest[idx+2:]

		end := strings.Index(rest, "}}")
		if end < 0 {
			return nil, fmt.Errorf("unclosed {{ in template: %s", raw)
		}

		exprText := strings.TrimSpace(rest[:end])
		rest = rest[end+2:]

		program, err := expr.Compile(exprText,
			expr.Env(env),
			expr.AllowUndefinedVariables(),
		)
		if err != nil {
			return nil, fmt.Errorf("expression %q: %w", exprText, err)
		}

		segments = append(segments, Segment{
			Program: program,
			Expr:    exprText,
		})
	}

	return segments, nil
}

// EvalString renders the template as a string, escaping each evaluated
// expression per mode. Literal segments are never escaped.
func (t *Template) EvalString(env map[string]any, mode EscapeMode) (string, error) {
	if len(t.Segments) == 0 {
		return "", nil
	}

	if len(t.Segments) == 1 && t.Segments[0].Program == nil {
		return t.Segments[0].Literal, nil
	}

	var b strings.Builder
	for _, seg := range t.Segments {
		if seg.Program == nil {
			b.WriteString(seg.Literal)
			continue
		}

		val, err := expr.Run(seg.Program, env)
		if err != nil {
			return "", fmt.Errorf("expression %q: %w", seg.Expr, err)
		}

		s := fmt.Sprintf("%v", val)
		switch mode {
		case EscapePath:
			s = url.PathEscape(s)
		case EscapeQuery:
			s = url.QueryEscape(s)
		case EscapeJSON:
			jsonBytes, err := json.Marshal(val)
			if err != nil {
				return "", fmt.Errorf("expression %q: JSON encode: %w", seg.Expr, err)
			}
			s = string(jsonBytes)
		}

		b.WriteString(s)
	}

	return b.String(), nil
}

// EvalValue: for IsSingle templates returns the raw evaluated value
// (preserving type); otherwise same as EvalString(env, EscapeNone).
func (t *Template) EvalValue(env map[string]any) (any, error) {
	if t.IsSingle {
		for _, seg := range t.Segments {
			if seg.Program != nil {
				val, err := expr.Run(seg.Program, env)
				if err != nil {
					return nil, fmt.Errorf("expression %q: %w", seg.Expr, err)
				}
				return val, nil
			}
		}
	}

	s, err := t.EvalString(env, EscapeNone)
	if err != nil {
		return nil, err
	}
	return s, nil
}
