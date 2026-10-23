# Studio control plane (v2)

## What it does
Studio is a multi-environment control plane. People sign in, edit upstreams, compositions and policies through forms or YAML, and publish versioned config per environment. Gateways pull the published artifact version for their environment with one URL and one gateway token, and push heartbeats and usage back. A gateway also runs standalone from one file with no Studio. One binary and one image serve every role, and a container platform needs only an image, a command, env vars and one port.

## Decisions
- An environment owns one draft and a list of artifact versions — Apollo variants and Hive targets work this way.
- Gateways pull artifacts and push telemetry; Studio never calls a gateway — this removes the admin proxy and the two-way dependency.
- A gateway token belongs to one environment, can only pull that artifact and push telemetry, is stored hashed, is shown once and is revocable — it is not an admin credential.
- The artifact holds upstreams, compositions, the auth policy, rate-limit policies, cache, retry, breaker and block_private_upstreams; process settings are env vars only — this is the Kong split between control plane and node.
- Secrets appear in the artifact only as ${VAR}; the gateway resolves them; Studio checks the name syntax and lists the names each environment needs — secrets never leave the gateway host.
- Publish and gateway load reject a non-public composition when the artifact has no auth policy — this closes the fail-open route.
- Config is stored as typed resources with names unique per environment by a database constraint — this replaces the fragment merge and its collisions.
- A resource save carries its base revision and a stale save gets 409 with a reload prompt — sluice handles concurrent edits this way.
- Publish compiles the whole draft, shows a diff and writes an immutable numbered artifact version with author and message — gateways only see validated config.
- Rollback republishes an old version as a new version, and promote copies a version into another environment's draft with a diff — history stays append-only.
- One Go validator serves gateway load, Studio save and publish, restitch check and restitch publish — the three disagreeing validators go away.
- Forms cover every field and come from a JSON Schema derived from the Go config types; the same schema drives the YAML view — forms cannot drift from the config.
- Custom widgets: a step graph for depends_on, expression fields with server lint, a JSON editor for request_schema, a ${VAR} picker; YAML comments are not kept — the database stores resources, not text.
- Sign-in uses auth-all: local users, generic OIDC, roles viewer, editor and owner, a bootstrap owner from env, API keys for CI — no user model exists today and auth-all is the house library.
- Deployed Studio uses Postgres; SQLite serves restitch dev, the quickstart and one node on local disk only — SQLite WAL fails on network file shares.
- Each gateway instance pushes a heartbeat with instance ID, binary version, applied artifact version and last pull result — Studio shows which instance runs which version.
- Gateways push one-minute aggregates per composition, step and upstream, every failed request, and a capped sample of successes with step timings and redacted headers, never bodies; retention is per environment, default 7 days — dashboards cover every replica.
- The gateway admin API and gateway-side storage go away; the data port serves /health and /ready, and the optional RESTITCH_OPS_PORT serves /metrics — container platforms probe only one port.
- A composition cannot claim /health or /ready — the probes stay reachable.
- A gateway that cannot reach Studio retries with backoff, answers /health 200 and /ready 503 until its first artifact, and keeps the last good version in memory with no disk cache — a Studio restart does not crash-loop gateways.
- Studio serves each artifact version at an immutable URL with an ETag — a CDN can front it later with no protocol change.
- One binary and one image: restitch gateway, studio, dev, check, publish and version — each container picks its role by command.
- Every flag maps to RESTITCH_ plus the flag name in upper snake case, with flag over env over default everywhere — one naming rule replaces 54 mixed inputs.
- restitch gateway takes RESTITCH_STUDIO_URL and RESTITCH_GATEWAY_TOKEN, or RESTITCH_CONFIG for standalone, never both — the two modes cannot mix.
- A standalone gateway reloads its file on change and on SIGHUP, and the file uses the artifact format — export, import and standalone share one format.
- restitch publish --env NAME FILE runs the UI publish path from CI with an API key — GitOps works without git sync.
- restitch dev runs both roles in one process on SQLite with no env vars — local work needs no setup.
- The release is v2.0.0 with module path github.com/alternayte/restitch-gateway/v2, and the loader rejects server: and admin: with the replacement env var in the error — no one depends on v1.
- The Studio API is huma v2 with a committed openapi.yaml and a hey-api TanStack Query client; the UI uses TanStack Router, Query, Table and Virtual and shadcn on Radix, with sluice patterns: DataState, confirm dialogs, toasts, light and dark themes, Cmd+K — this matches sluice.
- The deployment guide covers generic container platforms, Docker Compose and plain Kubernetes manifests; CI lints them with docker compose config and kubeconform — no vendor lock-in.

## Out
- Git sync, per-environment permissions, publish approvals, scheduled publishes, multiple organisations.
- Vendor recipes, Bicep, Helm and azd templates.
- A v1 compatibility layer or a v1 config converter.
- A gateway disk cache, and Studio dashboards for standalone gateways.
- Request and response bodies in telemetry.

## How I know it works
- restitch dev opens Studio; an upstream and a composition created through forms and published answer a curl to the gateway with the composed response.
- A second tab that saves a stale copy of the same resource gets a reload prompt.
- Publish shows a diff; rollback adds a new version; the instances view shows every gateway on the new version within one poll interval.
- Publish of a non-public composition with no auth policy fails with a message, and restitch gateway refuses the same file.
- A Studio API call with no session or API key gets 401, and a cross-site POST gets 403.
- A gateway started while Studio is down answers /health 200 and /ready 503, then /ready 200 after Studio starts and a version is published.
- restitch gateway with a file that contains server.port exits with an error that names RESTITCH_PORT.
- restitch publish exits non-zero on an invalid file, and a valid file appears as a new version in Studio.
- The dashboard shows request counts from two gateway replicas in one environment.
- CI passes docker compose config and kubeconform on the deployment files.
- A gateway with two env vars and Studio with three env vars on Postgres serve traffic on one container platform from the generic guide; the user checks this by hand.
