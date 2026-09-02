# Docker Compose Example

Runs the full Restitch stack: mock upstream, gateway, and Studio.

## Quick Start

```bash
docker-compose up --build
```

## Services

| Service | URL | Description |
|---------|-----|-------------|
| Gateway | http://localhost:8080 | Data plane — serves compositions |
| Admin API | http://localhost:9090 | Metrics, request log, config management |
| Studio | http://localhost:3080 | Web UI |
| Mock upstream | http://localhost:8081 | Test backend |

## Try It

```bash
# Fetch a composed user dashboard
curl -s http://localhost:8080/api/users/42/dashboard | python3 -m json.tool

# Check metrics
curl -s http://localhost:9090/metrics | grep restitch_requests_total

# View recent requests (admin key required since hardening C3)
curl -s -H "X-Admin-Key: restitch-dev" http://localhost:9090/admin/api/requests \
    | python3 -m json.tool

# Open Studio
open http://localhost:3080
```

## Configuration

Edit `restitch.yaml` to modify compositions. The gateway watches the config
file and reloads automatically, or trigger a reload via Studio or the admin
API:

```bash
curl -X POST -H "X-Admin-Key: restitch-dev" http://localhost:9090/admin/api/reload
```
