# Security Policy

## Supported versions

Only the current release and the tip of `main` receive security fixes.
There are no LTS branches yet.

## Trust model

RESTitch is a gateway: it sits on the trust boundary between clients and
upstream services, and its control plane manages the routing of secrets.
The model, enforced by the code since the 2026-09 hardening pass:

- **Gateway admin API** (`admin.*`): requires `admin.api_key`. With no key
  configured, every request is rejected. Binds `127.0.0.1` by default
  (`admin.bind`); `GET /metrics` and `GET /health` are the only open
  endpoints. `OPTIONS` preflights require the key too, which prevents CSRF
  against `POST /admin/api/reload`.
- **Studio registry API** (`/api/v1/configs*`, `/api/v1/registry/bundle`):
  requires `X-Admin-Key` matching the Studio's `-registry-key`. With no key
  configured, every request is rejected. Cross-origin requests are rejected.
  The Studio binds `127.0.0.1` by default (`-bind`).
- **Gateway-to-Studio polling**: the gateway sends its `-registry-key` as
  `X-Admin-Key`; operator must set the same key on both sides.
- **Browser preferences API** (`/api/v1/preferences`): cookie-bound only
  (256-bit random id, `HttpOnly`, `SameSite=Strict`), stores no secrets.
- **Inbound authentication** for the data plane is opt-in per composition
  (`public: false` requires an API key or JWT).
- **Studio never exposes the admin key to the browser**: it is attached
  server-side by the proxy, and gateway CORS headers are stripped at the
  Studio boundary.

What this means operationally:

- Never run the admin or registry APIs without a key.
- Never bind the Studio or the admin API to a non-loopback address on an
  untrusted network.
- The registry bundle may contain `${VAR}` references that the gateway
  expands from its own environment; treat the registry as trusted, exactly
  as you treat the config file.

## Reporting a vulnerability

Do not open a public issue for a vulnerability. Email the maintainers at
the address shown on the repository profile, or open a private security
advisory via GitHub ("Report a vulnerability" on the repository page).

Please include: affected version, a minimal reproduction (config, request),
impact, and a suggested fix if you have one. We aim to acknowledge within
three business days and to ship a fix as a release.
