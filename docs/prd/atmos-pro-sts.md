# PRD: Atmos Pro STS — JIT GitHub token broker for CI (`atmos/pro` provider + `github/sts` integration)

## Executive Summary

Add two new `atmos auth` kinds that let CI authenticate to private GitHub repositories with
just-in-time, least-privilege, short-lived tokens — with **zero `.tf` changes** and no long-lived
PATs or deploy keys:

1. **Provider `kind: atmos/pro`** — authenticates the Atmos CLI *to Atmos Pro* by federating the
   GitHub Actions runner's OIDC token (audience = Atmos Pro) through the existing
   `POST /api/v1/auth/github-oidc` endpoint, yielding a `ws:gh:action` session JWT cached in the
   keyring. v1 is OIDC-only.
2. **Integration `kind: github/sts`** — the Git-credentials analog of `aws/ecr`/`aws/eks`. On auth
   (auto-provision on by default) it calls `POST /api/v1/sts`, materializes the minted GitHub App
   installation tokens as per-owner `GIT_CONFIG_*` URL rewrites for child processes, and revokes
   each token directly against GitHub on `atmos auth logout` and (in CI) at command-end.

Per Cloud Posse's "if it could be a GitHub Action, it belongs in Atmos CLI core" principle, this is
built into the CLI — it is CI-native and OIDC-aware — rather than shipped as a GitHub Action.

## Problem Statement

Fetching private Terraform modules, Atmos `source:` components, and vendored artifacts in CI today
requires a GitHub credential with broad, long-lived scope (a PAT, a machine user, or a deploy key),
distributed as a CI secret. These credentials are over-privileged, rarely rotated, and a standing
breach risk. There is no clean way to scope a token to exactly the repositories a given run needs,
for exactly the duration of that run.

Atmos already federates GitHub Actions OIDC to Atmos Pro for telemetry (`pkg/pro`), and already
injects a single host-scoped token for go-getter operations (`pkg/downloader/custom_git_detector.go`).
But a single token cannot express *multiple* per-owner installation tokens, and Terraform's native
`git` (used by `terraform init` for `git::https://…` modules) never passes through Atmos's downloader.

## Design Goals

1. **Zero `.tf` changes.** The same minted credentials transparently authenticate `atmos vendor pull`,
   Atmos `source:` components, and `terraform init` of private `git::https://…` modules.
2. **Least privilege, deny-by-default.** Identity is derived server-side; the CLI never sends
   owner/repo. Empty grants with everything excluded is a normal outcome.
3. **Short-lived + revocable.** Tokens are revoked at command-end (CI) and on logout; they expire
   regardless.
4. **Model on existing integrations.** `github/sts` mirrors `aws/ecr` (Execute persists secret
   material to a deterministic on-disk location; Environment returns a pointer; Cleanup removes +
   revokes).
5. **Realm isolation.** Minted tokens are isolated per repository/customer environment, like keyring
   credentials.

## Wire Contract (frozen; built to it)

