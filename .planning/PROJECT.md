# Restitch

## What This Is

A REST API composition gateway that eliminates hand-written Backend-for-Frontend (BFF) layers. Teams declaratively compose multiple REST API endpoints into unified responses using YAML configuration — no backend code changes required. Think Apollo Router + GraphQL Hive, but for REST APIs.

Two binaries:
- **restitch-gateway** — High-performance Go proxy that executes compositions at the edge (data plane)
- **restitch-studio** — Web UI for observability, config management, and visual composition editing (control plane)

## Core Value

Frontend teams can compose data from multiple backend services without writing, deploying, or maintaining BFF code.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Composition engine resolves DAG of steps with parallel execution
- [ ] Expr language evaluates dynamic values in paths, params, and response shaping
- [ ] Upstream auth: header (static API key/token)
- [ ] Upstream auth: basic (username/password)
- [ ] Upstream auth: passthrough (forward caller's auth)
- [ ] Upstream auth: oauth2_client_credentials (automatic token fetch/refresh)
- [ ] Error handling with graceful degradation (fallback responses when upstreams fail)
- [ ] YAML configuration for compositions

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
Request → user step → ┬→ orders step ─┬→ merge → respond
                      └→ loyalty step ─┘
                         (parallel)
```

**Target users:**
- Platform engineers (write YAML, configure auth, tune performance)
- Backend devs (create compositions for their domain)
- Frontend devs (use visual editor for exploration, mostly consumers)

**Studio usage patterns (future):**
- Day 1: Import specs, register services, build compositions
- Day 2+: Monitor latency waterfalls, debug failures, tune caches, audit changes

## Constraints

- **Language**: Gateway in Go (performance at edge)
- **Config format**: YAML with Expr language for dynamic evaluation
- **Path**: Internal dogfooding → Open source → Commercial product

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go for gateway | Performance-critical data plane, goroutines for parallel step execution | — Pending |
| Expr for dynamic values | Established expression language, avoids custom DSL | — Pending |
| YAML for config | Human-readable, git-friendly, familiar to platform engineers | — Pending |
| Studio as separate binary | Optional component, can run gateway standalone | — Pending |
| Skip hot-reload for v1 | 200ms restart acceptable, reduces complexity | — Pending |
| Skip inbound auth for v1 | Internal use behind VPN/mesh, simple API key sufficient | — Pending |

---
*Last updated: 2026-02-03 after initialization*
