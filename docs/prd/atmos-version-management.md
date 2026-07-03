# PRD: Atmos Version Management

## Status: Draft

**Last Updated**: 2026-07-03

**Implementation Package**: `pkg/version/manager`

**Related PRDs**:
- [Toolchain Implementation](./toolchain-implementation.md)
- [Tool Dependencies Integration](./tool-dependencies-integration.md)
- [Toolchain Lock File Support](./toolchain-lock-file.md)
- [Version Constraint](./version-constraint.md)

---

## Summary

Build Atmos-native version management for external versions used by infrastructure, workflows, CI, and toolchain dependencies.

Atmos owns discovery, policy, grouping, locking, rendering, status, and CI workflows. The design borrows useful capabilities from tools such as Dependabot and Renovate, but does not integrate with them, invoke them, or generate their config.

Human-authored configuration lives in `atmos.yaml`. Resolved versions live in `versions.lock.yaml`.

---

## Problem

Atmos projects already need to coordinate versions across several surfaces:

- Toolchain dependencies such as OpenTofu, Terraform, Helmfile, `kubectl`, `jq`, and `uv`
- GitHub Actions refs such as `actions/checkout@v6`
- Container images and OCI artifacts from Docker Hub, GHCR, ECR, and generic OCI registries
- Helm chart versions
- GitHub and GitLab release or tag versions
- Runtime stack values such as image tags and helper package versions
- CI-generated files that must contain literal version refs

Today those versions are usually scattered across stack YAML, workflow YAML, toolchain config, vendoring config, and CI scripts. This creates several problems:

1. **Version drift**: different environments silently run different versions.
2. **No single policy surface**: cooldowns, allowed versions, labels, schedules, and grouping are not expressed consistently.
3. **Weak CI ergonomics**: CI needs deterministic versions, status checks, and rendered literal files.
4. **Poor auditability**: desired policy and resolved versions are not clearly separated.
5. **Duplicated automation**: teams rely on external dependency update tools for behavior Atmos should own natively.
6. **Workflow rendering mismatch**: GitHub parses workflow files before Atmos can evaluate YAML or template functions, so generated or checked literal workflows are required.

---

## Product Positioning

Atmos version management is an external dependency catalog and lock system.

It is complementary to the existing Atmos design patterns for component versioning:

- Folder-based component versioning
- Release tracks/channels for component folders
- Strict component version pinning
- Source-based component versioning
- Vendored component versions

Those patterns answer, "Which component source should this environment run?"

Atmos version management answers, "Which external artifact versions should Atmos resolve, lock, inject, render, and verify for this track?"

This keeps version definitions out of individual stacks where possible, while still allowing stacks to assert the track they require.

---

## Goals

1. Manage arbitrary external versions as first-class Atmos configuration.
2. Provide Dependabot/Renovate-style capabilities through an Atmos-native schema and command surface.
3. Support multiple ecosystems and providers:
   - GitHub
   - GitLab
   - Docker Hub
   - ECR
   - GHCR
   - generic OCI registries
   - Helm repositories
   - Atmos toolchain registries
4. Support deterministic runtime resolution through lock files.
5. Support tracks such as `dev`, `staging`, and `prod`.
6. Support grouped updates, defaults, cooldowns, schedules, labels, ignore rules, allowed version policies, and automerge intent.
7. Support runtime usage from stack/config YAML:
   - `!version name`
   - `{{ .version.name }}`
   - `dependencies.tools`
8. Support generated or checked GitHub Actions workflow refs with literal `uses:` output.
9. Provide CI-native commands for status, update, render checks, and verification.
10. Keep all human-authored version policy in `atmos.yaml`; do not introduce a separate authored config file.

---

## Non-Goals

1. Generate `renovate.json`.
2. Generate `.github/dependabot.yml`.
3. Invoke Renovate.
4. Invoke Dependabot.
5. Match every external tool option one-for-one.
6. Make version definitions a stack-owned concept.
7. Require GitHub Actions to support Atmos YAML functions directly inside workflow files.
8. Replace Atmos component versioning design patterns.
9. Replace toolchain installation and verification.

---

## Core Concepts

### Ecosystem

The dependency domain.

Examples:

- `docker`
- `oci`
- `github-actions`
- `helm`
- `terraform`
- `opentofu`
- `toolchain`
- `github`
- `gitlab`

### Datasource

The version lookup strategy.

Examples:

