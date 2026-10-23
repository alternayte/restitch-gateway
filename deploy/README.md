# Deploying Restitch

One image serves every role. The container command picks the role:
`gateway` or `studio`. The image has no default command.

- `docker-compose.yml` runs Studio on Postgres, two managed gateways, a mock
  upstream and Prometheus.
- `kubernetes/` holds plain manifests for Studio and the gateways.

CI checks both with `make deploy-lint`: `docker compose config` and
`kubeconform -strict`.

For one gateway from one file, with no Studio, see `examples/docker-compose/`.

## Docker Compose

The gateways need a gateway token, and only Studio can make one. The bring-up
therefore has two steps.

### Step 1: start Studio

```bash
cd deploy
cp .env.example .env
```

Set `POSTGRES_PASSWORD`, `RESTITCH_BOOTSTRAP_EMAIL` and
`RESTITCH_BOOTSTRAP_PASSWORD` in `.env`. Use URL-safe characters in the
Postgres password, because Studio receives it inside a database URL.

```bash
docker compose up -d --build --wait postgres studio
```

Studio is at http://localhost:3080. Sign in as the bootstrap owner.

### Step 2: make a token and start the gateways

1. In Studio, create an environment, for example `production`.
2. Open the environment, then open **Gateway tokens**. Create a token.
3. Copy the token (it starts with `rgt_`). Studio shows it once.
4. Set `RESTITCH_GATEWAY_TOKEN` in `.env` to the token.
5. Start the rest of the stack:

```bash
docker compose up -d --wait
```

Without a token, each gateway exits with
`gateway: -studio-url needs -gateway-token (RESTITCH_GATEWAY_TOKEN)`, and
Compose stops restarting it after five attempts.

### Step 3: publish

Add resources to the draft and publish, or publish an artifact file with
`restitch publish` and an API key (see `docs/studio.md`). The mock upstream
is `http://mockupstream:8081`; `examples/docker-compose/restitch.yaml` is an
artifact that uses it.
Each gateway applies the new version within one poll interval (10s).

```bash
curl -s http://localhost:8080/api/users/42/dashboard   # gateway-1
curl -s http://localhost:8082/api/users/42/dashboard   # gateway-2
```

Until the first version exists, each gateway answers `/health` with 200 and
`/ready` with 503.

| Service | Default URL |
|---------|-------------|
| Studio | http://localhost:3080 |
| gateway-1 | http://localhost:8080 |
| gateway-2 | http://localhost:8082 |
| Prometheus | http://localhost:9091 |

Each gateway serves `/metrics` on `RESTITCH_OPS_PORT` (9090) inside the
Compose network. Prometheus scrapes both.

### Teardown

```bash
docker compose down -v     # -v also deletes the Postgres and Prometheus data
```

## Kubernetes

The manifests assume a Postgres database that you run, and a namespace of
your choice. They create no Secret, so no credential is in the repository.

1. Create the Studio secret:

   ```bash
   kubectl create secret generic restitch-studio \
     --from-literal=database-url='postgres://USER:PASSWORD@HOST:5432/DB?sslmode=require' \
     --from-literal=bootstrap-email='owner@example.com' \
     --from-literal=bootstrap-password='CHANGE-ME'
   ```

2. In `kubernetes/studio.yaml`, set `RESTITCH_PUBLIC_URL` to the URL people
   use to reach Studio. Set the image tag in both files to a release.
3. Apply Studio, and expose the `restitch-studio` Service with your ingress:

   ```bash
   kubectl apply -f kubernetes/studio.yaml
   ```

4. Sign in, create an environment and a gateway token, as in the Compose
   steps. Create the gateway secret:

   ```bash
   kubectl create secret generic restitch-gateway --from-literal=gateway-token='rgt_...'
   ```

5. Apply the gateways:

   ```bash
   kubectl apply -f kubernetes/gateway.yaml
   ```

Studio probes: `/health` (process up) and `/ready` (database answers).
Gateway probes: `/health` and `/ready` on the data port. `/ready` stays 503
until the gateway applies its first artifact version.

## Tuning the latency alert

`prometheus/rules/alerts.yml` → `RestitchHighP95Latency` → `expr:`. The `> 1`
literal is the P95 threshold in seconds. Prometheus rule files have no
variable substitution, so this literal is the configuration point. After you
edit it, run the rule tests:

```bash
docker run --rm -v "$PWD/prometheus:/p:ro" --entrypoint=/bin/promtool \
  prom/prometheus:$(grep -E '^PROMETHEUS_VERSION=' .env.example | cut -d= -f2) \
  test rules /p/rules/alerts_test.yml
```

No Alertmanager ships here. `prometheus/prometheus.yml` has a commented
`alerting:` block that shows where to point your own.
