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
      version: 1.0.0

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
            key: /services/example/token
  identities:
    service:
      kind: secrets/env
      via:
        provider: service-secrets
      principal:
        environment:
          SERVICE_TOKEN: token
```

Install with `atmos plugin install`, commit `.atmos.plugins.lock.yaml`, and select `service` when running an Atmos command that supports identities. The selected process environment receives `SERVICE_TOKEN`. An identity maps only explicitly named fields; unused provider fields are not forwarded.

Credential references require `store` and `key`. Supply both `stack` and `component` to use a store's scoped lookup; omit both for direct key lookup. The store must be marked secret, and its value must be text. Structured values, NUL bytes, values above 1 MiB, unknown names, and invalid environment variable names are rejected. Environment names that differ only by case are rejected for cross-platform consistency.

A store's explicit `identity` is respected. Without one, the store uses ambient backend credentials rather than inheriting the identity currently being constructed. Direct and indirect cycles between providers, store identities, and identity chains are rejected before access. Secret backends without cancellation support can finish an in-flight read after cancellation; no result is returned to the canceled caller.

Atmos grants logical credential names to this provider invocation only; the plugin cannot request arbitrary store paths through the API. Returned values are registered with secret masking. Backend errors and `whoami` metadata never include the secret payload. Native plugins still execute as your operating-system user: API scoping is not a sandbox for an untrusted executable.

The provider rereads stores on authentication and returns a five-minute credential envelope. The current process may reuse the identity until that envelope expires. No refresh token or plugin-private session is saved; a fresh process reads stores again. Logout clears the identity environment. Secret values are applied to child-process environments without changing Atmos's own process environment or writing cloud credential files.

See [release infrastructure](../README.md) for independent versions, public package setup, and the six-platform release matrix.
