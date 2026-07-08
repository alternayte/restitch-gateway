package testenv

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestE2E_HappyPath(t *testing.T) {
	Run(t, Config{
		YAML: `
upstreams:
  mock:
    url: "@@UPSTREAM_mock@@"
compositions:
  dashboard:
    path: /api/dashboard
    steps:
      - name: user
        upstream: mock
        path: /users/1
      - name: posts
        upstream: mock
        path: /posts
    response:
      status: 200
      body:
        user: "{{ steps.user.body }}"
        posts: "{{ steps.posts.body }}"
`,
		Upstreams: map[string]UpstreamSpec{
			"mock": {Script: map[string]ScriptedResponse{
				"GET /users/1": {Body: `{"id":1,"name":"Alice"}`},
				"GET /posts":   {Body: `[{"id":101,"title":"Post 1"}]`},
			}},
		},
	}, func(t *testing.T, env *Env) {
		resp, err := env.Client.Get(env.GatewayURL + "/api/dashboard")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		if resp.Header.Get("X-Restitch-Complete") != "true" {
			t.Errorf("X-Restitch-Complete = %q, want true", resp.Header.Get("X-Restitch-Complete"))
		}

		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)
		if body["user"] == nil {
			t.Error("user should be populated")
		}
		if body["posts"] == nil {
			t.Error("posts should be populated")
		}
	})
}

func TestE2E_PartialResponse(t *testing.T) {
	Run(t, Config{
		YAML: `
upstreams:
  mock:
    url: "@@UPSTREAM_mock@@"
  dead:
    url: "@@UPSTREAM_dead@@"
compositions:
  partial:
    path: /p
    steps:
      - name: user
        upstream: mock
        path: /users/1
      - name: loyalty
        upstream: dead
        path: /x
        optional: true
      - name: bonus
        upstream: mock
        path: /bonus
        optional: true
    response:
      status: 200
      body:
        user: "{{ steps.user.body }}"
        points: "{{ steps.loyalty.body.points }}"
`,
		Upstreams: map[string]UpstreamSpec{
			"mock": {Script: map[string]ScriptedResponse{
				"GET /users/1": {Body: `{"id":1,"name":"Alice"}`},
				"GET /bonus":   {Body: `{"bonus":true}`},
			}},
			"dead": {CloseOnStart: true},
		},
	}, func(t *testing.T, env *Env) {
		resp, err := env.Client.Get(env.GatewayURL + "/p")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if resp.Header.Get("X-Restitch-Complete") != "false" {
			t.Errorf("X-Restitch-Complete = %q, want false", resp.Header.Get("X-Restitch-Complete"))
		}
		if resp.Header.Get("X-Partial-Response") != "true" {
			t.Errorf("X-Partial-Response = %q, want true", resp.Header.Get("X-Partial-Response"))
		}

		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)
		if body["user"] == nil {
			t.Error("user should be populated")
		}
		if body["points"] != nil {
			t.Errorf("points should be null, got %v", body["points"])
		}

		errList, ok := body["_errors"].([]any)
		if !ok || len(errList) == 0 {
			t.Fatal("expected _errors array")
		}
		foundLoyalty := false
		for _, e := range errList {
			em := e.(map[string]any)
			if em["step"] == "loyalty" {
				foundLoyalty = true
			}
		}
		if !foundLoyalty {
			t.Error("_errors should contain loyalty step")
		}
	})
}

func TestE2E_RequiredFailure_502(t *testing.T) {
	Run(t, Config{
		YAML: `
upstreams:
  dead:
    url: "@@UPSTREAM_dead@@"
compositions:
  fail:
    path: /fail
    steps:
      - name: data
        upstream: dead
        path: /x
    response:
      status: 200
      body:
        data: "{{ steps.data.body }}"
`,
		Upstreams: map[string]UpstreamSpec{
			"dead": {CloseOnStart: true},
		},
	}, func(t *testing.T, env *Env) {
		resp, err := env.Client.Get(env.GatewayURL + "/fail")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 502 {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 502, got %d: %s", resp.StatusCode, body)
		}

		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)
		if body["error"] != "upstream error" {
			t.Errorf("error = %v, want 'upstream error'", body["error"])
		}
		if body["step"] != "data" {
			t.Errorf("step = %v, want 'data'", body["step"])
		}
	})
}

