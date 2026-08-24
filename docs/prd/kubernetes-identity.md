# PRD: Native Kubernetes Identity

## Summary

Add a **native `kubernetes` identity family** to Atmos auth whose credential **is a kubeconfig** and
is **not minted by a cloud provider**. Today, Kubernetes access in Atmos exists only as an
*integration minted from a cloud identity* (`aws/eks` → kubeconfig built from AWS credentials). There
is no way to authenticate directly to a cluster whose access does not flow through AWS/GCP/Azure.

A `kubernetes/*` identity is a **root identity** (no upstream cloud provider) that yields a
kubeconfig. The kind selects the *source* of that kubeconfig; all kinds produce the same
`ICredentials` (a kubeconfig) and the same `Environment()` → `KUBECONFIG`. This mirrors how
`aws/user`, `aws/permission-set`, and `aws/assume-role` are sibling kinds that all yield AWS
credentials.

This primitive is independently useful (talk to any cluster without a cloud provider), and it is the
mechanism that lets the **kubernetes emulator** (k3s/k3d/kind, see [Emulators](emulators.md)) bind
exactly like the cloud emulators — via an identity, not a bespoke integration. It is documented
separately but **implemented in the same PR** as Emulators, since `kubernetes/emulator` is required
for k3s.

## Goals

- A native `kubernetes` identity family whose credential is a kubeconfig, sourced without a cloud.
- Three sources/kinds: `kubernetes/emulator`, `kubernetes/kubeconfig`, `kubernetes/service-account`.
- Inject `KUBECONFIG` into the subprocess environment via the existing env-composition path
  (`composeEnvironmentVariables` already special-cases `KUBECONFIG` with colon-append + dedup).
- Fit the existing auth model: root, non-cloud identities already exist (`ambient`, `atmospro`).

## Non-Goals

- Cloud-managed clusters (EKS / GKE / AKS). Their kubeconfig uses a **dynamic exec credential**
  (`aws eks get-token`, `gke-gcloud-auth-plugin`, `kubelogin`) minted from a cloud identity — they
  remain `aws/eks`-style **integrations** chained off a cloud identity, not native identities.
- A general Kubernetes "provider" abstraction. These are root identities; there is no upstream
  provider to authenticate.
- Cluster lifecycle management — that belongs to the emulator/component layer, not auth.

## Design

### §A The family — one credential type, pluggable source

All `kubernetes/*` identities share a `kubeconfigCredential` implementing `ICredentials`:

- `Authenticate(ctx, base)` produces the kubeconfig from the kind's source and writes it to a
  realm-scoped path.
- `Environment()` returns `KUBECONFIG=<path>` — merged via the existing
  `composeEnvironmentVariables` append/dedup so it composes with any pre-existing `KUBECONFIG`.
- `Paths()` returns the kubeconfig file.
- `Logout()` / `Cleanup()` removes the materialized file (idempotent, non-fatal).

They are **root identities** (no `via.provider`). Precedent: `pkg/auth/identities/ambient` and
`…/atmospro` are already root identities not minted by a cloud SSO provider.

### §B Sources / kinds

```yaml
auth:
  identities:
    # 1) You already have a kubeconfig — pick a context (most common real-cluster case).
    prod-cluster:
      kind: kubernetes/kubeconfig
      kubeconfig: ~/.kube/config          # path or inline data; default $KUBECONFIG / ~/.kube/config
      context: prod                       # Atmos materializes a context-scoped (minified) kubeconfig

    # 2) Server + CA + bearer token (headless / CI; secrets from a store).
    ci-deployer:
      kind: kubernetes/service-account
      server: https://k8s.example.com:6443
      certificate_authority_data: !env K8S_CA_B64   # or certificate_authority: /path/ca.crt
      token: !store kv/ci/k8s-token

    # 3) Harvested from a running emulator (k3s/k3d/kind).
    local-k8s:
      kind: kubernetes/emulator
      emulator: local/k3s
```

- **`kubernetes/kubeconfig`** — read an existing kubeconfig (file path or inline data), minify to the
  selected `context`, materialize, export `KUBECONFIG`.
