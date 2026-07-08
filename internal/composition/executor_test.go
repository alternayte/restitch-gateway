package composition

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExecutor(t *testing.T) {
	t.Run("parallel execution of independent steps", func(t *testing.T) {
		// Track execution timing to verify parallelism
		var mu sync.Mutex
		startTimes := make(map[string]time.Time)

		// Mock upstream server with delay
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			startTimes[r.URL.Path] = time.Now()
			mu.Unlock()

			// Add small delay to verify parallel execution
			time.Sleep(10 * time.Millisecond)

			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/user" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":   123,
					"name": "Alice",
				})
			} else if r.URL.Path == "/profile" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"bio": "Software engineer",
				})
			}
		}))
		defer server.Close()

		// Create composition with two independent steps
		config := createTestConfig(server.URL, `
compositions:
  test:
    path: /test
    steps:
      - name: user
        upstream: test-upstream
        path: /user
      - name: profile
        upstream: test-upstream
        path: /profile
    response:
      status: 200
      body:
        user: "{{ steps.user.body }}"
        profile: "{{ steps.profile.body }}"
`)

		executor := NewExecutor(config)

		// Execute composition
		req := httptest.NewRequest("GET", "/test", nil)
		result, err := executor.Execute(context.Background(), "test", req, nil, nil)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Verify both steps completed
		if len(result.Steps) != 2 {
			t.Errorf("Expected 2 steps, got %d", len(result.Steps))
		}

		// Verify steps executed in parallel (started within 5ms of each other)
		userStart := startTimes["/user"]
		profileStart := startTimes["/profile"]
		timeDiff := userStart.Sub(profileStart)
		if timeDiff < 0 {
			timeDiff = -timeDiff
		}
		if timeDiff > 5*time.Millisecond {
			t.Errorf("Steps didn't execute in parallel (time diff: %v)", timeDiff)
		}
	})

	t.Run("dependent steps wait for dependencies", func(t *testing.T) {
		// Track execution order
		var mu sync.Mutex
		executionOrder := []string{}

		// Mock upstream server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fullPath := r.URL.Path
			if r.URL.RawQuery != "" {
				fullPath += "?" + r.URL.RawQuery
			}

			mu.Lock()
			executionOrder = append(executionOrder, fullPath)
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/users/123" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":   123,
					"name": "Alice",
				})
			} else if strings.HasPrefix(r.URL.Path, "/orders") {
				json.NewEncoder(w).Encode([]interface{}{
					map[string]interface{}{"id": 1},
					map[string]interface{}{"id": 2},
				})
			}
		}))
		defer server.Close()

		// Create composition with dependent steps
		config := createTestConfig(server.URL, `
compositions:
  test:
    path: /test
    steps:
      - name: user
        upstream: test-upstream
        path: /users/123
      - name: orders
        upstream: test-upstream
        path: "/orders?user_id={{ steps.user.body.id }}"
    response:
      status: 200
      body:
        user: "{{ steps.user.body }}"
        orders: "{{ steps.orders.body }}"
`)

		executor := NewExecutor(config)

		// Execute composition
		req := httptest.NewRequest("GET", "/test", nil)
		result, err := executor.Execute(context.Background(), "test", req, nil, nil)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Verify both steps completed
		if len(result.Steps) != 2 {
			t.Errorf("Expected 2 steps, got %d", len(result.Steps))
		}

		// Verify user step executed before orders step
		mu.Lock()
		defer mu.Unlock()
		if len(executionOrder) != 2 {
			t.Fatalf("Expected 2 requests, got %d", len(executionOrder))
		}
		if executionOrder[0] != "/users/123" {
			t.Errorf("Expected user step first, got %s", executionOrder[0])
		}
		if !strings.HasPrefix(executionOrder[1], "/orders") {
			t.Errorf("Expected orders step second, got %s", executionOrder[1])
		}

		// Verify orders step received user ID from dependency
		if !strings.Contains(executionOrder[1], "user_id=123") {
			t.Errorf("Expected orders request to include user_id=123, got %s", executionOrder[1])
		}
	})

	t.Run("upstream 500 is not step failure - composition completes", func(t *testing.T) {
		// Mock upstream server - step1 returns 500, step2 succeeds
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/error" {
				// Upstream returns 500 - this is passthrough, not step failure
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "internal server error",
				})
				return
			}

			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}))
		defer server.Close()

		// Create composition with two independent steps (parallel wave)
		config := createTestConfig(server.URL, `
compositions:
  test:
    path: /test
    steps:
      - name: step1
        upstream: test-upstream
        path: /error
      - name: step2
        upstream: test-upstream
        path: /success
    response:
      status: 200
      body: {}
`)

		executor := NewExecutor(config)

		// Execute composition - should complete with both results
		// Per CONTEXT.md: "Upstream error passthrough" means we return the 500 status
		// in the result, but the step itself completed successfully
		req := httptest.NewRequest("GET", "/test", nil)
		result, err := executor.Execute(context.Background(), "test", req, nil, nil)
		if err != nil {
			t.Fatalf("Execute should not fail on upstream 500: %v", err)
		}

		// Verify both steps completed
		if len(result.Steps) != 2 {
			t.Errorf("Expected 2 steps, got %d", len(result.Steps))
		}

		// Verify step1 has 500 status (passthrough)
		if result.Steps["step1"].Status != 500 {
			t.Errorf("Expected step1 status 500, got %d", result.Steps["step1"].Status)
		}

		// Verify step2 has 200 status
		if result.Steps["step2"].Status != 200 {
			t.Errorf("Expected step2 status 200, got %d", result.Steps["step2"].Status)
		}
	})

	t.Run("step results accessible to dependent steps", func(t *testing.T) {
		// Mock upstream server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if r.URL.Path == "/user" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":    123,
					"email": "alice@example.com",
				})
			} else if r.URL.Path == "/profile" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"userId": 123,
					"bio":    "Engineer",
				})
			} else if strings.HasPrefix(r.URL.Path, "/combined") {
				// This step depends on both user and profile
				// Verify it can access both results
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": true,
				})
			}
		}))
		defer server.Close()

		// Create composition with diamond pattern:
		// user + profile (parallel) -> combined (depends on both)
		config := createTestConfig(server.URL, `
compositions:
  test:
    path: /test
    steps:
      - name: user
        upstream: test-upstream
        path: /user
      - name: profile
        upstream: test-upstream
        path: /profile
      - name: combined
        upstream: test-upstream
        path: "/combined?user_id={{ steps.user.body.id }}&bio={{ steps.profile.body.bio }}"
    response:
      status: 200
      body:
        user: "{{ steps.user.body }}"
        profile: "{{ steps.profile.body }}"
        combined: "{{ steps.combined.body }}"
`)

		executor := NewExecutor(config)

		// Execute composition
		req := httptest.NewRequest("GET", "/test", nil)
		result, err := executor.Execute(context.Background(), "test", req, nil, nil)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Verify all three steps completed
		if len(result.Steps) != 3 {
			t.Errorf("Expected 3 steps, got %d", len(result.Steps))
		}

		// Verify combined step has result
		combined, exists := result.Steps["combined"]
		if !exists {
			t.Fatal("Expected combined step result")
		}
		if combined.Status != 200 {
			t.Errorf("Expected combined status 200, got %d", combined.Status)
		}
	})

	t.Run("context cancellation stops execution", func(t *testing.T) {
		// Mock upstream server with delay
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}))
		defer server.Close()

		config := createTestConfig(server.URL, `
compositions:
  test:
    path: /test
    steps:
      - name: step1
        upstream: test-upstream
        path: /test
    response:
      status: 200
      body: {}
`)

		executor := NewExecutor(config)

		// Create context that will be cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Execute composition - should fail with context error
		req := httptest.NewRequest("GET", "/test", nil)
		_, err := executor.Execute(ctx, "test", req, nil, nil)
		if err == nil {
			t.Fatal("Expected error from cancelled context")
		}
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("Expected context canceled error, got: %v", err)
		}
	})

	t.Run("composition not found error", func(t *testing.T) {
		config := createTestConfig("http://localhost", `
compositions:
  existing:
    path: /existing
    steps: []
    response:
      status: 200
      body: {}
`)

		executor := NewExecutor(config)

		req := httptest.NewRequest("GET", "/test", nil)
		_, err := executor.Execute(context.Background(), "nonexistent", req, nil, nil)
		if err == nil {
			t.Fatal("Expected error for nonexistent composition")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("Expected 'not found' error, got: %v", err)
		}
	})
}

