# PRD: Emulators

## Summary

Add first-class support for **emulators** to Atmos: long-running container services that stand in
for cloud provider APIs (AWS, GCP, Azure), Kubernetes (k3s/k3d/kind), and select backing services
(Vault, an OCI/Terraform registry) during local development and testing. Examples include
[Floci](https://github.com/floci-io/floci) and [MiniStack](https://ministack.org/) (free AWS
emulators), the per-service GCP emulators, Azurite, and k3s.

An emulator is a **stack-scoped component kind** (`components.emulator.<name>`) that:

- starts/stops as a long-lived container that **outlives the `atmos` process**,
- is discoverable by label and operable with `up/down/reset/ps/logs/exec`,
- advertises a **connection profile** (SDK env vars and/or a kubeconfig) for the live container,
- is consumed by Atmos **auth identities** (`kind: <target>/emulator`) so `atmos auth`,
  `atmos terraform`, and a preconfigured shell transparently target it,
- can be referenced explicitly anywhere via a deferred **`!emulator`** YAML function,
- contributes Terraform provider settings via a generic
  [provider-config contributor](provider-config-contributor.md).

Today the only emulator support is buried in tests (`tests/floci_containers_test.go` auto-starts
Floci via testcontainers); there is no user-facing way to declare, start, stop, or wire an emulator
into a real Atmos workflow.

## Goals

- Declare an emulator as a stack component, addressable like any other component instance.
- Start/stop a persistent emulator container that survives the `atmos` process; discover it by label.
- Run **multiple concurrent emulators** (AWS + GCP + Azure + Kubernetes) in one stack.
- Open a host shell **preconfigured with the right SDK env vars** for an emulator (via
  `atmos auth shell --identity <emulator-identity>`).
- Make `atmos auth` / `atmos terraform` target a running emulator via an auth identity — with **no
  hand-written `providers.tf`**.
- Ship **built-in driver kinds** with sensible defaults that are fully overridable (the hooks model).
- Reuse the `container` component kind's lifecycle/discovery/runtime — do not reinvent it.

## Non-Goals

- V1 does not implement dedicated **store / terraform-backend / vendoring** consumers for the
  `vault` and `registry` targets — those ship as driver kinds usable via env injection and the
  `!emulator` function; their auto-wiring is a follow-up. The pluggable consumer **seam** is defined.
- V1 does not add first-class `digitalocean` / `oracle` / `alibaba` targets — no mainstream
  emulators exist. S3-compatible storage (DO Spaces, Cloudflare R2) is covered by an S3-capable
  driver; Cloudflare Workers (Miniflare/workerd) is a possible future target.
- V1 does not model cloud-managed Kubernetes (EKS/GKE/AKS) — those use a dynamic exec credential and
  remain integrations chained off a cloud identity (see [Kubernetes identity](kubernetes-identity.md)).
- V1 does not add per-service GCP/Azure driver granularity beyond what Floci covers (room is left in
  the registry).

## Design

### §A The `emulator` component kind

`emulator` is a new top-level component kind in `schema.Components`, a sibling of
terraform/helmfile/packer/ansible/container. It is **a thin specialization of the `container`
component kind**: it reuses that kind's lifecycle/discovery/runtime via a **nested `container:`
block**, and adds a cloud/driver layer on top.

```yaml
components:
  emulator:
    aws:                                   # instance named by role -> dev/emulator/aws
      driver: floci/aws                    # built-in driver kind (free; default for AWS)
      cloud: aws                           # target; optional when derivable from driver
      region: us-east-1
      container:                           # nested container-component config (reused as-is)
        # image defaults from the `floci/aws` driver; override only to pin/replace
        services: [s3, sqs, dynamodb]
        ports: [{ container: 4566 }]       # host auto-assigned unless pinned
    gcp:
      driver: floci/gcp                    # cloud=gcp derived from driver
      project: test-project
      container: { ports: [{ container: 4588 }] }
    azure:
      driver: floci/az
    k3s:
      driver: k3s                          # cloud/target=kubernetes derived from driver
      container:
        image: rancher/k3s:latest
        privileged: true
        ports: [{ container: 6443 }]
```

First-class fields: `driver`, `cloud`/target (optional, derived from `driver`), `region`/`project`,
and the nested `container:` block (reusing the container kind's `image`/`ports`/`services`/`env`/
`mounts`/`privileged`). Inheritance, catalogs, and deep-merge apply normally.

**Image resolution.** The image lives in `container.image`. Precedence: explicit `container.image`
**>** the `driver`'s default image. A driver always supplies a default, so `container.image` is
optional. Images may be pinned by tag or `@sha256:` digest.

