# Azure AKS Integration (`azure/aks`)

Atmos connects Azure AKS clusters to local Kubernetes tooling via the `azure/aks` integration and
two commands: `atmos azure aks update-kubeconfig` and `atmos azure aks token`. Unlike
`az aks get-credentials`, neither shells out to `az` nor requires the `kubelogin` binary.

## Commands

`atmos azure aks update-kubeconfig` writes kubeconfig entries for an AKS cluster — from a named
`auth.integrations` entry, or from an Atmos identity with explicit cluster details:

```shell
atmos azure aks update-kubeconfig --integration dev/aks
atmos azure aks update-kubeconfig --cluster-name dev-cluster --resource-group dev-rg --identity azure-dev
```

`atmos azure aks token` generates a Kubernetes `ExecCredential` token for kubectl. It is normally
invoked by kubectl from the generated kubeconfig, not run by humans:

```shell
atmos azure aks token --cluster-name dev-cluster --resource-group dev-rg --identity azure-dev
```

## Configuration

Configure an `azure/aks` integration under `auth.integrations`. Providers and identities are the
standard Azure Auth building blocks (see the main skill and
[providers-and-identities.md](providers-and-identities.md)):

```yaml
auth:
  providers:
    azure-device-code:
      kind: azure/device-code
      spec:
        tenant_id: 00000000-0000-0000-0000-000000000000

  identities:
    azure-dev:
      kind: azure/subscription
      via:
        provider: azure-device-code
      principal:
        subscription_id: 11111111-1111-1111-1111-111111111111

  integrations:
    dev/aks:
      kind: azure/aks
      via:
        identity: azure-dev
      spec:
        cluster:
          name: dev-cluster
          resource_group: dev-rg
          alias: dev-aks
```

`spec.cluster` is the same struct used by `aws/eks` integrations (`name`, `region` for AWS;
`name`, `resource_group`, `subscription_id` for Azure) — only the fields relevant to the
integration's `kind` matter.

## Guidance

- Prefer `--integration` when a named AKS integration exists; it centralizes cluster name,
  resource group, alias, and identity selection.
- Use `--identity` with `--cluster-name` and `--resource-group` for ad hoc kubeconfig generation
  through Atmos Auth.
- Only AAD-integrated clusters are supported (the modern default for AKS). Clusters using local
  Kubernetes accounts are rejected with a clear error — there is no fallback to static
  certificate-based auth.
- `subscription_id` is optional on `spec.cluster`; it defaults to the authenticated identity's
  subscription. Set it explicitly only when the cluster's subscription differs from the identity's.
- The default kubeconfig path is the XDG-compliant `~/.config/atmos/kube/config`; do not hard-code
  paths unless the repo already has a convention.
- Installing `kubectl` for a scripted job is out of scope here — route tool installation to the
  `atmos-toolchain` skill.
