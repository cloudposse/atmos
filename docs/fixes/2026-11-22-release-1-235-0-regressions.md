# Fix: Regressions found by the 1.235.0 regression test against v1.234.1

**Date:** 2026-11-22

## Summary

A regression and field test compared the 1.235.0 candidate with v1.234.1, the latest published release.
It covered Atmos Auth, auth integrations, the Terraform runtime, configuration and the CLI surface, and
the other runtimes. It found 15 confirmed regressions. Eight came from the Atmos Auth review fixes on
this branch; the other seven came from changes already on `main`. All 15 are fixed here.

The same pass found several bugs that already existed in v1.234.1; the user chose which to fix now.
Those fixes are also here, and the rest are listed under Follow-ups. Every fix has a test that failed on
the unfixed code.

## Context

The comparison was frozen at two commits:

- Baseline: v1.234.1 at `b148adc244`, published 2026-11-10.
- Candidate: branch `aknysh/fix-auth-review-3720` at `c0f3af7f57`, which is `main` at `2d7822f214`
  plus the #3720 review fixes.

Both binaries were rebuilt from those SHAs with the same toolchain: Go 1.27.4, CGO off, the release
ldflags. Five investigators each covered one area and ran both binaries on identical fixture copies:

- Atmos Auth commands.
- Auth integrations and credential files.
- Terraform/OpenTofu execution.
- Configuration, stacks and the CLI surface.
- Helmfile, Packer, containers, git, CI and docs generate.

Every run used an isolated HOME, XDG directories, a file keyring and its own Git root under `env -i`.
Local fakes or emulators stood in for STS, EKS, ECR, Kubernetes, Vault, S3 and the CI runners, and no
live cloud was used. Each finding was reproduced at least twice from clean state. Evidence stayed in the
local `.context/regression-test/` run directory and is not committed.

Two root causes explain most of the Atmos Auth regressions:

- When no realm is configured, Atmos sets the auth realm to a hash of the `atmos.yaml` path. So every
  user is realm-scoped. Code written for an explicitly configured realm also applied to that automatic
  hash: kube names were prefixed and legacy keyring keys were treated as ambiguous.
- The native Terragrunt work introduced configuration providers. Their selection and ownership checks
  read user-editable configuration (`settings.configuration`, unknown sections under
  `components.terraform`) as provider markers.

## Changes

### Regressions introduced by the Atmos Auth review fixes

- **A1. Kube names are realm-scoped only for an explicitly configured realm**
  (`pkg/auth/realm/explicit.go`, `pkg/auth/cloud/kube`, the EKS/AKS/GKE integrations and the
  `update-kubeconfig` commands).
  - The realm prefix on kube user and default context names, and `ATMOS_AUTH_REALM` in the exec env,
    now apply only when the realm comes from `auth.realm` or `ATMOS_AUTH_REALM`.
  - Without a configured realm, names have the v1.234.1 shape again, so `kubectl --context <cluster-arn>`
    keeps working.
  - `IntegrationConfig` carries the realm source. Contexts and users written under the automatic hash by
    candidate builds are treated as the integration's own and are migrated on the next login or logout.
- **A2. Legacy keyring keys migrate silently under the automatic realm**
  (`pkg/auth/credentials/keyring_legacy_auto_realm.go`, `pkg/auth/realm`).
  - The automatic realm is 16 lowercase hex characters. So the only other owner of a legacy key
    `atmos_<hash>_a_b` would be a configured realm named `<hash>_a`.
  - Such keys are now read, migrated, deleted and listed like unambiguous keys. Previously every
    `aws/user` identity with `_` in its name failed in CI until `auth user configure` was re-run.
  - An explicitly configured realm, including one with the hex format, keeps the confirmation prompt.
- **A3. Keychain logout deletes the ambiguous legacy key** (`pkg/auth/manager_logout_keyring.go`,
  `cmd/auth/logout_all_realms_keyring.go`).
  - `auth logout --keychain`, including `--all`, provider logout and `--all-realms`, removes the
    identity's previous-layout key, as v1.234.1 did. It logs a warning naming the other realm and
    identity the key may have belonged to.
  - Previously logout reported success but kept the key, and a later login offered to revive the
    deleted credentials.
- **A4. Logout never auto-picks an identity without a TTY** (`cmd/auth/logout_target.go`,
  `cmd/auth/logout_interactive.go`).
  - A bare `auth logout --identity`, or a plain `auth logout`, fails with a "requires a TTY" error when
    no interactive selection is possible. Previously the prompt read EOF and logged out the first entry
    of a randomly ordered list.
  - Selection menus and identity lists are sorted.
