//go:build e2e

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

package tests

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/alternayte/restitch-gateway/v2/internal/gwconfig"
)

var yamlFencePattern = regexp.MustCompile("(?m)^```yaml\\s*(.*?)\\n([\\s\\S]*?)^```")

func TestReadmeYAMLBlocks(t *testing.T) {
	readme, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}

	matches := yamlFencePattern.FindAllSubmatch(readme, -1)
	if len(matches) == 0 {
		// The README's YAML examples are part of the documented contract; a
		// README without them means the test cannot fail (finding M21).
		t.Fatal("no YAML fenced blocks found in README.md — the README contract is no longer tested")
	}

	for i, m := range matches {
		info := string(m[1])
		block := string(m[2])

		if strings.Contains(info, "fragment") {
			continue
		}

		t.Run(fmt.Sprintf("block_%d", i), func(t *testing.T) {
			var out interface{}
			if err := yaml.Unmarshal([]byte(block), &out); err != nil {
				preview := block
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				t.Errorf("YAML block %d is not valid YAML:\n%s\nerror: %v", i, preview, err)
			}
		})
	}
}

// commandEnvVars reads cmd/restitch and returns, for every RESTITCH_ name,
// the commands that read it. A command reads env vars when its function
// passes a flag set to gwconfig.BindEnv; every flag defined on that flag
// set gets the name gwconfig.EnvName derives.
func commandEnvVars(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob("../cmd/restitch/*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	out := map[string][]string{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			collectFuncEnvVars(t, fset, fn, out)
		}
	}
	return out
}

func collectFuncEnvVars(t *testing.T, fset *token.FileSet, fn *ast.FuncDecl, out map[string][]string) {
	t.Helper()
	bound := map[string]bool{}     // flag set variables passed to BindEnv
	setName := map[string]string{} // flag set variable -> command name
	flags := map[string][]string{} // flag set variable -> flag names

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if len(n.Lhs) != 1 || len(n.Rhs) != 1 {
				return true
			}
			call, ok := n.Rhs[0].(*ast.CallExpr)
			if !ok || selector(call.Fun) != "flag.NewFlagSet" || len(call.Args) == 0 {
				return true
			}
			if id, ok := n.Lhs[0].(*ast.Ident); ok {
				setName[id.Name] = stringLit(t, call.Args[0])
			}
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if selector(n.Fun) == "gwconfig.BindEnv" && len(n.Args) > 0 {
				if id, ok := n.Args[0].(*ast.Ident); ok {
					bound[id.Name] = true
				}
				return true
			}
			recv, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			method := sel.Sel.Name
			var nameArg ast.Expr
			switch {
			case strings.HasSuffix(method, "Var") && len(n.Args) >= 2:
				nameArg = n.Args[1]
			case isFlagConstructor(method) && len(n.Args) >= 1:
				nameArg = n.Args[0]
			default:
				return true
			}
			flags[recv.Name] = append(flags[recv.Name], stringLit(t, nameArg))
		}
		return true
	})

	for v := range bound {
		cmd, ok := setName[v]
		if !ok {
			t.Fatalf("%s: BindEnv gets %q, which is not a flag.NewFlagSet in the same function", fset.Position(fn.Pos()), v)
		}
		if len(flags[v]) == 0 {
			t.Fatalf("%s: command %q binds env vars but defines no flags in the same function", fset.Position(fn.Pos()), cmd)
		}
		for _, name := range flags[v] {
			env := gwconfig.EnvName(name)
			out[env] = append(out[env], cmd)
		}
	}
}

func isFlagConstructor(method string) bool {
	switch method {
	case "String", "Int", "Int64", "Uint", "Uint64", "Bool", "Duration", "Float64", "Func", "BoolFunc", "TextVar":
		return true
	}
	return false
}

func selector(e ast.Expr) string {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name + "." + sel.Sel.Name
}

func stringLit(t *testing.T, e ast.Expr) string {
	t.Helper()
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		t.Fatalf("flag name is not a string literal: %#v", e)
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// envRowPattern matches one entry of the env var table in
// docs/configuration.md: | `RESTITCH_X` | commands | ...
var envRowPattern = regexp.MustCompile("(?m)^\\| `(RESTITCH_[A-Z0-9_]+)` \\| ([^|]+)\\|")

var restitchNamePattern = regexp.MustCompile(`RESTITCH_[A-Z0-9_]+`)

func TestDocsListEveryEnvVar(t *testing.T) {
	code := commandEnvVars(t)
	if len(code) == 0 {
		t.Fatal("found no RESTITCH_ env vars in cmd/restitch")
	}

	doc, err := os.ReadFile("../docs/configuration.md")
	if err != nil {
		t.Fatalf("read docs/configuration.md: %v", err)
	}

	rows := map[string]int{}
	for _, m := range envRowPattern.FindAllStringSubmatch(string(doc), -1) {
		name := m[1]
		rows[name]++
		cmds, ok := code[name]
		if !ok {
			continue
		}
		sort.Strings(cmds)
		var listed []string
		for _, c := range strings.Split(m[2], ",") {
			listed = append(listed, strings.Trim(strings.TrimSpace(c), "`"))
		}
		sort.Strings(listed)
		if strings.Join(listed, ",") != strings.Join(cmds, ",") {
			t.Errorf("%s: docs list commands %v, the code binds it in %v", name, listed, cmds)
		}
	}

	for name := range code {
		switch rows[name] {
		case 0:
			t.Errorf("%s is read by the code but has no row in docs/configuration.md", name)
		case 1:
		default:
			t.Errorf("%s has %d rows in docs/configuration.md; want one", name, rows[name])
		}
	}

	// Any RESTITCH_ name anywhere in the file must be one the code reads.
	seen := map[string]bool{}
	for _, name := range restitchNamePattern.FindAllString(string(doc), -1) {
		if seen[name] {
			continue
		}
		seen[name] = true
		if _, ok := code[name]; !ok {
			t.Errorf("docs/configuration.md names %s, which no command reads", name)
		}
	}
}
