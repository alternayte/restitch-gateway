package gwconfig

import (
	"fmt"
	"os"
	"strings"
)

// ExpandEnvStrict expands ${VAR} and ${VAR:default}. "$$" → literal "$".
// A "$" not part of "$$" or "${...}" is an error. ${VAR} with VAR unset
// and no default is an error listing the variable name.
func ExpandEnvStrict(s string) (string, error) {
	var b strings.Builder
	i := 0

	for i < len(s) {
		if s[i] != '$' {
			b.WriteByte(s[i])
			i++
			continue
		}

		// Found '$'
		if i+1 >= len(s) {
			return "", fmt.Errorf("invalid '$' at end of string: use $$ for a literal dollar or ${VAR} syntax")
		}

		next := s[i+1]

		if next == '$' {
			b.WriteByte('$')
			i += 2
			continue
		}

		if next != '{' {
			return "", fmt.Errorf("invalid '$' at offset %d: use $$ for a literal dollar or ${VAR} syntax", i)
		}

		// ${...}
		end := strings.Index(s[i+2:], "}")
		if end < 0 {
			return "", fmt.Errorf("unclosed ${...} at offset %d", i)
		}

		content := s[i+2 : i+2+end]
		i = i + 2 + end + 1

		varName := content
		defaultVal := ""
		hasDefault := false

		if colonIdx := strings.Index(content, ":"); colonIdx >= 0 {
			varName = content[:colonIdx]
			defaultVal = content[colonIdx+1:]
			hasDefault = true
		}

		val, exists := os.LookupEnv(varName)
		if !exists || val == "" {
			if hasDefault {
				b.WriteString(defaultVal)
			} else {
				return "", fmt.Errorf("environment variable %s is not set", varName)
			}
		} else {
			b.WriteString(val)
		}
	}

	return b.String(), nil
}
