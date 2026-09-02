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

package server

import (
	"encoding/json"
	"net/http"
)

// Version is set at build time via ldflags.
var Version = "dev"

// HealthResponse represents the JSON response for the /health endpoint.
type HealthResponse struct {
	Status string `json:"status"`
}

// ReadyResponse represents the JSON response for the /ready endpoint.
type ReadyResponse struct {
	Status string `json:"status"`
}

// HealthHandler creates an HTTP handler for the /health endpoint.
func HealthHandler(srv *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
	}
}

// ReadyHandler creates an HTTP handler for the /ready endpoint.
func ReadyHandler(srv *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if srv.Ready() {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(ReadyResponse{Status: "ready"})
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(ReadyResponse{Status: "not_ready"})
		}
	}
}
