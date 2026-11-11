# auth-secrets

Use an Atmos secret store as an authentication credential source, then select fields for an identity's environment. The package exports `secrets/store` and `secrets/env` and is published at:

```text
oci://ghcr.io/cloudposse/atmos/plugins/auth-secrets
```

```yaml
plugins:
  packages:
    secrets:
      source: oci://ghcr.io/cloudposse/atmos/plugins/auth-secrets
      version: 1.1.0

stores:
  team:
    type: aws-ssm-parameter-store
    secret: true
    options:
      region: us-east-1

auth:
  providers:
    service-secrets:
      kind: secrets/store
      spec:
        credentials:
          token:
            store: team
            path: /services/example/token
  identities:
    service:
      kind: secrets/env
      via:
        provider: service-secrets
      spec:
        env:
          SERVICE_TOKEN: '{{ .credentials.token }}'
```

Install with `atmos plugin install`, commit `.atmos.plugins.lock.yaml`, and select `service` when running an Atmos command that supports identities. The selected process environment receives `SERVICE_TOKEN`. An identity maps only explicitly named fields; unused provider fields are not forwarded.

Credential references require `store` and `path`. Supply both `stack` and `component` to use a store's scoped lookup; omit both for direct lookup. For a structured backend such as Vault KV v2, add `field` to select one exact top-level document field. The `path` is relative to the configured KV mount; do not include the KV v2 `/data/` API segment. Field selection ignores the default scalar convention, so a document may contain both `value` and the selected field. The store must be marked secret, support exact field selection when `field` is present, and return text.

The identity's `spec.env` is a nonempty map of environment variable names to Go text templates. Templates run only after upstream authentication, with that credential envelope's data available as `.credentials`. Use `{{ .credentials.token }}` for ordinary names and `{{ index .credentials "api-access-key" }}` for hyphenated names. Plain strings are literal values, not implicit credential references; template composition is supported. Missing keys, malformed templates, NUL bytes, values above 1 MiB, and invalid environment names are rejected without exposing credential values in errors. Environment names that differ only by case are rejected, and each identity is limited to 256 variables. Filesystem, process-environment, shell, and store helpers are unavailable.

Keep the structural YAML keys `auth`, `identities`, `spec`, and `env` literal. Component and identity names may use templates. Write credential templates as quoted strings or block scalars, and put template control actions around auth mappings on standalone lines. Ambiguous structural keys and malformed credential declarations are rejected before general template helpers run.

Malformed template YAML is rejected before general helpers run when Atmos cannot prove auth credential ownership. The error explains: `Auth credential ownership cannot be proven from malformed YAML.` This conservative policy can reject dynamic keys or auth-shaped application data under `vars` or `settings`, even when rendering would produce valid YAML. Keep raw YAML valid, balance template controls, and use safely quoted values or block scalars.

Output actions and formatting helpers accept scalar values; select a credential instead of rendering the whole `.credentials` map. `printf` supports scalar Go formats, indexed arguments, dynamic integer widths/precision, and pipeline composition. Formatting is checked before allocation, including cumulative expansion from repeated directives. Escape and print helpers have the same 1 MiB limit. Conservative formatting estimates can reject a heavily escaped or high-precision value near that limit. Composite values, custom formatters, malformed formats, and noninteger dynamic widths/precision are rejected.

The unreleased `principal.environment` mapping has been replaced by `spec.env`; move mappings there and reference credentials explicitly with templates. The former credential-reference `key` setting is now named `path`.

A store's explicit `identity` is respected. Without one, the store uses ambient backend credentials rather than inheriting the identity currently being constructed. Direct and indirect cycles between providers, store identities, and identity chains are rejected before access. Secret backends without cancellation support can finish an in-flight read after cancellation; no result is returned to the canceled caller.

Atmos grants logical credential names to this provider invocation only; the plugin cannot request arbitrary store paths through the API. Returned values are registered with secret masking. Backend errors and `whoami` metadata never include the secret payload. Native plugins still execute as your operating-system user: API scoping is not a sandbox for an untrusted executable.

The provider rereads stores on authentication and returns a five-minute credential envelope. The current process may reuse the identity until that envelope expires. No refresh token or plugin-private session is saved; a fresh process reads stores again. Logout clears the identity environment. Secret values are applied to child-process environments without changing Atmos's own process environment or writing cloud credential files.

See [release infrastructure](../README.md) for independent versions, public package setup, and the six-platform release matrix.
