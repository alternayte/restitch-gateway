# Architecture

One binary, `restitch`, has two server roles: the gateway (data plane) and
Studio (control plane). Gateways pull from Studio and push telemetry to it.
Studio never connects to a gateway.

## Request lifecycle

1. A client request reaches the data port of the gateway.
2. Middleware assigns a request ID, logs the request, and applies the
   gateway-wide rate limit when the artifact has one.
3. The router matches the path. Inbound auth runs unless the composition is
   `public: true`. The rate limit of the composition applies.
4. The executor runs the step graph of the composition wave by wave. Steps
   in a wave run in parallel. Each upstream has its own client with retry,
   circuit breaker, optional response cache and optional coalescing.
   Templated path, query, body and header values evaluate against the
   request. A step failure marks its dependents skipped.
5. The response template evaluates against the step results and the
   optional `_errors`, and the gateway writes the merged response.
6. A managed gateway adds the request to its one-minute usage aggregate and
   to the request sample. Both go to Studio in the background. The response
   never waits for them.

## Configuration sources

- **Standalone mode**: the gateway reads the artifact file that
  `RESTITCH_CONFIG` names. It reloads the file when it changes and on SIGHUP.
- **Managed mode**: the gateway pulls the current artifact version of its
  environment from `GET /gateway/v1/artifact` on Studio, with its gateway
  token. An unchanged version costs a 304 through the ETag. A failed pull
  retries with backoff and keeps the last good version in memory.
  `/ready` answers 503 until the first version is applied.

Both modes validate an artifact before they apply it, and swap it
atomically. The two modes cannot be combined.

Inside Studio, each environment owns one draft of typed resources and a list
of immutable artifact versions. Publish validates the whole draft with the
same validator as the gateway and writes the next version.

## Trust boundaries

- The **Studio API** (`/api/v1`) requires a session cookie or an API key. A
  request with neither gets 401. A cookie request from another origin gets
  403. Each operation needs the role viewer, editor or owner.
- The **gateway API** (`/gateway/v1`) requires a gateway token. A token
  belongs to one environment and can only pull its artifact and push
  telemetry. Studio stores only the hash of the token.
- The **gateway** has no admin API. The data port serves compositions,
  `/health` and `/ready`. The optional ops port serves `/metrics` only.
- **Secrets** appear in the artifact only as `${VAR}`. The gateway expands
  them from its own environment, so the secret values never reach Studio.
  An artifact is as trusted as the environment of the gateway host.
- **Telemetry** never holds a request or response body. Sampled requests
  carry redacted headers.

## Design notes

- Expressions compile once when an artifact loads, never at request time.
- All upstream and request bodies are read through `io.LimitReader`; JSON
  schema validation guards composition request bodies.
- The per-step cache and coalesce key is derived from the outgoing upstream
  request (method, escaped URL, headers, body, inbound `Authorization`);
  responses with `Set-Cookie` or a private `Cache-Control` are never cached
  and never shared between coalesced callers.
- The pipeline swap uses an `atomic.Pointer`; reload never blocks requests.