### Step 0 — OIDC → session (existing, reused)
`POST {base_url}/api/v1/auth/github-oidc` with `{ "token": "<GH Actions OIDC JWT>", "workspaceId": "<id>" }`.
The field is **`token`** (this PRD corrects the original diagram's `oidcToken`). The OIDC token is
**single-use** server-side — the CLI mints it once and never retries the exchange with it. Response:
`{ "token": "<ws:gh:action JWT>", ... }`. Implemented by `pkg/pro.ExchangeOIDCToken` /
`pkg/pro.MintGitHubOIDCToken` (audience parameterized; default `atmos-pro.com`).

### Step 1 — mint (new)
`POST {base_url}/api/v1/sts`, `Authorization: Bearer <ws:gh:action JWT>`. Body (both optional):
`{ "sources": ["acme/modules"], "policyName": "default" }`. Response:

```jsonc
{ "tokens": [ { "host": "github.com", "owner": "acme", "token": "ghs_…",
               "expiresAt": "<ISO8601>", "repositories": ["acme/modules"],
               "permissions": { "contents": "read", "metadata": "read" } } ],
  "excluded": [ { "repo": "other/repo", "reason": "no_trust_policy" } ] }
```

One token per `(installation, permission-set)` → **iterate `tokens[]`, expect multiple**.
`excluded[].reason` ∈ `not_installed_in_workspace | no_trust_policy | trust_policy_denied |
trust_policy_invalid | installation_suspended` — surfaced via `log.Warn` at mint. (This PRD adds the
`permissions` map and `installation_suspended` reason that shipped in code but weren't in the original
example.) **400** = not a GH Actions session; **403** = no `sts` entitlement / workspace toggle off.
Empty `tokens[]` with everything excluded is a **normal** deny-by-default success. Token values are
never logged.

### Consumption (env-only)
Per-owner `insteadOf` rewrites are injected into the child env so each token is scoped to its owner's
URLs (covers `git::https://…`, shorthand, and `ssh://`):

```
url.https://x-access-token:${TOKEN}@github.com/acme/.insteadOf = https://github.com/acme/
                                                       (and)    = ssh://git@github.com/acme/
```

Two materialization modes (configurable; see Configuration):
- `env` (default): inline tokens via `GIT_CONFIG_KEY_n`/`GIT_CONFIG_VALUE_n` + `GIT_CONFIG_COUNT`.
- `file`: a `0600` gitconfig is written and an additive `include.path` is emitted via `GIT_CONFIG_*`
  (the `insteadOf` rewrites stay off the environment — safer for local dev; note the single-owner
  `token_env` bridge below still exports `ATMOS_PRO_GITHUB_TOKEN` by default so in-process git reads
  work).

### Raw-token export (`token_env`) — Octo-STS parity
The `GIT_CONFIG_*` rewrites only authenticate `git`. To use the minted token with non-git consumers
(`gh` CLI, `actions/checkout`, the GitHub REST API) — the [Octo-STS](https://github.com/octo-sts/action)
use case — `Environment()` also emits the raw token under the `spec.token_env` name-pattern, so
`atmos auth env --format github` writes it to `$GITHUB_ENV` (or `$GITHUB_OUTPUT` via `--output-file`)
for consumption by subsequent workflow steps. **`token_env` defaults to `ATMOS_PRO_GITHUB_TOKEN`**
(was empty): this bridges the single-owner token to non-git consumers AND to Atmos's own in-process git
detector (`pkg/downloader` `resolveToken` reads `ATMOS_PRO_GITHUB_TOKEN` live), so `github/sts`
composes with the default `settings.inject_github_token: true` with no workaround. A literal name
requires exactly one minted token — multi-owner mints skip the bare var (`log.Debug` when the value is
the implicit default, `log.Warn` when the user set an explicit literal) and rely on the `GIT_CONFIG_*`
rewrites; a Go-template name referencing `.owner`/`.host` (e.g. `GH_TOKEN_{{ .owner }}`) is rendered
per owner (result sanitized: uppercased, non-alphanumerics → `_`). An explicit `spec.token_env`
overrides the default. Token values are never logged.

### Avoiding broker/injection shadowing
For Atmos-native go-getter reads (vendor pull, `source:`, remote `import:`), `CustomGitDetector` would
otherwise inject `x-access-token:<TOKEN>@` into the URL; once the URL carries userinfo, git's
`insteadOf` left-side no longer prefix-matches and the broker's correct least-privilege token is
silently shadowed by the ambient `GITHUB_TOKEN`. The detector now scans the live `GIT_CONFIG_*` env for
an `insteadOf` rewrite matching the URL's host/owner and **skips URL injection when one matches**, so
git's rewrite wins. This is host/owner-scoped: a repo under a non-minted owner still gets the ambient
token injected as before. (env mode only — file mode keeps its rewrites in the gitconfig file, reached
via the `ATMOS_PRO_GITHUB_TOKEN` bridge above.)

### Revoke (client-direct)
`DELETE https://api.github.com/installation/token` with the minted token as Bearer (401/404 treated
as already-revoked). No server revoke route in v1.

## Configuration

```yaml
auth:
  providers:
    atmos-pro: { kind: atmos/pro, spec: { base_url: ..., workspace_id: ..., audience: atmos-pro.com } }
  identities:
    atmos-pro: { kind: atmos/pro, via: { provider: atmos-pro } }
  integrations:
    github-sts:
      kind: github/sts
      via: { provider: atmos-pro }   # binds to the provider directly; via: { identity: atmos-pro } also works
      spec:
        auto_provision: true         # mint on login AND lazily in CI before the first remote read (default true)
        repos: [acme/modules]        # optional sources[]
        policy_name: default         # optional
        git_config_mode: env         # env | file (overrides settings.pro.git_sts.git_config_mode)
        revoke_on_exit: true         # overrides settings.pro.git_sts.revoke_on_exit
        token_env: GH_TOKEN          # optional: export raw token as a named env var (Octo-STS parity); default ATMOS_PRO_GITHUB_TOKEN

settings:
  pro:
    git_sts:
      git_config_mode: env           # global default
      revoke_on_exit: true           # global default
```

Read-only is implicit — no `scopes`/`permissions` in consumer config. The provider reuses today's
`settings.pro` (`base_url`, `workspace_id`) + `ATMOS_PRO_*` env, with `spec.*` as a thin override.

### `ATMOS_PRO_GITHUB_TOKEN`
A new setting/env var consumed by Atmos-native go-getter operations (vendor pull, source provisioning,
imports) via `CustomGitDetector`, with precedence `ATMOS_PRO_GITHUB_TOKEN` > `ATMOS_GITHUB_TOKEN` >
`GITHUB_TOKEN`. This is a single-token convenience complementing the multi-owner `GIT_CONFIG_*` path
(which is what covers Terraform's native git).

### `via.provider` on integrations
Integrations may bind to a provider (`via.provider`) instead of an identity (`via.identity`). The
manager matches a provider-bound integration to any identity whose root provider matches. Exactly one
of `via.identity`/`via.provider` must be set. This removes the need to name a redundant passthrough
identity for the Atmos Pro case (the identity is still defined to authenticate/run as).

### CI auto-provision (no stack claims the identity)
The `atmos/pro` identity is not a cloud deployment identity, so no stack/component claims it the way
stacks claim an AWS/Azure/GCP identity. Without a claim, the `github/sts` integration would never be
provisioned during a normal `atmos terraform`, `atmos vendor pull`, or stack-import run, and Atmos
itself could not read private repos. To close this gap, Atmos provisions `github/sts` **lazily and
ambiently** the first time it is about to fetch a remote git source:

- A generic **credential-broker registry** (`pkg/auth/broker`) is consulted at the go-getter choke
  point (`downloader.NewClient` for remote sources) and just before the terraform/helmfile/packer
  subprocess env is assembled. It runs registered brokers **at most once per process** (`sync.Once`).
- The **Atmos Pro broker** (`pkg/auth/providers/atmospro/broker`, registered via blank import in
  `cmd`) is enabled only when `telemetry.IsCI()` **and** the auth config defines an `atmos/pro`
  provider + a `github/sts` integration with `auto_provision != false`. It authenticates the
  `atmos/pro` identity (cached-session-first — the OIDC token is single-use, so it never re-mints
  OIDC when a session is cached), provisions `github/sts`, and exports the resulting `GIT_CONFIG_*`
  into the process via `os.Setenv`.
- Because the in-process `git` subprocess (go-getter's `CustomGitGetter`) and the
  terraform/helmfile/packer subprocesses all inherit the process env, a single `os.Setenv` makes the
  per-owner `insteadOf` rewrites apply to **both** Atmos's own reads and Terraform's native git.
- **Reuse:** the integration's `Execute` short-circuits the network mint when persisted tokens are
  still fresh (every `expiresAt` in the future), so repeated `atmos` invocations within one CI job
  reuse the same tokens instead of re-minting.

This is gated purely on config presence + CI — there is no new `settings.pro.enabled` field.

## Lifecycle

`atmos auth login --identity atmos-pro` → session + auto-provision (mint) →
`atmos terraform` / `atmos vendor pull` / `atmos auth exec` inject `GIT_CONFIG_*` (and consume
`ATMOS_PRO_GITHUB_TOKEN` for go-getter) → command-end auto-revoke (`atmos auth exec`, CI +
`revoke_on_exit`) → `atmos auth logout` revokes via `Cleanup`. The workflow declares only
`permissions: id-token: write`.

In CI, no explicit `atmos auth login` is required: the first remote git read (vendor pull, a
`source:` component, a remote `import:`, or `terraform init` of a private `git::https://…` module)
triggers the ambient broker, which authenticates the `atmos/pro` identity and provisions `github/sts`
transparently — even though no stack claims that identity.

## Implementation Notes

- New: `pkg/auth/providers/atmospro`, `pkg/auth/identities/atmospro`, `pkg/auth/integrations/github`,
  `pkg/auth/types/pro_credentials.go` (registered in all keyring backends as type `atmos-pro`).
- New: `pkg/auth/broker` (generic ambient credential-broker registry; depends only on
  schema/log/perf so `pkg/downloader` can call it cycle-free) and
  `pkg/auth/providers/atmospro/broker` (the Atmos Pro `github/sts` broker). Wired at
  `downloader.NewClient` (remote sources only) and in `ExecuteTerraform` before the subprocess env is
  built. `auth.NewDefaultManager` is the package-level manager constructor the broker uses;
  `AuthManager.EnsureIdentityEnvironment` is the cached-first authenticate-and-provision primitive.
- Realm is plumbed into integrations via `integrations.IntegrationConfig.Realm` (populated at the three
  `integrations.Create` call sites); `github/sts` realm-scopes its state under
  `<xdg data>/atmos/auth/github-sts/<realm>/<integration>/`.
- Command-end auto-revoke is wired into `atmos auth exec` (a clean, non-nested bracket). It is gated on
  `telemetry.IsCI()` and the resolved `revoke_on_exit` (spec → global → default true).

## Security

- Token values are never logged; secret material is written `0600` and removed on cleanup.
- Realm isolation prevents cross-repository credential reuse.
- SSRF protections on the OIDC request URL are inherited from the existing `github/oidc` provider.

## Testing

Unit tests cover the provider (mint+exchange, default/override audience, missing workspace, error
paths), the passthrough identity, the integration (`Execute` with multiple/empty tokens and 400/403
errors, `Environment` in both modes, `Cleanup` revocation + idempotency, realm isolation, `0600`
perms), keyring round-trip, `via.provider` matching, `revoke_on_exit` resolution, and
`ATMOS_PRO_GITHUB_TOKEN` precedence. The ambient CI auto-provision path adds: broker registry
(enabled-only execution, env export, process-once, non-fatal errors), the Atmos Pro broker
(`Enabled` gating, `findProGitHubSTSIdentity` via.identity/via.provider resolution),
`EnsureIdentityEnvironment` (cached-first, error wrapping), `hasFreshState` reuse (fresh/expired/
unparseable/file-mode), and `isRemoteSource` classification. E2E is staged on
`cloudposse/infra-test`: one PR, one workflow with only `id-token: write`, exercising vendor pull +
a `source:` component + `terraform init` of a private module with **no stack claiming the atmos/pro
identity**, then confirming revocation.

## Future Work (out of scope for the initial PR)

These were explicitly deferred:

1. **Make `auth` the canonical home for Atmos Pro connection/auth config.** Today `settings.pro`
   (`base_url`, `workspace_id`, `token`, `github_oidc`) is the source of truth and is consumed by the
   non-auth Pro features (affected-stacks upload, locks, list instances, drift detection). A future
   change should let `auth.providers.atmos-pro.spec` be the canonical home with a shared resolver and
   precedence `auth.providers.atmos-pro.spec` → `settings.pro` (+ env) → defaults, while `settings.pro`
   retains the non-auth feature config (endpoint, drift detection, payload size, run IDs).
2. **Unify `pkg/pro` onto auth-issued sessions.** `pkg/pro` currently runs its own OIDC exchange. It
   should instead consume the `atmos/pro` identity's session JWT from the auth manager, eliminating the
   second authentication path.
3. **Extend command-end auto-revoke to `atmos terraform` / `vendor pull` / workflows / custom commands.**
   Deferred because `atmos terraform` invokes nested Terraform internally (`!terraform.output`,
   `!terraform.state`); safe command-end revocation there must guard against revoking tokens an outer
   invocation still needs (revoke only at the outermost invocation). In v1 these paths rely on logout +
   natural token expiry.
4. **Honor the global `settings.pro.git_sts.git_config_mode` default inside the integration.** The
   integration currently resolves `git_config_mode` from spec (default `env`); honoring the global
   default requires plumbing `atmosConfig` (or the resolved GitSTS defaults) into integrations.
5. **Multiple `github/sts` integrations per identity** would collide on `GIT_CONFIG_*` numeric indices
   in `composeEnvironmentVariables`. v1 assumes a single github/sts integration; a future change can
   renumber/offset indices.
6. **Surface `excluded[]` in `whoami`/`validate`.** v1 surfaces deny reasons via `log.Warn` at mint
   only.
