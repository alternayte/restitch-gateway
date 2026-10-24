/**
 * SECRET_FIELDS lists the form paths that get the secret picker. The rule:
 * a field gets the picker only when it carries a credential or an endpoint
 * location, the values that differ between environments or must not sit in
 * a resource. Names, methods, upstream references, durations, numbers,
 * booleans and enums never get it. Any string can still hold ${NAME} in
 * YAML; this list narrows only the picker.
 *
 * A path starts with the resource kind and follows the property names of
 * the artifact schema (GET /api/v1/schema). "*" stands for an array index.
 * A path to a string map (headers) gives the picker to the map's values,
 * never to its keys. A path to a string array (api_keys) gives it to the
 * comma list.
 */
export const SECRET_FIELDS: ReadonlySet<string> = new Set([
  // Upstream: where it is and how the gateway signs in to it.
  "upstream.url",
  "upstream.health_path",
  "upstream.auth.header.value",
  "upstream.auth.basic.username",
  "upstream.auth.basic.password",
  "upstream.auth.oauth2.token_url",
  "upstream.auth.oauth2.client_id",
  "upstream.auth.oauth2.client_secret",
  // Composition steps: the request templates sent to the upstream.
  "composition.steps.*.path",
  "composition.steps.*.headers",
  "composition.steps.*.body",
  // Inbound auth policy: the keys and the identity provider.
  "auth.api_keys",
  "auth.jwt.jwks_url",
  "auth.jwt.issuer",
  "auth.jwt.audience",
]);

/** takesSecret reports whether the form field at path gets the secret picker. */
export function takesSecret(path: string): boolean {
  return SECRET_FIELDS.has(path.replace(/\.\d+(?=\.|$)/g, ".*"));
}