- **A5. `auth whoami` validates against the configured STS endpoint**
  (`pkg/auth/types/aws_credentials.go`, `cmd/auth/whoami.go`).
  - Validation uses the identity's or provider's `spec.endpoint_url`.
  - Expired or rejected credentials still exit 1. When STS cannot be reached, unexpired credentials exit
    0 with a warning (`ErrCredentialValidationUnavailable`).
  - Previously offline, proxy and emulator users got exit 1 for valid credentials.
- **A6. Registries managed by a credential helper that cannot store are left to that helper**
  (`pkg/auth/cloud/docker/helper_managed.go`, the ECR, ECR Public and ACR integrations,
  `cmd/aws/ecr`, `cmd/azure/acr`).
  - A helper such as `ecr-login` gets registry credentials itself and replies "not implemented" to
    `store`. When `credHelpers` routes a registry to it, Atmos leaves `config.json` unchanged, prints a
    note and succeeds.
  - A helper that genuinely fails still fails the login.
- **A7. Logout removes the configuration digest record** (`pkg/auth/credentials/config_digest.go`,
  `pkg/auth/manager_logout.go`). Every logout, with or without `--keychain`, deletes the identity's
  `atmos-config-digest_*` entry.
- **A8. `auth list --format json|yaml` redacts only secret values** (`cmd/auth/list_redact.go`).
  - A documented denylist of secret key fragments, plus AWS access key IDs, is redacted recursively in
    identity `credentials`, `spec` and `env` and in provider `spec`.
  - Non-secret values such as `region` are shown, and booleans and numbers keep their type.

### Regressions from changes already on `main`

- **T1. A source that ships `backend.tf.json` provisions again** (`pkg/provisioner/source/refresh.go`).
  - With `auto_generate_backend_file` enabled, a shipped `backend.tf.json` is dropped from the staged
    copy with a warning, and the generated backend replaces it, as in v1.234.1.
  - It is never recorded as source-owned. A directory named `backend.tf.json` and every other ownership
    conflict still fail.
  - The "verify source URI and credentials" hint now appears only for fetch failures.
- **G1. A user's `settings.configuration` no longer marks provider ownership**
  (`pkg/schema/component_configuration.go`, `internal/exec/stack_processor_merge.go`).
  - Ownership is the reserved marker `settings.configuration.atmos_provider_owned: true`. Only the stack
    processor sets it, for components whose provider is selected by an enabled configuration-provider
    section, and it is removed if a manifest writes it.
  - `terraform plan`, varfile names and backend overrides behave as in v1.234.1 for user-defined
    `settings.configuration`.
- **G2. Unknown sections under `components.terraform` are ignored**
  (`pkg/component/configuration/providers.go`, `pkg/plugin/bootstrap/components_catalog.go`).
  - A section selects a configuration provider only when it names a built-in provider (`terragrunt`) or
    one an enabled plugin package exports. That is read from the lock or a local manifest, without
    installing or starting anything.
  - Other sections are dropped with a debug log and no longer appear in `describe config`.
- **G3. Unreleased edition journal entries are dated after v1.234.1** (`pkg/edition/journal.yaml`).
  - `plugins.default_alias`, `list.instances.engine_columns` and
    `components.source.working_directory_precedence` are dated 2026-11-21, the day they merged. They had
    been dated before v1.234.1 shipped, so pins taken on v1.234.1 got behavior it never had.
  - A guard test (`pkg/edition/journal_release_test.go`, data in
    `pkg/edition/testdata/latest-release.yaml`) fails when an entry that is not in the latest release is
    dated on or before it. The regeneration step after each release is documented in
    `docs/prd/editions.md`.
- **R1. Gitea Actions runs keep going when the API URL is missing or HTTP**
  (`pkg/ci/providers/gitea/client.go`).
  - An unusable API URL now disables only statuses, checks and comments (`ErrGiteaAPIUnavailable`),
    with one log line. The step summary and outputs are still written.
  - Requests are still never sent over HTTP without `ATMOS_CI_GITEA_ALLOW_INSECURE=true`, and a
    malformed URL still fails.
- **R2. A missing Gitea token is skipped quietly**, at debug level like GitHub
  (`pkg/ci/plugins/terraform/skip_reason.go`). This also quiets per-operation status warnings for a
  missing token on both providers.
