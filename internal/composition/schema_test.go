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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

type schemaDoc struct {
	Ref  string                    `json:"$ref"`
	Defs map[string]map[string]any `json:"$defs"`
}

// TestSchemaCoversEveryYAMLField walks the artifact types by reflection,
// so a new config field without schema coverage fails here.
func TestSchemaCoversEveryYAMLField(t *testing.T) {
	raw, err := ArtifactSchema()
	if err != nil {
		t.Fatal(err)
	}
	var doc schemaDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	seen := map[reflect.Type]bool{}
	var walk func(rt reflect.Type)
	walk = func(rt reflect.Type) {
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Map {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || rt == reflect.TypeOf(time.Time{}) || seen[rt] {
			return
		}
		seen[rt] = true
		def, ok := doc.Defs[schemaName(rt)]
		if !ok {
			t.Errorf("no schema definition for %s", schemaName(rt))
			return
		}
		props, _ := def["properties"].(map[string]any)
		if def["additionalProperties"] != false {
			t.Errorf("%s must refuse unknown keys, as the strict decoder does", schemaName(rt))
		}
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			name := strings.Split(f.Tag.Get("yaml"), ",")[0]
			if name == "" || name == "-" || !f.IsExported() {
				continue
			}
			if _, ok := props[name]; !ok {
				t.Errorf("%s.%s (yaml %q) is missing from the schema", rt.Name(), f.Name, name)
			}
			walk(f.Type)
		}
	}
	walk(reflect.TypeOf(Config{}))
	if len(seen) < 8 {
		t.Fatalf("walked only %d types; the walk is broken", len(seen))
	}
}

func TestSchemaDescribesDurationsAsStrings(t *testing.T) {
	raw, err := ArtifactSchema()
	if err != nil {
		t.Fatal(err)
	}
	var doc schemaDoc
	_ = json.Unmarshal(raw, &doc)
	timeout := doc.Defs[schemaName(reflect.TypeOf(Upstream{}))]["properties"].(map[string]any)["timeout"].(map[string]any)
	if timeout["type"] != "string" {
		t.Fatalf("upstream.timeout schema = %v, want a duration string", timeout)
	}
}

// TestSchemaAcceptsTheShippedExamples proves the schema is not stricter
// than the decoder: every example artifact in the repo passes it.
func TestSchemaAcceptsTheShippedExamples(t *testing.T) {
	raw, err := ArtifactSchema()
	if err != nil {
		t.Fatal(err)
	}
	var schemaDocAny any
	_ = json.Unmarshal(raw, &schemaDocAny)
	c := jsonschema.NewCompiler()
	if err := c.AddResource("artifact.json", schemaDocAny); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("artifact.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	files, _ := filepath.Glob("../../examples/*/restitch.yaml")
	files = append(files, "../../restitch.yaml")
	if len(files) < 3 {
		t.Fatalf("found only %v", files)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var doc any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		// Round-trip through JSON so integers and maps take JSON shapes.
		j, _ := json.Marshal(doc)
		var inst any
		_ = json.Unmarshal(j, &inst)
		if err := sch.Validate(inst); err != nil {
			t.Errorf("%s does not match the schema: %v", f, err)
		}
	}
}
