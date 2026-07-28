package mockupstream

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Handler returns the mock upstream HTTP handler with all test routes.
func Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		writeJSON(w, map[string]any{
			"id":     id,
			"name":   "user-" + id,
			"active": true,
		})
	})

	mux.HandleFunc("GET /orders", func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("userId")
		writeJSON(w, []map[string]any{
			{"id": 1, "userId": userID, "total": 9.5},
		})
	})

	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		ms, _ := strconv.Atoi(r.URL.Query().Get("ms"))
		if ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
		writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("GET /status/{code}", func(w http.ResponseWriter, r *http.Request) {
		code, _ := strconv.Atoi(r.PathValue("code"))
		if code < 100 || code > 599 {
			code = 200
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": code})
	})

	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		headers := make(map[string]string)
		for k, v := range r.Header {
			if len(v) > 0 {
				headers[k] = v[0]
			}
		}

		query := make(map[string]string)
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				query[k] = v[0]
			}
		}

		writeJSON(w, map[string]any{
			"method":  r.Method,
			"path":    r.URL.Path,
			"query":   query,
			"headers": headers,
			"body":    string(body),
		})
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "ok"})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
