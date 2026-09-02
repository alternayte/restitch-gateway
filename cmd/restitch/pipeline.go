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