- `github-tags`
- `github-releases`
- `gitlab-tags`
- `gitlab-releases`
- `docker-tags`
- `oci-tags`
- `helm`
- `toolchain`

### Provider

The concrete backend or auth target.

Examples:

- `github`
- `gitlab`
- `dockerhub`
- `ecr_prod`
- `ghcr`
- `generic-oci`

### Track

A named version lane such as `dev`, `staging`, or `prod`.

Tracks are configured in `atmos.yaml`. Stacks may assert the track they expect:

```yaml
version:
  track: prod
```

### Group

A batch of related updates with match rules and shared policy.

Groups are similar in spirit to Dependabot or Renovate groups, but they are Atmos-native and are not compatible with either external config format.

### Manager

An Atmos-native scanner or renderer for files that need literal version output.

The first required manager is a renderer for GitHub Actions workflow templates, because GitHub workflow YAML must contain literal `uses:` refs when GitHub reads it.

---

## Configuration

Version management configuration lives under the existing top-level `version` key in `atmos.yaml`.

This intentionally reuses `atmos.yaml` so teams get the same merge, profile, discovery, validation, and documentation behavior as the rest of Atmos configuration.

```yaml
version:
  track: prod
  lock_file: versions.lock.yaml

  providers:
    dockerhub:
      type: docker
      url: registry-1.docker.io

    ecr_prod:
      type: ecr
      region: us-east-1
      registry_id: "123456789012"

    ghcr:
      type: oci
      url: ghcr.io

  defaults:
    update:
      strategy: patch
      cooldown: 14d
      automerge: true
    allow: [stable]
    labels: [dependencies]

  groups:
    infrastructure:
      ecosystems: [docker, oci, helm, terraform, opentofu, github-actions]
      patterns: ["terraform*", "opentofu", "actions/*", "nginx"]
      update:
        strategy: minor
        cooldown: 14d
      labels: [infrastructure]

  tracks:
    prod:
      defaults:
        update:
          cooldown: 30d

      versions:
        opentofu:
          ecosystem: toolchain
          datasource: toolchain
          package: opentofu
          desired: "~1.10"

        dd_forwarder:
          ecosystem: github
          datasource: github-tags
          provider: github
          package: DataDog/datadog-serverless-functions
          desired: "~5.4"

        nginx:
          ecosystem: oci
          datasource: oci-tags
          provider: dockerhub
          package: library/nginx
          desired: "~1.28"

        private_api:
          ecosystem: oci
          datasource: oci-tags
          provider: ecr_prod
          package: platform/private-api
          desired: "~2.7"

        checkout:
          ecosystem: github-actions
          datasource: github-tags
          provider: github
          package: actions/checkout
          desired: "v6"
```

---

## Schema Requirements

### `version.track`

Default version track for the project.

If a stack asserts `version.track`, the stack assertion wins for runtime resolution.

### `version.lock_file`

Path to the lock file.

Default:

```yaml
version:
  lock_file: versions.lock.yaml
```

Relative paths resolve from the Atmos base path.

### `version.providers`

Defines named providers and their auth/address metadata.

Provider names are arbitrary and are referenced from version entries.

### `version.defaults`

Default policy inherited by all tracks and version entries.

Supported fields:

- `update.strategy`
- `update.cooldown`
- `update.schedule`
- `update.automerge`
- `allow`
- `ignore`
- `labels`

### `version.groups`

Defines update groups with match rules.

Supported match fields:

- `ecosystems`
- `datasources`
- `providers`
- `patterns`
- `exclude_patterns`

Supported policy fields:

- `update`
- `allow`
- `ignore`
- `labels`

### `version.tracks`

Defines named version tracks.

Each track supports:

- `extends`
- `defaults`
- `versions`

### `version.tracks.<track>.versions`

Defines version entries.

Supported fields:

- `ecosystem`
- `datasource`
- `provider`
- `package`
- `desired`
- `group`
- `update`
- `allow`
- `ignore`
- `labels`

---

## Policy Merge Semantics

Effective policy is resolved in this order:

1. Global defaults: `version.defaults`
2. Track defaults: `version.tracks.<track>.defaults`
3. Version entry: `version.tracks.<track>.versions.<name>`
4. Matched group policy: `version.groups.<group>`

Scalar fields use last-writer-wins.

List fields merge with de-duplication while preserving order.

Group matching should be deterministic. If multiple groups match, the lexically first group name wins unless the entry explicitly sets `group`.

