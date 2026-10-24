# Restitch Studio

Studio is the control plane of Restitch. People sign in, edit the config of
each environment, and publish numbered artifact versions. Managed gateways
pull the current version of their environment and send telemetry back.
Studio never connects to a gateway.

Studio is the `restitch studio` command of the one binary. The binary
embeds the web UI. To run Studio in a container, see
[deployment.md](deployment.md).

## Concepts

| Word | Meaning |
|------|---------|
| environment | A named deployment target, for example `staging` or `production`. It owns one draft and a list of artifact versions. |
| draft | The editable set of resources of one environment. Gateways never see the draft. |
| resource | One typed config item in a draft: an upstream, a composition, the auth policy, the rate limit, or the settings. |
| publish | Validate the whole draft and write a new artifact version. |
| artifact version | An immutable, numbered, validated copy of the config of an environment. It records the author and a message. |
| promote | Copy an artifact version of one environment into the draft of another environment. |
| gateway token | A credential of one environment. A gateway uses it to pull artifact versions and push telemetry. |

## Environments

The **Environments** page lists every environment. An owner creates an
environment with **New environment**. A new environment has an empty draft
and no version.

Each environment has these pages:

- **Overview**: usage charts, gateway instances and sampled requests.
- **Draft**: the resources of the draft.
- **Publish and history**: publish, the version list, rollback and promote.
- **Gateway tokens**: owners only.

## Drafts and resources

The **Draft** page lists the resources of the draft with their revision and
the time of the last change. To add a resource, click **New resource**,
select the kind and enter the name.

| Kind | Artifact key | Name |
|------|--------------|------|
| Upstream | `upstreams.<name>` | Your choice. |
| Composition | `compositions.<name>` | Your choice. |
| Auth policy | `auth` | Always `default`. One per draft. |
| Rate limit | `rate_limit` | Always `default`. One per draft. |
| Settings | `block_private_upstreams` | Always `default`. One per draft. |

A resource name starts with a letter or a digit and holds up to 128
letters, digits, `_`, `.` and `-`. A name is unique per kind in an
environment.

The resource editor has two views of the same data: a form and YAML. The
form covers every field. Studio derives the form from the JSON Schema of the
artifact types, which `GET /api/v1/schema` also serves. Studio stores the
resource, not the text, so it does not keep YAML comments.

Studio validates each resource when you save it. Expression fields are
checked by the server as you type.

### Concurrent edits

Each save carries the revision that you opened. When another person saved
the resource after you opened it, Studio refuses your save with 409 and
shows a reload prompt. Reload to see their version. Your edits are
discarded.

### Secrets

