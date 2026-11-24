# Fix: Kubeconfig entries for EKS, AKS and GKE are scoped per identity, account and realm

**Date:** 2026-11-22

## Summary

The EKS, AKS and GKE kubeconfig integrations, and the `aws eks update-kubeconfig` and
`azure aks update-kubeconfig` Go SDK paths, now write one kubeconfig user per identity and account.
When the user configures an auth realm, they also write one user per realm. Cleanup removes only the
contexts, clusters and users this integration owns. Kubeconfig exec entries keep working when the
checkout that wrote them is removed. `update: error` mode can be re-run safely. A dry run of
`aws eks update-kubeconfig` changes nothing.

Most items come from the Atmos Auth pre-release review (#3720) and its follow-up rounds. One item (the
realm prefix applied only to an explicitly configured realm) was found by the regression test that
compared this work with v1.234.1. The `--dry-run` fix is a bug that v1.234.1 already had and that the same
regression test found. Other #3720 and regression-test fixes land in separate PRs.

## Context

A kubeconfig entry written by Atmos has a cluster, a context and an exec-plugin user. The user runs
`atmos aws eks token`, `atmos azure aks token` or `atmos gcp gke token` with `ATMOS_IDENTITY` and, for EKS
since #3675, `ATMOS_CHDIR` in its env. Before this change:

- The user name was `atmos-eks-<name>-<region>` (EKS), `atmos-aks-<name>-<resource-group>[-<subscription>]`
  (AKS) or `atmos-gke-<name>-<project>-<location>` (GKE). It had no identity, and the EKS name had no
  account. Two contexts for the same cluster with different identities, or two same-named EKS clusters in
  different accounts, shared one user. The last write won, so `kubectl` authenticated as the wrong
  identity.
- Cleanup deleted the context, cluster and user by name, even when another context still used them.
- EKS cleanup found its cluster by the `cluster/<name>` suffix alone, so logging out of a `us-west-2`
  integration could remove a `us-east-2` cluster of the same name.
- Checkouts with different auth realms that wrote the same cluster into a shared kubeconfig shared one
  user and one default context, and each login repointed them.
- AKS and GKE exec entries had no `ATMOS_CHDIR`, so `kubectl` only worked from inside the repository.
- When the `ATMOS_CHDIR` directory was removed (for example, a deleted worktree), every `kubectl` call failed.
- `spec.cluster.kubeconfig.update: error` failed on every re-run, because any existing entry counted as a
  collision.

When no realm is configured, `pkg/config` sets `auth.realm` to a hash of the `atmos.yaml` path, or to
`default`, and records how it got the value in `AuthConfig.RealmSource`. `GetRealm()` reports that derived
value with the `config` source, so the realm value alone cannot tell a configured realm from a derived one.

## Changes

### One kubeconfig user per identity and account (#3720 item 2.5 and follow-up)

- **Problem.** Contexts for different identities, or for same-named EKS clusters in different accounts,
  shared one exec user and overwrote each other's credentials. AKS and GKE had the same bug.
