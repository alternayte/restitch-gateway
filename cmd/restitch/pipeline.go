package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"

	"github.com/restitch/restitch-gateway/internal/composition"
	"github.com/restitch/restitch-gateway/internal/inbound"
	"github.com/restitch/restitch-gateway/internal/server"
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

type PipelineDeps struct {
	Authenticator *inbound.Authenticator
	Server        *server.Server
}

func buildPipeline(ctx context.Context, path string, deps PipelineDeps) (*Pipeline, error) {
	cfg, err := composition.LoadConfigFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	compiled, err := composition.CompileConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("compile config: %w", err)
	}

	handler := composition.NewHandler(compiled, deps.Authenticator)

	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Handle(http.MethodGet, "/health", server.HealthHandler(deps.Server))
	router.Handle(http.MethodGet, "/ready", server.ReadyHandler(deps.Server))
	router.Finalize()

	hash := configHash(path)

	return &Pipeline{
		Hash:     hash,
		Compiled: compiled,
		Handler:  router,
		Executor: handler.Executor(),
	}, nil
}

func configHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := sha256.New()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
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
