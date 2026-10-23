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
	"path"
	"reflect"
	"time"

	"github.com/invopop/jsonschema"
)

// schemaName names a type's definition after its package, so auth.Config
// and ratelimit.Config do not collide.
func schemaName(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Name() == "" || t.PkgPath() == "" {
		return "" // anonymous and builtin types keep the reflector's own naming
	}
	return path.Base(t.PkgPath()) + "." + t.Name()
}

// ArtifactSchema returns the JSON Schema of the artifact, generated from
// the Go types that decode it. Studio builds its forms and its YAML
// checks from it, so neither can drift from what the gateway accepts.
func ArtifactSchema() ([]byte, error) {
	r := &jsonschema.Reflector{
		FieldNameTag:               "yaml",
		RequiredFromJSONSchemaTags: true,
		Namer:                      schemaName,
		Mapper: func(t reflect.Type) *jsonschema.Schema {
			if t == reflect.TypeOf(time.Duration(0)) {
				return &jsonschema.Schema{
					Type:        "string",
					Pattern:     `^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`,
					Description: "A Go duration, for example 500ms, 5s or 1m30s.",
				}
			}
			return nil
		},
	}
	s := r.Reflect(&Config{})
	s.Title = "Restitch artifact"
	return json.MarshalIndent(s, "", "  ")
}
