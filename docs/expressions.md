# Expression Language Reference

Restitch uses [expr-lang](https://expr-lang.org/) inside `{{ }}` delimiters to
wire request data and step results into steps and response templates. All
expressions are compiled at startup — syntax errors fail fast, not at request
time.

## Available Variables

Every `{{ }}` expression has access to:

| Variable | Type | Description |
|----------|------|-------------|
| `req.method` | string | Incoming HTTP method |
| `req.path` | string | Incoming URL path |
| `req.params` | map[string]string | Path parameters from route pattern (`{id}` → `req.params.id`) |
| `req.query` | map[string]string | First query value per key |
| `req.query_all` | map[string][]string | All query values per key |
| `req.headers` | map[string]string | Canonical-cased keys, first value per key |
| `req.body` | any | Parsed JSON body (`nil` if absent or not JSON) |
| `req.auth` | map | JWT claims (`nil` if no JWT auth) |
| `request` | same as `req` | Alias for `req` |
| `steps.<name>.status` | int | Upstream HTTP status code |
| `steps.<name>.headers` | map[string]string | Upstream response headers |
| `steps.<name>.body` | any | Parsed JSON body, or raw string if non-JSON |
| `steps.<name>` | nil | When step failed or was skipped |

## Examples

### Path parameters

```yaml
path: "/users/{{ req.params.id }}"
```

### Request headers

```yaml
headers:
  X-Tenant: "{{ req.headers['X-Tenant'] }}"
```

### Query parameters

```yaml
path: "/search?q={{ req.query.q }}"
```

### Step result fields

```yaml
body:
  name: "{{ steps.user.body.name }}"
  status_code: "{{ steps.user.status }}"
  server: "{{ steps.user.headers['X-Server'] }}"
```

### Ternary conditional

```yaml
body:
  label: "{{ steps.user.body.active ? 'active' : 'inactive' }}"
```

### Nil-safe access (optional chaining + null coalescing)

```yaml
body:
  points: "{{ steps.loyalty?.body?.points ?? 0 }}"
  tier: "{{ steps.loyalty?.body?.tier ?? 'none' }}"
```

### String concatenation

```yaml
body:
  greeting: "{{ 'Hello ' + steps.user.body.name }}"
```

### Array access

```yaml
body:
  first_order: "{{ steps.orders.body[0] }}"
```

### Map access

```yaml
body:
  email: "{{ steps.profile.body['email'] }}"
```

### Arithmetic

```yaml
body:
  total: "{{ steps.cart.body.subtotal + steps.cart.body.tax }}"
```

### Logical operators

```yaml
body:
  is_premium: "{{ steps.user.body.tier == 'gold' || steps.user.body.tier == 'platinum' }}"
```

### Request body forwarding (POST/PUT)

```yaml
steps:
  - name: create
    upstream: api
    path: "/items"
    method: POST
    body: "{{ req.body }}"
```

### Multi-value query parameters

```yaml
path: "/search?tags={{ req.query_all['tags'] }}"
```

### JWT claims

```yaml
path: "/users/{{ req.auth.sub }}/profile"
```

### Multiple path parameters

```yaml
path: "/api/v2/{{ req.params.resource }}/{{ req.params.id }}"
```

### Comparison

```yaml
body:
  needs_review: "{{ steps.order.body.total > 1000 }}"
```

## Nil and Failure Semantics

When a step fails or is skipped, its entry in `steps` is `nil`. The rules:

1. **Dependency skipping** — dependents of a failed or skipped step are
   themselves skipped (they never execute).

2. **Response template evaluation** — if an expression errors *and* it
   references a failed/skipped step, the result is `null` (for a whole-string
   expression like `"{{ steps.x.body }}"`) or `""` (inside string
   interpolation). This prevents optional-step failures from causing 500s.

3. **Real template bugs** — if an expression errors and *no* failed/skipped
   step explains the error, it is a genuine template bug and produces HTTP 500.

4. **Explicit defaults** — use `?.` (optional chaining) and `??` (null
   coalescing) for fine-grained control over missing data:

   ```yaml
   body:
     points: "{{ steps.loyalty?.body?.points ?? 0 }}"
   ```

## Unknown Step Name Warnings

At config load time, the gateway warns if a template references an unknown
step name (e.g., `steps.usre.body` when no step named `usre` exists). This
helps catch typos before they cause nil results at runtime.

## Escaping

- **Path escaping** — values interpolated into step `path` fields are
  automatically URL-encoded. You do not need to escape path or query values
  manually.

- **JSON body escaping** — values interpolated into JSON response templates
  are serialized correctly (strings are quoted, objects/arrays are nested).

- **Literal `$`** — config strings support `${VAR}` environment expansion.
  Use `$$` to produce a literal `$` character. See
  [Configuration](configuration.md) for details.
