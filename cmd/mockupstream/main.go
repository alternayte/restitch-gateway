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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/alternayte/restitch-gateway/internal/mockupstream"
)

// connCounter tracks accepted TCP connections and served requests. The M23 gate
// reads these to prove connection pooling reduces churn: with a large
// MaxIdleConnsPerHost the gateway reuses connections, so connsAccepted stays
// near peak parallelism instead of tracking request count.
type connCounter struct {
	connsAccepted atomic.Int64
	requests      atomic.Int64
}

func (c *connCounter) onConnState(_ net.Conn, state http.ConnState) {
	if state == http.StateNew {
		c.connsAccepted.Add(1)
	}
}

// countRequests wraps a handler, counting every request that is not itself a
// stats query.
func (c *connCounter) countRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.requests.Add(1)
		next.ServeHTTP(w, r)
	})
}

func main() {
	port := flag.Int("port", 8081, "server port")
	flag.Parse()

	counter := &connCounter{}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /__stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{
			"conns_accepted": counter.connsAccepted.Load(),
			"requests":       counter.requests.Load(),
		})
	})

	mux.HandleFunc("POST /__stats/reset", func(w http.ResponseWriter, _ *http.Request) {
		counter.connsAccepted.Store(0)
		counter.requests.Store(0)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"reset": true})
	})

	mux.Handle("/", counter.countRequests(mockupstream.Handler()))

	addr := fmt.Sprintf(":%d", *port)
	srv := &http.Server{
		Addr:      addr,
		Handler:   mux,
		ConnState: counter.onConnState,
	}

	log.Printf("mockupstream listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
