# Migrating from gcloud CLI Config

This reference is the agent's decision guide for users coming from the `gcloud` CLI. There is no
standalone prose tutorial for this migration yet -- for the full auth configuration schema, see
the [atmos-auth](../../atmos-auth/SKILL.md) skill and its
[providers-and-identities.md](../../atmos-auth/references/providers-and-identities.md) reference.

## Identifying the User's Shape

| User has...                                                          | Maps to                                          |
|------------------------------------------------------------------------|--------------------------------------------------|
| Run `gcloud auth application-default login`                            | [Application Default Credentials](#application-default-credentials--gcpadc) |
| CI/CD already configured for Workload Identity Federation              | [CI / Workload Identity Federation](#ci--workload-identity-federation--gcpworkload-identity-federation) |
| `gcloud config set auth/impersonate_service_account`, or scripts that pass `--impersonate-service-account` | [Service Account Impersonation](#service-account-impersonation--gcpservice-account) |
| `gcloud config set project` / `gcloud config configurations`           | [Project/Region Defaults](#projectregion-defaults--gcpproject) |
| A downloaded service-account JSON key file (`GOOGLE_APPLICATION_CREDENTIALS=key.json`) | [No Direct Equivalent: Static Key Files](#no-direct-equivalent-static-service-account-key-files) |

## Command Equivalence

| Old gcloud CLI workflow                                                | `atmos auth` equivalent |
|----------------------------------------------------------------------------|----------------------------|
| `gcloud auth login` / `gcloud auth application-default login`              | `atmos auth login -i x` |
| `gcloud config set project` / `gcloud config configurations activate x`    | No manual step -- the identity's `principal.project_id` selects this automatically |
| `gcloud auth print-access-token`                                           | `atmos auth whoami -i x` |
| Running `gcloud ...` / `terraform ...` under a specific config             | `atmos auth exec -i x -- gcloud ...` or `atmos auth shell -i x` |
| Manual `export GOOGLE_APPLICATION_CREDENTIALS=...` scripts                 | `eval $(atmos auth env -i x)` |

## Shells, `exec`, and Your Default gcloud Config

**By default, Atmos does not read or write your system's default gcloud config**
(`~/.config/gcloud/`). Same two recommended patterns as everywhere else in Atmos Auth:

- **`atmos auth shell -i <identity>`** -- subshell with that identity's credentials active.
- **`atmos auth exec -i <identity> -- <command>`** -- one-off command with credentials injected.

If the user wants `gcloud`/`terraform`/other tools to "just work" in their normal shell without a
subshell wrapper, `eval $(atmos auth env -i <identity>)` redirects the standard GCP SDK env vars
(`GOOGLE_APPLICATION_CREDENTIALS`, `CLOUDSDK_CONFIG`, `GOOGLE_CLOUD_PROJECT`) to Atmos-managed
files -- confirmed in `pkg/auth/cloud/gcp/env.go`. The user's actual
`~/.config/gcloud/application_default_credentials.json` and gcloud named configurations are never
touched; Atmos just points the current shell at its own managed credential files instead. Safe to
add to `.bashrc`/`.zshrc` -- doesn't trigger a login prompt by itself.

## Application Default Credentials → `gcp/adc`

**Before:**
```bash
gcloud auth application-default login
```
This writes `~/.config/gcloud/application_default_credentials.json`, which `gcp/adc` reads.

**After (`atmos.yaml`):**
```yaml
auth:
  providers:
    gcp-adc:
      kind: gcp/adc
      project_id: my-gcp-project   # optional: overrides gcloud config default
      region: us-central1          # optional
      scopes:                      # optional
        - https://www.googleapis.com/auth/cloud-platform
```

**Gotcha:** plain `gcloud auth login` (no `application-default`) only populates gcloud's own
credential store, **not** the ADC file Atmos reads. If `atmos auth login` fails with a
credentials-not-found error right after a user swears they "already logged in," this is almost
always why -- have them run `gcloud auth application-default login` specifically.

## CI / Workload Identity Federation → `gcp/workload-identity-federation`

**Before:** a GitHub Actions job using `google-github-actions/auth` with
`workload_identity_provider` and `service_account` inputs (no static JSON key).

**After (`atmos.yaml`):**
```yaml
auth:
  providers:
    gcp-wif:
      kind: gcp/workload-identity-federation
      project_number: "123456789012"          # required, numeric
      workload_identity_pool_id: github-pool
      workload_identity_provider_id: github-provider
      service_account_email: ci-sa@my-project.iam.gserviceaccount.com
```

In GitHub Actions, Atmos auto-detects the OIDC token source (`ACTIONS_ID_TOKEN_REQUEST_URL`/
`ACTIONS_ID_TOKEN_REQUEST_TOKEN`) -- don't hand-configure `token_source` unless running outside
GitHub Actions (e.g., a different CI system posting its own OIDC token to a URL or file).

## Service Account Impersonation → `gcp/service-account`

**Before:**
```bash
gcloud config set auth/impersonate_service_account terraform@my-project.iam.gserviceaccount.com
# or per-command: gcloud ... --impersonate-service-account=terraform@my-project.iam.gserviceaccount.com
```

**After (`atmos.yaml`):**
```yaml
auth:
  identities:
    terraform:
      kind: gcp/service-account
      default: true
      via:
        provider: gcp-adc   # or gcp-wif for CI
      principal:
        service_account_email: terraform@my-project.iam.gserviceaccount.com
        lifetime: 3600s     # optional, default 1h, max 12h -- note the trailing "s"
```

This requires the base identity (the user or the WIF principal) to already hold
`roles/iam.serviceAccountTokenCreator` on the target service account -- the same IAM binding
`gcloud --impersonate-service-account` needed.

## Project/Region Defaults → `gcp/project`

**Before:**
```bash
gcloud config set project production-project
gcloud config set compute/region us-central1
```

**After (`atmos.yaml`):**
```yaml
auth:
  identities:
    prod-project:
      kind: gcp/project
      via:
        provider: gcp-adc
      principal:
        project_id: production-project
        region: us-central1
        zone: us-central1-a   # optional
```

Multiple `gcloud config configurations` (`gcloud config configurations create <name>`) become
multiple `gcp/project` or `gcp/service-account` identities under one shared provider -- mark
whichever the user activates most often as `default: true`.

## No Direct Equivalent: Static Service-Account Key Files

There is no provider or identity kind that loads a downloaded service-account JSON key
(`GOOGLE_APPLICATION_CREDENTIALS` pointing at a file with a `private_key` field). This is
deliberate, not a gap to apologize for: frame it to the user as a security improvement. Recommend
`gcp/adc` (interactive) or `gcp/workload-identity-federation` (CI) as the base identity, plus
`gcp/service-account` impersonation for the target service account, and retiring the downloaded
key from GCP IAM once the migration is verified -- impersonation avoids ever holding exportable
long-lived key material.

## Common Gotchas

- **`gcloud auth login` vs `gcloud auth application-default login`** -- only the latter feeds
  `gcp/adc`. This is the most common first-run failure.
- **`gcp/service-account` needs `roles/iam.serviceAccountTokenCreator`** on the target SA for the
  base identity -- a missing grant here shows up as a 403, not a config error.
- **`gcp/oidc` is a reserved constant, not an implemented provider kind.** Don't suggest it even
  though it appears in the codebase's constants file -- it isn't wired up to any factory yet.
- **`lifetime` takes a duration-with-suffix string** (`"3600s"`), not a bare integer.

## Related Skills

[atmos-auth](../../atmos-auth/SKILL.md) for the full provider/identity schema and command
reference.
