# PRD: `atmos doctor` and `atmos doctor --fix`

## Status

Initial implementation delivered for review. This document defines the acceptance
contract; an advisory finding does not imply that its automatic migration is implemented.

The enabled repair domain is deliberately narrow: recognized custom-command semantic
fields, hook events, explicit Terraform reconfigure booleans, and archived
component-monorepo sources whose successor repository is verified identical at the same
version (`component-monorepo`). A repair is blocked by
the inputs that can change what it means (relevance-scoped), not by any dynamic value
anywhere in the project. Simple component-level Terraform dependency lists also migrate
to modern required semantics, as an explicit exception to behavior preservation.
Stack naming and unsupported dependency shapes remain manual. Remote schemas and
external schema references remain incomplete until their input closure can be captured.
GitHub and configured GitHub Enterprise metadata are supported; other source hosts
report incomplete coverage.

## Purpose and scope

Provide a polished project health command combining existing validation,
deprecation detection, and modernization guidance. `atmos doctor` diagnoses;
`atmos doctor --fix` applies verified migrations with the dependency-semantics exception below. Every run
explains what was checked, fixed, skipped, or could not be verified.

The initial release covers built-in project checks. Environment diagnostics,
project-defined executable checks/fixes, and AI-authored repairs are excluded.
Reuse schema validators, structured diagnostics, YAML CRUD, provenance, and the
streaming UI. Preserve existing `atmos validate` behavior.

