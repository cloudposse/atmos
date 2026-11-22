# Fix: Atmos Auth bugs from the v1.235.0 pre-release review (#3720)

**Date:** 2026-11-22

## Summary

A pre-release review of Atmos Auth (#3720) reported 27 confirmed bugs and 9 suspected ones. Every item
was re-checked against `main` at `2d7822f214`, where all 27 confirmed bugs still reproduced. This change
fixes all 27, plus 8 of the 9 suspected items, which turned out to be real; the remaining one is not a
bug. It also fixes the follow-up problems the first round found or left partial (section 6). Every fix
has a test that fails without it.

## Context

The review findings were recorded against `48111837d0`. Since then, `main` has merged the native Terragrunt
stack (#3561-#3600) and the saml2aws vendoring stack (#3722-#3726). The SAML provider and the vendored
`saml2aws` copy were out of scope; they were fixed in #3722.

Before any fix, each item was re-verified on the current code, with its current file and line. Each
confirmed item then got a reproducing test, which was run and seen to fail before the fix was applied.

The bugs fall into four groups:

- **Wrong identity or account.** The process cache and the keyring could hand out another identity's
  credentials. EKS contexts could share one exec user, and EKS cleanup ignored the region.
  `azure/pim-role` exported the parent's subscription.
- **Secrets printed in plain text.** `auth list --format json|yaml` printed passwords and inline keys.
  The UI channel stopped masking after a cast recording or `--ai` capture.
- **User files clobbered.** ECR/ACR login rewrote `~/.docker/config.json`. Azure login could reset the
  `az` MSAL cache, and an empty subscription cleared the `az` default.
- **Broken flows.** These include the 45-second limit on operation-scoped logins, the case-insensitive
  identity lookup, the `az` tenant/subscription conflict, the 80-column dialog overflow, and CLI argument
  handling.

## Changes

### 1. Regressions new since v1.234.1

- **`WithIsolatedIdentity` resolves case-insensitive names** (`pkg/auth/manager_isolated.go`). The name
  is resolved through `resolveIdentityName` first, as `Authenticate` already does.
- **No hard 45-second limit on operation-scoped login** (`manager_isolated.go`). The 45-second grace
  period starts only after the caller cancels or its deadline passes. A 30-minute hard ceiling remains.
- **SSO and webflow dialogs fit an 80-column terminal** (`pkg/auth/providers/aws/sso_ui.go`,
  `pkg/auth/identities/aws/webflow_ui.go`). The inline-URL budget now subtracts the box chrome, and the
  SSO instruction is split over two lines.
- **The plugin device-code URL is printed verbatim** (`pkg/plugin/adapter/auth/interactions.go`). It is
  plain text, not Markdown, so `a*b*c` is no longer rendered as `abc`.

### 2. High impact

- **The process credential cache is keyed on effective config** (`pkg/auth/manager_chain_fingerprint.go`).
  The key adds an HMAC fingerprint of each chain step's effective provider and identity config, using the
  per-process `pkg/auth/fingerprint`. A config that cannot be serialized bypasses the cache.
- **ECR and ACR login no longer rewrite `config.json`** (`pkg/auth/cloud/docker`).
  - The file is round-tripped losslessly as raw JSON, written atomically under the existing lock, and
    keeps its permissions and symlinks.
  - Routing follows docker/cli: `credHelpers[host]`, then `credsStore`, then inline `auths`. When a
    helper applies, the token is stored through `docker-credential-<name>`, the way `docker login` does.
    A helper failure returns `ErrDockerCredentialHelper` and leaves the file untouched.
  - `RemoveAuth` reverses exactly what `WriteAuth` did. The helper is behind a mockgen-generated
    `CredentialHelper` interface.
- **`azure/cli` passes only one of `--subscription` or `--tenant`** (`pkg/auth/providers/azure/cli.go`).
  It passes `--subscription` when one is set, otherwise `--tenant`.
- **`auth list --format json|yaml` redacts secrets** (`cmd/auth/list_redact.go`). `Provider.Password`
  and `Identity.Credentials` values are replaced with `[redacted]`, and output goes through `data.Write`.
- **EKS identities and accounts get their own kubeconfig user.**
  - `kube.ClusterInfo.Identity` was added, so the EKS user name is now
    `atmos-eks-<name>-<region>-<account>-<identity>`.
  - `Cleanup` also removes the legacy user name.
  - `RemoveClusterConfig` only removes clusters and users that no remaining context references. When
    clusters from several accounts match, cleanup picks the one used by this integration, or else
    returns `ErrEKSClusterAmbiguous`.

### 3. Medium

- **The `az` MSAL cache is never reset** (`pkg/auth/cloud/azure/cli_msal_cache.go`).
  - A UTF-8 BOM is stripped. An unparseable file returns `ErrAzureCLIMSALCacheUnparseable`, is left
    untouched, and logs a warning; authentication still succeeds.
  - Read, update and atomic write happen under one lock on `<cache>.lockfile`, the same file
    msal-extensions uses.
- **AKS and GKE exec entries set `ATMOS_CHDIR`** (`pkg/auth/cloud/{azure,gcp}/kube_exec_env.go`). This
  mirrors EKS (#3675) and is kept in its own commit (see Follow-ups).
- **`azure/pim-role` exports the scope's subscription** (`pkg/auth/identities/azure/pim_role.go`). A
  copy of the credentials carrying the `principal.scope` subscription is used for every
  post-authenticate step.
- **Keyring keys are unambiguous across realms** (`pkg/auth/credentials/store.go`).
  - Realm-scoped keys are now `atmos_<realm>__<alias>`. `realm.Validate` forbids `__` and a trailing
    `_`, so the first `__` always ends the realm.
  - The old layout is still read and deleted only when neither part contains `_`. Legacy cleanup never
    deletes another realm's entry. `docs/prd/auth-realm-architecture.md` is updated.
- **`auth env --format json` is unmasked** (`pkg/env/output.go`), the same as the bash, dotenv, env and
  github formats, unless masking was requested.
- **`-h`/`--help` after `--` reaches the child command** (`cmd/auth/helpers.go`). Arguments are
  truncated at `cmd.ArgsLenAtDash()`.
- **EKS cleanup matches the cluster ARN** (`pkg/auth/integrations/aws/eks.go`). It checks service,
  region and resource instead of only the `cluster/<name>` suffix.
- **`BuildAWSConfigFromCreds` uses `LoadIsolatedAWSConfig`** (`pkg/auth/cloud/aws/config.go`), so a
  stale `AWS_PROFILE` no longer breaks it.
- **Permission sets apply `principal.region`** (`pkg/auth/identities/aws/permission_set.go`). The
  credentials region comes from `resolveRegion()`, falling back to the SSO region when none is set.

### 4. Low

- **`auth exec` rejects arguments before `--`** (`cmd/auth/exec.go`), as `auth shell` already does.
- **A bare `auth logout --identity` prompts** (`cmd/auth/logout_target.go`) instead of looking up
  `__SELECT__`.
- **`ATMOS_IDENTITY` is ambient, not explicit** (`cmd/auth/identity_request.go`). It no longer conflicts
  with `logout --tags`, and `login` resolves in the order: explicit flag, `--tags`, then
  `ATMOS_IDENTITY`.
- **`--identity=false` / `ATMOS_IDENTITY=false` is honored.** `auth exec` runs the command
  unauthenticated, as documented. `login`, `logout`, `whoami`, `env`, `shell` and `console` return
  `ErrAuthDisabledIdentityRequired` with a hint.
- **SSO polling** (`pkg/auth/providers/aws/sso_poll.go`):
  - It waits on a context-aware timer and is bounded by the device-code expiry.
  - `slow_down` adds 5 seconds, per RFC 8628.
  - Terminal AWS errors keep their cause.
  - The old `TestPollForAccessToken_ContextCancellation`, which never called the function, is replaced.
- **GCP service accounts scrub the caller's GCP credential variables** (`pkg/auth/cloud/gcp/env.go`)
  before applying the identity's own.
- **An empty subscription ID leaves the `az` default alone** (`pkg/auth/cloud/azure/setup.go`).
- **ACR accepts `*.azurecr.us`, `*.azurecr.cn` and mixed-case names**
  (`pkg/auth/cloud/azure/acr.go`). `NormalizeRegistryURL` keeps the login server's own suffix.
- **AWS identities honor the provider's `spec.files.base_path`**. This applies to permission-set,
  assume-role and assume-root identities, through a new `filesBasePath` helper.

### 5. Suspected items

| Item | Result |
|---|---|
| `io.RegisterSecret` does not mask data/UI | Not as stated: startup rebinds both channels to the global masker, and a guard test now fails if that is removed. The real gap was `ui.ReinitFormatter`, which built a fresh masker after a cast recording or `--ai` capture. Fixed. |
| `ATMOS_CHDIR` in shared kubeconfig (#3675) | Real. Assessed only; see Follow-ups. |
| SSO token cache written non-atomically | Real. Writes are atomic, and the read and refresh exchange run under a cross-process lock. A best-effort fallback runs unlocked. |
| `WithIsolatedAWSEnv` without `envMu` | Real. One process-wide `LockAWSProcessEnv()` is shared by isolation, Atmos-managed config loading and the credentials loader. |
| `broker.EnsureCredentials` uses `sync.Once` | Real. A cancelled call is not recorded, a failure is retried after 30 seconds, and a success is refreshed after 30 minutes. |
| gcloud config name and properties path | Real. The active config is now `atmos`, and properties go to `configurations/config_atmos`. |
| WIF audience form | Not a bug. Google accepts the canonical resource name with or without the `https:` prefix. |
| `exchangeToken` accepts an empty `access_token` | Real. It now returns `ErrAuthenticationFailed`. |
| `auth whoami` exits 0 with expired credentials | Real. It now exits 1 in both formats. Documented in `auth-whoami.mdx`. |
| A signal-killed child exits 255 | Real. `pkg/shell` maps it to 128+N on Unix. |
| Plugin device-code flow bounded by `timeout` | Real. Fixed in section 6: the timeout no longer counts the user's sign-in wait. |

### 6. Follow-up fixes

The first round left these problems open or partial. Each fix has a test that fails without it.

- **Shared kubeconfig (#3675 follow-up).**
  - Kube user names include the auth realm when one is set, for EKS, AKS and GKE (for example
    `atmos-acme-eks-<name>-<region>-<account>-<identity>`), so repos with different realms never share
    a user. The empty realm keeps today's names. Realm-scoped users also carry `ATMOS_AUTH_REALM` in
    their exec env, so `kubectl` logs in with that realm's credentials.
  - A deleted `ATMOS_CHDIR` directory no longer breaks `kubectl`
    (`cmd/root_chdir_kube_exec.go`). When the value came from the `ATMOS_CHDIR` environment variable,
    the path is missing, and the command is `aws eks token`, `azure aks token` or `gcp gke token`,
    Atmos warns and runs from the current directory. An explicit `--chdir`, other commands, and a path
    that is a file still fail.
  - `update: error` is idempotent (`pkg/auth/cloud/kube/config_error_mode.go`). An identical re-run
    writes nothing, and a re-run that differs only in `ATMOS_CHDIR` refreshes it. A real conflict
    (server, CA, args, other env, context target) still errors, and users are now checked too.
- **AKS and GKE users per identity.** Both had the same shared-user bug as EKS item 2.5; their kube
  user names now include the identity. Cleanup removes the current, pre-realm and legacy names when no
  context references them.
- **Cross-process reuse of intermediate chain steps (rest of item 2.1).**
  - Cached credentials of an intermediate step are reused only when a SHA-256 digest of the non-secret
    config of every step from the chain root through that step still matches
    (`pkg/auth/manager_chain_step_digest.go`). A mismatch re-authenticates the step
    (`ErrChainStepConfigChanged`).
  - Keyring entries carry the digest in the credential envelope (`config_digest`, ignored by older
    Atmos). Credentials kept in an identity's own files get a digest record tied to their expiration.
  - Secrets are never hashed: provider `password`, identity `credentials`, and `principal`/`spec` keys
    that look like secrets. `TestPersistedStepConfig_CanonicalFields` pins the hashed fields.
  - Stores that persist nothing (the noop keyring) keep name-only reuse.
- **Keyring migration.** A legacy `atmos_<realm>_<alias>` entry read through the unambiguous fallback is
  rewritten under the new key and the legacy key is deleted, best effort and only when the new key is
  absent. Ambiguous legacy keys are still never read.
- **Sovereign-cloud ACR.**
  - The cloud comes from the login-server suffix (`azurecr.io` public, `azurecr.us` US Government,
    `azurecr.cn` China). Explicit `--registry` login requests the ambient token from that cloud's
    authority and scope, validates every registry before logging in to any, and handles mixed clouds.
  - Integrations build the login server from the provider's `cloud_environment`. Logout removes all
    three suffixes, because it has no credentials to tell the cloud. Plugin registry auth recognizes
    `.us` and `.cn` too.
- **Atmos's per-realm MSAL cache** (`pkg/auth/cloud/azure/msal_cache.go`).
  - An unparseable cache is never reset or overwritten; authentication still works without cached
    tokens. A BOM is stripped.
  - Export re-reads the file under the lock and merges only the entries MSAL changed, then writes
    atomically, so entries other processes (including `az`) added are kept.
  - It locks `<cache>.lockfile` (as `az` does) and then the legacy `<cache>.lock`, so new and old Atmos
    versions do not interleave writes.
- **Plugin device-code timeout.** `plugins.packages.<alias>.timeout` now counts only plugin work.
  - The budget pauses while the host waits for the user, after the device prompt and during the browser
    callback (`pkg/plugin/runtime/native/interaction.go`).
  - A hard deadline of the timeout plus 15 minutes still ends a stalled sign-in, with a hint.
  - Non-interactive calls keep the fixed deadline.
- **Remaining AWS `base_path` gaps.** The plugin adapter's AWS materializer and the mock AWS identity
  honor the provider's `spec.files.base_path`.
- **Docker.**
  - If saving `config.json` fails after a successful helper store, the stored credential is erased
    from the helper.
  - A warning is printed when `DOCKER_AUTH_CONFIG` takes precedence over `config.json`.
  - Registry keys are normalized as docker/cli does: scheme, path and trailing slash are stripped, and
    the port is kept. Logout removes legacy `https://host` style entries through the helper Docker uses
    for each key.
- **Docs.** The EKS, AKS and GKE pages show the kube user names the code writes. The ACR page has a
  "Sovereign Clouds" section. The plugin pages describe the new timeout behavior. The ACR agent-skill
  reference and the AKS/ACR PRD are updated.

New sentinels: `ErrDockerCredentialHelper`, `ErrAzureCLIMSALCacheUpdate`,
`ErrAzureCLIMSALCacheUnparseable`, `ErrAzureMSALCacheUnparseable`, `ErrEKSClusterAmbiguous`
(`errors/errors_integrations.go`), plus `ErrAuthDisabledIdentityRequired` and `ErrChainStepConfigChanged`
(`errors/errors_execution.go`).

## Validation

All commands were run on the integrated branch, based on `2d7822f214`. The two coverage tests added last
(`cmd/azure/acr`, `pkg/env`) were run on their own after the full race run.

- `go build ./...`: passed.
- `go vet` passed on every touched package, including with `GOOS=windows` and `GOOS=linux`.
- `gofumpt -l` reported no files on the changed Go files, and `go mod tidy -diff` is clean.
- `go test -race -count=1` passed for 92 packages: `pkg/auth/...`, `cmd/auth/...`, `cmd/aws/...`,
  `cmd/azure/...`, `pkg/env`, `pkg/plugin/adapter/auth`, `pkg/shell`, `pkg/ui`, `pkg/startup`,
  `pkg/store/authbridge`, `pkg/stack/imports`, `pkg/downloader`, `pkg/runner/step`, `pkg/ai/analyze` and
  `errors`. There were no data races.
- `go test ./tests -run 'TestCLICommands/(atmos_auth|auth|atmos_aws|atmos_azure)'`: 29 snapshot tests
  passed, with no regeneration needed.
- Every reproducing test was run on the unfixed code and failed before its fix was applied.
- Patch coverage of new and changed statement lines is 96.5% (870/902). The uncovered lines are
  unreachable marshal errors, an `os.Exit` branch, Linux-only `Getwd` failure paths and an interactive
  TTY save.
- Package coverage, `main` to this branch:

| Package | Before | After |
|---|---|---|
| `cmd/auth` | 82.2% | 83.2% |
| `cmd/azure/acr` | 88.6% | 89.9% |
| `pkg/auth` | 91.7% | 91.8% |
| `pkg/auth/broker` | 76.0% | 100.0% |
| `pkg/auth/cloud/aws` | 93.0% | 93.1% |
| `pkg/auth/cloud/azure` | 95.0% | 95.0% |
| `pkg/auth/cloud/docker` | 95.7% | 98.8% |
| `pkg/auth/cloud/gcp` | 88.5% | 88.6% |
| `pkg/auth/cloud/kube` | 91.7% | 92.6% |
| `pkg/auth/credentials` | 88.7% | 89.7% |
| `pkg/auth/identities/aws` | 90.8% | 90.9% |
| `pkg/auth/identities/azure` | 89.7% | 89.9% |
| `pkg/auth/identities/gcp_service_account` | 88.1% | 88.1% |
| `pkg/auth/integrations/aws` | 93.8% | 94.0% |
| `pkg/auth/providers/aws` | 86.7% | 87.1% |
| `pkg/auth/providers/azure` | 78.7% | 79.1% |
| `pkg/auth/providers/gcp_wif` | 96.6% | 96.6% |
| `pkg/env` | 98.2% | 98.2% |
| `pkg/plugin/adapter/auth` | 95.9% | 95.9% |
| `pkg/shell` | 88.3% | 88.8% |
| `pkg/ui` | 91.4% | 91.4% |

Follow-up fixes (section 6), run on the combined branch:

- `go build ./...` passed, and `go vet` passed on the touched packages with the host OS, `GOOS=windows`
  and `GOOS=linux`. `gofumpt -l` reported no files, and `go mod tidy -diff` is clean.
- `go test -race -count=1` passed for 131 packages, with no failures and no data races. The scope was
  `pkg/auth/...`, `pkg/plugin/...`, `cmd/auth/...`, `cmd/aws/...`, `cmd/azure/...`, `cmd/gcp/...`,
  `pkg/env`, `pkg/shell`, `pkg/ui`, `pkg/startup`, `pkg/store/authbridge`, `pkg/config/schema`,
  `pkg/datafetcher`, the agent-skill packages and `errors`.
- `go test -race -count=1 -timeout 45m ./cmd/` passed in 929 seconds. It covers the `ATMOS_CHDIR`
  fallback in the root command.
- `go test ./tests -run 'TestCLICommands/(atmos_auth|auth|atmos_aws|atmos_azure|atmos_gcp)'`: 29
  snapshot tests passed, with no regeneration needed.
- Patch coverage of new and changed statement lines, relative to the first round, is 99.5% (880/884),
  measured without the root `cmd` package. The 4 uncovered lines cannot be reached: a
  `CloudEnvironmentForRegistry` error after the registry was already validated, and a `MarshalIndent`
  error on just-decoded JSON. For the root `cmd` package, the chdir tests raise
  `processEarlyChdirFlag` from 76.0% to 88.5% and `processChdirFlag` from 85.2% to 89.3%.
- Package coverage, first round to this branch:

| Package | Before | After |
|---|---|---|
| `cmd/aws/eks` | 87.8% | 87.9% |
| `cmd/azure/acr` | 89.9% | 90.8% |
| `cmd/azure/aks` | 95.3% | 95.4% |
| `pkg/auth` | 91.8% | 92.3% |
| `pkg/auth/cloud/aws` | 93.1% | 93.1% |
| `pkg/auth/cloud/azure` | 95.0% | 96.2% |
| `pkg/auth/cloud/docker` | 98.8% | 99.1% |
| `pkg/auth/cloud/gcp` | 88.6% | 88.9% |
| `pkg/auth/cloud/kube` | 92.6% | 94.5% |
| `pkg/auth/credentials` | 89.7% | 91.9% |
| `pkg/auth/identities/aws` | 90.9% | 90.9% |
| `pkg/auth/integrations` | 94.3% | 94.3% |
| `pkg/auth/integrations/aws` | 94.0% | 94.9% |
| `pkg/auth/integrations/azure` | 93.6% | 94.4% |
| `pkg/auth/integrations/gcp` | 90.0% | 90.2% |
| `pkg/auth/providers/mock/aws` | 77.9% | 93.2% |
| `pkg/auth/registryauth` | 95.5% | 95.9% |
| `pkg/plugin/adapter/auth` | 95.9% | 96.3% |
| `pkg/plugin/bridge` | 94.1% | 94.2% |
| `pkg/plugin/runtime/native` | 91.2% | 93.1% |

Not run:

- `golangci-lint` was permission-denied in this environment, and git hooks are disabled in this clone,
  so CI lint is the first lint pass.
- No tests were run on Windows or Linux, only cross-OS `go vet`.
- The website build was not run (`website/node_modules` is not installed).

## Follow-ups

The follow-up problems from the first round are fixed in section 6. These items remain open and need a
decision before they become fixes or issues:

- **`spec.registry.tenant_id` is documented but unused.** `acr-login.mdx` says it overrides the token
  exchange tenant, but no code reads it; the exchange always uses the credentials' tenant. This
  predates this change.
- **Default kube context names are shared across realms.** The default context name is the cluster ID,
  so two realms that write the same cluster without an alias share one context, and the last writer
  picks which (realm-correct) user it points to. Cleanup in one realm also deletes that context by name.
  Options: realm-qualified context names (breaking), or removing a context only when it points at one of
  this integration's users.
- **Plugin interaction limit.** The 15-minute sign-in limit is a `Definition` field and a Go option,
  not configuration. Letting plugins report the device-code expiry would need a protocol change.
- **Ambiguous legacy keyring entries.** `aws/user` long-lived keys stored with a realm by an older
  version, where the realm or identity name contains `_`, are not migrated (the legacy key could belong
  to another realm). Those users must re-run `atmos auth user configure`.

Compatibility notes for users:

- Kubeconfig user names change: AKS and GKE names gain `-<identity>`, and every name gains `<realm>-`
  after `atmos-` when a realm is set. Context names are unchanged, so `kubectl --context` keeps working.
  The next login repoints each context to its new user, and the old user is removed at the next logout.
  Scripts that hardcode `atmos-aks-...` or `atmos-gke-...` user names must be updated.
- Realm-scoped keyring keys change to `atmos_<realm>__<alias>`; unambiguous legacy entries migrate on
  first read.
- After upgrading, each intermediate chain step re-authenticates once, because entries written by older
  versions have no config digest.
- `atmos auth whoami` exits 1 for expired credentials, and `--identity=false` disables auth as
  documented.
- ECR/ACR login stores tokens through a configured Docker credential helper and fails loudly if it
  cannot.
