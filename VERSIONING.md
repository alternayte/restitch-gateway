# Versioning and Compatibility Policy

## Versioning

RESTitch follows [Semantic Versioning](https://semver.org/) with tags of the
form `vMAJOR.MINOR.PATCH` on the `main` branch.

- **MAJOR**: breaking changes to the YAML config schema, the admin API, or
  the module path.
- **MINOR**: backward-compatible features and behavioral additions.
- **PATCH**: backward-compatible bug fixes and security fixes.

Before 1.0 the same rules apply: `0.x` releases may break the config schema
between minor versions, documented in the CHANGELOG.

## Compatibility commitments

For a given MAJOR:

1. **Config schema**: keys documented in `docs/configuration.md` and the
   README full schema remain valid. New keys are additive. Removals and
   renames land only in a MAJOR.
2. **Admin API**: `/admin/api/*` response shapes are additive only. New
   endpoints may appear in a MINOR.
3. **Go module path**: `github.com/alternayte/restitch-gateway` is stable
   for the 0.x and 1.x lines.
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