---

## Runtime Usage

### YAML Function

Use `!version` when the entire YAML value is the version:

```yaml
version:
  track: prod

components:
  terraform:
    datadog-forwarder:
      dependencies:
        tools:
          opentofu: !version opentofu
      vars:
        image:
          tag: !version dd_forwarder
```

### Template Context

Use `.version` when the version is embedded inside a string:

```yaml
components:
  terraform:
    datadog-forwarder:
      vars:
        image:
          uri: "public.ecr.aws/datadog/lambda-extension:{{ .version.dd_forwarder }}"
```

### Toolchain Dependencies

Atmos toolchain dependencies can use managed versions:

```yaml
dependencies:
  tools:
    opentofu: !version opentofu
```

This should resolve before existing dependency/toolchain installation logic runs.

### GitHub Actions

GitHub Actions workflow YAML cannot depend on Atmos parsing at runtime because GitHub parses workflow files before Atmos executes.

Author workflow templates with `.version`:

```yaml
uses: "actions/checkout@{{ .version.checkout }}"
```

Render literal workflow YAML:

```yaml
uses: actions/checkout@v6
```

The intended workflow is:

1. Author workflow templates.
2. Run `atmos version tracks render <track>`.
3. Commit rendered workflow YAML.
4. Run `atmos version tracks render <track> --check` in CI.

---

## Lock File

Resolved versions live in `versions.lock.yaml`.

```yaml
version: 1
tracks:
  prod:
    opentofu:
      version: 1.10.6
      ecosystem: toolchain
      datasource: toolchain
      package: opentofu
      resolved_at: "2026-07-03T00:00:00Z"

    private_api:
      version: 2.7.4
      ecosystem: oci
      datasource: oci-tags
      provider: ecr_prod
      package: platform/private-api
      digest: sha256:...
      resolved_at: "2026-07-03T00:00:00Z"
```

OCI and Docker entries should lock both tag and digest when available.

Runtime resolution reads from the lock file only. This keeps local runs and CI deterministic.

---

## Commands

All commands live under the existing `atmos version` namespace:

```shell
atmos version tracks list
atmos version tracks show prod
atmos version tracks lock prod
atmos version tracks update prod
atmos version tracks update prod --group infrastructure
atmos version tracks status prod --format json
atmos version tracks diff prod
atmos version tracks verify prod
atmos version tracks render prod --check
```

### `atmos version tracks list`

List configured tracks.

### `atmos version tracks show <track>`

Show the effective track configuration after defaults and groups are applied.

### `atmos version tracks lock <track>`

Resolve configured desired versions and write lock entries.

### `atmos version tracks update <track>`

Update locked versions according to policy.

Initial behavior may be equivalent to `lock`. Future behavior should respect schedules, cooldowns, update strategy, ignore rules, and allowed versions.

### `atmos version tracks update <track> --group <group>`

Update only entries in the selected group.

### `atmos version tracks status <track> --format json`

Report current lock status for CI.

Statuses:

- `current`
- `locked`
- `unlocked`
- `update-available`
- `error`

### `atmos version tracks diff <track>`

Show entries where the lock differs from the currently resolved target or is missing.

### `atmos version tracks verify <track>`

Fail if the lock is missing, stale, or invalid.

### `atmos version tracks render <track> --check`

Render managed files and fail if the generated outputs are not current.

---

## Datasource Requirements

### Toolchain

Datasource: `toolchain`

Required behavior:

- Resolve tool aliases through Atmos toolchain resolution.
- Resolve exact versions.
- Resolve `latest`.
- Resolve SemVer constraints.
- Reuse toolchain registry code where possible.

### GitHub

Datasources:

- `github-tags`
- `github-releases`

Required behavior:

- Resolve tags and releases.
- Support SemVer constraints with optional `v` prefixes.
- Support prerelease filtering through `allow`.
- Use GitHub auth when configured.
- Support rate-limit-aware error messages.

### GitLab

Datasources:

- `gitlab-tags`
- `gitlab-releases`

Required behavior:

- Resolve tags and releases.
- Support self-hosted GitLab providers.
- Use configured auth when available.

### Docker Hub

Datasource: `docker-tags`

Required behavior:

- Resolve image tags.
- Support SemVer-like tag constraints.
- Lock digest when available.

### OCI, GHCR, ECR, Generic OCI

Datasource: `oci-tags`