- **Fix.** `kube.ClusterInfo` gained `Identity` and `LegacyUserNames`, and moved to
  `pkg/auth/cloud/kube/cluster_info.go`. `kube.UserName` now builds
  `atmos-[<realm>-]<prefix>-<name>-<region>[-<account>][-<identity>][-<hash>]`:
  - EKS: `atmos-eks-<name>-<region>-<account>-<identity>-<hash>`. The account comes from the cluster ARN
    (`pkg/auth/cloud/aws/eks.go`).
  - AKS: `atmos-aks-<name>-<resource-group>-<subscription>-<identity>-<hash>` (`pkg/auth/cloud/azure/aks.go`).
  - GKE: `atmos-gke-<name>-<project>-<location>-<identity>-<hash>` (`pkg/auth/cloud/gcp/gke.go`).
  - **Hash suffix.** Realms, cluster names and identities can contain `-`, which is also the separator, so
    different tuples could build the same name (realm `r` with cluster `b-eks-c` and realm `r-eks-b` with
    cluster `c`) and the merge would overwrite one user with the other. When the name includes a realm or an
    identity, `kube.UserName` appends `-<hash>`: the first 8 hex characters of a SHA-256 over the realm,
    user prefix, cluster name, region, account and identity joined with a NUL byte. The readable part is
    unchanged, and names without a realm or identity are exactly what v1.234.1 wrote. The unhashed
    forms that earlier builds of this branch wrote (`kube.UnhashedUserName`) stay in the owned-name lists
    (`kube.UserNameForms`, `OwnedUserNames`, `EKSKubeUserNames` and the AKS and GKE equivalents), so
    cleanup and the error-mode migration still recognize and remove them.
  - New helpers `EKSKubeUserName(s)`, `AKSKubeUserName(s)` and `GKEKubeUserName(s)` return the current
    name plus every name older versions wrote. `LegacyEKSKubeUserName`, `LegacyAKSKubeUserName` and
    `LegacyGKEKubeUserName` return the pre-identity names.
- **Cleanup that never breaks another context** (`pkg/auth/cloud/kube/remove.go`).
  `RemoveClusterConfig` and the new `RemoveOwnedClusterConfig` remove a cluster or user only when no
  remaining context references it. Cleanup also removes the legacy user names.
- **Tests.** `TestEKSIntegration_Execute_DistinctUserPerIdentity`, `TestEKSIntegration_DistinctUserPerAccount`,
  `TestEKSIntegration_Cleanup_KeepsOtherIdentityContext`, `TestEKSIntegration_Cleanup_RemovesLegacyUser`,
  `TestEKSIntegration_Cleanup_KeepsLegacyUserStillInUse`, the matching `TestAKSIntegration_*` and
  `TestGKEIntegration_*` tests in `*_user_scope_test.go`, `TestEKSKubeUserName_MatchesWrittenUser`,
  `TestAKSKubeUserName_MatchesWrittenUser`, `TestGKEKubeUserName_MatchesWrittenUser`, the `Test*KubeUserNames`
  and `TestLegacy*KubeUserName` tests, `TestRemoveClusterConfig_KeepsClusterUsedByOtherContext` and
  `TestRemoveClusterConfig_ExtraUserNames`.

### EKS cleanup matches the cluster ARN (#3720 item 3.7)

- **Problem.** `EKSIntegration.Cleanup` matched any kubeconfig cluster key ending in `cluster/<name>`, so it
  could remove a same-named cluster in another region, or a non-EKS entry.
- **Fix** (`pkg/auth/integrations/aws/eks.go`). Each key is parsed as an ARN and must name the `eks`
  service, the integration's region and the exact `cluster/<name>` resource. Keys are checked in sorted
  order. A miss wraps `ErrEKSClusterNotFound`.
- When same-named clusters of several accounts match, cleanup picks the one whose context points at one of
  this integration's users. If none or more than one qualifies, it returns the new `ErrEKSClusterAmbiguous`
  (`errors/errors_integrations.go`) instead of guessing.
- **Tests.** `TestEKSIntegration_Cleanup_IgnoresSameNamedClusterInOtherRegion`,
  `TestEKSIntegration_Cleanup_RemovesOnlyMatchingRegion`, `TestEKSIntegration_FindClusterARN_MatchesParsedARN`,
  `TestEKSIntegration_Cleanup_PicksOwnAccount`, `TestEKSIntegration_FindClusterARN_AmbiguousAccounts`,
  `TestEKSIntegration_Cleanup_PicksOwnAccountByPreRealmUser`,
  `TestEKSIntegration_Cleanup_PicksOwnAccountByRealmContext`
  and `TestEKSIntegration_SelectOwnedClusterARN_UnreadableKubeconfig`.

### Realm-scoped user and context names, only for an explicitly configured realm (#3720 follow-ups)