### §B Drivers — built-in kinds with defaults, overridable

`driver` selects a **built-in emulator kind** whose values (image, ports, services, env) are
defaulted but fully overridable in `container:` — the same model as built-in hook kinds. It is a Go
code registry (`EmulatorDriver`) keyed by driver name:

```go
type EmulatorDriver interface {
    Name() string                 // "floci/aws", "ministack/aws", "floci/gcp", "k3s", "openbao", ...
    Target() string               // "aws" | "gcp" | "azure" | "kubernetes" | "vault" | "registry"
    Defaults() ContainerDefaults  // default image, ports, services (overridable in container:)
    Profile(ep *Endpoint) Profile // turn the live container endpoint into a connection profile
}

type Profile struct {
    Env        map[string]string // SDK / terraform / VAULT_ADDR / registry env vars
    Kubeconfig []byte            // kubernetes drivers (k3s/k3d/kind)
    ResolverURL string           // for Atmos-internal AWS SDK (aws only)
    Provider   map[string]any    // TF provider fragment (endpoints + skip-flags + creds)
}
```

Built-in drivers (V1). Cloud-API emulator drivers follow the **`<product>/<cloud>`** convention
(`floci/aws`, `floci/gcp`, `floci/az`); single-target drivers (kubernetes/vault/registry) use the
bare product name since there is no cloud axis:

| Target | Built-in drivers | Notes |
|--------|------------------|-------|
| `aws` | **`floci/aws` (default)**, `ministack/aws`, `localstack/aws` (opt-in/legacy) | LocalStack archived its repo and paywalled the community edition (BSL) in March 2026 — default to the free Floci/MiniStack. |
| `gcp` | `floci/gcp` | Per-service emulators (gcs/pubsub/firestore/…) may be added later. |
| `azure` | **`floci/az`** | Storage only; Cosmos/Service Bus/Event Hubs are separate emulators (later). A dedicated `azurite/az` driver may be added later. |
| `kubernetes` | **`k3s` (default)**, `k3d`, `kind` | All yield a kubeconfig. `kwok`/`minikube`/`microk8s` future. |
| `vault` | **`openbao` (default)**, `vault` (dev mode) | API-compatible (same `VAULT_ADDR`/`VAULT_TOKEN`); OpenBao is the open-source (MPL) fork of Vault (BSL). Consumed via env/`!emulator` in V1. |
| `registry` | `registry` (OCI / Terraform registry) | For vendoring / registry-cache. Consumed via env/`!emulator` in V1. |

The registry mirrors `pkg/store/registry.go` (kind → builder map). The mock is generated with
`go.uber.org/mock/mockgen`.

### §C Naming & target (avoid the coincidence trap)

The emulator **component name is a free-form label** (the address is `<stack>/emulator/<name>`); it
does **not** encode the target. The target (`aws|gcp|azure|kubernetes|vault|registry`) is
**structural** — derived from `driver` (or explicit `cloud`). An identity's `kind: <target>/emulator`
prefix declares which target it speaks, and `emulator: <name>` references the component; Atmos
**validates the identity target == the component target** (hard error on mismatch).

Recommended convention: name an emulator for its role and let `driver`/`kind` carry the semantics —
`kind: aws/emulator → emulator: aws`, `kind: kubernetes/emulator → emulator: k3s`.

### §D Lifecycle (reuses the container component kind)

Reuses `pkg/container/lifecycle.go` (delivered by the container component kind) by passing
`ComponentType:"emulator"` — no Sandbox generalization needed:

- `Up`/`UpWithRuntime` — start a long-lived named container (`NamedConfig{Stack, ComponentType,
  Component, Image, Ports, Mounts, Env, Labels, RuntimeName, RuntimeAutoStart, PullPolicy, …}`).
- `FindInstance` / `Down` — discover / stop+remove by label.
- `pkg/container/identity.go` — `InstanceAddress`/`InstanceLabels`/`DiscoveryFilter`/`RuntimeName` +
  `tools.atmos.{stack,component_type,component,instance}` labels.
- `pkg/component/container/config.go` — `FromComponentSection`/`ContainerSpec`/`Ports()`/`Mounts()`
  parse the nested `container:` block.

The container **persists across the `atmos` process** (it is started detached and not torn down on
exit). Host ports default to auto-assigned (`host:0`) so concurrent emulators don't collide; the
live host port is read back via `runtime.Inspect`. Discovery is by label, so a later process
(an auth identity, a subsequent command) finds the running container with no state file.

