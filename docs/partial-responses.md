# Partial Responses

When a composition includes `optional: true` steps, Restitch can return a
partial response — the successful parts of the composition plus structured
error information for any failed or skipped steps.

## When Partial Responses Happen

A partial response is returned when:

- An **optional** step fails (network error, timeout, upstream 5xx, circuit
  breaker open).
- A step is **skipped** because one of its dependencies (inferred or explicit)
  failed or was skipped.
- At least one **required** step succeeded, so the composition can still
  produce output.

If a **required** step fails, the gateway returns a hard error (502 or 504)
instead of a partial response.

## Response Headers

| Header | Value | Meaning |
|--------|-------|---------|
| `X-Restitch-Complete` | `true` | All steps succeeded |
| `X-Restitch-Complete` | `false` | One or more optional steps failed or were skipped |
| `X-Partial-Response` | `true` | Legacy alias — set whenever `X-Restitch-Complete: false` |

Clients should check `X-Restitch-Complete` to detect partial responses.

## The `_errors` Array

When the response is partial, Restitch appends an `_errors` field to the
response body:

```json
{
  "user": {"id": 1, "name": "Alice"},
  "points": null,
  "_errors": [
    {
      "step": "loyalty",
      "message": "timeout",
      "status": "failed"
    },
    {
      "step": "bonus",
      "message": "dependency_failed",
      "status": "skipped"
    }
  ]
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `step` | string | Name of the failed or skipped step |
| `message` | string | Sanitized reason (see below) |
| `status` | string | `"failed"` or `"skipped"` |

### Message Values

| Message | Meaning |
|---------|---------|
| `timeout` | Step exceeded its timeout |
| `upstream error` | Network error, upstream 5xx after retries, circuit breaker open, or auth failure |
| `dependency_failed` | Step was skipped because a dependency failed |

Messages are sanitized — internal error strings, upstream URLs, and
environment variable names are never exposed to clients. Full details are
logged server-side with the request ID.

## Nil Handling in Response Templates

When a step fails or is skipped, `steps.<name>` evaluates to `nil`.

Response template expressions handle this gracefully:

- If an expression errors **and** the error is explained by a failed/skipped
  step in its dependency set, the result is `null` (for whole-expression
  values) or `""` (for interpolated strings).
- If an expression errors with **no** failed dependency to explain it, that
  is a template bug and returns HTTP 500.

You can use explicit defaults with the nil-safe operators:

```yaml
body:
  points: "{{ steps.loyalty?.body?.points ?? 0 }}"
  name: "{{ steps.user.body.name }}"
```

## Dependency Inference

Restitch automatically infers step dependencies by scanning template
expressions for `steps.<name>` references. You do not need to declare
`depends_on` for most cases:

```yaml
steps:
  - name: user
    upstream: users-api
    path: "/users/{{ req.params.id }}"

  - name: orders
    upstream: orders-api
    path: "/orders?userId={{ steps.user.body.id }}"
    # ↑ Restitch infers depends_on: [user] from the expression
```

Inferred dependencies are merged with any explicit `depends_on` values. When
a dependency fails:

1. The dependent step is **skipped** (its upstream is never called).
2. The skipped step appears in `_errors` with `status: "skipped"` and
   `message: "dependency_failed"`.

## Example: Complete Partial Response Flow

Configuration:

```yaml
compositions:
  dashboard:
    path: "/api/dashboard"
    steps:
      - name: user
        upstream: users-api
        path: "/me"

      - name: loyalty
        upstream: loyalty-api
        path: "/points/{{ steps.user.body.id }}"
        optional: true

      - name: bonus
        upstream: bonus-api
        path: "/bonus/{{ steps.loyalty.body.tier }}"
        optional: true

    response:
      body:
        user: "{{ steps.user.body }}"
        points: "{{ steps.loyalty?.body?.points ?? 0 }}"
        bonus: "{{ steps.bonus?.body?.offer }}"
```

If `loyalty-api` is down:

- `user` succeeds → result available.
- `loyalty` fails → recorded in `_errors` with `status: "failed"`.
- `bonus` is skipped (depends on `steps.loyalty.body.tier`) → recorded in
  `_errors` with `status: "skipped"`.

Response (HTTP 200):

```json
{
  "user": {"id": 1, "name": "Alice"},
  "points": 0,
  "bonus": null,
  "_errors": [
    {"step": "loyalty", "message": "upstream error", "status": "failed"},
    {"step": "bonus", "message": "dependency_failed", "status": "skipped"}
  ]
}
```

Headers:

```
X-Restitch-Complete: false
X-Partial-Response: true
```