- **Problem.** Checkouts with different realms that wrote the same cluster into a shared kubeconfig shared
  one user and one default context.
- **Fix.** With an explicit realm (from `auth.realm` or `ATMOS_AUTH_REALM`):
  - The user name gets `<realm>-` after `atmos-`, for example
    `atmos-acme-eks-<name>-<region>-<account>-<identity>-<hash>`.
  - The exec env gets `ATMOS_AUTH_REALM=<realm>`, so `kubectl` gets its token in that realm
    (`execEnv` in `pkg/auth/cloud/kube/cluster_info.go`).
  - Without an alias, the default context name is `<realm>/<cluster-id>` (`kube.ContextName` in
    `pkg/auth/cloud/kube/context_name.go`). Realms cannot contain `/`, so the prefix is unambiguous. An
    alias is used unchanged.
- **Owner-aware cleanup and migration** (`pkg/auth/cloud/kube/context_name.go`, `remove.go`).
  - `kube.OwnedUserNames` lists the users that belong to an entry: the current name, the name without the
    realm, the names under legacy realms, and the legacy names.
  - `RemoveOwnedClusterConfig` removes a context only when it points at one of those users. So one realm's
    logout keeps another realm's context of the same name.
  - On write, `contextMigration` removes an older default context (the unprefixed ID, or a legacy-realm
    name) only when it points at one of this entry's own users. It drops this entry's older users once no
    other context uses them. A context of any other user is never touched.
  - `kube.ContextNames` returns the current and older default context names for cleanup, and
    `KubeconfigManager.LookupContext` reads one context.
- **Regression found by comparing with v1.234.1.** An earlier version of this change applied the realm
  prefix to the derived realm too. So every user without a configured realm saw their default context
  change from `<cluster-id>` to `<hash>/<cluster-id>`. Users became `atmos-<hash>-...`, and
  `ATMOS_AUTH_REALM` was pinned in the exec env. `kubectl --context <cluster-arn>` broke without any config
  change. The fix:
  - `pkg/auth/realm/explicit.go` adds `IsExplicit` and `IsExplicitSource`, plus the `SourceConfigPath` and
    `SourceDefault` constants for the two derived sources.
  - `IntegrationConfig` gains `RealmSource` and `RealmExplicit()` (`pkg/auth/integrations/types.go`).
    `manager.integrationRealmSource()` (`pkg/auth/manager_integration_realm.go`) passes `SourceAuto` for a
    derived realm. Every `integrations.Create` call in `pkg/auth/manager_*.go` sets it.
  - `kube.NameRealm` (`pkg/auth/cloud/kube/name_realm.go`) returns the explicit realm as the naming realm.
    It returns a derived realm only as a legacy realm.
  - The EKS, AKS and GKE integrations and the `aws eks` and `azure aks` `update-kubeconfig` SDK paths use
    `NameRealm`. Without an explicit realm, names have the v1.234.1 shape plus the identity and account
    suffixes above.
  - The `<hash>/<cluster-id>` contexts and `atmos-<hash>-...` users that pre-release builds wrote count as
    the integration's own (`ClusterInfo.LegacyRealms`). The next login or logout removes them.
