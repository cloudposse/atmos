# Example: Kustomize Components

Render and deploy Kustomize overlays with Atmos-native Kubernetes components and a local k3s cluster.

This example uses `provider: kustomize`, so Atmos renders the overlay with the Kustomize Go API and then applies the resulting objects through the Kubernetes Go SDK. It does not require the `kustomize` or `kubectl` binaries.

## Try It

```shell
cd examples/kustomize

# Start local k3s cluster
atmos k3s up

# Render the dev overlay
atmos kubernetes render demo -s dev --identity local-k3s

# Preview and apply through the Kubernetes SDK
atmos kubernetes diff demo -s dev --identity local-k3s
atmos kubernetes apply demo -s dev --identity local-k3s

# Clean up
atmos kubernetes delete demo -s dev --identity local-k3s
atmos k3s down
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Kubernetes component config, mock auth identity, and k3s commands |
| `components/kubernetes/demo/base/` | Kustomize base |
| `components/kubernetes/demo/overlays/dev/` | Kustomize overlay used by the dev stack |
| `stacks/catalog/demo.yaml` | Abstract component with provider and path configuration |
| `stacks/mixins/k3s.yaml` | Local kubeconfig environment wiring |

## Auth

The `local-k3s` identity uses `ambient`, so the command runs through Atmos Auth without binding the example to a cloud provider. The Kubernetes SDK client uses the `KUBECONFIG` value from the stack environment. For real EKS clusters, pair an AWS identity with an `aws/eks` integration that writes kubeconfig for the cluster.