```bash
atmos emulator up aws -s dev        # start detached (alias: start); --ephemeral skips persistence
atmos emulator down aws -s dev      # stop + remove (alias: stop); --all prunes by type=emulator
atmos emulator reset aws -s dev     # stop + remove AND wipe persisted state (--force skips prompt)
atmos emulator ps -s dev            # list running emulators (label discovery)
atmos emulator logs aws -s dev
atmos emulator exec aws -s dev -- <cmd>   # run a command inside the emulator container
```

`cmd/emulator/*` mirrors `cmd/container/verbs.go`; flags use `flags.NewStandardParser()`. There is
**no dedicated `atmos emulator shell` verb**: a host shell preconfigured with the emulator's SDK env /
`KUBECONFIG` is provided by `atmos auth shell --identity <emulator-identity> -s dev` (the emulator
identity injects the same profile), and in-container access is `atmos emulator exec`.

### §D′ Persistence

Emulator state **survives `down`/`up` by default**. Each driver declares a data directory
(`localstack/aws` → `/var/lib/localstack`, `k3s` → `/var/lib/rancher/k3s`, `openbao` → `/openbao/file`,
`registry` → `/var/lib/registry`, …); Atmos bind-mounts a per-instance directory under the XDG cache
onto it so the container can be recreated without losing data. See `pkg/emulator/persistence.go`
(`InstanceDataDir`/`LookupInstanceDataDir`) and `pkg/emulator/mounts.go` (`resolveMounts` auto-injects
the persistence mount).

- **Default-on**, gated by `Spec.PersistEnabled()` (`pkg/emulator/spec.go`); a driver with no data
  directory is simply not persisted.
- **Opt out per component** with `ephemeral: true` in the `components.emulator.<name>` block, or
  **per run** with `atmos emulator up --ephemeral` (throwaway instance, no mount).
- **Reset** with `atmos emulator reset` — stops and removes the container **and** deletes its persisted
  state directory (`--force` skips the confirmation prompt). Use it to start clean.

### §E Connection profile (per target)

A driver's `Profile(endpoint)` turns the live container endpoint into a connection profile:

- **AWS** — env `AWS_ENDPOINT_URL`, `AWS_ACCESS_KEY_ID=test`, `AWS_SECRET_ACCESS_KEY=test`,
  `AWS_SESSION_TOKEN=test`, `AWS_REGION`/`AWS_DEFAULT_REGION`, optional per-service
  `AWS_ENDPOINT_URL_<SERVICE>` (multi-port); plus `ResolverURL` for Atmos's internal SDK; plus a
  `Provider` fragment (endpoints + skip-flags + creds) for the contributor.
- **GCP** — env `STORAGE_EMULATOR_HOST`, `PUBSUB_EMULATOR_HOST`, `FIRESTORE_EMULATOR_HOST`,
  `BIGTABLE_EMULATOR_HOST`, `CLOUDSDK_CORE_PROJECT`/`GOOGLE_CLOUD_PROJECT`,
  `CLOUDSDK_AUTH_DISABLE_CREDENTIALS=true`. (Per-service; no global endpoint.)
- **Azure** — env `AZURE_STORAGE_CONNECTION_STRING` (Azurite-style), `AZURE_STORAGE_ACCOUNT`,
  `AZURE_STORAGE_KEY`. (`*_SERVICE_URL` is SDK-version-dependent; validate against the image.)
- **Kubernetes** — `Kubeconfig` bytes harvested from the container (server URL rewritten to the live
  host port; CA + client cert/key preserved), exported as `KUBECONFIG`.
- **Vault / OpenBao** — env `VAULT_ADDR` (+ dev-root `VAULT_TOKEN`). Both drivers are
  API-compatible; OpenBao (`openbao`, MPL) is the default, HashiCorp Vault (`vault`, BSL) is opt-in.
- **Registry** — env/URL for the OCI/Terraform registry.

### §F Binding — pluggable consumers; identity is the V1 consumer

An emulator advertises a profile; consumers subscribe to it. The **binding seam is pluggable**:

- **auth identity** (`kind: <target>/emulator`) — **built in V1**. Cloud targets inject SDK env;
  the kubernetes target injects `KUBECONFIG` (see [Kubernetes identity](kubernetes-identity.md)).
- **store** (Vault → `VAULT_ADDR`; SSM/SecretsManager via AWS env) — seam defined; auto-wiring
  follow-up (usable now via env/`!emulator`).
- **terraform backend** (S3/GCS/azurerm) — for AWS rides the same `AWS_ENDPOINT_URL_*` env already
  threaded into `terraform_backend_s3.go`.
- **vendoring / registry** — seam defined; auto-wiring follow-up.

#### Identity binding (the common path)