- **Success messages** show the context name actually written (`kube.ContextName`) on every path.
- **Tests.** `TestIsExplicit`, `TestIsExplicitSource`, `TestIntegrationConfig_RealmExplicit`, `TestNameRealm`,
  `TestManager_IntegrationRealmSource`, `TestManager_ExecuteIntegration_AutoRealmKeepsV1234ContextName`,
  `TestManager_ExecuteIntegration_ExplicitRealmPrefixesContext`, `TestUserName_Realm`,
  `TestBuildClusterConfig_RealmEnv`,
  `TestContextName`, `TestOwnedUserNames`, `TestBuildClusterConfig_RealmPrefixesDefaultContext`,
  `TestWriteClusterConfig_MigratesUnprefixedOwnContext`,
  `TestWriteClusterConfig_MigrationKeepsOldUserStillInUse`,
  `TestWriteClusterConfig_MigrationOnlyChange`, `TestWriteClusterConfig_NoMigration`,
  `TestRemoveOwnedClusterConfig`,
  `TestRemoveClusterConfig_KeepsContextOfOtherUser`, the tests in `pkg/auth/cloud/kube/legacy_realm_test.go`,
  and the integration tests in `eks_realm_test.go`, `eks_context_realm_test.go`, `eks_auto_realm_test.go`
  and their AKS and GKE counterparts. Further tests are `TestExecuteAKSUpdateKubeconfigDirect_RealmScopesUser`,
  `TestExecuteAKSUpdateKubeconfigDirect_RealmPrefixesDefaultContext`,
  `TestExecuteAKSUpdateKubeconfigDirect_AutoRealmKeepsNames`, and the realm cases added to
  `TestExecuteEKSUpdateKubeconfigDirect_AuthContext`.

### AKS and GKE exec entries set `ATMOS_CHDIR` (#3720 item 3, suspected item on #3675)

