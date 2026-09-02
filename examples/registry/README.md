# Registry mode, end to end

This example runs the full centralized-config loop: a mock upstream, a
Studio that owns the config registry, and a gateway in **registry mode** —
it polls the Studio bundle instead of reading a config file.

## Run

```bash
docker compose up -d --build
```

## Drive the loop

The registry API requires the key (hardening C1). Seed a composition:

```bash
curl -X POST http://localhost:3080/api/v1/configs \
  -H 'Content-Type: application/json' \
  -H 'X-Admin-Key: example-registry-key' \
  -d '{
    "name": "hello",
    "yaml_content": "upstreams:\n  mock:\n    url: \"http://mockupstream:8081\"\ncompositions:\n  hello:\n    path: /api/hello\n    method: GET\n    steps:\n      - name: e\n        upstream: mock\n        path: /echo\n    response:\n      body:\n        hello: \"{{ steps.e.body }}\"\n"
  }'
```

Within one poll interval (5s here) the gateway serves it:

```bash
curl http://localhost:8080/api/hello
# {"hello":{"body":"","headers":{...},"method":"GET","path":"/echo","query":{}}}
```

Confirm the gateway is polling the registry (admin key required):

```bash
curl -H 'X-Admin-Key: example-admin-key' \
  http://localhost:9090/admin/api/registry/status
# {"mode":"registry","composition_count":1,...}
```

Update the composition (PUT creates a new version), activate it, and watch
the gateway pick it up on the next poll. Kill the Studio and the gateway
keeps serving the last known-good bundle; restarting Studio resumes polling.

## Clean up

```bash
docker compose down -v
```
