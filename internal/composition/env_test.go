package composition

import (
	"net/http/httptest"
	"testing"
)

func TestBuildRequestEnv_Aliases(t *testing.T) {
	req := httptest.NewRequest("GET", "/test?a=1&a=2&b=3", nil)
	req.Header.Set("X-Custom", "val")

	params := map[string]string{"id": "42"}
	body := map[string]any{"key": "value"}

	env := buildRequestEnv(req, params, body, nil)

	reqData := env["req"].(map[string]any)
	requestData := env["request"].(map[string]any)

	// Same map (alias)
	if reqData["method"] != requestData["method"] {
		t.Error("request and req should be the same map")
	}

	// method
	if reqData["method"] != "GET" {
		t.Errorf("method = %v, want GET", reqData["method"])
	}

	// params
	p := reqData["params"].(map[string]string)
	if p["id"] != "42" {
		t.Errorf("params.id = %v, want 42", p["id"])
	}

	// body
	b := reqData["body"].(map[string]any)
	if b["key"] != "value" {
		t.Errorf("body.key = %v, want value", b["key"])
	}

	// query (first value)
	q := reqData["query"].(map[string]string)
	if q["a"] != "1" {
		t.Errorf("query.a = %v, want 1", q["a"])
	}

	// query_all
	qa := reqData["query_all"].(map[string][]string)
	if len(qa["a"]) != 2 || qa["a"][0] != "1" || qa["a"][1] != "2" {
		t.Errorf("query_all.a = %v, want [1, 2]", qa["a"])
	}

	// headers
	h := reqData["headers"].(map[string]string)
	if h["X-Custom"] != "val" {
		t.Errorf("headers.X-Custom = %v, want val", h["X-Custom"])
	}
}
