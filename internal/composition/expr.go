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

