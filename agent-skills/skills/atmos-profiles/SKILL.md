---
name: atmos-profiles
description: "Atmos profiles: profile directories, --profile and ATMOS_PROFILE activation, profile merge behavior, environment switching, and routing profile-specific auth/toolchain/config overrides"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Profiles

Use this skill when configuring or troubleshooting Atmos profiles: named configuration overlays
selected with `--profile` or `ATMOS_PROFILE`.

## Purpose

Profiles let teams switch configuration contexts without changing the base project config. Use them
for developer, CI, tenant, cloud account, or environment-specific overrides.

```bash
atmos --profile developer terraform plan vpc -s dev
ATMOS_PROFILE=ci atmos terraform deploy vpc -s prod
```

## Profile Layout

Keep profile files focused and aligned with the sections they override:

```text
profiles/
  developer/
    atmos.yaml
    auth.yaml
  ci/
    atmos.yaml
    auth.yaml
```

Profile configuration is merged into the active Atmos configuration. Put shared defaults in the
base config, then keep each profile to the smallest override needed.

## Profile Config

Configure the profile search location in root config when the default location is not sufficient:

```yaml
profiles:
  base_path: profiles
```

Example `profiles/ci/auth.yaml` override:

```yaml
auth:
  providers:
    github-oidc:
      kind: github/oidc
      region: us-east-1
  identities:
    deploy:
      kind: aws/assume-role
      via:
        provider: github-oidc
```

## Routing

| Need | Load |
|---|---|
| Profile activation, directory layout, merge behavior | stay in `atmos-profiles` |
| Auth providers, identities, OIDC, SSO, keyring in profiles | `atmos-auth` |
| CI use of `ATMOS_PROFILE` | `atmos-ci` |
| Tool versions or registries that differ by profile | `atmos-toolchain` |
| Base paths and profile-relative file layout | `atmos-project-layout` |

## Guardrails

- Keep profile overrides small and predictable; avoid duplicating the whole base `atmos.yaml`.
- Use the same provider and identity names across profiles when callers should not care which
  profile is active.
- Prefer `ATMOS_PROFILE` in CI and `--profile` for one-off local commands.
- Verify the active merged configuration with `atmos describe config` or the relevant
  `atmos describe component` command.
