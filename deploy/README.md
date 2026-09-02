# Restitch production stack

Gateway in **registry mode** against Studio, with Prometheus and Jaeger. This
is the stack that exercises the centralized-config loop (M20 + M21): Studio
owns configuration, and the gateway polls `/api/v1/registry/bundle` for it.

For a simple file-config quickstart instead, see `examples/docker-compose/`.

## Bring-up

```bash
cd deploy
cp .env.example .env      # then edit ports/keys as needed
docker compose up -d --build
```

The first build runs `npm ci` and two Go builds — expect several minutes.

| Service | Default URL |
|---------|-------------|
| Gateway | http://localhost:8080 |
| Gateway admin + `/metrics` | http://localhost:9090 |
| Studio | http://localhost:3080 |
| Prometheus | http://localhost:9091 |
| Jaeger UI | http://localhost:16686 |

**Prometheus is on 9091, not its conventional 9090** — the gateway admin
server already owns 9090 on the host.

## Seeding the registry

A fresh Studio has no compositions, so the registry bundle is empty and the
gateway starts serving nothing. Create one (the registry API requires the
key from `RESTITCH_REGISTRY_KEY` in your `.env`, hardening C1; the field is
`yaml_content`, finding M26):

```bash
curl -X POST http://localhost:3080/api/v1/configs \
  -H 'Content-Type: application/json' \
  -H 'X-Admin-Key: restitch-dev' \
  -d '{"name":"example","yaml_content":"compositions:\n  hello:\n    path: /hello\n    method: GET\n    steps: []\n"}'
```

The gateway picks it up within one poll interval (10s by default).

## Tuning the latency alert

`prometheus/rules/alerts.yml` → `RestitchHighP95Latency` → `expr:`. The `> 1`
literal is the P95 threshold in **seconds**. Prometheus rule files have no
variable substitution, so this literal is the configuration point. After
editing, re-run the unit tests:

```bash
docker run --rm -v "$PWD/prometheus:/p:ro" --entrypoint=/bin/promtool \
  prom/prometheus:$(grep -E '^PROMETHEUS_VERSION=' .env.example | cut -d= -f2) \
  test rules /p/rules/alerts_test.yml
```

## Alerting

No Alertmanager ships here on purpose — a demo receiver that discards every
notification proves nothing. `prometheus/prometheus.yml` has a commented
`alerting:` block marking where to point your own.

## Teardown

```bash
docker compose down -v     # -v also drops the Studio database and Prometheus TSDB
```