Required behavior:

- Resolve image or artifact tags.
- Support SemVer-like tag constraints.
- Lock digest when available.
- Use provider-specific auth:
  - ECR auth
  - GHCR auth
  - generic registry credentials

### Helm

Datasource: `helm`

Required behavior:

- Resolve chart versions from Helm repositories.
- Support SemVer constraints.
- Use configured repository auth.

---

## Update Policy Requirements

### Strategy

Supported values:

- `major`
- `minor`
- `patch`
- `digest`
- `pin`

### Cooldown

Delay update eligibility after a version is released.

Example:

```yaml
update:
  cooldown: 14d
```

### Schedule

Restrict update eligibility to configured windows.

Example:

```yaml
update:
  schedule:
    - "before 6am on monday"
```

The initial implementation may store schedule strings without interpreting every natural language form. The schema should leave room for richer schedule parsing later.

### Allow

Policy for allowed versions.

Initial values:

- `stable`
- `prerelease`
- `deprecated`

Future values may include explicit patterns or predicates.

### Ignore

Ignore rules for versions or patterns.

### Labels

Labels used by CI automation and PR creation.

### Automerge

Boolean intent. Atmos records whether an update may be automatically merged, but actual merge behavior belongs to the CI workflow or future PR automation.

---

## CI Requirements

Atmos version management must be CI-native.

Required CI flows:

1. Check whether versions are current:

   ```shell
   atmos version tracks status prod --format json
   ```

2. Verify lock determinism:

   ```shell
   atmos version tracks verify prod
   ```

3. Check rendered workflow files:

   ```shell
   atmos version tracks render prod --check
   ```

4. Update a specific group:

   ```shell
   atmos version tracks update prod --group infrastructure
   ```

Future CI automation may open PRs, attach labels, group updates, and apply automerge intent, but the first responsibility is deterministic status/update/render behavior.

---

## Architecture

### Package Layout

Core implementation belongs in:

```text
pkg/version/manager
```

Responsibilities:

- Parse effective version track configuration.
- Merge defaults, groups, and entries.
- Load and save `versions.lock.yaml`.
- Resolve locked runtime values.
- Resolve desired target versions through datasource interfaces.
- Return status/diff/verify payloads.
- Provide renderer inputs for command and CI managers.

Command wiring belongs in:

```text
cmd/version
```

Runtime YAML and template integration should call `pkg/version/manager`, not command code.

### Interfaces

The manager package should expose resolver interfaces so provider support can be added incrementally:

```go
type Resolver interface {
    Supports(entry EffectiveEntry) bool
    Resolve(ctx context.Context, entry EffectiveEntry) (ResolvedVersion, error)
}
```

Provider-specific resolvers should live behind the manager package or in subpackages if they grow large.

### Runtime Resolution

Runtime resolution must read from lock files, not perform network lookup.

Network lookup belongs to `lock`, `update`, `status`, and future update automation flows.

### Error Handling

Errors should distinguish:

- Missing track
- Missing version entry
- Missing lock entry
- Unsupported datasource
- Auth failure
- Registry/network failure
- Constraint parse failure
- No matching version
- Digest lookup failure
- Render mismatch

---

## Implementation Plan

### Phase 1: Schema, Locking, Runtime Resolution

1. Extend `schema.Version` with managed version fields.
2. Add `pkg/version/manager`.
3. Add lock file load/save support.
4. Add effective policy merge support.
5. Add `atmos version tracks` command group.
6. Add `!version` YAML tag.
7. Add `.version` template context for stack rendering.
8. Support exact desired versions for all datasources.
9. Support SemVer constraint resolution for `datasource: toolchain`.
10. Add docs and focused tests.

### Phase 2: Provider Resolvers

1. Add GitHub tags/releases resolver.
2. Add GitLab tags/releases resolver.
3. Add Docker Hub tags resolver.
4. Add generic OCI tags and digest resolver.
5. Add GHCR provider support.
6. Add ECR provider support.
7. Add Helm chart resolver.
8. Add provider auth integration.

### Phase 3: Policy Enforcement

1. Implement update strategy filtering.
2. Implement cooldown checks.
3. Implement schedule checks.
4. Implement allow and ignore rules.
5. Implement grouped status/update behavior.
6. Add structured reasons for skipped updates.

### Phase 4: Managers and CI

