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
	"regexp"
	"strings"
)

// IsExpression checks if a string contains expression syntax using {{ }} delimiters.
// This follows the template style per CONTEXT.md decisions.
//
// Examples:
//   - "{{ req.query.user_id }}" -> true
//   - "/users/{{ req.query.id }}" -> true
//   - "/static/path" -> false
func IsExpression(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}

// exprPattern matches {{ expr }} patterns for extraction
var exprPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// ExtractExpressions finds all {{ expr }} patterns in a string and returns
// the expression content (without the delimiters).
//
// Example:
//   - "/users/{{ req.query.id }}/orders/{{ req.query.order }}"
//     returns: ["req.query.id", "req.query.order"]
func ExtractExpressions(s string) []string {
	matches := exprPattern.FindAllStringSubmatch(s, -1)
	if matches == nil {
		return nil
	}

	var exprs []string
	for _, match := range matches {
		if len(match) > 1 {
			// Trim whitespace from expression content
			exprs = append(exprs, strings.TrimSpace(match[1]))
		}
	}
	return exprs
}
