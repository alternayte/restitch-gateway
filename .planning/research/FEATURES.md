# Feature Research: REST API Composition Gateways

**Domain:** API Gateway / API Composition Tools
**Researched:** 2026-02-03
**Confidence:** HIGH

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = product feels incomplete.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Request Routing** | Core purpose of any gateway - directing traffic to backends | LOW | URL path, query params, headers-based routing |
| **Load Balancing** | Distributes requests across instances, essential for availability | MEDIUM | Health checks, circuit breakers, weighted routing |
| **Rate Limiting / Throttling** | Protects backends from overload, prevents abuse | MEDIUM | Per IP, per API key, per consumer policies |
| **Authentication** | Security baseline - JWT, OAuth, API keys, basic auth | MEDIUM | Multiple auth strategies, pluggable providers |
| **TLS/SSL Termination** | HTTPS is mandatory in 2026, offload from backends | LOW | Certificate management, mTLS support |
| **Request/Response Transformation** | Modify headers, query params, paths without backend changes | MEDIUM | Parameter mapping, header injection |
| **Error Handling** | Standard error responses, HTTP status code management | LOW | Consistent error format, client-friendly messages |
| **Health Checks** | Monitor backend availability, prevent routing to dead services | MEDIUM | Active/passive checks, configurable intervals |
| **Observability - Metrics** | Request counts, latency, error rates - table stakes monitoring | MEDIUM | Integration with Prometheus, CloudWatch, etc. |
| **Observability - Logs** | Request/response logging for debugging and audit | LOW | Structured logs, log levels, PII filtering |
| **Multi-Protocol Support** | REST is baseline, but HTTP/2, WebSocket expected | MEDIUM | gRPC and GraphQL increasingly expected too |
| **Declarative Configuration** | YAML/JSON config files, version-controlled | LOW | GitOps-friendly, Infrastructure as Code |
| **Timeout Configuration** | Prevent requests hanging indefinitely | LOW | Per-route timeouts, global defaults |
| **CORS Handling** | Browser-based clients need proper CORS headers | LOW | Preflight handling, configurable origins |
| **Request Validation** | Basic validation of required params, headers | MEDIUM | OpenAPI schema validation increasingly expected |

### Differentiators (Competitive Advantage)