- **R3. `git push` failures name the remote and branch again** (`pkg/git/providers/cli/push.go`). The
  message reads `<cause>: <remote>/<branch> after N attempt(s), M retries`. The elapsed time moved to a
  debug log so the message is deterministic, and URL remotes are redacted.

### Pre-existing bugs fixed in this change

- **`auth console` no longer prints credentials when federation fails**
  (`pkg/auth/cloud/aws/console.go`). The error is rebuilt with the `Session` query redacted, and the
  credential values are registered with the masker. Previously the access key, secret key and session
  token appeared on stderr.
- **`atmos version` compares versions semantically** (`pkg/upgrade/compare.go`,
  `internal/exec/version.go`, `cmd/cmd_utils.go`). Newer builds and release candidates are no longer
  told to "update" to an older release, and unparsable versions get no suggestion.
- **`auth shell` exits 128+N for a signal-killed shell** (`pkg/shell/interactive.go`), like `auth exec`.
- **Case-insensitive identity names work everywhere** (`pkg/auth/manager*.go`, `cmd/auth/logout.go`).
  This covers `exec`, `env`, `shell`, `logout`, workflows, hooks and custom-command `identity:`.
- **SIGTERM and SIGINT reach the `auth exec`/`auth shell` child** (`pkg/shell/supervise*.go`).
  - Atmos forwards termination to the child and waits up to 30 seconds. It keeps running on Ctrl-C until
    the child exits, so no child is left behind.
  - This also applies to `secret exec` and `secret shell`.
- **`aws eks update-kubeconfig --dry-run` has no side effects** (`cmd/aws/eks/update_kubeconfig*.go`).
  - It skips integrations, writes no kubeconfig and runs no registry login.
  - It prints only the entries that would be written. It still authenticates and describes the cluster.
- **The `describe affected --ref`/`--sha` deprecation notice goes to stderr** (`cmd/describe_affected.go`).
- **A `null:` stack manifest key is a validation error** (`internal/exec/stack_manifest_null_keys.go`).
  It returns `ErrInvalidStackManifest` naming the file and key path, instead of a panic in
  `validate stacks` and `describe stacks`.
- **`packer --dry-run` previews the command instead of running Packer** (`cmd/packer.go`,
  `internal/exec/packer.go`). The flag was never registered and was silently dropped.
- **HCL varfiles and backend files are truncated before writing** (`pkg/utils/hcl_utils.go`), so
  shrinking content no longer leaves stale data behind.

### Related documentation

- `docs/fixes/2026-10-28-terraform-source-refresh-safety.md` now describes the T1 behavior.
- `docs/fixes/2026-10-28-source-identity-disable-and-workdir-precedence.md` now gives the new transition
  date.
- Website pages for auth logout/whoami/list/exec/shell, ECR/ACR login, EKS/AKS/GKE kubeconfig, Gitea
  setup, git push, packer, editions-dated features and the editions PRD and skill are updated.

## Validation

**Build and static checks** (on darwin, plus `GOOS=windows` and `GOOS=linux` cross-builds):

- `go build ./...` passes.
- `go vet` passes on the 43 changed packages for all three operating systems.
- `gofumpt -l` reports no files.
- `go mod tidy -diff` reports no changes.

golangci-lint could not run locally, so CI lint is the first lint pass.

**Tests.**

- `go test -race` over `./pkg/...` and the `cmd` subpackages: 522 packages pass, with no races.
  - One test in `pkg/asciicast` failed, `TestRunSessionSlowStartupHonorsCallerDeadline`. It hit its 40-second
    deadline while two race suites were running at once.
  - That package is unchanged and does not depend on any changed package. The test passed alone and
    three times in a row.
- `go test ./internal/exec` passes (973 s) and `go test ./cmd` passes (1051 s).
- Full `TestCLICommands`: 641 passed, 0 failed, 2 skipped, with no snapshots regenerated.
- Each new regression test was checked against the unfixed code:
  - every one failed there;
  - every one passes with the fix.

**End-to-end rechecks.** The same isolated fixtures were run on the v1.234.1 baseline, the frozen
candidate, and the fixed build:

- G1: `describe component` JSON is byte-identical to the baseline.
- G2: `list stacks` output is byte-identical to the baseline.
- A4: a bare `auth logout --identity` in CI with stdin at EOF:
  - the baseline exits 1;
  - the candidate exits 0 after choosing `mock-identity` by itself;
  - the fixed build exits 1 with `interactive identity selection: requires a TTY`.
