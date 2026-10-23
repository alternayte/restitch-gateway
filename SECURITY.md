# Security Policy

## Supported versions

Only the current release and the tip of `main` receive security fixes.
There are no LTS branches yet.

## Trust model

RESTitch is a gateway: it sits on the trust boundary between clients and
upstream services, and its control plane manages the routing of secrets.
The v2 model:

- **The gateway has no admin API.** The data port serves compositions,
  `/health` and `/ready`. The optional ops port (`RESTITCH_OPS_PORT`) serves
  `/metrics` only.
- **Studio API** (`/api/v1/*`): requires a session cookie or an API key. A
  request with neither gets 401. A cookie request from another origin gets
  403. Each operation requires the role viewer, editor or owner. Studio has
  no public sign-up.
- **Gateway API** (`/gateway/v1/*`): requires a gateway token. A token
  belongs to one environment, can only pull that environment's artifact and
  push telemetry, is stored hashed, is shown once and is revocable.
- **Studio never calls a gateway.** Gateways pull artifacts and push
  telemetry.
- **Inbound authentication** for the data plane: a composition that is not
  `public: true` requires the artifact's `auth` policy (API key or JWT).
  Publish and gateway load refuse an artifact that has such a composition
  and no `auth` policy.
- **Telemetry** carries no request or response bodies. Sampled requests
  carry redacted headers.

What this means operationally:

- Serve Studio over HTTPS and set `RESTITCH_PUBLIC_URL` to its `https://`
  URL, so the session cookie is `Secure`.
- Treat a gateway token and an API key as secrets. Revoke them when they
  leak.
- An artifact may contain `${VAR}` references that the gateway expands from
  its own environment. The values never reach Studio. Treat everyone who
  can publish to an environment as trusted with that environment's
  gateway environment variables.

## Reporting a vulnerability

Do not open a public issue for a vulnerability. Email the maintainers at
the address shown on the repository profile, or open a private security
advisory via GitHub ("Report a vulnerability" on the repository page).

Please include: affected version, a minimal reproduction (config, request),
impact, and a suggested fix if you have one. We aim to acknowledge within
three business days and to ship a fix as a release.