A component selects an identity (`settings.identity` or `--identity`); the identity references an
emulator by **bare name**, resolved against the stack the command runs in:

```yaml
# atmos.yaml
auth:
  identities:
    local-aws:
      kind: aws/emulator        # cloud emulator identity; cloud derived from prefix
      emulator: aws             # -> dev/emulator/aws when run with -s dev
    local-k8s:
      kind: kubernetes/emulator # native, non-cloud-minted identity (see kubernetes-identity.md)
      emulator: k3s
```

Flow for `atmos terraform apply vpc -s dev` (component has `settings.identity: local-aws`):

1. Auth selects `local-aws`; it has `emulator: aws`.
2. Atmos scopes the bare name to the current stack → address `dev/emulator/aws`.
3. Validate: identity target (`aws`) == emulator component target; emulator declared in `dev` →
   else hard error with a hint.
4. The injected resolver (`pkg/emulator.Manager`) discovers the container by label, `Inspect`s the
   live host port, builds the profile. Not running → actionable error
   (`atmos emulator up aws -s dev`).
5. `Profile.Env` is merged into the subprocess env (`PrepareShellEnvironment`); for AWS the live URL
   also feeds the existing `resolver.url` → `config.WithBaseEndpoint` path so Atmos's own SDK calls
   (`!terraform.output`, store auth) hit the emulator; `Profile.Provider` is contributed to provider
   generation (§G).

`atmos auth shell --identity local-aws -s dev` provides the host shell preconfigured for the
emulator: it runs the same resolution and exports the same env an emulator identity injects.

### §G Provider settings via the provider-config contributor

Env injection cannot set Terraform provider *behavior* flags (`skip_requesting_account_id`,
`s3_use_path_style`, `skip_credentials_validation`, `skip_metadata_api_check`) — those are provider
arguments. The emulator registers a contributor in the generic
[provider-config contributor](provider-config-contributor.md) mechanism that injects, for an
emulator-bound component's provider (e.g. `aws`), the `endpoints {}` + skip-flags + dummy creds. The
result: emulator-backed Terraform needs **no hand-written `providers.tf`**.

Two complementary paths, no duplication: env (identity path) drives SDKs/shell/non-Terraform; the
provider contributor drives Terraform provider behavior. Both read the same `Manager.Resolve` profile.

### §H `!emulator` YAML function (explicit escape hatch)

A deferred YAML function places a live emulator value anywhere in config (component `vars`, `!store`
config, backend config, custom-command env):

```yaml
vars:
  s3_endpoint: !emulator aws endpoint
env:
  KUBECONFIG:  !emulator k3s kubeconfig
  S3_URL:      !emulator dev/emulator/aws endpoint   # full-address reference (ref may contain "/")
```

Grammar: **`!emulator <ref> <key>`** — **space-separated positional arguments** (consistent with
`!store <store> <key>` and `!terraform.output <component> <stack> <output>`), *not* dot-notation.
This keeps the grammar unambiguous and lets `<ref>` contain a `/` for a full-address reference.

- `<ref>` — the emulator reference, resolved against the stack the command runs in: the bare
  component name (`aws`, `k3s`), or a full `<stack>/emulator/<name>` address to target a specific
  stack.
- `<key>` — `endpoint`/`url`, `host`, `port`, `region`, `project`, `kubeconfig`, or `env.<VAR>`.

It is **deferred/lazy** (like `!terraform.output`) so it reads the live host port at use time,
erroring with a hint if the emulator isn't running. Registered in the YAML function registry
alongside `!store`/`!terraform.output`; resolves through `Manager.Resolve`.

## Public Interface

- Component kind: `components.emulator.<name>` with `driver`, `cloud`/target, `region`/`project`,
  nested `container:`.
- Identities: `aws/emulator`, `gcp/emulator`, `azure/emulator`, `kubernetes/emulator`, with
  `emulator: <name>`.
- Commands: `atmos emulator up|down|reset|ps|logs|exec` (+ `start`/`stop` aliases, `down --all`,
  `up --ephemeral`, `reset --force`). Persistence is on by default (`ephemeral: true` opts out).
- YAML function: `!emulator <ref> <key>` (space-separated; `<ref>` may be `<name>`,
  `<target>/<driver>`, or `<stack>/emulator/<name>`).

## Implementation Notes

- New package `pkg/emulator/` (all feature logic): `driver.go` (registry), built-in `driver_*.go`,
  `consumer.go` (pluggable consumer seam), `endpoint.go`, `manager.go` (over
  `pkg/container/lifecycle.go`), `resolver.go` (satisfies the auth + contributor seams; no `pkg/auth`
  import).