Features that set the product apart. Not required, but valued.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Intelligent API Composition** | Compose data from multiple backends into single response - core BFF pattern | HIGH | DAG-based dependencies, parallel execution |
| **Graceful Degradation** | Return partial data when some backends fail vs all-or-nothing | HIGH | Configurable fallback strategies per endpoint |
| **Expression Language for Dynamic Values** | Reference response data in subsequent requests without code | HIGH | Restitch's Expr language enables declarative dependencies |
| **Automatic Parallelization** | Execute independent backend calls concurrently for performance | MEDIUM | Analyze DAG, schedule parallel execution |
| **GraphQL-Level DX for REST** | Declarative data fetching across services without GraphQL complexity | HIGH | Single endpoint, multiple data sources, type safety |
| **Response Caching (Entity-Level)** | Cache at entity/field level, not just HTTP caching | HIGH | Apollo Router's approach - GraphQL-aware caching for REST |
| **Built-in Response Filtering** | Allow clients to select fields (like GraphQL field selection) | MEDIUM | Reduce bandwidth, improve mobile performance |
| **Built-in Response Grouping** | Namespace responses to avoid key collisions in composition | LOW | KrakenD approach - prevent field name conflicts |
| **Advanced Circuit Breaking** | Intelligent failure detection beyond simple error counts | HIGH | Latency-based, success rate thresholds, half-open state |
| **Distributed Tracing Integration** | OpenTelemetry support out of box for request flow visibility | MEDIUM | Span propagation across composed requests |
| **Real-Time Configuration Updates** | Change routes/config without restart or downtime | MEDIUM | Dynamic routing, hot-reload capabilities |
| **API Versioning Support** | Built-in version management strategies | MEDIUM | Header-based, path-based, gradual migration support |
| **Developer Portal / API Catalog** | Self-service discovery, interactive testing, documentation | HIGH | AWS API Gateway Portal (Nov 2025) sets new bar |
| **Cost Tracking / Analytics** | Per-endpoint cost attribution, usage analytics | MEDIUM | Especially valuable for internal platform teams |
| **AI/LLM Integration** | Universal LLM API routing, prompt caching, rate limiting | MEDIUM | Kong and Tyk added in 2025/2026 |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but create problems.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Gateway as ESB** | Centralize all internal routing through gateway | Creates monolithic gateway, single point of failure, tight coupling | Use gateway only for external traffic, service mesh for internal |
| **Complex Business Logic in Gateway** | Avoid writing backend code, do everything in gateway | Gateway becomes bloated, hard to test, violates separation of concerns | Keep gateway focused on infrastructure concerns, push logic to services |
| **Infinite Retry Policies** | "Always retry until success" for reliability | Retry storms overwhelm failing services, prevent recovery | Bounded retries with exponential backoff, circuit breakers |
| **Synchronous Fan-Out to Many Services** | One request triggers 20+ backend calls synchronously | Latency compounds, timeout risks, cascade failures | Limit composition depth, use async patterns, event-driven for fan-out |
| **Real-Time Everything** | WebSocket/SSE for all APIs | Complexity without value, connection overhead, harder to cache/scale | Use HTTP/REST for request-response, reserve real-time for actual real-time needs |
| **Gateway-Level Database Access** | Query databases directly from gateway for composition | Bypasses service boundaries, tight coupling to schemas, security risk | Gateway calls service APIs, services own their data |
| **Overly Permissive CORS** | `Access-Control-Allow-Origin: *` for all endpoints | Security vulnerability, credentials exposure | Explicit origin whitelisting, environment-specific configs |
| **Single Shared Gateway for All Teams** | One gateway to rule them all | Becomes deployment bottleneck, changes affect everyone, governance nightmare | BFF pattern - separate gateways per client type or bounded context |
| **Complete GraphQL Replacement** | "We'll expose everything as GraphQL through gateway" | GraphQL complexity, N+1 query problems, schema maintenance burden | Restitch approach: REST composition with GraphQL-like DX, not GraphQL itself |
| **Client-Side Composition** | Let frontend make multiple calls and compose | Increased latency (network round-trips), mobile performance issues, logic duplication | Server-side composition at gateway/BFF layer |

## Feature Dependencies

```
Authentication
    └──requires──> Request Routing (need to auth before route)

Rate Limiting
    └──requires──> Authentication (rate limit per authenticated user)

API Composition
    └──requires──> Request Routing (route to multiple backends)
    └──requires──> Error Handling (handle partial failures)
    └──requires──> Timeout Configuration (prevent hanging)
    └──enhances──> Response Caching (cache composed results)

Graceful Degradation
    └──requires──> API Composition (need multi-backend to degrade)
    └──requires──> Error Handling (determine what to degrade)

Expression Language
    └──requires──> API Composition (use data from one call in next)
    └──enables──> Dynamic Routing (route based on response data)

Automatic Parallelization
    └──requires──> API Composition (analyze dependencies)
    └──conflicts──> Synchronous Processing (must support async)

Circuit Breakers
    └──requires──> Health Checks (detect failures)
    └──enhances──> Graceful Degradation (fail fast, return defaults)

Distributed Tracing
    └──enhances──> API Composition (trace multi-hop requests)
    └──requires──> Observability - Logs (trace correlation)

Developer Portal
    └──requires──> Request Validation (generate from schemas)
    └──requires──> API Versioning (show version history)
    └──enhances──> Authentication (manage API keys)
```

### Dependency Notes

- **Authentication before Routing**: Must authenticate requests before determining routing to prevent unauthorized access to routing logic
- **Composition enables Degradation**: Can't gracefully degrade without composing multiple sources (single source = all or nothing)
- **Expression Language is Restitch's Core Differentiator**: Enables DAG-based dependencies without writing code, bridges gap between declarative config and dynamic behavior
- **Parallelization conflicts with naive implementations**: Requires async runtime, can't bolt onto synchronous request handlers
- **Developer Portal requires OpenAPI/schemas**: Interactive testing and documentation generation need machine-readable API definitions