func TestE2E_Timeout_504(t *testing.T) {
	Run(t, Config{
		YAML: `
upstreams:
  slow:
    url: "@@UPSTREAM_slow@@"
    timeout: 50ms
compositions:
  timeout:
    path: /timeout
    steps:
      - name: s
        upstream: slow
        path: /slow
    response:
      status: 200
      body:
        data: "{{ steps.s.body }}"
`,
		Upstreams: map[string]UpstreamSpec{
			"slow": {Script: map[string]ScriptedResponse{
				"GET /slow": {Body: `{"ok":true}`, DelayMS: 500},
			}},
		},
	}, func(t *testing.T, env *Env) {
		resp, err := env.Client.Get(env.GatewayURL + "/timeout")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 504 {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 504, got %d: %s", resp.StatusCode, body)
		}

		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)
		if body["error"] != "upstream timeout" {
			t.Errorf("error = %v, want 'upstream timeout'", body["error"])
		}
	})
}

func TestE2E_PathParams(t *testing.T) {
	Run(t, Config{
		YAML: `
upstreams:
  mock:
    url: "@@UPSTREAM_mock@@"
compositions:
  user:
    path: "/api/users/{id}"
    steps:
      - name: u
        upstream: mock
        path: "/users/{{ req.params.id }}"
    response:
      status: 200
      body:
        user: "{{ steps.u.body }}"
`,
		Upstreams: map[string]UpstreamSpec{
			"mock": {Script: map[string]ScriptedResponse{
				"GET /users/42": {Body: `{"id":"42","name":"user-42"}`},
			}},
		},
	}, func(t *testing.T, env *Env) {
		resp, err := env.Client.Get(env.GatewayURL + "/api/users/42")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)
		user, ok := body["user"].(map[string]any)
		if !ok {
			t.Fatalf("user not a map: %T", body["user"])
		}
		if user["id"] != "42" {
			t.Errorf("id = %v, want 42", user["id"])
		}
	})
}

func TestE2E_Retry(t *testing.T) {
	var attempt atomic.Int64
	Run(t, Config{
		YAML: `
upstreams:
  flaky:
    url: "@@UPSTREAM_flaky@@"
    retry:
      max_attempts: 3
      interval: 10ms
      backoff_on: [503]
compositions:
  retry:
    path: /retry
    steps:
      - name: s
        upstream: flaky
        path: /data
    response:
      status: 200
      body:
        result: "{{ steps.s.body }}"
`,
		Upstreams: map[string]UpstreamSpec{
			"flaky": {Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := attempt.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if n < 2 {
					w.WriteHeader(503)
					fmt.Fprint(w, `{"error":"retry"}`)
					return
				}
				w.WriteHeader(200)
				fmt.Fprint(w, `{"ok":true}`)
			})},
		},
	}, func(t *testing.T, env *Env) {
		resp, err := env.Client.Get(env.GatewayURL + "/retry")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		hits := env.Hits("flaky")
		if hits != 2 {
			t.Errorf("expected 2 upstream hits (1 fail + 1 success), got %d", hits)
		}
	})
}

func TestE2E_ResponseSizeCap(t *testing.T) {
	bigBody := `{"data":"` + strings.Repeat("x", 200) + `"}`
	Run(t, Config{
		YAML: `
upstreams:
  big:
    url: "@@UPSTREAM_big@@"
    max_response_bytes: 100
compositions:
  big:
    path: /big
    steps:
      - name: s
        upstream: big
        path: /data
    response:
      status: 200
      body:
        result: "{{ steps.s.body }}"
`,
		Upstreams: map[string]UpstreamSpec{
			"big": {Script: map[string]ScriptedResponse{
				"GET /data": {Body: bigBody},
			}},
		},
	}, func(t *testing.T, env *Env) {
		resp, err := env.Client.Get(env.GatewayURL + "/big")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 502 {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 502 for oversized response, got %d: %s", resp.StatusCode, body)
		}
	})
}
