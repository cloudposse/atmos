# Native Helm Component Example

A minimal, credential-free example of a native Helm component. The chart lives in
`components/helm/demo` and is configured by `stacks/deploy/dev.yaml`.

## Render (no cluster, no credentials)

```shell
atmos helm template demo -s dev
```

## Diff

`atmos helm diff` (alias `plan`) shows a real unified diff (via the embedded
[helm-diff](https://github.com/databus23/helm-diff) library — no plugin to install).
Secret values are redacted.

```shell
# Offline: capture a baseline render, change a value, then diff against the baseline.
atmos helm template demo -s dev --output=baseline.yaml
# (edit stacks/deploy/dev.yaml, e.g. set replicaCount: 3)
atmos helm diff demo -s dev --from-manifest=baseline.yaml

# Against the deployed release (requires a reachable cluster):
atmos helm diff demo -s dev

# Against a GitOps deployment repository configured under `provision`:
atmos helm diff demo -s dev --against=target
```

## Apply

```shell
# Install/upgrade the release on the current kubecontext.
atmos helm apply demo -s dev
```

See the [`atmos helm`](https://atmos.tools/cli/commands/helm/usage) docs for the full
command and flag reference.