- **`kubernetes/service-account`** — build a kubeconfig from `server` + CA (`certificate_authority` or
  `certificate_authority_data`) + bearer `token`, materialize, export `KUBECONFIG`.
- **`kubernetes/emulator`** — resolve the named emulator's profile (`Profile.Kubeconfig`),
  materialize, export `KUBECONFIG`. **Required** for the Emulators feature.

### §C The emulator source (k3s) — harvested, not minted

k3s is self-contained: on start it generates its own cluster CA and an **admin kubeconfig** at
`/etc/rancher/k3s/k3s.yaml` with **embedded client-cert credentials** (server
`https://127.0.0.1:6443`). There is no login step — *the kubeconfig is the auth*. The
`kubernetes/emulator` identity:

1. reads that kubeconfig out of the running container (`runtime.Exec … cat`, or mount
    `/etc/rancher/k3s` as a known volume → plain file read);
2. rewrites **only** `server:` → `https://localhost:<live-host-port>` (CA + client cert/key kept
    verbatim — that *is* the credential);
3. materializes it and `Environment()` returns `KUBECONFIG=<path>`.

So for aws/gcp/azure the `<cloud>/emulator` identity *injects* dummy creds + endpoint, whereas the
`kubernetes/emulator` identity *harvests the real admin credentials k3s issued itself* and only
repoints the URL — but **both bind the same way**: an identity selected by the component. The
emulator's self-signed CA is the trust root.

**TLS-SAN caveat.** k3s ships `127.0.0.1`/`localhost` SANs by default so localhost verifies cleanly;
reaching it by another host/IP needs `--tls-san` on the k3s container (or `insecure-skip-tls-verify`
as a fallback).

## Public Interface

- Identity kinds: `kubernetes/emulator`, `kubernetes/kubeconfig`, `kubernetes/service-account`.
- Fields: `emulator` (emulator kind); `kubeconfig` + `context` (kubeconfig kind); `server` +
  `certificate_authority`/`certificate_authority_data` + `token` (service-account kind).
- Environment contribution: `KUBECONFIG`.

## Implementation Notes

- New package `pkg/auth/identities/kubernetes/` with the shared `kubeconfigCredential` and the three
  kind implementations; register all in the identity factory next to the cloud kinds.
- Schema: add the identity kinds and their fields in `pkg/schema/schema_auth.go`; update
  `pkg/datafetcher/schema/`.
- `Environment()` → `KUBECONFIG` flows through the existing `composeEnvironmentVariables` append/dedup
  in `pkg/auth/manager_environment.go`.
- The `kubernetes/emulator` kind consumes the emulator profile via the narrow resolver seam injected
  into auth (no `pkg/emulator` ↔ `pkg/auth` cycle).
- Confirm the auth chain/provider model tolerates a provider-less root identity — follow the
  `ambient`/`atmospro` precedent.
- Scope: `kubernetes/emulator` is required for the Emulators feature. If `kubernetes/kubeconfig` and
  `kubernetes/service-account` add cost, ship `emulator` first and land the other two as fast-follow
  within this PRD.

## Test Plan

- **Per-source unit tests:** each kind produces a valid kubeconfig and `Environment()` → `KUBECONFIG`;
  `Logout`/`Cleanup` removes the file; `Paths()` returns it.
- **kubeconfig kind:** context minification selects the right context; missing context → error.
- **service-account kind:** kubeconfig assembled from `server`+CA+`token`; CA from both
  `certificate_authority` and `certificate_authority_data`.
- **emulator kind:** harvest from a fake emulator profile; only `server:` rewritten to the live port;
  CA + client cert preserved; not-running → actionable error.
- **Env composition:** `KUBECONFIG` appends/dedups with a pre-existing `KUBECONFIG`.
- **Negative path:** missing source config → clear error; provider-less root identity authenticates
  without a cloud chain.
- Follow repo testing mandates: table-driven, `cmd.NewTestKit(t)` where applicable, cross-platform
  paths (`filepath.Join`), no platform-specific binaries.

## Related

- [Emulators](emulators.md) — the kubernetes emulator (`k3s`/`k3d`/`kind`) bound by
  `kubernetes/emulator`.
