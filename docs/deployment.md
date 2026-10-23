# Deployment

This guide applies to every platform that runs a container image with a
command, environment variables and one port. For the Docker Compose stack
and the Kubernetes manifests, see [deploy/README.md](../deploy/README.md).

## The image

One image serves every role:

```
ghcr.io/alternayte/restitch-gateway:<version>
```

The entrypoint is `/restitch`. The image has no default command. Set the
container command (the arguments after the entrypoint) to pick the role:

| Command | Role |
|---------|------|
| `studio` | Studio: the web UI, the API and the store of environments and artifact versions. |
| `gateway` | A gateway. It serves compositions. |

The image runs as the distroless non-root user (UID 65532). It has no shell.

To build the image from source, run `make docker`. That builds
`restitch:dev`.

## Studio

### Environment variables

Studio on Postgres needs these variables:

| Variable | Value |
|----------|-------|
| `RESTITCH_DATABASE_URL` | `postgres://USER:PASSWORD@HOST:5432/DB?sslmode=require` |
| `RESTITCH_BOOTSTRAP_EMAIL` | Email of the first owner. |
| `RESTITCH_BOOTSTRAP_PASSWORD` | Password of the first owner. |
| `RESTITCH_PUBLIC_URL` | The URL that people use to reach Studio, for example `https://studio.example.com`. |

Studio creates the bootstrap owner at start when the database has no user.
When a user exists, Studio ignores the two bootstrap variables. You can
remove them after the first start.

Set `RESTITCH_PUBLIC_URL` to the external URL. Studio checks the `Origin`
header of each browser request against it. A wrong value makes sign-in fail.

Store the database URL and the bootstrap password as secrets of your
platform. Use URL-safe characters in the database password, or
percent-encode them.

For every other Studio variable, see
[configuration.md](configuration.md#environment-variables).

### Port and probes

| Item | Value |
|------|-------|
| Port | `3080` (`RESTITCH_PORT`) |
| Liveness probe | `GET /health` |
| Readiness probe | `GET /ready`. It answers 200 when the database answers. |

Studio has no TLS listener. Put TLS on the platform load balancer or the
ingress.

### Database

Use Postgres for a deployed Studio. Studio runs its migrations at start.
More than one Studio replica can share one Postgres database.

SQLite (`RESTITCH_DATABASE_URL=sqlite:/data/studio.db`) is for one Studio
replica on local disk only. Do not put the SQLite file on a network file
share such as NFS, SMB or Azure Files: SQLite WAL mode fails on them. The
image has a `/data` directory that the non-root user can write.

## Gateways

### Managed mode

A managed gateway pulls the current artifact version of one environment
from Studio. It needs two variables:

| Variable | Value |
|----------|-------|
| `RESTITCH_STUDIO_URL` | The URL of Studio that the gateway can reach, for example `http://restitch-studio:3080`. |
| `RESTITCH_GATEWAY_TOKEN` | A gateway token of the environment. It starts with `rgt_`. |

To get a gateway token:

1. Sign in to Studio as an owner.
2. Create the environment, or open it.
3. Open **Gateway tokens** and create a token.
4. Copy the token. Studio shows it once.

Store the token as a secret of your platform. A gateway token can only pull
the artifact of its environment and push telemetry. Revoke it in the same
page.

The gateway pulls every 10 seconds (`RESTITCH_POLL_INTERVAL`). It applies a
new version without a restart. It sends a heartbeat, usage aggregates and
sampled requests to Studio. Studio never connects to a gateway.

When the gateway cannot reach Studio, it retries with backoff. It keeps
serving the last version it applied. It keeps that version in memory only,
so a gateway that starts while Studio is down serves no composition until
Studio answers.

### Standalone mode

A standalone gateway reads one artifact file and needs no Studio. Set
`RESTITCH_CONFIG` to the path of the file, and mount the file into the
container. The gateway reloads the file when it changes and on SIGHUP.

Do not set `RESTITCH_CONFIG` and `RESTITCH_STUDIO_URL` together. The gateway
exits with status 2.

For a Compose example, see
[examples/docker-compose/](../examples/docker-compose/README.md).

### Secrets in the artifact

An artifact refers to a secret as `${NAME}`. The gateway expands it from its
own environment when it loads each version. Set every variable that the
artifact uses on each gateway container. A version that refers to an unset
variable fails to load, and the gateway keeps the version it had. See
[configuration.md](configuration.md#environment-variable-expansion).

### Ports and probes

| Item | Value |
|------|-------|
| Data port | `8080` (`RESTITCH_PORT`). It serves compositions, `/health` and `/ready`. |
| Liveness probe | `GET /health` on the data port. It answers 200 while the process runs. |
| Readiness probe | `GET /ready` on the data port. It answers 503 until the gateway applies its first artifact version, then 200. |
| Metrics | `GET /metrics` on `RESTITCH_OPS_PORT`. Set the variable, for example to `9090`, to turn it on. The ops port is off by default. |
| HTTPS | Set `RESTITCH_TLS_CERT` and `RESTITCH_TLS_KEY` to serve HTTPS on `RESTITCH_TLS_PORT` (`8443`). The data port stays on plain HTTP. |

A composition cannot use the path `/health` or `/ready`.

For a Docker `HEALTHCHECK` in the shell-less image, use the binary itself:

```dockerfile
HEALTHCHECK CMD ["/restitch", "healthcheck", "http://localhost:8080/health"]
```

### Shutdown

On SIGTERM or SIGINT, the gateway stops accepting requests and drains the
requests in progress. The deadline is `RESTITCH_SHUTDOWN_TIMEOUT` (`30s`).
A managed gateway then pushes its last telemetry. Set the termination grace
period of your platform to more than the deadline.

## Order of the first deployment

1. Create a Postgres database.
2. Deploy Studio with its four variables. Expose its port to the people who
   use it.
3. Sign in as the bootstrap owner. Create an environment and a gateway
   token.
4. Deploy the gateways with `RESTITCH_STUDIO_URL` and
   `RESTITCH_GATEWAY_TOKEN`. Expose the data port to your clients.
5. Add resources to the draft, or run `restitch publish` from CI. Publish.
   Each gateway applies the version within one poll interval, and `/ready`
   turns 200.

## Example: one gateway with `docker run`

```bash
docker run -d --name restitch-gateway -p 8080:8080 \
  -e RESTITCH_STUDIO_URL=https://studio.example.com \
  -e RESTITCH_GATEWAY_TOKEN=rgt_... \
  ghcr.io/alternayte/restitch-gateway:<version> gateway
```

## Example: Studio with `docker run`

```bash
docker run -d --name restitch-studio -p 3080:3080 \
  -e RESTITCH_DATABASE_URL='postgres://restitch:PASSWORD@db.example.com:5432/restitch?sslmode=require' \
  -e RESTITCH_PUBLIC_URL=https://studio.example.com \
  -e RESTITCH_BOOTSTRAP_EMAIL=owner@example.com \
  -e RESTITCH_BOOTSTRAP_PASSWORD='CHANGE-ME' \
  ghcr.io/alternayte/restitch-gateway:<version> studio
```

## Without a container

The release binaries are the same program. Run `restitch studio` and
`restitch gateway` under your process manager with the same variables.
