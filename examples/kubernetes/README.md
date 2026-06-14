# Example: Kubernetes Components

Deploy Kubernetes manifests with Atmos-native Kubernetes components and a local k3s cluster.

This example uses `provider: kubectl`, which means kubectl-compatible manifest behavior through the Kubernetes Go SDK. It does not require the `kubectl` binary.

## Try It

```shell
cd examples/kubernetes

# Start local k3s cluster
atmos k3s up

# Render manifests without contacting the cluster
atmos kubernetes render demo -s dev --identity local-k3s

# Preview and apply through the Kubernetes SDK
atmos kubernetes diff demo -s dev --identity local-k3s
atmos kubernetes apply demo -s dev --identity local-k3s

# Process every Kubernetes component in DAG order
atmos kubernetes apply --all -s dev --identity local-k3s

# Clean up
atmos kubernetes delete demo -s dev --identity local-k3s
atmos k3s down
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Kubernetes component config, mock auth identity, and k3s commands |
| `docker-compose.yml` | Local k3s Kubernetes cluster |
| `components/kubernetes/demo/files/` | File-backed Kubernetes manifests loaded through `paths` |
| `stacks/catalog/demo.yaml` | Abstract Kubernetes component with inline `manifests` |
| `stacks/mixins/k3s.yaml` | Local kubeconfig environment wiring |

## Auth

The `local-k3s` identity uses `ambient`, so the command runs through Atmos Auth without binding the example to a cloud provider. The Kubernetes SDK client uses the `KUBECONFIG` value from the stack environment. For real EKS clusters, pair an AWS identity with an `aws/eks` integration that writes kubeconfig for the cluster.
