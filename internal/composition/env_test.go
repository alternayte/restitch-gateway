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
	"context"
	"net/http/httptest"
	"testing"
)

func TestBuildRequestEnv_Aliases(t *testing.T) {
	req := httptest.NewRequest("GET", "/test?a=1&a=2&b=3", nil)
	req.Header.Set("X-Custom", "val")

	params := map[string]string{"id": "42"}
	body := map[string]any{"key": "value"}

	rd := NewRequestData(req, params, body)
	env := buildRequestEnv(context.Background(), rd, nil)

	reqData := env["req"].(map[string]any)
	requestData := env["request"].(map[string]any)

	if reqData["method"] != requestData["method"] {
		t.Error("request and req should be the same map")
	}

	if reqData["method"] != "GET" {
		t.Errorf("method = %v, want GET", reqData["method"])
	}

	p := reqData["params"].(map[string]string)
	if p["id"] != "42" {
		t.Errorf("params.id = %v, want 42", p["id"])
	}

	b := reqData["body"].(map[string]any)
	if b["key"] != "value" {
		t.Errorf("body.key = %v, want value", b["key"])
	}

	q := reqData["query"].(map[string]string)
	if q["a"] != "1" {
		t.Errorf("query.a = %v, want 1", q["a"])
	}

	qa := reqData["query_all"].(map[string][]string)
	if len(qa["a"]) != 2 {
		t.Errorf("query_all.a len = %d, want 2", len(qa["a"]))
	}

	h := reqData["headers"].(map[string]string)
	if h["X-Custom"] != "val" {
		t.Errorf("headers.X-Custom = %v, want val", h["X-Custom"])
	}
}