- A1 and A2: credentials created by v1.234.1 are found again after the upgrade.
- R1, T1 and P4: each matches the baseline.

**Coverage.** Coverage on new and changed lines, compared with `c0f3af7f57` and excluding the `cmd`
root package and `internal/exec`, is 93.6% (907 of 969 lines). Most uncovered lines are interactive
logout menu code that moved without changing, plus some error branches. Package statement coverage,
before and after:

| Package | Before | After |
| --- | --- | --- |
| `cmd` | 85.58% | 85.60% |
| `cmd/auth` | 83.2% | 83.9% |
| `cmd/aws/ecr` | 87.2% | 89.7% |
| `cmd/aws/eks` | 88.4% | 92.8% |
| `cmd/azure/acr` | 90.8% | 91.2% |
| `cmd/azure/aks` | 96.0% | 96.0% |
| `errors` | 87.3% | 87.3% |
| `internal/exec` | 85.19% | 85.58% |
| `pkg/auth` | 92.3% | 92.4% |
| `pkg/auth/cloud/aws` | 93.1% | 93.2% |
| `pkg/auth/cloud/azure` | 96.5% | 96.5% |
| `pkg/auth/cloud/docker` | 99.1% | 99.2% |
| `pkg/auth/cloud/gcp` | 88.9% | 88.9% |
| `pkg/auth/cloud/kube` | 95.1% | 95.2% |
| `pkg/auth/credentials` | 93.5% | 93.9% |
| `pkg/auth/integrations` | 94.3% | 94.4% |
| `pkg/auth/integrations/aws` | 95.3% | 95.9% |
| `pkg/auth/integrations/azure` | 95.7% | 96.3% |
| `pkg/auth/integrations/gcp` | 92.0% | 92.2% |
| `pkg/auth/realm` | 97.4% | 97.8% |
| `pkg/auth/types` | 28.1% | 30.2% |
| `pkg/ci/plugins/terraform` | 93.6% | 94.0% |
| `pkg/ci/providers/gitea` | 96.5% | 96.6% |
| `pkg/component/configuration` | 85.4% | 86.2% |
| `pkg/component/terraform` | 0.4% | 0.4% |
| `pkg/component/terraform/terragruntexec` | 98.4% | 98.4% |
| `pkg/config` | 91.0% | 91.1% |
| `pkg/git/providers/cli` | 90.4% | 90.6% |
| `pkg/list` | 76.0% | 76.0% |
| `pkg/plugin/bootstrap` | 91.1% | 91.2% |
| `pkg/provisioner/source` | 90.8% | 91.0% |
| `pkg/schema` | 91.0% | 91.1% |
| `pkg/shell` | 88.8% | 91.0% |
| `pkg/terraform/backend` | 74.0% | 74.0% |
| `pkg/upgrade` | 100% | 100% |
| `pkg/utils` | 83.7% | 85.8% |

`pkg/component/terraform` is low because its logic is covered from `internal/exec`. Packages showing
no change got one-line fixes whose lines were already covered.

**Timing.** `go tool mage bench:fieldTest v1.234.1,5a1bf052a6` was run twice: once with the default
workload, then with `ATMOS_BENCH_FIELD_FILTER='^BenchmarkCLI$'` for the whole CLI suite. Both builds
used the same harness and Go 1.27.4, on darwin/arm64 (Apple M3 Max), `GOMAXPROCS=2`, with 10 samples
each and nothing else running.

- **No significant regressions.** The threshold is 10% and p < 0.05.
- **Backend reads are unchanged:**
  - 22 for `describe affected` store lookups;
  - 22 for caller-stack store lookups;
  - 11 for `describe stacks` store lookups.
- The largest timing change is `list instances` with 1,000 instances: 0.685 s to 0.709 s (+3.5%,
  p < 0.001).
- Most other workloads changed by 0% to 1.5%. Examples:
  - `describe affected` with 10,000 instances: 17.36 s to 17.42 s (+0.4%);
  - `describe stacks` with 1,000 instances: 1.084 s to 1.094 s (+1.0%).

**Website.** `cd website && npm run build` succeeds.

- It reports no broken links.
- It reports broken anchors only on AI skill pages that this change does not touch
  (`atmos-aws-cloudformation`, `atmos-cast`, `atmos-ci`, `atmos-helm`, `atmos-hooks`).

**Not run.**

- golangci-lint.
- Native Linux and Windows test runs, beyond the cross-compiles above.

## Follow-ups

