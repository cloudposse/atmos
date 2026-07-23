---
name: atmos-azure-aks
description: "Azure AKS commands in Atmos: atmos azure aks update-kubeconfig, atmos azure aks token, kubeconfig generation, kubectl exec credentials, AKS auth integrations"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Azure AKS

Use this skill for Atmos commands that connect Azure AKS clusters to local Kubernetes tooling.
It owns `atmos azure aks update-kubeconfig` and `atmos azure aks token`.

## Command Model

`atmos azure aks update-kubeconfig` writes kubeconfig entries for an AKS cluster. It can run from
a named `auth.integrations` entry, or an Atmos identity with explicit cluster details. Unlike
`az aks get-credentials`, it never shells out to `az` or requires the `kubelogin` binary.

```shell
atmos azure aks update-kubeconfig --integration dev/aks
atmos azure aks update-kubeconfig --cluster-name dev-cluster --resource-group dev-rg --identity azure-dev
```

`atmos azure aks token` generates a Kubernetes `ExecCredential` token for kubectl. It is normally
called by kubectl from generated kubeconfig rather than run by humans.

```shell
atmos azure aks token --cluster-name dev-cluster --resource-group dev-rg --identity azure-dev
```

## Configuration

For integration mode, configure an `azure/aks` integration in `auth.integrations`. Route provider,
identity, device-code, OIDC, and Azure CLI details to `atmos-auth`.

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

## Agent Guidance

- Prefer `--integration` when a named AKS integration exists; it centralizes cluster name,
  resource group, alias, and identity selection.
- Use `--identity` with `--cluster-name` and `--resource-group` for ad hoc kubeconfig generation
  through Atmos Auth.
- Only AAD-integrated clusters are supported (the modern default for AKS). Clusters using local
  Kubernetes accounts are rejected with a clear error — there is no fallback to static
  certificate-based auth.
- `subscription_id` is optional on `spec.cluster`; it defaults to the authenticated identity's
  subscription. Set it explicitly only when the cluster's subscription differs from the identity's.
- Do not hard-code kubeconfig paths unless the repo already has a convention; the default is the
  XDG-compliant path (`~/.config/atmos/kube/config`).
- If `kubectl` must be installed for a scripted job, route tool installation to `atmos-toolchain`.

## Routing

| Need | Skill |
|------|-------|
| Azure identity/provider setup, device-code, OIDC, Azure CLI | `atmos-auth` |
| Installing `kubectl` or other command-line tools | `atmos-toolchain` |
| Component/stack lookup for cluster settings | `atmos-components`, `atmos-stacks` |
