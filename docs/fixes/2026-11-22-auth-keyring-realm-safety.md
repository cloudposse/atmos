# Fix: Atmos Auth keyring keys, credential caches and logout are realm-safe

**Date:** 2026-11-22

## Summary

Atmos Auth could hand one identity's credentials to another. Realm-scoped keyring keys were ambiguous
across realms, and both the in-process cache and the cross-process chain step reuse were keyed by name
only. Logout also left entries behind that a later login could revive.

This change fixes those bugs from the Atmos Auth pre-release review (#3720), plus two regressions and a
logout gap found by comparing the 1.235.0 candidate with v1.234.1. It also includes three smaller
#3720 fixes in the same code paths: credential broker retry and refresh, the operation identity grace
period, and case-insensitive names for isolated identities. Every fix has a test. Other #3720 and
regression-test fixes land in separate PRs.

## Context

- Realm-scoped keyring keys were `atmos_<realm>_<alias>`. Realms and aliases may both contain `_`, so
  realm `acme` + identity `prod_admin` and realm `acme_prod` + identity `admin` shared the key
  `atmos_acme_prod_admin`. One realm read and overwrote the other's credentials.
- When no realm is configured, Atmos sets the realm to a hash of the `atmos.yaml` path (16 lowercase
  hex characters). So every user is realm-scoped, and any rule written for ambiguous keys also applied
  under that automatic realm. This caused regression A2 in the regression test.
- Stack and component `auth` overrides can give the same identity or provider name a different
  account, role or start URL. The process cache, the keyring and identity credential files were keyed
  by name, so a second manager or a later process could reuse credentials minted for another
  configuration.
- `aws/user` long-lived keys are stored by `atmos auth user configure`. A login cannot re-create them,
  so a key layout change must not silently drop them.

## Changes

### Keyring key layout and legacy keys

- **Realm and alias are separated by `__`** (#3720 finding 3.4, `pkg/auth/credentials/store.go`).
  - Realm-scoped keys are now `atmos_<realm>__<alias>` (`KeyringRealmAliasSeparator`). The existing
    `realm.Validate` already forbids `__` and a trailing `_` in realms, so the first `__` always ends
    the realm.
  - The previous layout `atmos_<realm>_<alias>` is still read, and deleted by `Delete`, only when
    neither part contains `_` (`legacyRealmKeyIsUnambiguous`). Ambiguous old keys are never read or
    deleted by `Retrieve` and `Delete`.
  - The pre-realm `atmos_<alias>` cleanup after a realm-scoped login (`deleteLegacyKeyringEntry` in
    `pkg/auth/realm_mismatch.go`) and the stale ambient entry purge (`purgeCachedCredentials` in
    `pkg/auth/manager_chain.go`) skip aliases containing `_`, whose key another realm can share
    (`EmptyRealmKeyIsUnambiguous`).
  - `List` returns current and unambiguous legacy aliases, each once. With an empty realm it still
    strips only the `atmos_` prefix, so callers now see `realm__alias`.
  - `docs/prd/auth-realm-architecture.md` describes the new layout (version 1.2).
  - Tests: `TestKeyringKeys_DoNotCollideAcrossRealms`,
    `TestKeyringKeys_EmptyRealmNeverCollidesWithRealmScoped`,
    `TestKeyringStore_ReadsUnambiguousLegacyRealmKey`, `TestKeyringStore_IgnoresAmbiguousLegacyRealmKey`,
    `TestKeyringStore_LegacyFallbackPropagatesBackendErrors`, `TestFilterRealmKeys_SeparatesRealms`,
    `TestKeyringStore_DeleteReportsLegacyDeleteFailure`, `TestEmptyRealmKeyIsUnambiguous`,
    `TestLegacyKeyringCleanup_NeverDeletesAnotherRealm`,
    `TestLegacyKeyringCleanup_RemovesUnambiguousLegacyEntry`. Existing key-format assertions in
    `keyring_file_test.go`, `keyring_memory_test.go` and `storage_contract_test.go` were updated.
- **Unambiguous legacy entries migrate on read** (#3720, `pkg/auth/credentials/keyring_store.go`).
  - An entry found through the legacy fallback is copied unchanged to the new key, and the legacy key is
    deleted, so later reads no longer need the fallback. `aws/user` keys go through the same
    `Retrieve`, so they migrate on first use.
  - The migration is best effort and never fails the read. It is skipped when the new key already
    exists (newer credentials win). Only entries that decode are copied. On a write or delete failure
    the legacy key is kept and the fallback keeps serving it.
  - Tests: `TestKeyringStore_MigratesUnambiguousLegacyRealmKey`,
    `TestKeyringStore_MigratesLegacyAWSUserLongLivedKeys`,
    `TestKeyringStore_AmbiguousLegacyKeyIsNeverMigrated`, `TestKeyringStore_CorruptLegacyEntryIsNotMigrated`,
    `TestKeyringStore_LegacyMigrationIsBestEffort`.
- **Under the automatic realm, legacy keys with `_` migrate silently** (regression A2 vs v1.234.1,
  `pkg/auth/credentials/keyring_legacy_auto_realm.go`, `pkg/auth/realm/realm.go`).
  - Problem: after the layout change, every `aws/user` identity with `_` in its name failed in CI
    until `atmos auth user configure` was re-run, and session identities lost cached credentials.
  - Cause: the automatic realm never contains `_`, so the only other owner of a legacy key
    `atmos_<hash>_a_b` is a configured realm named `<hash>_a`, which nobody configures. The code still
    treated those keys as ambiguous.
  - Fix: when the realm has the exact automatic format (`realm.HasAutoValueFormat`, 16 lowercase hex
    characters), such keys are read, migrated, deleted and listed like unambiguous keys. A realm that
    the store's config says was set explicitly (`auth.realm` or `ATMOS_AUTH_REALM`) keeps the
    confirmation, even when it has the hex format. Legacy keys containing `__` are never touched,
    because they are also another alias's current key.
  - The store receives only the realm string, so the automatic realm is recognized by its format.
    `NewCredentialStoreWithConfig` records the explicit realm on the store to exclude it.
  - Tests: `TestKeyringStore_AutoRealmLegacyKeyWithUnderscoreAliasIsMigrated`,
    `TestKeyringStore_AutoRealmLegacyKeyIsDeleted`, `TestKeyringStore_ExplicitRealmLegacyKeyStaysAmbiguous`,
    `TestKeyringStore_AutoRealmNeverTouchesAnotherAliasCurrentKey`,
    `TestKeyringStore_AutoRealmFromConfigPathIsNotExplicit`,
    `TestUserIdentity_LegacyKeyring_AutoRealmMigratesWithoutPrompt`, `TestHasAutoValueFormat`, and in
    `pkg/config` `TestHashRealmFromPath_HasAutoValueFormat` and `TestSetAuthRealm_Sources`, which pin the
    coupling with the realm Atmos derives from the `atmos.yaml` path.
- **Ambiguous legacy `aws/user` keys are adopted only after confirmation** (#3720,
  `pkg/auth/identities/aws/user_legacy_keyring.go`, `pkg/auth/credentials/keyring_legacy_ambiguous.go`).
  - Problem: long-lived keys stored under an ambiguous old key were never read, so affected users
    silently lost them.
  - The prompt appears only when an `aws/user` identity has no keys under the current key and the
    ambiguous old key decodes to complete long-lived AWS keys (not a session). Old keys that contain
    `__` are never offered.
  - It is shown only in an interactive terminal (stdin and stderr are terminals, not CI, interactive
    mode on, prompts allowed). It names the identity, the realm, the old key and the other realm and
    identity pairs that shared it. The default answer is No.
  - Yes copies the entry to the new key (unless the new key appeared meanwhile), deletes the old key and
    continues authentication.
  - No, or a non-interactive session, leaves the keyring unchanged and returns `ErrAwsUserNotConfigured`
    with `ErrAwsUserLegacyKeyringEntryNotAdopted` as its cause and hints that name the old key. A No is
    not asked again for that identity in the same invocation.
  - Browser sign-in still runs when enabled, and if it fails the old-key hints are kept in the error.
  - The prompter is an interface with a mockgen mock (`mock_user_legacy_keyring_test.go`), so tests need
    no terminal. `website/docs/cli/commands/auth/user/configure.mdx` gained a "Credentials Stored by
    Older Versions" section.
  - Tests: `TestFindAmbiguousLegacyRealmEntry_ReportsEntryAndOtherOwners`,
    `TestFindAmbiguousLegacyRealmEntry_NothingToOffer`, `TestFindAmbiguousLegacyRealmEntry_KeyringErrors`,
    `TestAdoptAmbiguousLegacyRealmEntry_MovesEntry`,
    `TestAdoptAmbiguousLegacyRealmEntry_NeverOverwritesCurrentKey`,
    `TestAdoptAmbiguousLegacyRealmEntry_Failures`, `TestOtherLegacyRealmOwners`,
    `TestUserIdentity_LegacyKeyring_YesAdoptsEntryAndAuthenticates`,
    `TestUserIdentity_LegacyKeyring_NoReturnsHintedErrorAndKeepsKeyring`,
    `TestUserIdentity_LegacyKeyring_NonInteractiveNeverPrompts`,
    `TestUserIdentity_LegacyKeyring_NoPromptWhenCurrentKeyExists`,
    `TestUserIdentity_LegacyKeyring_NoPromptWithoutLegacyKey`,
    `TestUserIdentity_LegacyKeyring_SessionEntryIsNotOffered`,
    `TestUserIdentity_LegacyKeyring_NoRealmNeverPrompts`,
    `TestUserIdentity_LegacyKeyring_WebflowFailureKeepsLegacyHints`,
    `TestUserIdentity_LegacyKeyring_PromptFailures`, `TestLegacyKeyringPromptText`,
    `TestHuhLegacyKeyringPrompter_FailsFastWhenNotInteractive`, `TestUserIdentity_LegacyPrompterOrDefault`,
    `TestAdoptAmbiguousLegacyKeyringEntry_LookupFailureKeepsNotFoundHandling`.

### Credential reuse keyed on effective configuration

- **The process credential cache is keyed on effective config** (#3720 finding 2.1,
  `pkg/auth/manager_chain_fingerprint.go`).
  - Problem: with `--all`, `describe affected` or the MCP server, a second manager in the same process
    whose overrides gave an identity a different role or account received the first manager's
    credentials.
  - Fix: the key is `realm:chain#fingerprint`, where the fingerprint is an HMAC (per-process secret from
    `pkg/auth/fingerprint`) of each chain step's effective provider or identity config. The config can
    hold secrets, so it is never a plain digest. A config that cannot be serialized bypasses the cache.
  - Tests: `TestProcessCredentialCache_DifferentIdentityConfigMisses`,
    `TestProcessCredentialCache_DifferentProviderConfigMisses`,
    `TestProcessCredentialCache_UnserializableConfigSkipsCache`. `manager_chain_process_cache_test.go` now
    seeds the cache under the manager's real key.
- **Cross-process reuse of intermediate chain steps checks a config digest** (rest of #3720 finding 2.1,
  `pkg/auth/manager_chain_step_digest.go`, `pkg/auth/manager_chain_step_cache.go`,
  `pkg/auth/manager_credential_load.go`, `pkg/auth/credentials/config_digest.go`).
  - Problem: after an override changed an intermediate step, a later process reused that step's cached
    credentials as the source of the next step.
  - The digest is a plain SHA-256 of canonical JSON over the non-secret config of every step from the
    chain root through that step. Providers contribute `kind`, `start_url`, `url`, `region`, `username`
    and `spec`. Identities contribute `kind`, `provider`, `via`, `principal`, `spec` and `emulator`.
  - Secrets are never hashed: the provider `password`, identity `credentials`, and `principal`/`spec`
    keys that name secret material (`*secret*`, `*token*`, `*password*`, `*private_key*`,
    `*access_key*`, `external_id` and others). References ending in `_path`, `_file`, `_url`, `_uri`,
    `_endpoint`, `_env` or `_arn` are kept.
  - Keyring envelopes gain an optional `config_digest` field. It is omitted when empty, so envelopes
    without it are byte-identical to older ones, and older versions ignore it.
  - Credentials an identity keeps in its own files (AWS session credentials) get a record
    `{"digest", "expiration"}` under `atmos-config-digest_<realm>__<identity>` after login. The
    expiration ties the record to those exact files. `List` never reports these keys.
  - An intermediate step whose digest is missing or different is a cache miss (`ErrChainStepConfigChanged`)
    and is authenticated again. The target step is always re-authenticated, as before. Stores that
    cannot persist digests (custom stores, the noop keyring) keep name-only reuse.
  - `auth whoami` caches non-session credentials with their digest (`storeNamedCredentials` in
    `pkg/auth/manager_whoami.go`). The operation-scoped store forwards digests with the same realm
    redirection as the credentials (`pkg/auth/manager_operation_digest.go`).
  - Credential loading moved from `manager_chain.go` to `manager_credential_load.go` and now reports
    where the credentials came from.
  - Tests: `TestCredentialEnvelope_DigestRoundTrip`, `TestCredentialEnvelope_CompatibleWithOlderVersions`,
    `TestExternalConfigDigest_RoundTripAndCleanup`, `TestExternalConfigDigest_Errors`,
    `TestSupportsConfigDigests`, `TestBuildConfigDigestKey`, `TestRetrieveWithConfigDigest_BackendError`,
    `TestChainStepKeyringCache_ChangedIntermediateConfigMisses`,
    `TestChainStepKeyringCache_ChangedProviderConfigMisses`,
    `TestChainStepFileCache_ChangedIntermediateConfigMisses`,
    `TestChainStepKeyringCache_EntryWithoutDigestMissesForIntermediateStep`,
    `TestChainStepKeyringCache_WhoamiCacheKeepsDigest`, `TestPersistedStepConfig_CanonicalFields` (pins the
    hashed fields), `TestChainPrefixDigest_TracksOnlyPrincipalFields`,
    `TestNamedStepDigest_MatchesChainIndex`, `TestIsSecretConfigKey`,
    `TestChainStepFileCache_RecordMustMatchCredentials`,
    `TestChainStepCache_NoDigestPersistenceKeepsNameOnlyReuse`,
    `TestChainStepCache_StoreWithoutDigestsKeepsNameOnlyReuse`,
    `TestRecordExternalStepDigest_FailureIsNonFatal`, `TestRecordExternalStepDigest_SkipsUndigestableConfig`,
    `TestVerifyChainStepDigest_Reasons`, `TestExpirationStamp`,
    `TestOperationCredentialStore_ForwardsDigests`.

### Logout cleanup

- **Keychain logout deletes the ambiguous legacy key** (regression A3 vs v1.234.1,
  `pkg/auth/manager_logout.go`, `pkg/auth/manager_logout_keyring.go`).
  - Problem: `atmos auth logout --keychain` reported success but kept the old `atmos_<realm>_<name>`
    key whenever the realm or name contains `_`. A later login then offered to revive the credentials
    the user had just deleted. v1.234.1 deleted that key as the identity's own.
  - Fix: keychain logout of an identity (also through `--all`) and of a provider deletes that key too,
    and logs a warning naming the other realm and name pairs it may have belonged to. A failed delete
    makes the logout partial, unless the delete was only forced (best effort). Keys containing `__` are
    never deleted, and logout without `--keychain` still keeps the keyring.
  - Tests: `TestManager_LogoutKeychain_DeletesAmbiguousLegacyKey`,
    `TestManager_LogoutAllKeychain_DeletesAmbiguousLegacyKeys`,
    `TestManager_LogoutWithoutKeychain_KeepsLegacyKeys`,
    `TestManager_LogoutKeychain_AmbiguousLegacyDeleteFailure`,
    `TestManager_LogoutKeychain_AmbiguousLegacyDeleteSkipped`,
    `TestManager_LogoutProviderKeychain_DeletesAmbiguousLegacyKey`, `TestDescribeLegacyRealmOwners`,
    `TestDeleteAmbiguousLegacyRealmEntry_DeletesEntryAndNamesOtherOwners`,
    `TestDeleteAmbiguousLegacyRealmEntry_DeletesUndecodableEntry`,
    `TestDeleteAmbiguousLegacyRealmEntry_NothingToDelete`,
    `TestDeleteAmbiguousLegacyRealmEntry_KeyringFailures`.
- **Logout removes the configuration digest record** (regression A7 found by the same comparison,
  `pkg/auth/manager_logout.go`, `pkg/auth/credentials/config_digest.go`).
  - Problem: only a keychain delete removed `atmos-config-digest_*`, so every plain logout left the record
    behind although it removes the files the record describes.
  - Fix: identity logout, with or without `--keychain`, calls `DeleteExternalConfigDigest`. It is best
    effort. `Delete` also removes the record.
  - Tests: `TestManager_Logout_DeletesConfigDigestRecord`, `TestManager_Logout_DigestDeleteFailureIsNonFatal`,
    `TestManager_LogoutAll_DeletesConfigDigestRecords`.
- **`logout --all-realms --keychain` clears legacy keys and digests** (part of regressions A3 and A7,
  `cmd/auth/logout_all_realms_keyring.go`).
  - Problem: the all-realms keychain logout deleted only each identity's current-layout entry.
  - Fix: `deleteRealmKeyringEntries` also deletes the ambiguous legacy key (with the same warning) and the
    digest record, for each configured identity in sorted order. Every step stays best effort.
  - Tests: `TestDeleteRealmKeyringEntries_RemovesLegacyAndDigest`,
    `TestDeleteRealmKeyringEntries_MissingEntriesAreNotErrors`.

### Smaller fixes in the same paths

- **Credential brokers retry and refresh** (#3720 suspected item, `pkg/auth/broker/broker.go`).
  - Problem: `EnsureCredentials` used a process-wide `sync.Once`. A first call that was cancelled or
    failed transiently disabled brokering for the whole process. A long-lived process (the MCP server,
    a long `--all` run) never refreshed a `github/sts` token after its one-hour lifetime.
  - Fix: a mutex records the last run. A run interrupted by the caller's cancellation is not recorded,
    an already cancelled caller does not run the brokers, a failed run is retried after 30 seconds, and
    a successful run is refreshed after 30 minutes.
  - Tests: `TestEnsureCredentials_RetriesAfterCancelledFirstCall`,
    `TestEnsureCredentials_SkipsAlreadyCancelledCaller`, `TestEnsureCredentials_RefreshesAfterInterval`,
    `TestEnsureCredentials_RetriesFailedRunAfterBackoff`, `TestSwapRegistryForTest_ResetsRunState`.
    `broker_test.go` uses the new reset helper.
- **No hard 45-second limit on operation-scoped login** (#3720 finding 1.2, a regression since v1.234.1,
  `pkg/auth/manager_isolated.go`).
  - Problem: `WithIsolatedIdentity` wrapped the whole acquisition in a 45-second cleanup context, which
    dropped the caller's deadline and cut off interactive logins (SSO device code, browser OIDC) during
    store operations.
  - Fix: the 45-second grace starts only when the caller cancels or its deadline passes, with a 30-minute
    hard ceiling for an acquisition nobody cancels (`operationAcquisitionContext`).
  - Tests: `TestWithIsolatedIdentityAcquisitionHasNoShortHardLimit`,
    `TestWithIsolatedIdentityAcquisitionGraceStartsOnCallerCancellation`,
    `TestOperationAcquisitionContextStopsWithoutCallerCancellation`,
    `TestOperationAcquisitionContextReleaseDuringGrace`.
- **`WithIsolatedIdentity` resolves case-insensitive names** (#3720 finding 1.1, a regression since
  v1.234.1, `pkg/auth/manager_isolated.go`).
  - Problem: a store configured with the YAML spelling of an identity (for example `identity: Automation`
    with `identity_lifetime: operation`) failed every operation with identity not found.
  - Fix: the name goes through `resolveIdentityName` first, as `Authenticate` already does. An unknown
    name is kept so the chain builder reports it.
  - Test: `TestWithIsolatedIdentityResolvesCaseInsensitiveNames`.

New sentinels in `errors/errors_execution.go`: `ErrChainStepConfigChanged` and
`ErrAwsUserLegacyKeyringEntryNotAdopted`.

## Validation

All commands were run in this PR's worktree, on top of `origin/main` at `2d7822f214`.

- `go build ./...`: passed.
- `go vet` on `./cmd/auth`, `./errors`, `./pkg/auth`, `./pkg/auth/broker`, `./pkg/auth/credentials`,
  `./pkg/auth/identities/aws`, `./pkg/auth/realm` and `./pkg/config`: passed.
- `gofumpt -l` on the changed Go files reported no files.
- `go test -count=1 -cover` passed for all eight packages above. The first combined run failed to link
  three test binaries because the disk was full (`No space left on device`, other builds were running);
  those three packages (`pkg/auth`, `pkg/auth/identities/aws`, `pkg/config`) were re-run one at a time
  and passed.
- The PR does not touch the `./cmd` root package or `./internal/exec`. Their full suites run in CI.
- Package statement coverage, `origin/main` to this PR (measured with `go test -count=1 -cover`, one
  package at a time, in a temporary detached worktree of `origin/main` that was then removed):

| Package | Before | After |
|---|---|---|
| `cmd/auth` | 82.2% | 82.1% |
| `errors` | 87.3% | 87.3% |
| `pkg/auth` | 91.7% | 92.4% |
| `pkg/auth/broker` | 76.0% | 100.0% |
| `pkg/auth/credentials` | 88.7% | 93.9% |
| `pkg/auth/identities/aws` | 90.8% | 91.0% |
| `pkg/auth/realm` | 97.4% | 97.4% |

`cmd/auth` drops by 0.1 point. The new `deleteRealmKeyringEntries` is 80% covered: its three uncovered
statements are debug logs for failed deletes. The keychain branch of `performLogoutAllRealms` that calls
it is not reached by the package tests. `errors` only gains variable declarations. `pkg/config` changed
only by new tests, so it is not in the table.

Not run:

- golangci-lint.
- Race-detector runs, Linux and Windows runs, and cross-OS `go vet`.
- `TestCLICommands` snapshot tests and the website build (`cd website && npm run build`) for the doc
  changes in `configure.mdx`.
- The interactive `huh` confirmation form itself, which needs a real terminal; tests use a mock
  prompter.

## Follow-ups

Choices made in these fixes that can be revisited:

- **Automatic realm recognized by format.** The credential store receives only the realm string, so
  the automatic realm is recognized by its 16-hex-character format, and the explicit realm is excluded
  only when the store was built from the auth config. Restoring `RealmSource` through
  `MergeComponentAuthConfig` would narrow this.
- **Keychain logout deletes an ambiguous legacy key** even if it belonged to another realm, matching
  v1.234.1, with a warning naming the other owners.
- **Legacy `aws/user` key prompt.** After a No, browser sign-in still runs when it is enabled (the
  default). A No lasts only for the current invocation, so a user who does not own the entry is asked
  again on later runs until they configure keys.

Compatibility notes for users:

- Realm-scoped keyring keys change to `atmos_<realm>__<alias>`. Unambiguous legacy entries, and under
  the automatic realm also entries with `_`, migrate on first read. Under an explicit realm, ambiguous
  `aws/user` entries need a one-time confirmation or `atmos auth user configure`; other identities log
  in again once.
- `List` with an empty realm now reports realm-scoped aliases as `realm__alias`.
- After upgrading, each intermediate chain step re-authenticates once, because entries written by older
  versions have no config digest.