- **Problem.** EKS entries record the working directory in `ATMOS_CHDIR` (#3675), but AKS and GKE entries
  did not. So `kubectl` outside the repository could not find `atmos.yaml`.
- **Fix.** `appendWorkingDirExecEnv` (`pkg/auth/cloud/azure/kube_exec_env.go`,
  `pkg/auth/cloud/gcp/kube_exec_env.go`) adds the absolute working directory. If `os.Getwd` fails, the
  entry is left out and a debug line is logged.
- **Tests.** `TestBuildKubeClusterInfo_WorkingDirectory` and
  `TestAppendWorkingDirExecEnv_WorkingDirectoryUnavailable` in both packages, plus the updated
  `TestBuildKubeClusterInfo` assertions.

### Token commands survive a deleted `ATMOS_CHDIR` (#3720 follow-up)

- **Problem.** When the checkout that wrote an entry was removed, every `kubectl` call failed with
  "directory does not exist" until the integration was re-run.
- **Fix** (`cmd/root_chdir_kube_exec.go`, `cmd/root.go`). The duplicated directory check in
  `processEarlyChdirFlag` and `processChdirFlag` moved to `checkChdirTarget`. The fallback applies only
  when all three hold:
  - The value came from the `ATMOS_CHDIR` environment variable.
  - The path does not exist.
  - The command is `aws eks token`, `azure aks token` or `gcp gke token`.

  Atmos then warns on stderr how to refresh the kubeconfig, unsets `ATMOS_CHDIR`, and runs from the
  current directory. An explicit `--chdir`, every other command, and a path that is a file still fail.
- **Tests.** `TestProcessEarlyChdirFlag_MissingKubeExecChdir`, `TestProcessChdirFlag_MissingKubeExecChdir`,
  `TestIsKubeExecTokenInvocation` and `TestMissingKubeExecChdirMessage`.

### `update: error` is idempotent (#3720 follow-up)

- **Problem.** With `spec.cluster.kubeconfig.update: error`, every re-run of an integration failed with
  "cluster ... already exists". Auto-provisioned integrations re-run on every login, so the mode was
  unusable.
- **Fix** (`pkg/auth/cloud/kube/config_error_mode.go`). Error mode compares each cluster, context and user
  it would write with the existing one. It fails only on a real difference: server, CA, context target,
  exec args or other exec env. Users are now checked too. `ATMOS_CHDIR` is ignored, because it differs
  between checkouts of one repository, and a re-run that differs only there refreshes it. The realm
  context migration is allowed in error mode. An unreadable file still falls through to the merge.
- **Tests.** `TestWriteClusterConfig_ErrorMode_IdenticalRerunIsNoOp`,
  `TestWriteClusterConfig_ErrorMode_OnlyChdirDiffers`,
  `TestWriteClusterConfig_ErrorMode_RealConflicts`, `TestAuthInfosEqualIgnoringChdir`,
  `TestWriteClusterConfig_ErrorMode_UnreadableFile`, `TestWriteClusterConfig_ErrorModeRealmContext`,
  `TestEKSIntegration_Execute_ErrorModeRerun`, `TestEKSIntegration_Execute_ErrorModeConflict`,
  `TestEKSIntegration_Execute_ErrorModeRealmContext`, `TestAKSIntegration_Execute_ErrorModeRealmContext`
  and the updated `TestGKEKubeconfigUpdateModesAndNoOp`.

### `aws eks update-kubeconfig --dry-run` has no side effects (pre-existing in v1.234.1)

- **Problem.** On the Go SDK paths (`--identity` with `--name`, and `--integration`), `--dry-run` was parsed
  but never used. The kubeconfig was written, and authenticating the identity ran its auto-provisioned
  integrations, such as an ECR login.
- **Fix** (`cmd/aws/eks/update_kubeconfig.go`, `update_kubeconfig_sdk.go`, `update_kubeconfig_dry_run.go`).
  - Both paths authenticate with `auth.ContextWithSkipIntegrations` and still describe the cluster.
  - They print only the cluster, context and user entries that would be merged to stdout, not the rest of
    the file, so other users' credentials never reach the terminal. The target path and context go to
    stderr. The kubeconfig is not touched.
  - The `--integration` path resolves the integration's cluster, identity, alias and kubeconfig path, with
    the CLI overrides applied, instead of running it.
  - The AWS CLI path is unchanged.
- **Tests.** `TestUpdateKubeconfig_DryRunDirectWritesNothing`,
  `TestUpdateKubeconfig_DryRunIntegrationWritesNothing`,
  `TestUpdateKubeconfig_WithoutDryRunWrites` and `TestUpdateKubeconfig_DryRunIntegrationErrors`.

### Realm source constants in `pkg/config` (refactor)

`setAuthRealm` (`pkg/config/auth_realm.go`) records the realm source with `realm.SourceEnv`,
`realm.SourceConfig`, `realm.SourceConfigPath` and `realm.SourceDefault` instead of string literals. So
`realm.IsExplicitSource` and the value `pkg/config` writes cannot drift apart. There is no behavior change.

### Documentation

- `website/docs/cli/commands/aws/aws-eks-token.mdx`, `azure/azure-aks-token.mdx`, `gcp/gke/gke.mdx` and
  `website/docs/tutorials/eks-kubeconfig-authentication.mdx` show the user names the code writes, the
  `ATMOS_CHDIR` env and its fallback, realm-prefixed context names (only for a configured realm), and
  owner-aware cleanup. The tutorial gains "User Names" and "Context Names" sections and describes the new
  `update: error` behavior.
- The `aws eks update-kubeconfig` and `azure aks update-kubeconfig` pages describe the realm default for
  `--alias`. The EKS page describes the new `--dry-run` behavior.
- These pages also replace em dashes with hyphens in their link lists and prose.

## Validation

All commands were run in this PR's worktree, on top of `origin/main` at `2d7822f214`.

- `go build ./...`: passed.
- `go vet` passed on `./cmd`, `./cmd/aws/eks`, `./cmd/azure/aks`, `./errors`, `./pkg/auth`,
  `./pkg/auth/realm`, `./pkg/auth/cloud/{aws,azure,gcp,kube}`, `./pkg/auth/integrations`,
  `./pkg/auth/integrations/{aws,azure,gcp}` and `./pkg/config`.
- `go test -count=1 -cover` passed for the 14 changed packages other than the root `cmd` package:
  `cmd/aws/eks`, `cmd/azure/aks`, `errors`, `pkg/auth`, `pkg/auth/realm`, `pkg/auth/cloud/{aws,azure,gcp,kube}`,
  `pkg/auth/integrations`, `pkg/auth/integrations/{aws,azure,gcp}` and `pkg/config`.
- For the root `cmd` package, only the tests for the changed chdir code ran:
  `go test -count=1 -run 'TestProcessEarlyChdirFlag|TestProcessChdirFlag|TestIsKubeExecTokenInvocation|TestMissingKubeExecChdirMessage' ./cmd`
  passed. The full `./cmd` suite takes about 17 minutes and runs in CI.
- Package statement coverage, `origin/main` (`2d7822f214`, in a temporary detached worktree) to this PR,
  with the same `go test -count=1 -cover` command. The root `cmd` package is not included:

| Package | Before | After |
|---|---|---|
| `cmd/aws/eks` | 87.8% | 92.8% |
| `cmd/azure/aks` | 95.3% | 96.0% |
| `errors` | 87.3% | 87.3% |
| `pkg/auth` | 91.7% | 91.7% |
| `pkg/auth/realm` | 97.4% | 97.8% |
| `pkg/auth/cloud/aws` | 93.0% | 93.1% |
| `pkg/auth/cloud/azure` | 95.0% | 94.8% |
| `pkg/auth/cloud/gcp` | 88.5% | 88.5% |
| `pkg/auth/cloud/kube` | 91.7% | 95.2% |
| `pkg/auth/integrations` | 94.3% | 94.4% |
| `pkg/auth/integrations/aws` | 93.8% | 95.3% |
| `pkg/auth/integrations/azure` | 93.6% | 95.4% |
| `pkg/auth/integrations/gcp` | 90.0% | 92.2% |
| `pkg/config` | 91.0% | 91.0% |

The small drop in `pkg/auth/cloud/azure` is the `os.Getwd` failure branch of `appendWorkingDirExecEnv`.
Its test, `TestAppendWorkingDirExecEnv_WorkingDirectoryUnavailable`, runs only on Linux, because only Linux
reliably fails `Getwd` after the current directory is removed. This run was on macOS. The same applies to
the GKE copy in `pkg/auth/cloud/gcp`.

Not run:

- The full `./cmd` suite (runs in CI).
- `golangci-lint`, `gofumpt` and cross-OS `go vet`. CI lint is the first lint pass.
- The race detector, Linux and Windows test runs, and the `TestCLICommands` snapshot tests.
- The website build (`cd website && npm run build`).
- Live EKS, AKS and GKE clusters and a real `kubectl`. All tests use fakes and temporary kubeconfig files.

## Follow-ups

No open work remains for this change. These choices can be revisited:

- **Kube ownership is broad.** Pre-realm, legacy-realm and legacy user names count as an integration's
  own. So a checkout with a realm migrates, and its logout removes, the bare-ID context that a checkout
  without a realm wrote for the same identity and cluster. The legacy EKS name `atmos-eks-<name>-<region>`
  has no account or identity, so it is broader still.

Compatibility notes for users (release notes):

- Kubeconfig user names change. EKS names gain `-<account>-<identity>`, and AKS and GKE names gain
  `-<identity>`; a name that includes an identity or realm also ends with an 8-character hash. With an explicitly configured realm, every name also gains `<realm>-` after `atmos-`.
  Scripts that hardcode `atmos-eks-...`, `atmos-aks-...` or `atmos-gke-...` user names must be updated.
- Context names change only when `auth.realm` or `ATMOS_AUTH_REALM` is set and no alias is configured. The
  default context becomes `<realm>/<cluster-id>`, so `kubectl --context <cluster-id>` must switch to the
  prefixed name or set an alias. Without a configured realm, context names are unchanged from v1.234.1, and
  the worktrees of one repository share a kube context and user, as before.
- The next login moves each owned context to its new name and user. Logout removes only this integration's
  own contexts, and keeps clusters and users that another context still uses.
- `update: error` no longer fails on an identical re-run. It now also errors when an existing user of the
  same name has different exec settings.
- `aws eks update-kubeconfig --dry-run` on the `--identity` and `--integration` paths no longer writes the
  kubeconfig or runs auto-provisioned integrations. It still authenticates and calls `DescribeCluster`.