## MVP Definition

### Launch With (v1)

Minimum viable product — what's needed to validate the concept.

- [ ] **Request Routing** — Can't be a gateway without routing
- [ ] **API Composition (Basic)** — Core value prop: compose multiple backends
- [ ] **Expression Language** — Differentiator: declarative dependencies without code
- [ ] **Automatic Parallelization** — Performance critical for composition to be viable
- [ ] **Error Handling (Basic)** — Must handle failures gracefully
- [ ] **Timeout Configuration** — Prevent hanging requests in composition
- [ ] **Declarative YAML Config** — Infrastructure as Code baseline
- [ ] **Observability - Logs** — Need to debug composition logic
- [ ] **Health Checks (Basic)** — Know when backends are down
- [ ] **Authentication (Simple)** — At least API key or JWT support
- [ ] **TLS/SSL Termination** — Security baseline

**MVP Focus**: Prove that declarative API composition with expression language delivers value to frontend teams. Must be reliable enough for staging/testing environments.

### Add After Validation (v1.x)

Features to add once core is working.

- [ ] **Graceful Degradation** — Trigger: Users report all-or-nothing failures frustrating
- [ ] **Response Caching (HTTP-Level)** — Trigger: Performance profiling shows duplicate calls
- [ ] **Rate Limiting** — Trigger: Production deployment needs abuse protection
- [ ] **Advanced Circuit Breaking** — Trigger: Cascading failures observed
- [ ] **Request Validation** — Trigger: Bad requests causing backend errors
- [ ] **Distributed Tracing** — Trigger: Debugging multi-hop compositions is painful
- [ ] **Observability - Metrics** — Trigger: Need dashboards for SLOs
- [ ] **Load Balancing** — Trigger: Backends have multiple instances
- [ ] **Request/Response Transformation** — Trigger: Backend changes break clients
- [ ] **API Versioning** — Trigger: Need to evolve APIs without breaking clients

### Future Consideration (v2+)

Features to defer until product-market fit is established.

- [ ] **Developer Portal** — Why defer: High complexity, uncertain if self-service is priority
- [ ] **Response Filtering (Field Selection)** — Why defer: Optimization, not core functionality
- [ ] **Entity-Level Caching** — Why defer: Complex, depends on type system, better after v1 lessons
- [ ] **Real-Time Configuration Updates** — Why defer: Deployment automation acceptable initially
- [ ] **AI/LLM Integration** — Why defer: Not relevant to initial target users
- [ ] **Multi-Protocol (gRPC, GraphQL)** — Why defer: REST composition proves concept first
- [ ] **Cost Tracking / Analytics** — Why defer: Platform maturity feature, not early-stage need

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| API Composition (Basic) | HIGH | HIGH | P1 |
| Expression Language | HIGH | HIGH | P1 |
| Automatic Parallelization | HIGH | MEDIUM | P1 |
| Request Routing | HIGH | LOW | P1 |
| Error Handling (Basic) | HIGH | LOW | P1 |
| Timeout Configuration | HIGH | LOW | P1 |
| Declarative YAML Config | HIGH | LOW | P1 |
| TLS/SSL Termination | HIGH | LOW | P1 |
| Authentication (Simple) | MEDIUM | LOW | P1 |
| Health Checks (Basic) | MEDIUM | LOW | P1 |
| Observability - Logs | MEDIUM | LOW | P1 |
| Graceful Degradation | HIGH | HIGH | P2 |
| Distributed Tracing | HIGH | MEDIUM | P2 |
| Rate Limiting | MEDIUM | MEDIUM | P2 |
| Request Validation | MEDIUM | MEDIUM | P2 |
| Advanced Circuit Breaking | MEDIUM | HIGH | P2 |
| Response Caching (HTTP) | MEDIUM | MEDIUM | P2 |
| Load Balancing | MEDIUM | MEDIUM | P2 |
| Observability - Metrics | MEDIUM | MEDIUM | P2 |
| Request/Response Transform | MEDIUM | MEDIUM | P2 |
| API Versioning | MEDIUM | MEDIUM | P2 |
| Developer Portal | LOW | HIGH | P3 |
| Response Filtering | MEDIUM | MEDIUM | P3 |
| Entity-Level Caching | MEDIUM | HIGH | P3 |
| Real-Time Config Updates | LOW | MEDIUM | P3 |
| Multi-Protocol Support | LOW | HIGH | P3 |
| Cost Tracking | LOW | MEDIUM | P3 |

