# Contributing to Restitch

Thanks for considering a contribution. This document describes the workflow,
the quality bar, and the rules the maintainers apply.

## Reporting issues

Use the GitHub issue templates. A good bug report includes:

- the version (`restitch version`) and how the gateway was run (config file
  or registry mode);
- the composition YAML involved;
- the exact request that failed and the response;
- logs at `-log-level debug`.

## Development setup

Requirements: Go 1.25.7, Node.js 24, Docker (for container work).

```bash
make build        # gateway binary
make build-all    # gateway + studio binaries
make ci           # vet + lint + race
make e2e          # golden end-to-end specs
cd studio && npm ci && npm run test
```

## Making a change

1. Open an issue or PR first for anything larger than a typo fix.
2. Follow the existing conventions: the codebase is strict about
   constant-time key comparisons, bounded I/O, parameterized SQL, and error
   wrapping. Do not weaken a test to make a check pass.
3. Run `gofmt -l .` (must print nothing), `go vet ./...`, and
   `go test -race ./...` before pushing.
4. If your change touches the admin API, Studio, or the trust boundary,
   update the docs in `docs/` and the README.

## Verification gates

The repo carries milestone verification gates under `scripts/gates/`,
driven by `make verify GATE=M<n>` and `make verify-all`. They encode the
acceptance criteria for each feature area. A gate change is a special event:

- gate scripts record the repo's proof-of-work ledger; a gate must be able
  to FAIL when its assertion fails. Vacuous passes are treated as defects.
- Changing a gate, `scripts/verify.sh`, or `scripts/check-ledger.sh`
  requires maintainer approval and a dedicated commit whose message starts
  with `gate:`.

## Commit messages

Conventional style with a scope: `feat(M<n>): ...`, `fix: ...`,
`test: ...`, `docs: ...`, `gate: ...` for verification infrastructure.

## License

By contributing you agree that your work is licensed under Apache-2.0, the
project license.
