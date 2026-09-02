# Deployment

## Docker

### Building the Image

The multi-stage Dockerfile builds both binaries (gateway and Studio):

```bash
docker build -t restitch:latest .
docker build --build-arg VERSION=v1.2.0 -t restitch:v1.2.0 .
```

Build stages:

1. **Node 20** — builds the Studio React app.
2. **Go 1.25** — compiles both binaries with the Studio dist embedded.
3. **Distroless** — final minimal image with both binaries.

### Exposed Ports

| Port | Service |
|------|---------|
| 8080 | Gateway HTTP |
| 8443 | Gateway HTTPS (when TLS configured) |
| 9090 | Admin API + Prometheus metrics |
| 3080 | Restitch Studio |

### Running

```bash
# Gateway only
docker run -p 8080:8080 -p 9090:9090 \
  -v $(pwd)/restitch.yaml:/etc/restitch/restitch.yaml \
  restitch:latest run -config /etc/restitch/restitch.yaml

# Studio
# The image ENTRYPOINT is /restitch, so the studio binary is selected with
# --entrypoint; running `docker run restitch:latest /restitch-studio` would
# start the gateway with an unknown command (finding M27).
docker run -p 3080:3080 \
  -e STUDIO_GATEWAY_ADMIN_URL=http://restitch:9090 \
  --entrypoint /restitch-studio \
  restitch:latest -port=3080
```

## Docker Compose

A complete demo setup is available in `examples/docker-compose/`:

```bash
cd examples/docker-compose
docker-compose up --build
```

This starts the mock upstream, gateway, and Studio. See
[examples/docker-compose/README.md](../examples/docker-compose/README.md) for
details.

## Binary Deployment

### Building

```bash
make build-all
# produces: bin/restitch, bin/restitch-studio
```

With version stamping:

```bash
go build -ldflags "-s -w -X main.version=$(git describe --tags --always)" \
  -o bin/restitch ./cmd/restitch
```

### systemd Unit

```ini
[Unit]
Description=Restitch API Gateway
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/restitch run -config /etc/restitch/restitch.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
User=restitch
Group=restitch

[Install]
WantedBy=multi-user.target
```

For Studio:

```ini
[Unit]
Description=Restitch Studio
After=restitch.service

[Service]
Type=simple
ExecStart=/usr/local/bin/restitch-studio
Environment=STUDIO_GATEWAY_ADMIN_URL=http://127.0.0.1:9090
Restart=on-failure
User=restitch
Group=restitch

[Install]
WantedBy=multi-user.target
```

## Environment Variables

### Gateway (`RESTITCH_*`)

These override `server.*` and `admin.*` YAML fields without editing the
config file:

| Variable | YAML field | Default |
|----------|------------|---------|
| `RESTITCH_CONFIG` | Config file path | `restitch.yaml` |
| `RESTITCH_PORT` | `server.port` | `8080` |
| `RESTITCH_TLS_PORT` | `server.tls_port` | `8443` |
| `RESTITCH_TLS_CERT` | `server.tls_cert` | (none) |
| `RESTITCH_TLS_KEY` | `server.tls_key` | (none) |
| `RESTITCH_LOG_FORMAT` | `server.log_format` | `json` |
| `RESTITCH_LOG_LEVEL` | `server.log_level` | `info` |
| `RESTITCH_ADMIN_PORT` | `admin.port` | `9090` |
| `RESTITCH_ADMIN_BIND` | `admin.bind` | `127.0.0.1` |
| `RESTITCH_ADMIN_ENABLED` | `admin.enabled` | `true` |
| `RESTITCH_ADMIN_API_KEY` | `admin.api_key` | (none; required — requests without a key are rejected) |
| `RESTITCH_REGISTRY_URL` | `-registry-url` | (none) — enables registry mode |
| `RESTITCH_REGISTRY_KEY` | `-registry-key` | (none) — key sent to the Studio registry API |
| `RESTITCH_POLL_INTERVAL` | `-poll-interval` | `10s` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OpenTelemetry collector URL | (none) |

Precedence: CLI flags > `RESTITCH_*` env > YAML file > defaults.

`OTEL_EXPORTER_OTLP_ENDPOINT` is a standard OpenTelemetry variable, not a
`RESTITCH_*` override. When set, the gateway exports traces via OTLP HTTP
to the specified endpoint.

### Studio

| Variable | Flag | Default |
|----------|------|---------|
| `STUDIO_PORT` | `-port` | `3080` |
| `STUDIO_BIND` | `-bind` | `127.0.0.1` |
| `STUDIO_GATEWAY_ADMIN_URL` | `-gateway-admin-url` | `http://localhost:9090` |
| `STUDIO_ADMIN_KEY` | `-admin-key` | (none) |
| `STUDIO_REGISTRY_KEY` | `-registry-key` | (none) |

The Studio binds loopback by default. Set `STUDIO_BIND` only for deliberate
remote access. The registry API requires `STUDIO_REGISTRY_KEY`; give the
gateway the same value via `-registry-key`.

### Config file `${VAR}` expansion

All string values in the YAML config support `${VAR}` and `${VAR:default}`
expansion. Use `$$` for a literal `$`. See
[Configuration](configuration.md#environment-variable-expansion).

## TLS

TLS is enabled when both `tls_cert` and `tls_key` are set:

```yaml
server:
  tls_port: 8443
  tls_cert: /etc/restitch/tls/cert.pem
  tls_key: /etc/restitch/tls/key.pem
```

Or via environment:

```bash
RESTITCH_TLS_CERT=/path/to/cert.pem \
RESTITCH_TLS_KEY=/path/to/key.pem \
restitch run -config restitch.yaml
```

The gateway serves both plain HTTP (port 8080) and HTTPS (port 8443)
simultaneously when TLS is configured.

## Health Checks

The data port serves two health endpoints:

| Endpoint | Purpose | Response |
|----------|---------|----------|
| `GET /health` | Liveness check | `{"status":"ok"}` — always 200 |
| `GET /ready` | Readiness check | 200 when ready to serve, 503 during startup/shutdown |

Use `/ready` for load balancer health checks and Kubernetes readiness probes.
Use `/health` for liveness probes.

The admin port also serves `GET /health` for admin liveness.

### Docker health check

```dockerfile
HEALTHCHECK --interval=10s --timeout=3s \
  CMD wget -qO- http://localhost:8080/ready || exit 1
```

### Kubernetes probes

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
```

## Admin API Security

The admin API (default port 9090) is separate from the data plane. When
`admin.api_key` is set, every request must include `X-Admin-Key`:

```yaml
admin:
  port: 9090
  api_key: "${ADMIN_KEY}"
```

When `admin.api_key` is set, CORS is restricted to the requesting origin
(the `Origin` header value) rather than `*`. This prevents open cross-origin
access to the admin API in production.

In production, do not expose the admin port externally. Use network policies
or firewall rules to restrict access to internal traffic.

## Hot Reload

Restitch supports hot config reload with zero downtime. A new config is
validated and compiled before swapping — a bad config never takes down a
running gateway.

### Triggers

1. **SIGHUP** — `kill -HUP <pid>`
2. **File watch** — automatic on config file changes (debounced 500ms)
3. **Admin API** — `POST /admin/api/reload`

### Behavior

1. Load and validate the new config file.
2. Compile the full pipeline (expressions, DAG, upstream clients).
3. If validation or compilation fails → log the error, keep the old config.
4. If the config hash is identical → skip (log "config unchanged").
5. On success → atomic swap of the request handler. In-flight requests
   complete on the old pipeline.