**Priority key:**
- P1: Must have for launch (MVP)
- P2: Should have, add when possible (post-validation)
- P3: Nice to have, future consideration (v2+)

## Competitor Feature Analysis

| Feature | Kong Gateway | KrakenD | Tyk | Apollo Router | Restitch Approach |
|---------|--------------|---------|-----|---------------|-------------------|
| **API Composition** | Via plugins | ✓ Core feature | Via middleware | ✓ Core (GraphQL) | ✓ Core (REST) |
| **Parallel Execution** | Manual | ✓ Automatic | Manual | ✓ Automatic | ✓ Automatic |
| **Expression Language** | Lua scripting | Limited (JMESPath) | JavaScript | GraphQL query | ✓ Expr (declarative) |
| **Graceful Degradation** | Plugin-based | ✓ Partial responses | Circuit breakers | ✓ Field-level | ✓ Step-level |
| **Response Filtering** | Transform plugin | ✓ Allow/deny lists | JSON middleware | ✓ GraphQL fields | Planned (v2) |
| **Declarative Config** | ✓ YAML/JSON | ✓ JSON | ✓ JSON/YAML | ✓ YAML | ✓ YAML |
| **Developer Portal** | ✓ Enterprise | ✗ | ✓ | ✓ GraphOS | Future (v2) |
| **Caching Strategy** | HTTP caching | HTTP caching | HTTP caching | ✓ Entity-level | HTTP first, entity later |
| **Observability** | Extensive plugins | ✓ OpenTelemetry | ✓ Built-in | ✓ Extensive | Logs + tracing |
| **Multi-Protocol** | ✓ REST, gRPC, GraphQL | ✓ REST, gRPC, SOAP | ✓ REST, GraphQL, gRPC | GraphQL only | REST focus |
| **Complexity** | High (plugin ecosystem) | Medium (stateless) | Medium | Medium | LOW (opinionated) |
| **Primary Use Case** | Enterprise API platform | High-performance aggregation | API management | GraphQL federation | REST composition for frontends |

### Key Differentiators for Restitch

1. **GraphQL-level DX without GraphQL complexity**: Declarative composition with expression language, not query language
2. **Opinionated simplicity**: Kong requires learning plugin ecosystem; Restitch has composition built-in
3. **Frontend-first**: BFF pattern baked in, not general-purpose API gateway
4. **Graceful degradation by default**: Step-level failures don't kill entire composition (KrakenD has this, others don't)
5. **DAG-based execution model**: Explicit dependency graph vs sequential/manual orchestration

### What Restitch Doesn't Try to Compete On

- **Enterprise features**: No management UI, no multi-tenancy, no complex RBAC (initially)
- **Protocol diversity**: REST-first, not trying to be universal protocol translator
- **Plugin ecosystem**: Opinionated built-in features vs extensibility through plugins
- **AI/LLM features**: Not chasing 2025/2026 AI gateway trend

## Sources

