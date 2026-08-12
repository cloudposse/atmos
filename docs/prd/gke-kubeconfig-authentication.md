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

1. The integration requires `GCPCredentials` and uses its current OAuth2 access token directly with the Google Kubernetes Engine API. It does not invoke `gcloud`, bootstrap Application Default Credentials (ADC) directly, or use `gke-gcloud-auth-plugin`; it can consume credentials obtained by a configured Atmos GCP provider such as `gcp/adc`.
2. Atmos requests `projects/{project}/locations/{location}/clusters/{name}` and validates the public API endpoint and base64 CA certificate.
3. The shared cloud-neutral `kube.ClusterInfo` writer from the EKS/AKS implementation writes only the endpoint, CA, context, and Atmos exec plugin. It never persists the access token.
4. When Kubernetes needs credentials, it invokes `atmos gcp gke token`. The command resolves or refreshes the selected identity through the existing Auth manager with integration auto-provisioning suppressed, then emits only a Kubernetes `ExecCredential` JSON document.
5. The integration contributes both `KUBECONFIG` and `KUBE_CONFIG_PATH`. The existing identity-environment composition makes the generated path available to Helm, Kubernetes, Helmfile, kubectl subprocesses, and workflows that select the identity.

The canonical target key is a deterministic `gcp/gke:` query over the project, location, name, and output settings. The kubeconfig cluster key uses the fully-qualified GKE resource name, and the generated user includes project and location, preventing collisions across projects.

The process cache key also includes kubeconfig path, mode, update behavior, context alias, and exec-plugin identity. Bulk commands therefore deduplicate identical GKE discovery/provisioning work without skipping a distinct output configuration.

## Helm Safety Guard

Native Helm preserves its current ambient-kubeconfig behavior by default. A GKE component can opt into fail-closed targeting with `auth.require_identity: true` and a component-level default identity. For guarded apply/deploy/delete operations, Atmos resolves that default when no CLI identity was supplied, requires the GKE integration to provision an expected endpoint, and compares it with the effective Helm REST configuration before contacting the cluster.

A concrete component can disable an inherited guard while retaining the rest of its inherited auth configuration:

```yaml
auth:
  require_identity: false
```

If the component must also deselect an inherited default identity marker, it can override that marker explicitly:

```yaml
auth:
  identities:
    example-deployer:
      default: false
```

An empty `identities: {}` map does not clear inherited identities; normal deep-merge behavior intentionally preserves them.

The comparison uses the API server endpoint, not the local context name. Context names are aliases and cannot prove cluster identity. This guard is intentionally GKE-scoped in this change and does not alter existing EKS or AKS behavior.

The default kubeconfig path is Atmos-owned under the XDG config directory. `update: merge` sets `current-context` in that Atmos-owned file. Mutating a shared user kubeconfig happens only when the user explicitly configures that shared path.

## Security and Permissions

- The identity needs permission to describe the target cluster, normally including `container.clusters.get`.
- The configured GCP provider must be able to obtain or refresh the OAuth2 access token used by Atmos.
- Kubernetes RBAC authorization is separate; a valid Google access token does not itself grant Kubernetes permissions.
- Tokens are short lived, returned only on exec-plugin stdout, and never stored in kubeconfig.

## Non-Goals

- GKE cluster lifecycle management.
- Artifact Registry authentication.
- General Helm-specific behavior beyond the documented opt-in GKE Helm safety guard.
- Static bearer tokens in kubeconfig.
- Private endpoint or DNS endpoint selection in the initial implementation.
- Changing existing EKS or AKS targeting behavior.

## Manual Verification (live GKE cluster)

The `gcp/gke` integration was exercised end-to-end on 2026-08-12 against a real regional GKE
cluster using an Atmos `gcp/project` identity backed by a `gcp/adc` provider. Project, cluster,
endpoint, principal, and node names are omitted below; the published aliases are generic. The
cluster had four nodes running GKE `v1.34.9-gke.1065000`.

**1. What the live run surfaced.** On first use, `atmos auth exec` set `KUBECONFIG` to the
integration path but did not create the file. The provider-backed `gcp/project` identity's local
credential loader returned a project-only `GCPCredentials` value with no access token. Because
that value had no expiry, the Auth manager treated it as a valid cached credential, skipped the
ADC provider, and never provisioned the linked GKE integration. `kubectl` consequently fell back
to `http://localhost:8080`.

