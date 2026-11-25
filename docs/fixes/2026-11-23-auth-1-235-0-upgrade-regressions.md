# Fix: Atmos Auth upgrade regressions found by the 1.235.0 regression test

**Date:** 2026-11-23

## Summary

The regression test that compared the 1.235.0 candidate (`main` at `31b76b6b70`) with v1.234.1 found seven
Atmos Auth workflows that behave worse after upgrading. All seven come from the Atmos Auth review fixes
(#3741 to #3748), which are not released yet. This change fixes them and keeps the security goals of those
fixes:

1. `atmos auth whoami` exits 0 again for ambient identities, and says their credentials are not managed by
    Atmos.
2. `auth whoami` no longer exits 1 for valid Azure credentials when Azure Resource Manager cannot be
    reached or no subscription is set. Azure now behaves like AWS.
3. ECR and ACR login and logout work again when Docker's `config.json` is mounted as a single file, or
    lives in a read-only directory. A logout that cannot remove the credential now says so.
4. ECR and ACR login no longer run `docker-credential-ecr-login get` before `store`.
5. Atmos removes `msal_token_cache.json.lockfile` when it releases the Azure CLI cache lock, and keeps a
    symlinked MSAL cache a symlink.
6. The keyring key migration copies the old entry instead of moving it, so going back to v1.234.1 still
    finds `aws/user` long-lived keys.
7. Login no longer writes a configuration digest entry to the keyring for every identity. The record now
    lives in a file.

Every fix has a test that was written first and seen failing.

## Context

The regression test ran both binaries on the same isolated fixtures, with local fakes and file keyrings
(`.context/regression-test/run-20261124`, items 2, 3, 4, 7, 8 and 10 of its report, and the two
"unresolved compatibility" items about `get` and the MSAL cache). The fixes that introduced the behaviors
are recorded in `docs/fixes/2026-11-22-auth-command-behavior.md`,
`docs/fixes/2026-11-22-auth-docker-credential-store.md`,
`docs/fixes/2026-11-22-auth-azure-gcp-credentials.md` and
`docs/fixes/2026-11-22-auth-keyring-realm-safety.md`. Their goals are kept:

- `whoami` still exits 1 for expired or rejected managed credentials.
- `config.json` is still replaced atomically whenever the kernel allows it.
- Helper credentials are still snapshotted and restored for helpers that store them.
- Atmos and `az` still exclude each other while either one rewrites the shared MSAL cache.
- Ambiguous legacy keyring keys are still never read or deleted without confirmation.
- Intermediate chain steps are still reused only when their configuration digest matches.

## Changes

### 1. `auth whoami` exits 0 for ambient identities

- **Problem.** For a generic `ambient` identity, `atmos auth whoami` and `atmos auth whoami -o json` exited
  1 and printed "Credentials may be expired or invalid" (v1.234.1 exited 0).
- **Cause.** These identities return no credentials by design. `validateCredentials` treated missing
  credentials as invalid, and the exit status from #3746 turned that into exit 1.
- **Fix.**
  - `WhoamiInfo` has a new `Ambient` field (`json:"ambient,omitempty"`, `pkg/auth/types/whoami.go`).
    `buildWhoamiInfo` sets it when the identity minted no credentials (`pkg/auth/manager_whoami.go`). That
    covers the generic `ambient` kind and the emulator identities.
  - `validateCredentials` treats an ambient identity as valid (`cmd/auth/whoami.go`). The human output
    shows a check mark and the row `Credentials: ambient, not managed by Atmos`. The JSON output has
    `"ambient": true` and no warning.
  - An identity that is not ambient and has no credentials still exits 1. Expired or rejected
    credentials still exit 1.
- **Tests.** `TestReportWhoami_AmbientIdentityExitsZero` (human and JSON),
  `TestReportWhoami_MissingManagedCredentialsStillFail` (`cmd/auth`);
  `TestManager_Whoami_AmbientIdentityIsMarkedAmbient`, `TestManager_buildWhoamiInfo_CredentialsAreNotAmbient`
  (`pkg/auth`).

### 2. Azure credentials that cannot be validated are not invalid

- **Problem.** `auth whoami` exited 1 for valid Azure credentials when Azure Resource Manager (ARM) was
  unreachable, or when no subscription was set. For AWS the same situation prints "Could not validate"
  and exits 0.
- **Cause.** `AzureCredentials.Validate` wrapped every failure in `ErrAuthenticationFailed`, and an empty
  subscription in `ErrInvalidAuthConfig`. `whoami` counts both as invalid.
- **Fix** (`pkg/auth/types/azure_credentials.go`, new `pkg/auth/types/azure_credentials_validation.go`).
  - Expired credentials fail with `ErrExpiredCredentials` without a network call.
  - With no subscription, or when ARM cannot be reached, `Validate` returns
    `ErrCredentialValidationUnavailable`, as AWS does. `whoami` then uses the cached expiration time and
    prints a warning.
  - Only an HTTP 401, or a token rejection code such as `InvalidAuthenticationToken` or
    `ExpiredAuthenticationToken`, wraps `ErrAuthenticationFailed`. Other ARM answers are about the
    subscription, not the token, and count as unavailable. Examples are `AuthorizationFailed` (403),
    `SubscriptionNotFound` (404) and server faults.
  - A per-retry pipeline policy (`azureStatusRecorder`) records the HTTP status of the last answer. It
    runs below the bearer token policy, which drops the response when the body cannot be read, so a
    401 with a truncated body is still a rejection. `policy.WithCaptureResponse` does not work here:
    the bearer token policy returns no response in that case.
  - The call is bounded to 15 seconds and 1 retry. It targets the ARM endpoint of the credentials' cloud
    (`CloudEnvironment`: public, US Government or China). A sovereign-cloud token sent to the public ARM
    endpoint is rejected although it is valid.
  - A runtime-only, unexported `validationClientOptions` field lets tests point the call at a local TLS
    server. It is never serialized.
- **Tests.** `TestAzureCredentials_ValidateUnreachableARM`, `TestAzureCredentials_ValidateWithoutSubscription`,
  `TestAzureCredentials_ValidateExpiredSkipsNetwork`, `TestAzureCredentials_ValidateClassifiesARMAnswers`
  (401, 403 with two codes, 404, 500), `TestAzureCredentials_ValidateSucceeds`,
  `TestAzureCredentials_ValidationTargetsTheCredentialCloud`,
  `TestAzureCredentials_ValidateClientSetupFailure`, `TestAzureCredentials_ValidateKeeps401WhenBodyIsTruncated`.
  - Two existing tests now assert the corrected contract. `TestAzureCredentials_ValidateCanceled`
    expects `ErrCredentialValidationUnavailable`, and `TestAzureCredentials_Validate_EmptySubscriptionID`
    no longer expects `ErrInvalidAuthConfig`.
  - `TestSubscriptionIdentity_Authenticate_PreservesAllProviderFields` skips unexported fields. That test
    covers persisted provider fields only.

### 3. `config.json` writes work on single-file mounts and read-only directories

- **Problem.**
  - On Linux, a devcontainer or Kubernetes `subPath` can mount `config.json` as a single file. There the
    rename onto the file fails with EBUSY. A read-only Docker directory with a writable `config.json`
    fails too, because the temporary file cannot be created.
  - ECR and ACR login exited 1, and `atmos auth logout` reported success but left the credential behind.
    v1.234.1 wrote the file in place.
- **Cause.** `saveConfig` only used `WriteFileAtomic` (temporary file and rename). Logout logged
  integration cleanup failures as warnings and still reported a complete logout.
- **Fix.**
  - New `filesystem.WriteFileAtomicOrInPlace` (`pkg/filesystem/replace.go`). It tries the atomic write
    first. When the rename is refused with EBUSY, EXDEV, EROFS, EACCES or EPERM and the target is an
    existing regular file, it writes the same bytes in place, which keeps the file's permission bits.
    Bits the requested mode does not grant are removed first, so a token cache that other users can
    read (for example `0644`) is restricted to `0600` before new tokens go in, as the atomic write would
    have done. If the mode cannot be restricted, the in-place write is refused. When the in-place write
    fails too, both errors are returned.
  - A read-only Docker directory works only when `config.json.lock` already exists and is writable:
    Atmos cannot create the lock file there, so the first login in a directory that never held one fails
    before the in-place write. v1.234.1 failed the same way. The ECR and ACR pages state this.
  - New `filesystem.ResolveSymlinkTarget`, moved from the Docker package so the MSAL cache can share it.
    Docker's `resolveConfigTarget` delegates to it.
  - `ConfigManager` has an injectable `fs filesystem.FileSystem`, so tests simulate the refused rename on
    every OS with the mockgen `MockFileSystem` (`pkg/auth/cloud/docker/config.go`).
  - `cleanupIntegrations` now returns the cleanup errors, wrapped in the new `ErrIntegrationCleanup`
    (`errors/errors_execution.go`). `Logout` adds them to its errors, so the logout is partial
    (`ErrPartialLogout`) and `auth logout` prints "Some credentials could not be removed" with the cause
    (`pkg/auth/manager_logout.go`). The rest of the logout still runs. The `Integration.Cleanup` contract
    comment now says this (`pkg/auth/integrations/types.go`).
- **Tests.**
  - `pkg/filesystem`: `TestWriteFileAtomicOrInPlace_FallsBackWhenReplaceIsRefused` (all five errnos),
    `TestWriteFileAtomicOrInPlace_NoFallback` (success, unrelated error, missing target, directory),
    `TestWriteFileAtomicOrInPlace_InPlaceFailureReportsBoth`, `TestResolveSymlinkTarget`,
    `TestWriteFileAtomicOrInPlace_TightensBroadModeBeforeInPlaceWrite`,
    `TestWriteFileAtomicOrInPlace_RefusesInPlaceWriteWhenModeCannotBeTightened`. Fixture modes are set with
    `os.Chmod`, so they do not depend on the umask.
  - `pkg/auth/cloud/docker`: `TestConfigManager_BindMountedConfig_LoginAndLogout` (EBUSY through the mock),
    `TestConfigManager_ReadOnlyDockerDir_WritableConfig` (real file system, Unix, not root),
    `TestConfigManager_SaveFailsWhenInPlaceWriteFailsToo`.
  - `pkg/auth`: `TestManager_Logout_IntegrationCleanupFailureIsReported`,
    `TestManager_Logout_IntegrationCleanupSuccessIsComplete`. They use a mockgen mock of
    `integrations.Integration` (`pkg/auth/mock_integration_test.go`, with a `//go:generate` directive).
  - `TestConfigManager_RemoveAuth_SaveFailureIsReturned` relied on a read-only directory alone. It now
    also makes `config.json` read-only, since a writable file is now written in place.

### 4. ECR and ACR login skip `get` for `docker-credential-ecr-login`

- **Problem.** For a registry routed to `ecr-login`, login ran `docker-credential-ecr-login get` before
  `store`. The real helper answers `get` by calling ECR with the ambient AWS credentials. That is slow,
  may use the wrong account, or fails before Atmos stores anything.
- **Why `get` exists.** #3742 added `snapshotHelperCredential` to read the previous credential before a
  store. Some helpers (osxkeychain) delete the old credential before adding the new one, so a failed store
  or a failed `config.json` save can be rolled back to the working login.
- **Fix** (`pkg/auth/cloud/docker/config.go`, `pkg/auth/cloud/docker/helper_managed.go`).
  - `helperResolvesCredentialsItself` matches the helper name `ecr-login`. For it, the snapshot is
    "unknown", so a rollback leaves the helper alone, and `store` always runs. Its "not implemented"
    reply still marks the registry as helper-managed, as before.
  - Other helpers are still read first.
- **Tests.** `TestConfigManager_WriteAuth_EcrLoginHelperIsNotRead` (through `credHelpers` and `credsStore`),
  `TestConfigManager_WriteAuth_EcrLoginStoreFailureLeavesHelperAlone`,
  `TestConfigManager_WriteAuth_OtherHelpersAreStillRead`.

### 5. The MSAL cache lock file is removed, and symlinks are kept

- **Problem.**
  - Atmos left `~/.azure/msal_token_cache.json.lockfile` behind, so every later `az` command waited about
    5 seconds for it.
  - A symlinked `msal_token_cache.json` was replaced by a regular file.
- **Cause.**
  - Atmos only took an flock on `<cache>.lockfile`. msal-extensions (the library `az` uses) treats the
    file's existence as the lock signal. Its `CrossPlatLock` waits up to 5 seconds, polling every 0.25
    seconds, until it can create the file exclusively. Then it locks the file and writes `<pid> <argv0>`.
    On release it unlocks and deletes the file.
  - The atomic write renamed a new file onto the symlink.
- **Fix** (new `pkg/auth/cloud/azure/msal_lockfile.go`, `pkg/auth/cloud/azure/cli_msal_cache.go`,
  `pkg/auth/cloud/azure/msal_cache.go`).
  - `withMSALCacheFileLock` follows the msal-extensions convention:
    - It first claims the lock file (`claimMSALLockfile`): it creates it exclusively and writes the owner
      record, or waits while another process holds it.
    - A lock file that exists but that nobody holds is taken over at once. It was left by a crashed
      process or an earlier build.
    - After 5 seconds Atmos waits on the lock itself, like msal-extensions.
    - The file is deleted when the lock is released (`releaseMSALLockfile`).
    - When the lock cannot be acquired after this process created the lock file (a timeout or a
      cancellation), the claim is withdrawn (`msalLockfileClaim.abandon`). It is removed only while the
      path still names the file this process created and a probe lock shows nobody holds it, so a lock
      file another process created since, or this same file after another process took it over and
      locked it, is kept (`TestMSALLockfileClaim_AbandonKeepsAFileAnotherProcessLocked`).
    - The flock still provides the exclusion. The legacy `<cache>.lock` is unchanged.
  - `UpdateAzureCLIMSALCache` and `msalCache.Export` write through the new `writeSharedMSALCache`. It
    resolves symlinks first (`filesystem.ResolveSymlinkTarget`), so the target is updated and the link
    kept. It also uses `WriteFileAtomicOrInPlace`, so the shared cache gets the same in-place fallback as
    `config.json`. v1.234.1 wrote this file in place too.
- **Tests.** `TestUpdateAzureCLIMSALCache_RemovesLockfile`, `TestMSALCache_ReplaceAndExportRemoveLockfile`,
  `TestMSALLockfile_WaitsForLiveLockfile` (a holder that creates and locks the file as az does),
  `TestMSALLockfile_StaleLockfileIsTakenOver`, `TestMSALLockfile_TimeoutFallsBackToTheLock`,
  `TestClaimMSALLockfile_StopsWaiting` (cancellation, a lock file that cannot be probed),
  `TestUpdateAzureCLIMSALCache_KeepsSymlink`, `TestMSALCache_ExportKeepsSymlink`,
  `TestMSALCache_ExportReadOnlyDirWritesInPlace`, `TestMSALCacheLock_LockFailureRemovesOwnClaim`,
  `TestMSALCacheLock_LockFailureKeepsForeignLockfile`, `TestMSALLockfileClaim_AbandonOnlyRemovesTheSameFile`,
  `TestMSALCacheLock_DoesNotHoldARemovedLockfile` (a holder deletes the path while locked and a new
  holder locks a new file there; Atmos waits for the new one, because the lock reopens the path on
  every attempt). `TestMSALCache_ExportWriteError` now pre-creates both
  lock files and makes the cache file read-only, because Replace no longer leaves the lock file behind and
  a writable file is now written in place.

### 6. Keyring migration copies the legacy entry

- **Problem.** After one 1.235.0 login, v1.234.1 could not find `aws/user` long-lived keys.
  `atmos_<realm>_<alias>` had been moved to `atmos_<realm>__<alias>` and the old key deleted. A user who
  rolled back had to enter the access keys again.
- **Fix** (`pkg/auth/credentials/keyring_store.go`).
  - `migrateLegacyRealmEntry` copies the entry and keeps the legacy key.
  - Every later store of the alias (`atmos auth user configure` replacing the keys, or a login) also
    rewrites the kept legacy entry (`syncRetainedLegacyEntry`), so a downgrade never uses superseded
    keys. Only an existing, unambiguous legacy entry that belongs to the alias is rewritten. None is
    created.
  - New code always reads the current key first, so the legacy copy is only a fallback for older
    versions.
  - `Delete` already removes both keys, with the same ownership checks. A keychain logout and
    `logout --all-realms --keychain` still remove everything.
  - The realm-safety rules from the 2026-11-22 fix are unchanged. Only unambiguous legacy keys (and, under
    the automatic realm, legacy keys with `_`) are read, copied or deleted. Ambiguous keys still need
    confirmation.
  - Adopting a confirmed ambiguous entry still deletes the legacy key, for the reason recorded in
    `AdoptAmbiguousLegacyRealmEntry`.
- **Tests.**
  - `TestKeyringStore_MigratesUnambiguousLegacyRealmKey` and `TestKeyringStore_MigratesLegacyAWSUserLongLivedKeys`
    now assert that the legacy key is kept. The second also asserts that `Delete` removes both keys.
  - `TestKeyringStore_AutoRealmLegacyKeyWithUnderscoreAliasIsMigrated` covers the automatic realm with
    `prod_admin`, the regression-test case. It asserts the key is kept, listed once and deleted by
    `Delete`.
  - New `TestKeyringStore_MigratedEntryReadsOnlyTheCurrentKey`, `TestKeyringStore_StoreKeepsRetainedLegacyKeyInSync`
    and `TestKeyringStore_StoreDoesNotCreateOrTouchOtherLegacyKeys` (no legacy key, an ambiguous one,
    one recorded for another alias).
  - The best-effort subtest "legacy delete fails" became "legacy key is never deleted".
  - `TestUserIdentity_LegacyKeyring_AutoRealmMigratesWithoutPrompt` (`pkg/auth/identities/aws`) asserts the
    kept key.
  - `docs/prd/auth-realm-architecture.md` was updated.

### 7. The configuration digest record is no longer a keyring entry

- **Problem.** `auth login` wrote `atmos-config-digest_<realm>__<identity>` to the keyring for every
  identity. That included identities that never otherwise use the keyring, such as `aws/user` with inline
  keys, whose session credentials live in files. On macOS a new Keychain item can show a prompt, and a
  login once hung for more than 2 minutes. The file keyring asks for its password.
- **Design.** The record is written by `recordExternalStepDigest` (`pkg/auth/manager_chain_step_cache.go`).
  It guards credentials that an identity keeps in its own files. `verifyChainStepDigest` reuses those
  files as an intermediate chain step only when the record matches the current configuration, the
  expiration and the access key ID. Credentials stored in the keyring carry their digest inside the
  envelope instead. Skipping the record would make every such intermediate step authenticate again, which
  means an MFA prompt each time for an inline `aws/user` with MFA. Name-only reuse would undo #3720
  finding 2.1. So the record is kept, but it is stored as a file.
- **Fix** (new `pkg/auth/credentials/config_digest_file.go`, `pkg/auth/credentials/config_digest.go`,
  `pkg/auth/credentials/keyring_backends.go`).
  - The persistent backends keep the record in a plain file and never touch the keyring for it:
    - File keyring: `<keyring dir>.config-digests/`, so a custom keyring path keeps its records with it.
    - System keyring: the Atmos XDG cache directory, `auth/config-digests/`.
  - The file name is the SHA-256 of the record key, so identity names with `/` or `:` are safe, and realms
    stay separated. Permissions are owner only. The record holds nothing secret and is as trustworthy as
    the credential files it describes.
  - The memory backend still keeps the record in its process-local keyring.
  - `ConfigDigestStore` is unchanged, so `Logout`, `Delete` and `logout --all-realms` remove the file.
- **Tests.** `TestExternalConfigDigest_PersistentBackendsNeverTouchTheKeyring` (system and file, with a
  mocked keyring that fails on any call), `TestExternalConfigDigest_FileRecordErrors`,
  `TestExternalConfigDigest_FileRecordWriteFailure`, `TestExternalConfigDigest_DeleteRemovesFileRecord`,
  `TestRecordIsMissing` (a non-directory in place of the record directory is a store failure on every
  platform; Windows reports it as "path not found"),
  `TestPersistentStoresKeepDigestRecordsOutsideTheKeyring`. `TestDeleteRealmKeyringEntries_RemovesLegacyAndDigest`
  (`cmd/auth`) now seeds and checks the record through the store.

### Docs

- `website/docs/cli/commands/auth/auth-whoami.mdx`: ambient identities and Azure validation.
- `website/docs/cli/commands/aws/ecr-login.mdx` and `website/docs/cli/commands/azure/acr-login.mdx`:
  single-file mounts, read-only directories, partial logout, and no `get` for `ecr-login`.
- `website/docs/cli/commands/auth/user/configure.mdx`: the legacy key is kept for downgrades.
- `docs/prd/auth-realm-architecture.md`: migration and digest record storage.

## Validation

All commands ran in this worktree on branch `aknysh/fix-1235-auth-regressions`, based on `origin/main` at
`31b76b6b70`, with Go on darwin/arm64.

- Every new regression test was run before its fix and failed. That covers whoami ambient (cmd and
  manager), the Azure unreachable, no-subscription and classification cases, the bind-mount and read-only
  `config.json` cases, the `ecr-login` `get`, the MSAL lock file and symlink, keyring copy-not-move, the
  digest keyring entry, and the logout cleanup error. Temporary stubs let the new tests compile.
- `go build ./...`: passed.
- `go vet` on `./pkg/filesystem/ ./pkg/auth/... ./cmd/auth/ ./errors/` with `GOOS=darwin`, `GOOS=windows` and
  `GOOS=linux`: passed.
- `gofumpt -l` on the changed directories: no files reported.
- `go test -count=1 ./pkg/auth/... ./cmd/auth/... ./pkg/filesystem/... ./errors/... ./cmd/aws/... ./cmd/azure/...
  ./pkg/keyring/... ./pkg/cache/...`: all 74 packages passed.
- Real XDG cache check: the tests wrote no digest records to `~/.cache/atmos/auth` or
  `~/Library/Caches/atmos`.
- Patch coverage, from `go test -coverpkg` over the changed packages (`cmd/auth`, `pkg/auth`,
  `pkg/auth/types`, `pkg/auth/cloud/docker`, `pkg/auth/cloud/azure`, `pkg/auth/credentials`,
  `pkg/filesystem`, `errors`), measured on added and changed non-test lines against `origin/main`:
  189 of 193 statement lines are covered (97.9%). Uncovered lines:
  - `msal_lockfile.go`: the debug log when writing the owner record into a just-created lock file fails.
  - `replace.go`: `os.Readlink` failing right after `os.Lstat` reported a symlink, which is a race.

  Both are defensive. This measures the touched packages only, not the full suite; CI's Codecov run is
  authoritative.

Not run:

- `golangci-lint run --new-from-rev=origin/main ./...`. The session's command permissions refused it, so
  CI lint is the first lint pass (Git hooks are disabled in this clone).
- The website build, `go test -race`, and tests on Linux or Windows.
- A real single-file bind mount, a real `docker-credential-ecr-login`, a real `az`, or the macOS Keychain.
  The tests use a mocked file system, fake helpers, an az-style lock holder, and file or memory keyrings.

## Follow-ups

- The Atmos SSO token cache (`pkg/auth/providers/aws/sso_cache.go`) also changed to an atomic write in this
  release. The regression test lists the same symlink pattern there but did not run it. It is an
  Atmos-owned cache, so it was left alone here. It is part of the unreleased 1.235.0 changes, so any change
  belongs in this release work, not in an issue.
- Suspected and not reproduced in the regression test: the Windows atomic write removes the target before
  the rename (`pkg/filesystem/atomic_windows.go`), so a failed rename there leaves no `config.json`.
  `WriteFileAtomicOrInPlace` cannot recover that case, because the target is gone.