### Official Documentation
- [Kong Gateway Documentation](https://developer.konghq.com/gateway/) - Kong gateway core features
- [KrakenD API Composition Documentation](https://www.krakend.io/docs/endpoints/response-manipulation/) - Composition and aggregation patterns
- [Apigee Anti-Patterns Guide](https://docs.apigee.com/api-platform/antipatterns/intro) - What to avoid in gateway design
- [AWS API Gateway Data Transformations](https://docs.aws.amazon.com/apigateway/latest/developerguide/rest-api-data-transformations.html) - Request/response transformation approaches
- [AWS Graceful Degradation Best Practices](https://docs.aws.amazon.com/wellarchitected/latest/reliability-pillar/rel_mitigate_interaction_failure_graceful_degradation.html) - Resilience patterns
- [Tyk Request Validation](https://tyk.io/docs/api-management/traffic-transformation/request-validation) - Schema validation with OpenAPI
- [Apollo Router Releases](https://github.com/apollographql/router/releases) - Latest Apollo Router features (2026)
- [Azure BFF Pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/backends-for-frontends) - Microsoft's BFF guidance

### Industry Analysis (2026)
- [Best API Gateway in 2026: Top Tools, Features](https://www.digitalapi.ai/blogs/best-api-gateway) - Market landscape
- [6 Must-Have Features of an API Gateway](https://zuplo.com/learning-center/top-api-gateway-features) - Table stakes features
- [Top 10 Best API Gateways for Developers in 2026](https://apidog.com/blog/best-api-gateways/) - Feature comparison
- [Kong Gateway Reviews 2026](https://www.g2.com/products/kong-gateway/reviews) - User feedback
- [Tyk API Gateway Features 2026](https://tyk.io/api-gateway/) - Tyk capabilities

### Anti-Patterns and Best Practices
- [The 5 Worst Anti-Patterns in API Management](https://thenewstack.io/the-5-worst-anti-patterns-in-api-management/) - Common mistakes
- [Microservices Architecture Gateway Pattern - Dos and Don'ts](https://akfpartners.com/growth-blog/microservices-architecture-gateway-pattern-dos-and-donts) - Architecture guidance
- [Common Mistakes in API Gateway and How to Avoid Them](https://www.syncloop.com/blogs/13-04-2025/common-mistakes-in-api-gateway-and-how-to-avoid-them.html) - Practical pitfalls
- [10 Common API Resilience Design Patterns](https://api7.ai/blog/10-common-api-resilience-design-patterns) - Resilience approaches
- [Best Practices of API Degradation](https://api7.ai/blog/degradation-in-api-gateway) - Graceful degradation strategies

### Composition and BFF Patterns
- [API Composition Pattern](https://microservices.io/patterns/data/api-composition.html) - Microservices.io reference
- [AWS API Composition Pattern](https://docs.aws.amazon.com/prescriptive-guidance/latest/modernization-data-persistence/api-composition.html) - AWS guidance
- [Backend for Frontend Pattern (BFF)](https://samnewman.io/patterns/architectural/bff/) - Sam Newman's original pattern
- [Building Secure & Scalable BFF Architecture (2026)](https://vishal-vishal-gupta48.medium.com/building-a-secure-scalable-bff-backend-for-frontend-architecture-with-next-js-api-routes-cbc8c101bff0) - Modern implementation
- [GraphQL Mesh Documentation](https://the-guild.dev/graphql/mesh) - Multi-source composition approach

### Observability and Performance
- [Observability at the API Gateway (2026)](https://medium.com/@seshankannan82/observability-at-the-api-gateway-tracing-metrics-and-downstream-visibility-c2c63294f3b0) - Recent best practices
- [API Gateway Caching Strategies](https://www.cloudthat.com/resources/blog/api-gateway-caching-strategies-for-high-performance-apis) - Caching approaches
- [Istio Retries and Timeouts Configuration (2026)](https://oneuptime.com/blog/post/2026-01-07-istio-retries-timeouts/view) - Resilience configuration
- [KrakenD Circuit Breaker](https://www.krakend.io/docs/backends/circuit-breaker/) - Circuit breaker implementation

### Recent Developments
- [Apollo Router Generally Available](https://www.apollographql.com/blog/announcement/backend/apollo-router-is-now-generally-available/) - GA announcement
- [AWS API Gateway Portal (Nov 2025)](https://aws.amazon.com/blogs/compute/improve-api-discoverability-with-the-new-amazon-api-gateway-portal/) - Developer portal features
- [Kong AI Gateway Features](https://konghq.com/products/kong-gateway) - AI capabilities (2026)
- [Tyk Latest Features (2026)](https://tyk.io/blog/catch-up-with-tyks-latest-features-and-capabilities/) - Recent updates

---
*Feature research for: REST API Composition Gateways*
*Researched: 2026-02-03*
*Confidence: HIGH - Multiple authoritative sources cross-referenced*
