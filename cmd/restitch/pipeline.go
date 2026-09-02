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
	"net/http"
	"sync/atomic"

	"github.com/alternayte/restitch-gateway/internal/composition"
)

// Pipeline is everything derived from one config file version.
type Pipeline struct {
	Hash     string
	Compiled *composition.CompiledConfig
	Handler  http.Handler
	Executor *composition.Executor
}

func (p *Pipeline) Close() {
	if p.Executor != nil {
		p.Executor.Close()
	}
	if p.Compiled != nil {
		// Release the idle connections of every per-upstream client so
		// repeated reloads do not leak sockets (finding L10).
		for _, up := range p.Compiled.Upstreams {
			if up != nil && up.Client != nil {
				up.Client.CloseIdleConnections()
			}
		}
	}
}

// Swapper dispatches requests to the current Pipeline via atomic swap.
type Swapper struct {
	ptr atomic.Pointer[Pipeline]
}

func newSwapper(p *Pipeline) *Swapper {
	s := &Swapper{}
	s.ptr.Store(p)
	return s
}

func (s *Swapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.ptr.Load().Handler.ServeHTTP(w, r)
}

func (s *Swapper) Swap(p *Pipeline) *Pipeline {
	return s.ptr.Swap(p)
}

func (s *Swapper) Current() *Pipeline {
	return s.ptr.Load()
}