- Schema: `Emulator` kind in `schema.Components`; `EmulatorInstance` (driver/cloud/region/project +
  nested container config); auth `Identity.Emulator` field and the new identity kinds; update
  `pkg/datafetcher/schema/`.
- Reuse the container lifecycle by passing `ComponentType:"emulator"`. Host ports auto-assigned;
  live port via `Inspect`.
- `cmd/emulator/*` thin call sites (mirror `cmd/container/verbs.go`); register via `cmd/internal`
  command registry; blank import in `cmd/root.go`.
- Provider settings via the [provider-config contributor](provider-config-contributor.md).
- Kubernetes binding via [native Kubernetes identity](kubernetes-identity.md).
- Examples & docs (see §I) and CI tests.

### §I Examples, docs, CI (ship with the feature)

- **`quick-start-simple`** — unchanged (no-cloud `weather` example; the zero-dependency intro).
- **`quick-start-advanced`** — redesigned as an emulator-backed, fully CI-green showcase using only
  well-emulated services (S3, DynamoDB, SQS, SNS, IAM, KMS, SSM/SecretsManager). Demonstrates the
  documented patterns: catalog + inheritance + abstract components/mixins; multiple environments
  runnable locally; `dependencies`/DAG ordering; cross-component data via `!terraform.output` and
  stores + hooks (write to emulated SSM, read via `!store`); the `!emulator` function; OPA/JSON
  schema validation; vendoring. Wired to `components.emulator.aws` (driver `floci/aws`) + `aws/emulator`
  identity + the provider contributor.
- **`emulator-aws`** — migrated from its bespoke `atmos floci up/down` custom commands + docker-compose
  to the `components.emulator.aws` + `atmos emulator up/down` model. Minimal reference.
- **CI tests** — runtime-gated (reuse `tests/floci_containers_test.go` autostart; skip gracefully via
  `tests/test_preconditions.go`): `atmos emulator up` → `atmos terraform apply`/`destroy` across the
  redesigned advanced components against Floci; assert outputs and cross-component flow; tear down.
- **Docs** — website quick-start pages + `website/docs/cli/commands/emulator/*.mdx` (Intro +
  Screengrab + `<dl>` flags) for the `up → apply → inspect → down` workflow; `cd website && npm run build`.

## Test Plan

- **Unit (fake runtime, `tests/testhelpers/fake_container_runtime.go`):** label stamping, persistence
  (no teardown on exit), live-port discovery from faked `Inspect`, `Up/Down/Ps/Resolve` by-label,
  multiple concurrent targets (aws+gcp+az+k3s) in one stack.
- **Per-driver tables:** `Profile` for single-port (`floci/aws`, `ministack/aws`), multi-port (`floci/gcp`, `floci/az`),
  k3s/k3d/kind (kubeconfig), vault/registry (`VAULT_ADDR`/registry URL); mock `EmulatorDriver`.
- **Driver defaults merge:** explicit `container.image` beats the built-in default; `cloud` derived
  from `Target()` and only overridden when set.
- **`!emulator` function:** key resolution; deferred evaluation reads the live port at use time;
  not-running → actionable error.
- **Provider contributor:** emulator contributor deep-merges `Profile.Provider` into
  `ProvidersSection`; explicit stack `providers:` wins; no contribution when not emulator-bound.
- **Auth seam:** fake `EmulatorEndpointResolver`; `aws/emulator` → `AWS_ENDPOINT_URL` + dummy creds
  in `PrepareShellEnvironment` and `AWSAuthContext.EndpointURL`; negative path (no emulator).
- **Schema:** decode/round-trip for the `emulator` kind (nested `container:`) and identity kinds;
  isolation tests both directions for any merge.
- **Runtime-gated integration (Floci/k3s):** `atmos emulator up/ps/exec/down` against `floci/floci`
  (4566) and `rancher/k3s`; container survives process exit (re-discover by label, then `down`);
  persistence survives `down`/`up` and `reset` wipes it; concurrent aws+gcp+az+k3s; `atmos auth shell`
  for a k3s identity exports a working `KUBECONFIG`; quick-start-advanced E2E.
- Follow repo testing mandates: `cmd.NewTestKit(t)` for cmd tests, `filepath.Join`, no
  platform-specific binaries, `require.Positive` safety guards.

## Related

- [Kubernetes identity](kubernetes-identity.md) — the native, non-cloud-minted identity that binds
  the kubernetes emulator.
- [Provider-config contributor](provider-config-contributor.md) — the generic mechanism the emulator
  uses to set Terraform provider flags.
- [Container components](container-components.md) — the lifecycle this kind reuses.
- [Compositions](compositions.md) — emulators can be composition members.
