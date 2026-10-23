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

package gwconfig

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// EnvName returns the environment variable that sets a flag: RESTITCH_
// plus the flag name in upper snake case. Every process setting follows
// this one rule, so the docs can list the variables from the flag sets.
func EnvName(flagName string) string {
	return "RESTITCH_" + strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))
}

// BindEnv sets each flag that the command line did not give from its
// EnvName variable. Precedence is flag, then env, then default. An env
// value the flag cannot parse is an error that names the variable.
func BindEnv(fs *flag.FlagSet, lookup func(string) (string, bool)) error {
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })

	var errs []error
	fs.VisitAll(func(f *flag.Flag) {
		if given[f.Name] {
			return
		}
		name := EnvName(f.Name)
		v, ok := lookup(name)
		if !ok || v == "" {
			return
		}
		if err := fs.Set(f.Name, v); err != nil {
			errs = append(errs, fmt.Errorf("%s=%q: %w", name, v, err))
		}
	})
	return errors.Join(errs...)
}

// ExpandEnvStrict expands ${VAR} and ${VAR:default}. "$$" → literal "$".
// A "$" not part of "$$" or "${...}" is an error. ${VAR} with VAR unset
// and no default is an error listing the variable name.
func ExpandEnvStrict(s string) (string, error) {
	return ExpandWith(s, os.LookupEnv)
}

// ExpandWith is ExpandEnvStrict with the variable lookup supplied by the
// caller. Studio validates an artifact without the gateway's environment
// by passing a lookup that answers a placeholder for every name.
func ExpandWith(s string, lookup func(string) (string, bool)) (string, error) {
	return scan(s, func(r Reference) (string, error) {
		val, exists := lookup(r.Name)
		if exists && val != "" {
			return val, nil
		}
		if r.HasDefault {
			return r.Default, nil
		}
		return "", fmt.Errorf("environment variable %s is not set", r.Name)
	})
}

// Reference is one ${NAME} or ${NAME:default} in a string.
type Reference struct {
	Name       string
	Default    string
	HasDefault bool
}

// VarName is the syntax of a name inside ${...}: a shell variable name.
var VarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// References returns every ${...} in s in order, with the same syntax
// rules as ExpandWith. It does not check names against VarName.
func References(s string) ([]Reference, error) {
	var refs []Reference
	_, err := scan(s, func(r Reference) (string, error) {
		refs = append(refs, r)
		return "", nil
	})
	return refs, err
}

// scan copies s, replacing each ${...} with what replace returns. "$$" is
// a literal "$"; any other "$" is an error.
func scan(s string, replace func(Reference) (string, error)) (string, error) {
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

		r := Reference{Name: content}
		if colonIdx := strings.Index(content, ":"); colonIdx >= 0 {
			r = Reference{Name: content[:colonIdx], Default: content[colonIdx+1:], HasDefault: true}
		}

		val, err := replace(r)
		if err != nil {
			return "", err
		}
		b.WriteString(val)
	}

	return b.String(), nil
}
