# Restitch

## What This Is

A REST API composition gateway that eliminates hand-written Backend-for-Frontend (BFF) layers. Teams declaratively compose multiple REST API endpoints into unified responses using YAML configuration — no backend code changes required. Think Apollo Router + GraphQL Hive, but for REST APIs.

Two binaries:
- **restitch-gateway** — High-performance Go proxy that executes compositions at the edge (data plane)
- **restitch-studio** — Web UI for observability, config management, and visual composition editing (control plane)

## Core Value

Frontend teams can compose data from multiple backend services without writing, deploying, or maintaining BFF code.

## Current State

**Version:** v1.0 MVP (shipped 2026-02-03)

**Codebase:**
- 8,475 lines of Go
- Tech stack: Go 1.21+, expr-lang/expr v1.17.7, gopkg.in/yaml.v3, oklog/ulid
- 93 files across cmd/, internal/, and .planning/

**Capabilities:**
- YAML-configured compositions with DAG-based parallel execution
- Expr language for dynamic path/param/response evaluation
- 4 auth strategies: header, basic, passthrough, OAuth2 client credentials
- Graceful degradation with optional steps and partial responses
- Structured JSON logging with request ID tracing and per-step timing
- Health endpoints (/health, /ready, /health/upstreams)
- Graceful shutdown with connection draining

## Requirements

### Validated

- GATE-01 through GATE-06 — Gateway core (routing, TLS, health, shutdown) — v1.0
- COMP-01 through COMP-11 — Composition engine (YAML, DAG, parallel, Expr, merging) — v1.0
- AUTH-01 through AUTH-06 — Upstream auth (header, basic, passthrough, OAuth2) — v1.0
- ERR-01 through ERR-05 — Error handling (rules, optional, partial, timeout) — v1.0
- OBS-01 through OBS-04 — Observability (logging, request ID, timing, DAG order) — v1.0

### Active

(Pending next milestone planning)

### Out of Scope

- Caching — use CDN or upstream caching for now
- Request coalescing — optimization, not correctness
- Studio — not needed until 20+ compositions
- WASM plugins — hardcode what's needed
- OpenAPI import — manually write YAML for known services
- Circuit breakers — let upstreams handle their own availability
- Hot-reload — restart binary (200ms), implement later
- Inbound auth (JWT validation) — behind VPN/service mesh for internal use

## Context

**Problem:** GraphQL Federation elegantly solved API composition for GraphQL. REST APIs have no equivalent — teams write BFF services that are just glue code wrapping fetch calls, each requiring its own repo, deployment, CI pipeline, monitoring, and on-call rotation.

**Hidden costs being solved:**
- Teams replicating data to avoid cross-service calls
- Teams reading from other systems' databases directly
- One failing upstream takes down entire BFF responses

**Composition model:**
- Steps define upstream calls with dependencies
- Dependencies form a DAG — independent steps execute in parallel
- Expr language interpolates values from earlier steps into later ones
- Response block merges/reshapes data from all steps

**Example execution flow:**
```
Request -> user step -> +-> orders step -+-> merge -> respond
                        +-> loyalty step -+
                           (parallel)
```

**Target users:**
- Platform engineers (write YAML, configure auth, tune performance)
- Backend devs (create compositions for their domain)
- Frontend devs (use visual editor for exploration, mostly consumers)

## Constraints

- **Language**: Gateway in Go (performance at edge)
- **Config format**: YAML with Expr language for dynamic evaluation
- **Path**: Internal dogfooding -> Open source -> Commercial product

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go for gateway | Performance-critical data plane, goroutines for parallel step execution | Good |
| Expr for dynamic values | Established expression language, avoids custom DSL | Good |
| YAML for config | Human-readable, git-friendly, familiar to platform engineers | Good |
| Studio as separate binary | Optional component, can run gateway standalone | Good |
| Skip hot-reload for v1 | 200ms restart acceptable, reduces complexity | Good |
| Skip inbound auth for v1 | Internal use behind VPN/mesh, simple API key sufficient | Good |
| Steps required by default | Explicit optional marking for graceful degradation | Good |
| Fail-fast config validation | Parse-time DAG validation catches errors at startup | Good |
| HTTP 200 for partial | Composition succeeded, partial data is valid | Good |
| MaxIdleConnsPerHost: 100 | Avoids 4-5x latency penalty from default 2 | Good |
| UpstreamInfo bridge type | Avoids import cycle between server and composition | Good |

---
*Last updated: 2026-02-03 after v1.0 milestone*
