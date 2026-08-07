# PRD: Native GKE Kubeconfig Authentication

## Summary

Add `gcp/gke` as an Atmos Auth integration. A linked GCP identity describes a GKE cluster, writes its endpoint and CA certificate to kubeconfig, and configures Kubernetes to obtain each bearer token by executing `atmos gcp gke token --identity <identity>`.

GKE belongs in the integration layer because the durable credential is still the GCP identity. The kubeconfig is a client-side materialization derived from that identity, like `aws/eks` and `azure/aks`; it is not a standalone Kubernetes identity and does not manage cluster lifecycle.

## Public Configuration

```yaml
auth:
  integrations:
    example-gke:
      kind: gcp/gke
      via:
        identity: example-deployer
      spec:
        cluster:
          name: example-cluster
          project_id: example-project
          location: us-central1
          alias: example
          kubeconfig:
            path: /tmp/example-kubeconfig
            update: replace
```

`name`, `project_id`, and `location` are required. Requiring the project explicitly keeps API addressing, deduplication, cleanup, error messages, and generated kubeconfig names deterministic even when credentials carry a default project.

## Design

1. The integration requires `GCPCredentials` and uses its current OAuth2 access token directly with the Google Kubernetes Engine API. It does not use `gcloud`, Application Default Credentials, or `gke-gcloud-auth-plugin`.
2. Atmos requests `projects/{project}/locations/{location}/clusters/{name}` and validates the public API endpoint and base64 CA certificate.
3. The shared cloud-neutral `kube.ClusterInfo` writer from the EKS/AKS implementation writes only the endpoint, CA, context, and Atmos exec plugin. It never persists the access token.
4. When Kubernetes needs credentials, it invokes `atmos gcp gke token`. The command resolves or refreshes the selected identity through the existing Auth manager with integration auto-provisioning suppressed, then emits only a Kubernetes `ExecCredential` JSON document.
5. The integration contributes both `KUBECONFIG` and `KUBE_CONFIG_PATH`. The existing identity-environment composition makes the generated path available to Helm, Kubernetes, Helmfile, kubectl subprocesses, and workflows that select the identity.

The canonical target key is `gcp/gke:<project_id>:<location>:<name>`. The kubeconfig cluster key uses the fully-qualified GKE resource name, and the generated user includes project and location, preventing collisions across projects.

## Security and Permissions

- The identity needs permission to describe the target cluster, normally including `container.clusters.get`.
- The configured GCP provider must be able to obtain or refresh the OAuth2 access token used by Atmos.
- Kubernetes RBAC authorization is separate; a valid Google access token does not itself grant Kubernetes permissions.
- Tokens are short lived, returned only on exec-plugin stdout, and never stored in kubeconfig.

## Non-Goals

- GKE cluster lifecycle management.
- Artifact Registry authentication.
- Helm-specific behavior.
- Static bearer tokens in kubeconfig.
- Private endpoint or DNS endpoint selection in the initial implementation.