The project identity now reports no local cached credentials when it has an upstream provider or
identity. This forces the configured chain to authenticate and preserves the access token. A
standalone `gcp/project` identity retains its existing project-context-only behavior.

**2. First-use `auth exec` path.** The isolated kubeconfig was deleted before this command; no
preparatory `atmos auth login` or `gcloud container clusters get-credentials` was run. Atmos
described the cluster, wrote the kubeconfig, injected its path into the child environment, and
then allowed `kubectl` to authenticate through the generated exec plugin:

```console
$ test ! -e /tmp/example-kubeconfig
$ atmos auth exec --identity example-deployer -- sh -c "kubectl get nodes -o json | jq '{nodeCount: (.items | length), readyCount: ([.items[] | select(any(.status.conditions[]?; .type == \"Ready\" and .status == \"True\"))] | length), versions: ([.items[].status.nodeInfo.kubeletVersion] | unique)}'"
✓ GKE kubeconfig: example → /tmp/example-kubeconfig
{
  "nodeCount": 4,
  "readyCount": 4,
  "versions": ["v1.34.9-gke.1065000"]
}
```

**3. Generated kubeconfig.** Inspection confirmed that the file contained one HTTPS cluster with
CA data and one Atmos exec user, with no stored bearer token:

```console
$ kubectl --kubeconfig=/tmp/example-kubeconfig config view --raw -o json | jq '{currentContext: .["current-context"], serverUsesHTTPS: (.clusters[0].cluster.server | startswith("https://")), caDataPresent: ((.clusters[0].cluster["certificate-authority-data"] // "") != ""), execCommand: .users[0].user.exec.command, execArgs: .users[0].user.exec.args, storedBearerToken: ((.users[0].user.token // "") != "")}'
{
  "currentContext": "example",
  "serverUsesHTTPS": true,
  "caDataPresent": true,
  "execCommand": "atmos",
  "execArgs": ["gcp", "gke", "token", "--identity=example-deployer"],
  "storedBearerToken": false
}
```

**4. Exec credential.** The token command was also run directly with its secret value reduced to
a boolean before display:

```console
$ atmos gcp gke token --identity example-deployer | jq '{apiVersion, kind, tokenPresent: ((.status.token // "") != ""), expirationPresent: ((.status.expirationTimestamp // "") != "")}'
{
  "apiVersion": "client.authentication.k8s.io/v1beta1",
  "kind": "ExecCredential",
  "tokenPresent": true,
  "expirationPresent": true
}
```

The GKE integration and exec plugin did not bootstrap ADC directly; they consumed the access
token supplied by Atmos Auth. In this verification, Atmos Auth obtained that token through the
configured `gcp/adc` provider. A second
first-use run restricted `PATH` to the locally built `atmos`, `kubectl`, `jq`, and base system
directories; a preflight check confirmed that neither `gcloud` nor `gke-gcloud-auth-plugin` was
available, and all four nodes were still reachable and Ready.

**5. Combined guarded Helm run.** A separate combined integration run declared the GKE integration
only in global auth configuration. Each Helm component contained the opt-in guard and a default
marker for the existing global identity; no component duplicated the integration. With ambient
`KUBECONFIG` unset, Atmos selected the default component identity without `--identity`, provisioned
the integration, and kept progress output visible. Atmos itself invoked neither `gcloud` nor
`gke-gcloud-auth-plugin`.

One real guarded apply succeeded. Deliberately mismatched apply and delete targets were both
rejected before mutation with exit code 1 after Atmos compared the expected and effective API
server endpoints exactly. The Helm release revision and the target resource's `resourceVersion`
remained unchanged after both rejected operations.

An `apply --all --dry-run` covered 10 components successfully without mutation. All 10 resolved
the same GKE target, which was discovered and provisioned once in the process. A subsequent real
`apply --all` was blocked by a separate native Helm server-side-apply field-manager defect, so this
run does not establish a successful full rebuild. That Helm defect is outside this GKE auth change.

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-07 | Initial as-built GKE kubeconfig authentication design |
| 1.1 | 2026-08-12 | Live-cluster verification and provider-backed `gcp/project` credential-loading fix |
| 1.2 | 2026-08-12 | Global integration merge fix, component guard opt-out semantics, and combined guarded Helm evidence |
