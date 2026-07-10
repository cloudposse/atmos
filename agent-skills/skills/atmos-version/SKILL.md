---
name: atmos-version
description: "Atmos Version Tracker: version tracks, lock files, managed external dependency versions, atmos version track commands, !version, file managers, update policy, pinning, and CI verification"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Version Tracker

Use this skill for Atmos-managed software versions under the top-level `version:` section of
`atmos.yaml`.

The Version Tracker manages external dependency versions that Atmos should resolve, lock, apply to
files, and verify. It is separate from the top-level `atmos version` command, which reports the
Atmos CLI version.

## Related Skills

| Need | Load |
|---|---|
| Tool installation from tracked versions | [atmos-toolchain](../atmos-toolchain/SKILL.md) |
| Vendored component source versions | [atmos-vendoring](../atmos-vendoring/SKILL.md) |
| Component source provisioning | [atmos-components](../atmos-components/SKILL.md) |
| YAML `!version` function | [atmos-yaml-functions](../atmos-yaml-functions/SKILL.md) |
| CI gates for lock/file drift | [atmos-ci](../atmos-ci/SKILL.md) |

## Core Model

Version policy lives in `atmos.yaml`; resolved versions live in a lock file, usually
`versions.lock.yaml`.

```yaml
version:
  track: prod
  lock_file: versions.lock.yaml

  dependencies:
    checkout:
      ecosystem: github/actions
      datasource: github-tags
      package: actions/checkout
      desired: v6
      update:
        pin: sha

    opentofu:
      ecosystem: toolchain
      datasource: toolchain
      package: opentofu
      desired: "~1.10"

  tracks:
    prod:
      defaults:
        update:
          strategy: patch
          cooldown: 14d
```

Important concepts:

- `track`: named lane such as `dev`, `staging`, or `prod`.
- `dependencies`: base catalog of external versions.
- `tracks.<name>.dependencies`: per-track overrides.
- `defaults`, entry `update`, and groups: inherited update policy.
- `lock_file`: resolved, deterministic versions read by runtime, file managers, and CI.
- `pin: sha` / `pin: digest`: lock immutable Git SHAs or OCI digests.

## Command Workflow

Use `atmos version track` (alias `tracks`) for the managed-version command group:

```shell
atmos version track list
atmos version track show prod
atmos version track add checkout --package=actions/checkout --pin=sha
atmos version track set checkout --desired=v6
atmos version track lock prod
atmos version track update prod --group=infrastructure
atmos version track status prod --format=json
atmos version track diff prod
atmos version track apply prod --check
atmos version track verify prod
```

Track selection resolves in this order: positional track argument, `--track`, `version.track`, then
`default`.

Use `lock` to resolve current desired versions as-is. Use `update` to advance from the locked state
within policy: strategy caps, cooldown windows, include/exclude filters, prerelease settings, and
groups.

## Managed Files

Use `version.files` when literal files must be rewritten from the lock:

```yaml
version:
  files:
    - manager: github-actions
      paths:
        - .github/workflows/*.yaml
    - manager: marker
      paths:
        - Dockerfile
    - manager: template
      paths:
        - "**/*.tmpl"
```

File managers:

- `github-actions`: rewrites workflow `uses:` refs from locked GitHub Action versions.
- `marker`: rewrites annotated arbitrary text lines such as `# atmos:version nginx`.
- `template`: renders `*.tmpl` files with `.version` context.

Use `atmos version track apply <track> --check` or `atmos version track verify <track>` in CI to
fail when lock files or managed files drift.

## Runtime Usage

Use `!version name` when a YAML value should come from the active locked track. Use
`{{ .version.name }}` in templates when rendering managed files.

Do not use Version Tracker as a replacement for component versioning patterns. Folder-based
component versions, component `source:`, and vendoring answer "which component source should this
stack run?" Version Tracker answers "which external artifact versions should Atmos resolve, lock,
apply, and verify?"

## Guardrails

- Keep human-authored policy in `atmos.yaml`; do not create Renovate or Dependabot config unless
  the user explicitly asks for those tools.
- Commit the lock file when tracked versions affect CI, runtime, or generated files.
- Prefer `pin: sha` for GitHub Actions so `uses:` refs are immutable.
- Use `update` for policy-aware advancement; use `lock` for bootstrap or repair.
- Validate with `status`, `diff`, `apply --check`, and `verify` before relying on a track in CI.
