# Auth Realm Isolation Issues (v1.206.0)

After merging PR #2043 (auth realm isolation with credential separation), several issues have been reported by users.
All four issues are now fixed.

## Issue #1: Profile Incorrectly Required for Default Identity (FIXED)

**GitHub Issue:** https://github.com/cloudposse/atmos/issues/2071

**Reported Behavior:**
After upgrading to v1.206.0, any atmos command (including `atmos version`) fails with a "profile not found" error when
`ATMOS_PROFILE` env var is set to an auth identity name.

**Root Cause:**
Users set `ATMOS_PROFILE=root-admin` thinking it controls auth identity selection, but `ATMOS_PROFILE` controls
configuration profiles (log levels, terminal settings). When set to an identity name that doesn't match any profile
directory, `loadProfiles()` fails.

**Fix:**
Enhanced the "profile not found" error in `pkg/config/profiles.go` to detect when the profile name matches an auth
identity name and suggest using `ATMOS_IDENTITY` instead. The error now shows:

- "`root-admin` is an auth identity, not a configuration profile"
- "If you want to select an auth identity, use `ATMOS_IDENTITY` or `--identity` instead of `ATMOS_PROFILE`"

**Files changed:** `pkg/config/profiles.go`

---

## Issue #2: Realm-Scoped Credential Paths Break CI/CD Workflows (FIXED)

**Reported Behavior:**
After upgrading to v1.206.0, CI/CD pipelines fail with AWS credential errors because credential file paths changed from
`{baseDir}/aws/{provider}/credentials` to `{baseDir}/{realm}/aws/{provider}/credentials`.

**Root Cause:**
PR #2043 auto-generated a realm from a SHA256 hash of the CLI config path, making realm isolation always-on. This broke
backward compatibility for CI/CD environments.

**Fix:**
Made realm isolation opt-in by changing the auto-realm behavior in `pkg/auth/realm/realm.go`:

- When `auth.realm` is not configured AND `ATMOS_AUTH_REALM` is not set, use empty realm (no realm subdirectory)
- Credential paths default to `{baseDir}/aws/{provider}/credentials` (backward-compatible)
- Users who want isolation explicitly set `auth.realm` in config or `ATMOS_AUTH_REALM` env var

Also updated `pkg/auth/cloud/aws/files.go`:

- `CleanupAll()` safely handles empty realm (removes `{baseDir}/aws/` instead of `{baseDir}/`)
- `GetDisplayPath()` handles empty realm correctly

**Files changed:** `pkg/auth/realm/realm.go`, `pkg/auth/cloud/aws/files.go`

---

## Issue #3: Loading Unrelated Stack Files for Auth Defaults (FIXED)

**GitHub Issue:** https://github.com/cloudposse/atmos/issues/2072

**Reported Behavior:**
When running a command scoped to a specific stack (e.g., `atmos terraform apply network -s staging`), Atmos loads ALL
stack files to resolve auth identity defaults, causing "multiple default identities" error when different stacks define
different defaults.

**Root Cause:**
`LoadStackAuthDefaults()` scanned all stack files from `IncludeStackAbsolutePaths` and returned all defaults found,
without knowing which stack is the target (chicken-and-egg problem).

**Fix:**
Modified `LoadStackAuthDefaults()` in `pkg/config/stack_auth_loader.go` to detect conflicting defaults:

- When all stack files agree on the same default identity, that identity is used
- When different stack files define DIFFERENT default identities, the conflicting defaults are discarded (returns empty)
- This prevents the false "multiple default identities" error
- The per-stack default will be resolved after full stack processing

**Files changed:** `pkg/config/stack_auth_loader.go`

---

## Issue #4: ECR Docker Push Fails with 403 After Upgrading to v1.206.0 (FIXED)

**Reported Behavior:**
After upgrading to v1.206.0, GitHub Actions CI/CD pipeline fails with `403 Forbidden` when Docker buildx
pushes to AWS ECR. The Docker login step succeeds, but the push fails:

```
ERROR: failed to push <account>.dkr.ecr.<region>.amazonaws.com/<org>/<repo>:sha-...
unexpected status from HEAD request to .../v2/<org>/<repo>/blobs/sha256:...: 403 Forbidden
```

Pinning Atmos to v1.205.1 immediately resolves the issue.

**Workflow Pattern:**

```yaml
env:
  ATMOS_PROFILE: github  # Loads profile overlay containing auth config

steps:
  - name: Install Atmos
    uses: cloudposse/github-action-setup-atmos@v2
    with:
      atmos-version: ${{ vars.ATMOS_VERSION }}

  - name: ECR Login
    run: |
      atmos auth env --identity <identity-name> --login --format dotenv | sed "s/'//g" >> $GITHUB_ENV

  - name: Build
    uses: cloudposse/github-action-docker-build-push@v2
    with:
      registry: ${{ vars.ECR_REGISTRY }}
```

The auth config lives in a profile overlay (loaded via `ATMOS_PROFILE`) and defines a GitHub OIDC
provider + AWS assume-role identities. The main `atmos.yaml` has no auth section.

**Root Cause:**
The `assumeRoleIdentity` type in `pkg/auth/identities/aws/assume_role.go` hardcoded an empty realm (`""`)
in three methods (`Environment()`, `PrepareEnvironment()`, `CredentialsExist()`) instead of using `i.realm`
like all other identity types (`assume_root.go`, `user.go`, `permission_set.go`).

This caused a credential path mismatch:

- `Authenticate()` → `PostAuthenticate()` → `SetupFiles()` writes credentials to
  `{baseDir}/{realm}/aws/{provider}/credentials` (using the actual realm)
- `GetEnvironmentVariables()` → `Environment()` returns `AWS_SHARED_CREDENTIALS_FILE` pointing to
  `{baseDir}/aws/{provider}/credentials` (using empty realm)

The env var pointed to a path where no credentials existed, causing the AWS SDK to fall back to
the runner's default IAM role, which had `ecr:GetAuthorizationToken` (login succeeds) but lacked
`ecr:PutImage` (push fails with 403).

**Fix:**
Changed all three `NewAWSFileManager("", "")` calls in `assume_role.go` to `NewAWSFileManager("", i.realm)`
to match the pattern used by all other identity types.

**Files changed:** `pkg/auth/identities/aws/assume_role.go`

---

## Summary of Changes

| Issue | Severity | Status | Fix                                                              |
|-------|----------|--------|------------------------------------------------------------------|
| #2071 | Critical | FIXED  | Better error message when ATMOS_PROFILE matches an identity name |
| CI/CD | Critical | FIXED  | Made realm isolation opt-in (empty realm by default)             |
| #2072 | High     | FIXED  | Discard conflicting stack auth defaults                          |
| ECR   | Critical | FIXED  | assume_role.go used empty realm instead of i.realm in 3 methods  |

## Test Coverage

All fixes include comprehensive tests in:

- `pkg/config/auth_realm_issues_test.go` - Issue reproduction and fix verification
- `pkg/auth/realm/realm_test.go` - Updated realm behavior tests
- `pkg/auth/cloud/aws/files_logout_test.go` - Updated cleanup tests
- `pkg/config/stack_auth_loader_test.go` - Updated conflicting defaults tests
- `pkg/auth/identities/aws/assume_role_test.go` - Issue #4 realm path consistency tests