func TestExecutor_InferredDependencySkipped(t *testing.T) {
	var bHits int
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/b-endpoint" {
			bHits++
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": 1})
	}))
	defer mockServer.Close()

	// Step a is optional and targets a dead upstream (will fail).
	// Step b references steps.a.body.id in its path but has NO depends_on.
	// With resolved deps, b should be skipped because a is an inferred dep.
	fullYAML := `
upstreams:
  test-upstream:
    url: ` + mockServer.URL + `
  dead:
    url: "http://127.0.0.1:1"
compositions:
  test:
    path: /test
    steps:
      - name: a
        upstream: dead
        path: /fail
        optional: true
      - name: b
        upstream: test-upstream
        path: "/b-endpoint/{{ steps.a.body.id }}"
        optional: true
    response:
      status: 200
      body:
        result: "ok"
`
	cfg, err := ParseConfig([]byte(fullYAML))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	config, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}

	executor := NewExecutor(config)
	req := httptest.NewRequest("GET", "/test", nil)
	result, err := executor.Execute(context.Background(), "test", req, nil, nil)
	if err != nil {
		t.Fatalf("Execute should not return error (both steps optional): %v", err)
	}

	if bHits != 0 {
		t.Errorf("step b upstream was called %d times, expected 0 (should be skipped)", bHits)
	}

	if !result.IsPartial {
		t.Error("expected partial result")
	}

	// Check that b is skipped in timings
	for _, timing := range result.StepTimings {
		if timing.Name == "b" && timing.Status != "skipped" {
			t.Errorf("step b status = %q, want 'skipped'", timing.Status)
		}
	}

	// Check _errors contains dependency_failed for step b
	foundSkipped := false
	for _, e := range result.StepErrors {
		if e.Step == "b" && e.Message == "dependency_failed" {
			foundSkipped = true
		}
	}
	if !foundSkipped {
		t.Errorf("expected _errors to contain step b with dependency_failed, got: %+v", result.StepErrors)
	}
}

// createTestConfig creates a CompiledConfig from YAML string for testing.
func createTestConfig(upstreamURL, yamlContent string) *CompiledConfig {
	// Build full YAML with upstream
	fullYAML := `
upstreams:
  test-upstream:
    url: ` + upstreamURL + `
` + yamlContent

	cfg, err := ParseConfig([]byte(fullYAML))
	if err != nil {
		panic(err)
	}

	compiled, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		panic(err)
	}

	return compiled
}
