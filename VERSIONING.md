# Versioning and Compatibility Policy

## Versioning

RESTitch follows [Semantic Versioning](https://semver.org/) with tags of the
form `vMAJOR.MINOR.PATCH` on the `main` branch.

- **MAJOR**: breaking changes to the artifact schema, the `RESTITCH_`
  environment variables, the Studio and gateway APIs, or the module path.
- **MINOR**: backward-compatible features and behavioral additions.
- **PATCH**: backward-compatible bug fixes and security fixes.

Before 1.0 the same rules apply: `0.x` releases may break the config schema
between minor versions, documented in the CHANGELOG.

## Compatibility commitments

For a given MAJOR:

1. **Artifact schema and env vars**: keys and `RESTITCH_` variables
   documented in `docs/configuration.md` remain valid. New keys and
   variables are additive. Removals and renames land only in a MAJOR.
2. **Studio API and gateway API**: `/api/v1/*` (described in
   `api/openapi.yaml`) and `/gateway/v1/*` response shapes are additive
   only. New endpoints may appear in a MINOR. A gateway of one MAJOR pulls
   from a Studio of the same MAJOR.
3. **Go module path**: `github.com/alternayte/restitch-gateway/v2` is stable
   for the 2.x line. The 1.x line used `github.com/alternayte/restitch-gateway`.
4. **Go and Node floors**: `go.mod` and the CI workflow declare the minimum
   toolchain; raising the floor is a MINOR+ change announced in the
   CHANGELOG.
5. **Security**: security fixes backport to the latest release of the
   previous MAJOR when one exists. See SECURITY.md.

## Releasing

1. Update `CHANGELOG.md` with a dated entry.
2. Tag `vX.Y.Z` on `main` and push the tag. CI runs the load-test gate on
   tags (`release/*` branches and tags).
3. Attach release binaries and the container image to the GitHub release.

The CHANGELOG is the single list of user-visible changes between releases.