Use [React Doctor's severity threshold](https://www.react.doctor/docs/reference/cli-reference)
and [Next.js codemods](https://nextjs.org/docs/app/guides/upgrading/codemods) as
precedents. Warnings are advisory by default.

## Verification: preserving configuration behavior

Schema validity is necessary but insufficient. A valid configuration can select
a different Terraform workspace, change dependency requirements, or override an
inherited value. Every fixer must pass two independent gates.

**Dependency migration policy:** simple component-level Terraform `settings.depends_on`
lists containing literal `component` and optional `stack` fields move to
`dependencies.components`. This intentionally adopts modern required semantics rather
than preserving legacy tolerance or implicit context matching. Report resulting graph
errors (including missing/unavailable targets and cycles) without rejecting the migration.
Dry runs report projected errors and write nothing. Comments, target names, explicit stack
names, and unrelated YAML must remain unchanged. Reject new schema or unrelated stack
errors. Mixed old/new declarations, context selectors, file/folder entries, and incomplete
static source closures remain manual. The equivalence requirements below apply to other
repairs and future dependency variants that claim to preserve behavior.

### Gate A: migration contract tests

Define the supported input domain, then test actual production consumers before
and after transformation, including failure behavior. Do not merely assert the
transformer's expected YAML. Dependency migrations use paired fixtures and the
production graph, dependents, and affected-selection logic.

<dl>
<dt>Existing same-stack target</dt>
<dd>Identities, edges, required/optional status.</dd>
<dt>Explicit cross-stack target</dt>
<dd>Target stack and component.</dd>
<dt>Missing target</dt>
<dd>Failure/success classification and missing-target handling.</dd>
<dt>Disabled or abstract target</dt>
<dd>Availability and failure behavior.</dd>
<dt>Disabled or abstract source</dt>
<dd>Participation in dependency resolution.</dd>
<dt>Legacy context selectors</dt>
<dd>Namespace/tenant/environment/stage matching.</dd>
<dt>Imports, inheritance, profiles</dt>
<dd>Effective declarations and precedence.</dd>
<dt>Mixed legacy/modern fields</dt>
<dd>Precedence, including empty declarations.</dd>
<dt>Duplicate edges, self-dependencies, cycles</dt>
<dd>Edge semantics and rejection behavior.</dd>
<dt>File/folder dependencies</dt>
<dd>Affected selection for representative changed paths.</dd>
<dt>Dependency/dependent filters</dt>
<dd>Forward/reverse closure and ordering constraints.</dd>
</dl>

Compare ordering constraints, not incidental ordering of independent nodes.
The existing legacy parser accepts missing targets with a warning, whereas modern
required dependencies return an error. `required: false` is not automatically
equivalent either: it changes optional-edge metadata. Only enable variants that
satisfy the contract, even if none initially qualify. Do not add compatibility
semantics or weaken the guarantee to increase fix coverage.

Include generated/property-based cases and negative controls that deliberately
alter a target, stack, or requirement and demonstrate rejection.

### Gate B: verification against the user's project

1. Capture original bytes and hashes for the relevant source closure.
2. Stage edits through a source overlay retaining original logical paths.
3. Evaluate original/candidate configurations with identical inputs and fresh caches.
4. Compare unaffected YAML nodes and rule-specific effective behavior.
5. Validate the candidate and reject new errors.
6. Confirm all source files still match their baseline before writing.

Naming migrations compare every affected stack identity and Terraform workspace.
Dependency migrations compare identities, edges, optional status, reverse
dependents, affected selection, and error classifications. Only explicitly
defined representation changes and nonsemantic provenance may differ. Compare
unmasked values internally; mask displayed evidence.

Do not evaluate remote state, secrets, executable YAML functions, unresolved
templates, or incomplete profile/import coverage to justify a fix. Report
`verification-incomplete` and skip. Blocking is relevance-scoped:

- A configuration repair is blocked by layers that can change the edited value (imports,
  `atmos.d` files, active profiles including `profiles.default`, and ambient home or system
  configuration), and by a dynamic value at or under the edited key or a key the repair reads.
- A template or YAML function elsewhere in the file, such as custom command `steps` or
  `stacks.name_template`, does not block it. Stack, workflow, and vendor files are not layered
  configuration, so an unverifiable one limits only the checks that read it.
- Every blocked finding names the blocking `file:line` (or layer). Remediation never suggests
  `--fix` for a finding that `--fix` cannot resolve.
- Imported template files (`*.yaml.tmpl`) are reported as not evaluated and never parsed as YAML.
- A check with no inputs reports `skipped`, never `passed`. Finite tests do not prove arbitrary future
configurations; guarantees are limited to tested contracts and verified inputs.

## CLI, findings, rules, and recovery

```text
atmos doctor
atmos doctor --fix
atmos doctor --fix --dry-run
atmos doctor --format=json
atmos doctor --fail-on=warning
atmos doctor --only=deprecations
atmos doctor --affected --base=origin/main
atmos doctor --exclude='tests/fixtures/**'
atmos doctor --list-checks
atmos doctor --only=sources
atmos doctor --offline
```

Formats: `text`, `rich`, `json`, `sarif`. Failure threshold: `error` (default),
`warning`, or `none`. Exit codes: 0 for threshold satisfied, 1 for blocking
findings, 2 for operational/invocation failures (including no project configuration
found), 130 for cancellation. Evaluate after fixes; dry runs evaluate unchanged files.
`--fix` also exits 1 when a requested repair ends `verification-failed`, whatever the
threshold; a dry run only applies the threshold. `--fix` authorizes eligible writes
without another prompt. No flag bypasses semantic verification.

Invalid options and combinations exit 2 before any input is read and name the option and
its valid values. In `json` and `sarif` they are reported on stdout through
`operational_error`, the same shape as any other run. `--list-checks` prints the catalog to
stdout and rejects every flag except `--format` and `--fail-on`. An operational failure in
one check is recorded against it; the other checks still run and the run exits 2.

Configuration is located exactly as the loader does, without parsing it: parent
directories, `ATMOS_CLI_CONFIG_PATH`, `--config`/`--config-path` (which disable discovery),
`atmos.d` relative to the configuration file, and only the active profiles.

Each finding has a stable rule ID, category, severity, location, explanation,
remediation, documentation link, and eligibility/reason. Reports distinguish
`fixed`, `manual`, `verification-failed`, `verification-incomplete`, and operational
failures, and include verification coverage and backup location.

### Initial rules

- Configuration/custom-schema, stack, EditorConfig, and GitHub Actions validation
  (actionlint honors `.github/actionlint.yaml`).
- Every schema-annotated deprecation.
- Custom-command argument `type -> provides` and flag `semantic_type -> provides`
  for recognized semantic values. Ordinary flag `type` is not deprecated.
- Recognized legacy hook-event spelling.
- Explicit Terraform `init_run_reconfigure`, using current runtime semantics.
- Naming transformations require both equivalence gates; dependency transformations follow the explicit modern-semantics policy above.
- Advisory legacy CI actions.
- `component-monorepo`: component sources in the archived `cloudposse/terraform-aws-components`
  monorepo move to their `cloudposse-terraform-components` repository at the same version,
  only when a GitHub tree comparison proves the files identical (see below).
- Repository health for vendor and source-provisioned component sources: archived
  repositories and unavailable or removed repositories, with explicit access uncertainty.

Repository checks cover vendor `source` entries, component `source`/`source.uri`, and
`component.yaml` `spec.source.uri`. They query metadata without cloning, fetching
component code, or changing source URLs. Start with GitHub and configured GitHub Enterprise endpoints,
reusing existing host-scoped credentials. Cache requests per repository for the run
and enforce timeouts/cancellation. Report unsupported hosts and unresolved source
templates as incomplete coverage. `--offline` disables network checks and reports notice-level findings, which never count
toward `--fail-on=warning`.

An archived repository is a warning with manual remediation. A confirmed removal
is a separate warning. GitHub's 404 can mean a private repository or insufficient
access: report it as unavailable, never as confirmed deletion. Authentication,
rate-limit, timeout, and connectivity failures remain inconclusive. Repository health never
infers a replacement repository or rewrites a source; only the `component-monorepo` rule moves a source, from its published map.

Terminal color, docs relocation, auth/version fields, Helmfile patterns, and
architectural changes remain advisory until dedicated fixers pass their contracts.
Add missing reconfigure schema metadata. Keep rules, schemas, and modernization
skills consistent. Never execute replacement prose as migration instructions.

### Component monorepo move

The monorepo is archived and every component has its own repository whose `src` directory
holds the component code. The successor repositories keep the historical tags, and the Git tree
of `modules/<component>` equals the tree of `src` at the same tag, so the repair changes the
repository and the path and never the version.

- The embedded map (`pkg/doctor/monorepo/components.yaml`) is a frozen copy of the map published
  by `github-action-atmos-component-updater`. Names are never derived, because several differ.
- Locations: `component.yaml` `spec.source.uri`, every vendor manifest `spec.sources[].source`
  (discovery follows `spec.imports` as Atmos does), and a stack component's `source` or
  `source.uri`. Mixins that name the monorepo are reported manually.
- The version is the literal `ref`, or the `version` field beside a `{{ .Version }}` placeholder.
  Ranges and placeholders that are not one tag are reported manually.
- Verification precedes any write: the tree SHAs are compared through the GitHub API with the
  existing credentials and timeouts, once per component and ref per run. Equal trees make the
  repair available. Differing trees or a missing tag are `verification-failed`. `--offline`, rate
  limits, and network failures are `verification-incomplete`.
- Semantic verification accepts exactly the derived source change; comments on the line are kept.
- Split components (`tgw`, `spacelift`), components removed before the migration, and sources with
  templated repositories or paths are reported with a specific manual remediation.

### Git and backups

Git is recovery, not evidence of equivalence. Diagnosis works outside Git.
Warn before applying edits when Git is unavailable, the project is outside a
repository, or affected files are dirty (staged or unstaged), untracked, or ignored.
Unrelated changes do not trigger warnings. Continue without prompting.

Back up exact pre-fix copies of every edited file under the Atmos XDG data
directory `doctor/backups/<run-id>`. A private recovery manifest records original
paths, permissions (an octal string such as `"0644"`), and before/after hashes. Display location and recovery
instructions. Complete all backups before the first write; backup failure prevents
writes. Never stash, reset, commit, or otherwise change Git state.

Stage and verify the complete candidate first. Replace files atomically and
sequentially. Reject destination conflicts and concurrent source changes. Stop
further writes on failure or cancellation, retain backups, and report exactly
which files changed. A multi-file run is not a filesystem transaction.

## Architecture and streaming UI

Use the command-provider registry, injectable interfaces, and separate discovery,
checking, planning, verification, application, and rendering. A diagnostic bootstrap
allows malformed config to reach the report without hooks, provisioning, auth,
executable YAML functions, or automatic tool installation. Explain unavailable checks.

Reuse the raw YAML editor and extend staging instead of introducing a second
writer. Reuse `pkg/ui/batch`, including diagnostic severity and doctor phase labels,
without changing vendor behavior.

```text
Discover -> Check -> Stage fixes -> Verify -> Back up -> Apply -> Recheck
```

- Immediate spinners during discovery and indeterminate work.
- Active rows, elapsed time, phase-local counts, bars only with known totals.
- Persistent completed rows/findings, correct warning/manual styling.
- Terminal width, themes, masking, and NO_COLOR support.
- Reliable cancellation, terminal restoration, lossless diagnostic/completion events.
- Human UI on stderr; machine reports on stdout. Plain streaming in CI/non-TTY;
  JSON/SARIF suppress live rendering.

```text
✓ Configuration schema                 18 files checked
✓ Custom-command migration             behavior verified
! Legacy dependency migration          manual change required
⠋ Verifying stack names                42/60 instances

Fixed 2 findings in 1 file.
Stack names and Terraform workspaces unchanged across 60 instances.
1 migration skipped: missing-target behavior would change.
Backup: <backup location>
```

## Acceptance and delivery

- Every enabled fixer passes production-consumer contract and project tests.
- Known legacy missing-target differences are refused with explanations.
- Negative controls detect material changes; second --fix is a no-op.
- Preserve unrelated values, comments, tags, anchors, types, and permissions. Comments on a
  migrated key move to its replacement. Blank lines and folded scalars may be reformatted by
  the YAML editor; source-preserving output is planned in `yaml-editing-get-set.md` Phase 3.
- Dry runs write no project files or backups.
- Cover no-Git, dirty/staged/ignored/untracked files, worktrees, failed backups,
  concurrent edits, cancellation, and partial application.
- Backups match original bytes and support exact recovery.
- PTY tests cover streaming/resizing/warnings/cancellation/summaries; non-TTY tests
  cover machine output and exit thresholds.
- Malformed configuration is diagnosed without executing project commands.
- Preserve validation and vendor UI behavior with regression coverage.

Deliver this PRD, engine/CLI, verified fixers, streaming UI, command/rule docs,
updated modernization skills, and a recorded demo. The pre-implementation baseline
passed `go test ./pkg/validator ./pkg/yaml ./pkg/config/schema`.

## Implementation verification

Production-consumer tests cover semantic command fields, hook event matching, and
Terraform reconfigure policy. Negative controls change semantic targets, hook events,
unrelated values, and types; custom schemas that reject a migration prevent writes.
The dependency contract test confirms the legacy missing-target success versus modern
required-target error. The simple-list fixer deliberately adopts modern required semantics
and reports those errors; it does not claim legacy behavior equivalence.

Recovery tests cover Git worktrees and unavailable Git, dirty/staged/untracked/ignored
files, exact backup bytes and permissions, concurrent source changes, partial writes,
cancellation, dry runs, and repeated fixes. PTY tests exercise streaming, resize, and
cancellation. Existing validator, YAML editor, generated schema, and vendor batch UI
tests remain in the verification suite.

The reproducible demo lives in `demo/casts/atmos.d/demo/fixtures/doctor/doctor.yaml`
and is published at `/casts/demo/fixtures/doctor/doctor.cast`.