Do not put a secret in a resource. Write `${NAME}` in its place. The gateway
expands the variable from its own environment. Studio validates the draft
with every variable replaced by `x`, and never sees the value. See
[configuration.md](configuration.md#environment-variable-expansion).

A field that holds a credential or an endpoint location has a key button on
its right. Click it to insert `${NAME}` at the cursor. The list shows the
names that the draft already uses. Type a new name to add it. A name uses
letters, digits and `_`, and does not start with a digit. Studio refuses a
save or a publish with any other name.

These fields have the key button:

- upstream: `url`, `health_path`, `auth.header.value`, `auth.basic.username`,
  `auth.basic.password`, `auth.oauth2.token_url`, `auth.oauth2.client_id`,
  `auth.oauth2.client_secret`.
- composition steps: `path`, `body`, and the value of each header.
- auth policy: `api_keys`, `jwt.jwks_url`, `jwt.issuer`, `jwt.audience`.

Names, methods, upstream references, durations, numbers and switches have no
key button. You can still type `${NAME}` into any text field, or into the
YAML tab. Studio validates and lists it the same way.

The **Draft** page lists every name that the draft uses, with the resources
that use it. Set each name as an environment variable on every gateway of
the environment. A name that every reference gives a default
(`${NAME:default}`) shows **has default**. Click **View** on a version in
**Publish and history** to see the names that the version needs.

## Publish

On **Publish and history**, click **Publish**. Studio compiles the whole
draft with the same validator that the gateway uses. Then it shows:

- the errors, when the draft does not validate, or
- the diff from the current version to the draft.

Enter a message and publish. Studio writes the next version number with
your email as the author. Each managed gateway of the environment applies
the version within one poll interval.

A composition that is not `public: true` needs an auth policy. Publish
refuses the draft without one.

### Versions and rollback

The history lists every version with its message, author and time. The
newest version is the current version. Versions never change and are never
deleted.

To roll back, click the rollback action of an old version. Studio publishes
that version's YAML again as a new version. The draft does not change.

### Promote

To promote, open the environment that receives the config and click
**Promote from another environment**. Select the source environment and the
version. Studio replaces the draft of this environment with the resources
of that version and shows the diff. Publish to apply it.

## Gateway tokens

Owners manage gateway tokens on the **Gateway tokens** page of an
environment.

1. Click the create action and enter a name for the token.
2. Copy the token. It starts with `rgt_`. Studio shows it once and stores
   only its hash.
3. Set `RESTITCH_STUDIO_URL` and `RESTITCH_GATEWAY_TOKEN` on the gateway.

The list shows when each token was last used. Revoke a token to stop every
gateway that uses it. A gateway token can only pull the artifact of its
environment and push telemetry. It cannot call the Studio API.

## Overview and telemetry

Each managed gateway sends to Studio:

- a heartbeat every 15 seconds, with its instance ID, binary version, the
  artifact version it applied and the result of its last pull,
- one-minute usage aggregates per composition, step and upstream,
- every failed request, and a capped sample of successful requests. A sample
  holds step timings and redacted headers. It never holds a body.

The **Overview** page sums the data of every instance. It shows requests per
minute, p95 latency, the error rate, per-upstream counts, the instances with
the version each one runs, and the sampled requests. An instance that has
has sent no heartbeat for one minute is marked stale.

Studio keeps usage for 7 days and 10 successful samples per minute by
default. An owner changes both per environment with
`PATCH /api/v1/environments/{env}`, fields `usage_retention_days` (1 to 365)
and `request_samples_per_minute` (0 to 1000). Studio always keeps failed
requests.

## Users and roles

| Role | Can |
|------|-----|
| viewer | Read environments, drafts, versions and telemetry. |
| editor | Everything a viewer can, and change drafts, publish, roll back, promote and import. |
| owner | Everything an editor can, and create environments, manage gateway tokens, manage users and change telemetry settings. |

The first owner comes from `RESTITCH_BOOTSTRAP_EMAIL` and
`RESTITCH_BOOTSTRAP_PASSWORD`. Studio has no public sign-up.

Owners manage users on **Settings > Users**:

- **Add user**: enter the email and the role. Studio generates a temporary
  password. The user changes it at the first sign-in.
- Change the role of a user.
- Reset the password of a user.
- Disable a user. Every session and API key of that user stops working at
  once. You can enable the user again.

To use single sign-on, set `RESTITCH_OIDC_ISSUER`, `RESTITCH_OIDC_CLIENT_ID`
and `RESTITCH_OIDC_CLIENT_SECRET`. A person who signs in through OIDC needs
no invite and gets the viewer role.

## API keys and CI

An API key lets a script call the Studio API. A key starts with `rak_`. Its
role is the role of its owner, or a lower role that you request. Create the
key as the person who owns it. Studio has no page for API keys, so use the
API:

```bash
STUDIO=https://studio.example.com
curl -s -c jar -H "Origin: $STUDIO" -H 'Content-Type: application/json' \
  -d '{"email":"ci@example.com","password":"..."}' "$STUDIO/api/auth/sign-in/email"
curl -s -b jar -H "Origin: $STUDIO" -H 'Content-Type: application/json' \
  -d '{"name":"ci","role":"editor"}' "$STUDIO/api/auth/api-keys"
```

The response holds the key in `plaintext`. Studio shows it once. Send the
key as `Authorization: Bearer rak_...`. Revoke a key with
`POST /api/auth/api-keys/{id}/revoke`.

### restitch publish

`restitch publish` replaces the draft of an environment with a file and
publishes it. It takes the same path as the UI. Run it from CI with an API
key of an editor or an owner:

```bash
export RESTITCH_STUDIO_URL=https://studio.example.com
export RESTITCH_API_KEY=rak_...
restitch publish -env production -m "Release 42" restitch.yaml
```

The command prints the diff and then `Published production version N.`. It
exits with status 1 when the file does not validate, and the draft does not
change. It exits with status 2 when a setting is missing.

## Import and export

The artifact format is the same in three places: export, import and the
file of a standalone gateway.

| Operation | API | Role |
|-----------|-----|------|
| Export the draft as YAML | `GET /api/v1/environments/{env}/export` | viewer |
| Read the YAML of a version | `GET /api/v1/environments/{env}/versions/{number}` | viewer |
| Replace the draft with a YAML file | `POST /api/v1/environments/{env}/import`, body `{"yaml": "..."}` | editor |

Import validates the whole file first. Studio refuses an invalid file and
the draft does not change. Import returns the diff from the old draft.
`restitch publish` is import followed by publish.

## The Studio API

The API is under `/api/v1`. `api/openapi.yaml` in the repository describes
every operation. A request needs a session cookie or an API key. A request
with neither gets 401. A browser request from another origin gets 403.

Gateways use a separate API under `/gateway/v1` with a gateway token.
Studio serves each artifact version with an ETag, so an unchanged version
costs a 304.

## restitch dev

`restitch dev` runs Studio and one managed gateway in one process. It needs
no environment variables and no database server.

```bash
restitch dev
```

| Item | Value |
|------|-------|
| Studio | http://localhost:3080, bound to `127.0.0.1` |
| Sign-in | `dev@restitch.local` / `restitch-dev-password` |
| Environment | `dev` |
| Gateway | http://localhost:8080 |
| Database | SQLite in `.restitch/studio.db` |

Edit resources in the `dev` environment and publish. The gateway pulls every
second. Change the ports with `-port` and `-studio-port`, and the database
directory with `-data-dir`.

## Develop the UI

The UI source is in `studio/`. It uses Vite, React, TanStack Router and
Query, and a client generated from `api/openapi.yaml`.

```bash
make studio            # npm ci and npm run build into internal/studio/dist
cd studio && npm run dev
cd studio && npm run test
```

The Vite dev server sends API calls to the Studio at `127.0.0.1:3080`. Run
`restitch dev` beside it. Set `VITE_API_TARGET` to use another Studio.

`go build ./...` fails until `make studio` produced `internal/studio/dist`,
because the binary embeds that directory.
