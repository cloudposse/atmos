# Example: Demo Helmfile

Deploy Kubernetes resources using Helmfile with Atmos stack patterns.

Learn more about [Helmfile Components](https://atmos.tools/core-concepts/components/helmfile/).

## What You'll See

- [Helmfile component](https://atmos.tools/core-concepts/components/helmfile/) configuration
- Stack inheritance for Kubernetes deployments
- Local k3s environment via Docker Compose

## Try It

```shell
cd examples/demo-helmfile

# Start local k3s cluster
atmos k3s up

# Deploy to dev environment
atmos helmfile apply demo -s dev

# Check status
atmos k3s status

# Clean up
atmos k3s down
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Helmfile component config and k3s commands |
| `docker-compose.yml` | Local k3s Kubernetes cluster |
| `components/helmfile/nginx/` | Helmfile component with manifests |
| `stacks/deploy/` | Environment-specific stack configs |