**Intentional changes since v1.234.1 that need release notes.** The regression test confirmed these;
none is edition-gated.

- New realm-scoped keyring key layout. `auth whoami` exits 1 for expired or rejected credentials.
  `--identity=false` disables auth. `auth exec` rejects arguments before `--`. `--tags` wins over
  `ATMOS_IDENTITY`. Removed SAML drivers fail at login.
- EKS/ECR clients no longer read `ca_bundle` or `endpoint_url` from `~/.aws/config`; `AWS_CA_BUNDLE`
  still works.
- With `spec.files.base_path`, the first `auth env` after upgrading points at files that a
  re-authentication then creates.
- With an explicit realm, kube user and default context names are realm-prefixed. Without one, the
  worktrees of a repository share a kube context and user again, as in v1.234.1.
- Ctrl-C during `auth exec`/`auth shell` waits for the child and returns its exit code.
- CI plan parsing reports `has_changes=true` for import, move and replace plans, and summaries gain a
  "View full run" footer.
- `--log-order grouped` prefixes stdout lines.
- With `-detailed-exitcode`, a plan with changes counts as success for hooks.
- `list instances` shows Type and Engine columns for pins after 2026-11-21. `describe config` adds
  `artifacts: {}`.
- Container build output changed.

**Choices made in these fixes that can be revisited.**

- The automatic realm is recognized by its value format, because the credential store receives only
  the realm string. Restoring `RealmSource` through `MergeComponentAuthConfig` would narrow this.
- Keychain logout deletes an ambiguous legacy key even if it belonged to another realm, matching
  v1.234.1, with a warning.
- A child that ignores termination is left running after 30 seconds rather than killed.
- A Gitea HTTP URL without the opt-in logs a warning only when a token is configured.
- Edition journal entries are dated on the day they merge into `main`, and
  `pkg/edition/testdata/latest-release.yaml` must be regenerated after each release (documented in
  `docs/prd/editions.md`).
- `settings.configuration.atmos_provider_owned` is visible in describe output for discovered units.

**Help cast.** `atmos packer --help` now lists `--dry-run`, and its screengrab cast was regenerated
with `atmos --chdir=demo/casts casts generate screengrabs cli --filter "atmos packer"`. The packer
subcommand casts render the same text as before, so they were left unchanged.

**Bugs present in both v1.234.1 and the candidate that were not fixed here.** Each needs a decision
(fix now, or open an issue):

- Auth:
  - `atmos aws ecr login --identity X --registry Y` ignores the identity.
  - A `github/oidc` assume-role logout leaves its AWS files.
  - Ambient `GOOGLE_*`, `CLOUDSDK_CORE_ACCOUNT`, `ARM_CLIENT_SECRET` and `AZURE_CLIENT_ID` pass through
    for `gcp/project` and `azure/cli` identities.
  - Re-login takes `current-context` back after `kubectl config use-context`.
- Stores: the SSM `!store.get` key is prefixed twice. The Vault error path is nondeterministic.
- Terraform:
  - With a local backend, `init_run_reconfigure: true` and existing workspaces, a forced
    `init -reconfigure` hangs on Terraform's migration prompt without a terminal.
  - `generate varfiles -s` is silently ignored (the filter is `--stacks`).
  - After-apply hooks run for `apply --dry-run`.
  - `plan-diff` ignores `-var`.
  - Discovery commands that evaluate functions select workspaces and rewrite `.terraform/environment`.
  - `resources_to_replace` is always 0.
- CLI and config:
  - `--profile <name>` fails when the name equals a command (for example `ci`).
  - `--base-path /nonexistent list stacks` exits 0.
  - An `atmos.yaml` `!exec` runs 6 times per invocation.
  - `describe workflows`/`list workflows` order, and duplicate-component resolution in `list values`,
    are nondeterministic.
  - The quick-start-advanced example workflows fail their own schema validation.
- Runtimes:
  - Helmfile flags placed after the component without `--` are dropped.
  - `docker build` ignores a custom `DOCKER_HOST`.
  - A workflow container step's `with.env` does not reach `docker exec`.
  - A git rebase conflict leaves the workdir mid-rebase.

**Not covered by the regression test.**

- Live Kubernetes, live cloud and CI APIs.
- Interactive TTY flows: SSO, SAML, webflow and the legacy-key prompt form.
- AKS/GKE/ACR exchanges and Azure MSAL caches.
- Terraform Cloud and remote backends.
- Plugin, Terragrunt and artifact features (checked only for absence of side effects).
- Linux and Windows.
