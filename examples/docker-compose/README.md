# Standalone gateway with Docker Compose

Runs one gateway in standalone mode from `restitch.yaml`, with a mock upstream.
There is no Studio. For Studio on Postgres with managed gateways, see `deploy/`.

## Run

```bash
docker compose up --build
```

| Service | URL |
|---------|-----|
| Gateway | http://localhost:8080 |
| Mock upstream | http://mockupstream:8081, inside the Compose network only |

## Try it

```bash
curl -s http://localhost:8080/api/users/42/dashboard
curl -s http://localhost:8080/api/echo
curl -s http://localhost:8080/ready
```

## Change the config

Edit `restitch.yaml`. The gateway reloads the file when it changes. Some editors
replace the file instead of writing to it, and a single-file bind mount then
keeps the old content. In that case, restart the gateway:

```bash
docker compose restart gateway
```