1. Add GitHub Actions workflow renderer/checker.
2. Add file manager registry for future scanners/renderers.
3. Add generated file metadata or provenance where useful.
4. Add JSON output designed for CI summaries.
5. Add docs for native CI workflows.

### Phase 5: Automation

1. Add optional PR creation flow.
2. Apply group labels.
3. Apply automerge intent.
4. Add changelog/release note summaries where datasource supports them.
5. Add security-focused update mode.

---

## Current Implementation Slice

The initial implementation should be congruent with this PRD but does not need full provider parity immediately.

Expected first slice:

- Schema fields under `version`.
- `pkg/version/manager` package.
- `versions.lock.yaml` load/save.
- `atmos version tracks` command group.
- Exact version locking for all datasources.
- Toolchain SemVer constraint resolution by reusing toolchain registry behavior.
- `!version` runtime resolution from lock file.
- `.version` template context.
- Render/check support for template files.
- Documentation for configuration, commands, YAML function, and design pattern positioning.

Explicit first-slice limitation:

- Non-toolchain datasource constraints should return a clear unsupported-resolver error until provider-specific resolvers are implemented.

This is preferable to silently treating constraints as concrete versions.

---

## Test Plan

### Schema Tests

- Providers deserialize correctly.
- Defaults deserialize correctly.
- Groups deserialize correctly.
- Tracks deserialize correctly.
- Version entries deserialize correctly.
- Existing `version.use`, `version.check`, and `version.constraint` remain compatible.

### Policy Tests

- Global defaults apply.
- Track defaults override global defaults.
- Entry policy overrides inherited defaults.
- Group policy applies after entry policy.
- Labels and allow/ignore lists merge with de-duplication.
- Explicit `group` wins over match rules.
- Group match order is deterministic.

### Lock Tests

- Missing lock file returns empty lock.
- Lock file writes with `version: 1`.
- Exact versions persist.
- OCI/Docker digests persist when present.
- `VersionMap` returns locked versions.
- Missing lock entries produce clear errors.

### Resolver Tests

- Toolchain exact version.
- Toolchain `latest`.
- Toolchain SemVer constraint.
- Unsupported non-toolchain constraint.
- No matching version.
- Invalid constraint.

Future resolver tests:

- GitHub tags/releases.
- GitLab tags/releases.
- Docker Hub tags.
- GHCR/OCI tags and digest.
- ECR tags and digest.
- Helm chart versions.

### Runtime Integration Tests

- `!version name` resolves from default track.
- `!version name` resolves from stack-asserted track.
- `{{ .version.name }}` resolves in stack templates.
- `dependencies.tools` receives a concrete version.
- Missing lock fails with a useful error.
- Stack `version.track` assertion works.

### Command Tests

- `tracks list`
- `tracks show`
- `tracks lock`
- `tracks update`
- `tracks status --format json`
- `tracks diff`
- `tracks verify`
- `tracks render`
- `tracks render --check`
- `--group` filtering

### CI Tests

- Status command emits stable JSON.
- Verify fails on stale/missing locks.
- Render check fails on stale generated workflows.
- Update produces deterministic lock output.

---

## Open Questions

1. Should `atmos version tracks update` eventually create PRs itself, or should it only update local files and let CI handle PR creation?
2. Should schedules use a strict machine-readable schema instead of natural language strings?
3. Should `allow` and `ignore` use a common expression language for all datasources?
4. Should GitHub Actions rendering manage only explicit files, or should it include a manager that scans `.github/workflows/*.yaml.tmpl` automatically?
5. Should lock entries include changelog URLs and release timestamps?
6. How should provider auth reuse existing Atmos auth patterns for GitHub, GitLab, ECR, GHCR, and generic registries?
7. Should update groups be eligible to define commit message or PR title templates?

---

## Success Criteria

1. Teams can declare external version policy once in `atmos.yaml`.
2. Teams can commit deterministic resolved versions in `versions.lock.yaml`.
3. Stacks can assert a track without owning version definitions.
4. `!version` and `{{ .version.name }}` work in normal Atmos stack/config processing.
5. Toolchain dependencies can consume managed versions.
6. GitHub Actions workflows can be generated or checked with literal refs.
7. CI can detect stale locks and rendered files.
8. The schema leaves room for provider parity with Dependabot/Renovate-style capabilities without adopting their config formats.
9. Runtime commands do not perform network version lookup.
10. The feature fits the existing Atmos version-management positioning without replacing component versioning patterns.
