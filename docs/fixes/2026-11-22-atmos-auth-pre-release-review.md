# Fix: Atmos Auth bugs from the v1.235.0 pre-release review (#3720)

**Date:** 2026-11-22

## Summary

A pre-release review of Atmos Auth (#3720) reported 27 confirmed bugs and 9 suspected ones. Every item
was re-checked against `main` at `2d7822f214`, where all 27 confirmed bugs still reproduced. This change
fixes all 27, plus 7 of the 9 suspected items, which turned out to be real. One suspected item is real
and is documented rather than changed, and one is not a bug. Every fix has a test that fails without it.

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
| Plugin device-code flow bounded by `timeout` | Real. Documented in `plugins.mdx` and `plugin/usage.mdx`; no code change. |

New sentinels: `ErrDockerCredentialHelper`, `ErrAzureCLIMSALCacheUpdate`,
`ErrAzureCLIMSALCacheUnparseable`, `ErrEKSClusterAmbiguous` (`errors/errors_integrations.go`), and
`ErrAuthDisabledIdentityRequired` (`errors/errors_execution.go`).

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

Not run:

- `golangci-lint` was permission-denied in this environment, and git hooks are disabled in this clone,
  so CI lint is the first lint pass.
- No tests were run on Windows or Linux, only cross-OS `go vet`.
- The website build was not run.

## Follow-ups

These items predate this change (they come from already-merged features) and need a decision before they
become fixes or issues:

- **`ATMOS_CHDIR` in the shared kubeconfig (#3675).**
  - The absolute working directory is written into `~/.config/atmos/kube/config`. The repo or worktree
    that writes it last wins for every context, and a deleted directory breaks `kubectl` auth.
  - `update: error` mode fails on every re-run, with or without `ATMOS_CHDIR`.
  - A correct design scopes the kubeconfig per repo or realm, falls back when the directory is missing,
    and compares entries while ignoring `ATMOS_CHDIR`. The AKS/GKE commit can be dropped if this design
    replaces it.
- **The cross-process cache of intermediate chain steps (part of item 2.1) still keys on identity names.**
  The target step is always re-authenticated, so the worst case is a stale source principal, not
  wrong final-role credentials. Closing it means storing a digest of each step's non-secret config in
  the credential envelope.
- **Keyring migration.** Old-layout keys are read only when neither the realm nor the identity contains
  `_`. Other users get one cache miss; for `aws/user` long-lived keys under such names, that means
  re-running `atmos auth user configure`.
- **Sovereign ACR is partial.** Registries are no longer rejected or rewritten, but explicit `--registry`
  login still requests a public-cloud token, and integrations configured by name build `*.azurecr.io`.
- **AKS has the same shared-user problem as EKS (item 2.5).** Its kubeconfig user name has no identity.
- **The Atmos-owned per-realm MSAL cache** (`msal_cache.go`) still uses `.lock` and starts fresh on
  unparseable data.
- **The plugin device-code timeout** is documented only. Excluding the interactive wait needs a change
  in `pkg/plugin/runtime/native`.
- **The website EKS docs** (`aws-eks-token.mdx`, `eks-kubeconfig-authentication.mdx`) show kubeconfig
  user names that did not match the code before this change either.
