# October 2026 feature graduation decision

**Review date:** 2026-10-15. **Baseline:** `4d982b2bb8` (`origin/main`).
**Decision:** graduate CI, Auth, Toolchain, Emulator, Source Vendoring, and workdir management commands in the next release containing this change.
**Reviewing maintainer:** anonymized project maintainer, through the graduation review.
**Technical evidence review:** repository history, public reports, and local regression checks.
**Release:** unassigned; the latest published release at review time is `v1.232.0`.
Do not describe this change as shipped in that release. Associate the announcement with the actual release tag when assigned.

## Criteria and evidence boundaries

This decision applies the existing [graduation criteria](../../website/docs/experimental.mdx):
implemented documented functionality, interfaces validated through use, incorporated feedback,
and no expected breaking changes. The maintainer clarified the release gate during this review:
**unresolved defects in core functionality block graduation**. Bounded limitations in ancillary
integrations are recorded explicitly. Graduation is not a promise of zero bugs.

The preferred review window is 2026-07-17 through 2026-10-15. It is retrospective, not a new
90-day waiting period. The review distinguishes interface changes, additive capabilities,
default changes, fixes, and internal refactoring. Commit counts and time since the last commit
are not measurements of interface stability. Existing edition gates remain unchanged.

Customer adoption statements below are maintainer attestations. The separate telemetry
review measures recorded command activity, not independently verified customer adoption.
No customer, employer, project, person, account, workspace, or infrastructure identifiers are
included. No named testimonials or raw issue bodies are reproduced. A quiet issue search is
not proof of active use or absence of defects.

The maintainer's latest attestation is production use of CI and Toolchain by dozens
of their own customers, alongside broader community adoption. These directly
confirmed deployments are a subset of adoption, not a total customer count.
Telemetry opt-outs and unobserved deployments are not quantified.

The issue review retrieved all 97 open issues and 140 issues updated during the review window,
without restricting the search to the `bug` label. Titles were screened across candidates;
relevant bodies were read to distinguish reports, enhancements, and already-fixed behavior.
Closed reports and fixes without issues were included. These are a dated snapshot, not a
claim about subsequently filed reports. Recheck core-functionality reports at release cutoff.

## Anonymous telemetry review

Queried on 2026-10-15 through PostHog MCP. The window is **2026-07-17 00:00:00 UTC
through 2026-10-14 23:59:59.999999 UTC**: 90 complete days, excluding the partial
review day. The governed metric catalog returned no definitions. These are one-off
review aggregates, not saved or approved business metrics.

Native `query-trends` queries selected the `command` event with
`filterTestAccounts: true`, no additional query sampling, and `math: total`.
Feature series filter the `command` property using the command-family regexes below.
Weekly queries reproduce the same feature totals; calendar weeks at the window's
edges are partial. No raw event, installation ID, customer identifier, or account
identifier is published.

| Observed scope | Retained command events | Weeks with recorded activity | Command-property filter |
|---|---:|---:|---|
| All Atmos commands | 7,676,152 | Not separately assessed | None |
| Auth | 595,068 | 14 of 14 intersecting weeks | `^atmos auth( \|$)` |
| Toolchain | 361,171 | 14 of 14 | `^atmos toolchain( \|$)` |
| Explicit CI commands | 74,696 | 14 of 14 | `^atmos ci( \|$)` |
| Emulator | 1,262 | 13 of 14 | `^atmos emulator( \|$)` |
| Explicit source-management commands | 748 | 14 of 14 | `^atmos terraform source( \|$)` |

The announcement's **more than a million executions in the past 90 days** refers
only to the combined Auth, Toolchain, and explicit CI command families:
595,068 + 361,171 + 74,696 = **1,030,935** recorded executions. These command-family
filters are disjoint. The broader 7,676,152 total is retained here as context and
is not presented as usage of the graduating features.

Breaking down the same Auth series by the event's `ci` property returned
**118,374 with `ci=false`** and **476,694 with `ci=true`**, totaling 595,068 with
no missing CI flags. This supports the announcement's "nearly 120,000 authentication
commands" outside CI pipelines. It describes Atmos's CI-environment detection,
not a count of human logins or distinct people; non-CI automation can be included.

The observed current ingestion configuration contains three enabled downsampling
rules, all using **distinct-ID-aware sampling**:

| Commands targeted | Configured retention | Configuration last updated |
|---|---:|---|
| `describe component`, `describe config`, `__complete` | 20% | 2025-12-23 |
| `version` | 1% | 2026-04-09 |
| `describe component` (an additional overlapping rule) | 10% | 2026-04-09 |

These rules do not target the five command families in the table. The current
configuration timestamps predate the review window. The revision-history check
returned no entries; these timestamps are not a complete historical audit of all
ingestion settings. Two other enabled transformations drop selected completion,
backend/varfile-generation and workspace commands, and events with version `test`.
The project's standard internal/test exclusions were also applied at query time.
These exclusions are not proof that every internal or automated invocation is removed.

PostHog's [downsampling documentation](https://posthog.com/docs/cdp/transformations/downsampling-plugin)
distinguishes ingestion downsampling, which drops events without scaling charts
back up, from query sampling, which estimates totals. **The 7,676,152 figure is an
observed retained count, not a 1% sample of all Atmos commands.** No multiplier is
applied. In particular, do not multiply the overlapping 20% and 10% rules as if
they were independent random event samples. Their combined retention has not been
verified, and no overall extrapolation is published.

Interpretation limits:

- These counts show repeated activity, not customer counts or proof of successful
  production deployments. Customer evidence remains the separate anonymized
  maintainer attestation. The populations can overlap and must not be added together.
- Telemetry records the invoked command and an error indicator. A command error is
  not necessarily an Atmos defect; a recorded invocation is not proof of success.
  This review does not turn event counts into a bug rate or reliability percentage.
- Auth invoked inside Terraform, native CI hooks, and automatic source fetching
  are not separately identifiable from these command-family filters. Do not count
  every Terraform command in CI as adoption of native Atmos CI. Likewise, legacy
  `vendor pull` activity does not measure newly graduated JIT source provisioning.
- Installation IDs are generated and cached locally. Disposable CI environments
  or cache resets can create additional IDs. They are not people or organizations;
  no installation-ID total is used as an adoption headline.
- The five feature totals corroborate activity at differing scales. They do not
  establish provider/driver coverage, interface compatibility, or absence of core bugs;
  the repository review, regression checks, and field reports supply that evidence.

## Graduation decisions

| Capability and supported scope | Introduction evidence | Interface and regression history | Anonymous use evidence | Decision |
|---|---|---|---|---|
| CI: native Terraform plan/apply/deploy reporting, summaries, outputs, comments and configured planfile storage; GitHub Actions and generic CI | Introduced 2026-03-17, commit `fd3e534090`, first containing stable tag `v1.210.0` | Reviewed 90-day history. New reporting APIs and fork gates continued through 2026-10-14; no claim that the whole expanding surface was unchanged for 90 days. Existing command/configuration entrypoints remain. Core storage-priority and oversized-comment defects addressed in this change. | Used in production by dozens of the reporting team’s own customers, with broader community use reported; frequently combined with Auth and Toolchain. Duration was not quantified. The repository also runs native-CI workflows. | Graduate the established command/configuration contract. |
| Auth: identity selection, login, credential export/execution, session refresh/logout, and the shared authentication contract | `v1.196.0`; announcement 2025-10-20 | Reviewed 90-day history. Additive providers/integrations and fixes continued through October. Recent SSO pagination, identity merge, ambient environment, and session-chain fixes preclude a claim of 90 bug-free days. Provider-specific adoption is uneven. | Reporting team has used Auth exclusively for several months; frequently used with CI and Toolchain in production. | Graduate the Auth interface; explicitly retain provider/integration limitations below. |
| Toolchain: version declarations, resolution, installation, tool selection, lockfiles and verification | Introduced 2026-01-01, commit `3292013be0`, first containing stable tag `v1.203.0` | Reviewed 90-day history. `get` output formats changed 2026-08-01 (75 days before review); `add` default/version-list and `clean` confirmation behavior changed 2026-08-14 (62 days); lockfile default changed 2026-09-01 with edition compatibility; path/declaration fixes 2026-09-24 and shared baseline behavior 2026-10-11. Do not claim the complete interface was unchanged for 90 days. | Used in production by dozens of the reporting team’s own customers, with broader community use reported; frequently combined with CI/Auth. Repository automation installs and executes Toolchain tools. Duration was not quantified. | Graduate; preserve exact release tags on metadata lookup failure and retain existing pin compatibility. |
| Emulator: stack-scoped component lifecycle, discovery, endpoints, execution, persistence and teardown for local development/testing | Introduced 2026-06-27, commit `3562be2ee8`, first containing stable tag `v1.222.0` | Reviewed 90-day history. Shared stack networking added 2026-08-19; cancellation propagation 2026-09-07; Floci health check fixed 2026-09-15. Existing lifecycle commands retained. This does not establish an unchanged 90-day behavior window. | Nearly all of the reporting team's demos use Emulator; this repeated use has exposed and helped fix numerous core defects. Exact duration and per-driver usage were not supplied. | Graduate Atmos's lifecycle interface, without claiming exhaustive cloud API fidelity or equal field coverage of every driver/runtime. |
| Source vendoring: stack `source` declarations, JIT fetching, version/URI refresh, TTL caching, and automatic workdir staging; `terraform source` management | `v1.203.0`, #1877, introduced 2025-12-30 | Reviewed 90-day history. Retry fixes landed 2026-09-08; workdir path encoding 2026-08-26; global-default and generated-file isolation fixes 2026-09-24. These are fixes, not evidence of a 90-day freeze. New GHES support is additive. | Reporting team uses source vendoring and attests to substantial community use. Prior instability is acknowledged; current stability is reported. Exact duration and transport-specific usage were not supplied. | Graduate JIT source vendoring and its automatic workdir support; include standalone workdir management under the separate scope below. |
| Workdir management: `terraform workdir list`, `describe`, `show`, and `clean`, including formats and TTL/dry-run cleanup | `v1.203.0`, #1876 | Existing command entrypoints retained through the 90-day review. August 26 path/instance resolution fixes incorporated. A component-specific `clean --dry-run` deletion was reproduced and fixed during this review; regression checks verify preserved files and actual cleanup. | 179 retained explicit workdir command events in the same 90-day telemetry window; no customer/production or per-subcommand success claim inferred. Shared staging is exercised by the attested source-vendoring use. | Graduate management commands after the dry-run fix. Local-only provisioning remains deferred. |
| Pro: existing CLI integration and commands | CLI and current Pro documentation already lack experimental markers; hosted-service GA date not established here | No experimental transition to apply. Recent upload-size handling landed in #3314. This repository does not establish the hosted service's release/support commitments. | The repository's automation uses Pro; customer adoption was not separately asserted for this review. | Preserve existing CLI status. Do not announce a new hosted-service GA milestone. |

The shorter-history rationale is explicit: strong production use of CI/Auth/Toolchain together,
repeated Emulator use in real demos, and reported team/community source-vendoring use support graduation of their established interfaces
despite additive development and recent fixes. There is no assertion that the entire surfaces
were frozen for 90 days, that all combinations were field-tested, or that no future change is possible.

## Interface stability accounting

The command-tree comparison uses `c04641e964` (2026-07-16) through the review baseline.
It checks command names, flags, defaults, exit behavior, and structured output, alongside
configuration and implementation history. The supported interface continues to grow;
additions do not establish that a newly added provider was field-tested for the whole window.

| Capability | Established interface observation | Changes that limit the stability claim |
|---|---|---|
| CI | Existing command/configuration entrypoints retained across the 90-day review window | Additive workflow validation/reporting; storage-selection behavior corrected here. The new validation command has explicit failure exits and is not claimed to have 90 days of field use. |
| Auth | Existing command names/flags retained across the 90-day window | Added `login --webflow`, tag environment/config bindings, providers and integrations. Credential/session bug fixes change erroneous behavior. No planned breaking interface change was identified in this review. |
| Toolchain | Current `get` output formats: 75 days; version-list/add and clean-confirmation behavior: 62 days | Lockfile-default edition transition on 2026-09-01 is 44 days old. The shorter intervals qualify through production-use attestation and focused compatibility checks, not an assumed 90-day freeze. |
| Emulator | Existing lifecycle command names and flags retained across the 90-day window | Added shared networking; cancellation and health-check fixes change runtime behavior. Driver-specific fidelity and duration remain unverified. |
| Source vendoring | Source commands and `source.uri`, `version`, and TTL entrypoints retained across the 90-day window | Corrected workdir default configuration on 2026-09-24 is 21 days old; retry, path and isolation fixes are included. The nonfunctional `settings.provision` location is not a supported compatibility contract. |

## Reports, dispositions, and limitations

| Report or evidence | Review result |
|---|---|
| [CI storage priority #2368](https://github.com/cloudposse/atmos/issues/2368) | Confirmed in the native-CI factory: explicit priority fell through to environment detection. Reproduced before the fix. Now uses the existing artifact selector, honors the explicit default/priority order, skips entries without available backend credentials, and errors instead of silently redirecting an invalid explicit selection. Regression checks inspect the actual selected store's contents. |
| [Oversized PR comments #3088](https://github.com/cloudposse/atmos/issues/3088) | Confirmed: the provider submitted an oversized body. Reproduced before the fix. A bounded UTF-8 body preserves the update marker and puts a visible warning before the retained summary. The caller's complete summary remains unchanged. Repeated posting still updates one comment. |
| [Toolchain tag normalization #3211](https://github.com/cloudposse/atmos/issues/3211) | Confirmed metadata-failure path guessed a `v` prefix. Fixed to preserve the actual tag; only explicit registry metadata strips a prefix. Existing metadata-aware tests cover explicit/no-prefix conventions. The broader request to remove download compatibility fallback remains deferred: legacy bare pins still use it, and existing verification aligns sidecars with the downloaded artifact. The install/lock regression check confirms that path works; do not claim the entire issue is closed. |
| [Shared tool manifest #2256](https://github.com/cloudposse/atmos/issues/2256) | Existing component and workflow default-resolution tests cover skipping unusable/unresolvable manifest entries. Explicitly requested invalid dependencies still fail intentionally. Issue state alone does not establish a current failure in the automatic baseline path. |
| [Auth recent fixes #3361](https://github.com/cloudposse/atmos/issues/3361), [#3348](https://github.com/cloudposse/atmos/issues/3348), [#3343](https://github.com/cloudposse/atmos/issues/3343), [#3335](https://github.com/cloudposse/atmos/issues/3335), [#3232](https://github.com/cloudposse/atmos/issues/3232) | Closed reports affecting core use, including ambient env, user/role session chains, SSO pagination, merged identities and Terraform output authentication. Include them in the evidence; do not advertise an absence of core reports. |
| [Azure integration flags #3386](https://github.com/cloudposse/atmos/issues/3386) | Open bounded limitation: the separate Azure AKS integration command ignores path/alias flag overrides. Core identity authentication is not the reported failure. Use integration configuration for these settings; this review does not claim the flag override path is fixed or that Azure has equivalent adoption evidence. |
| [Helmfile/EKS #2279](https://github.com/cloudposse/atmos/issues/2279), [#2596](https://github.com/cloudposse/atmos/issues/2596), [#2188](https://github.com/cloudposse/atmos/issues/2188) | Legacy Helmfile `use_eks` bootstrap/profile compatibility is not certified by the Auth graduation. Credential forwarding and shorthand passthrough reports require separate integration work. Current code automatically runs `aws eks update-kubeconfig` when `use_eks` is enabled. Preparing kubeconfig alone does not bypass that path; the earlier blanket workaround was not verified and has been removed from the announcement. Core Auth credential/session APIs and Terraform integration are the promoted contract. |
| Volatile computed values in plan comparison | Current `pkg/terraform/plandiff` contains configurable exclusion policy and regression tests for `aws_db_instance.latest_restorable_time`. Verified separately; no claim that all provider-specific volatile fields should be ignored automatically. |
| [OpenTofu state format #3282](https://github.com/cloudposse/atmos/issues/3282) | Upstream state-format compatibility is outside the Toolchain installer contract. Toolchain does not migrate Terraform/OpenTofu state; operators must use a compatible pinned tool version. |
| Source/workdir regression fixes #3073, #2985, closed #3192 and #3197 | Retrieval retries, hyphenated paths, generated-file races, and inherited workdir defaults were fixed in repository history. Current source/workdir regression suites pass. The review additionally reproduced a cache miss for an unchanged URI containing `?ref=`: provenance redaction removed the query before comparison. The fix compares the retained source declaration, preserving cache hits and detecting changed refs, with older-metadata fallback. Core reports existed; no claim of an absence of reports. |
| [Vendor update #3076](https://github.com/cloudposse/atmos/issues/3076), [base path #2409](https://github.com/cloudposse/atmos/issues/2409), [invalid URL #1825](https://github.com/cloudposse/atmos/issues/1825) | These concern `vendor.yaml` update receipts, vendor target roots, and misleading authentication prompts for invalid repositories. JIT provisioning uses separate source-version/URI metadata comparisons (including the cache-key fix above) and per-component targets; it does not use the vendor-update receipt skip decision. Existing JIT version/URI refresh tests cover that distinction. Invalid-source diagnostics remain a bounded limitation; these reports are not declared closed. The legacy vendor commands are not newly promoted here. |
| [Affected custom types #3208](https://github.com/cloudposse/atmos/issues/3208) | Open core limitation of affected-component detection. Keep `list affected` experimental; graduating CI does not graduate its optional affected-discovery inputs. |
| [Compatibility matrix #2865](https://github.com/cloudposse/atmos/issues/2865) | Proposed generalized pairwise gate is not implemented by this change. Use focused integration checks of existing providers, hooks, storage, installer verification and experimental policy. Do not present these as an exhaustive compatibility matrix. |

## Other candidates screened

Every row below remains experimental. Unless explicitly stated, capability-specific adoption,
exact introduction tag, and a complete interface-stability interval are **not established** by
this review. Announcement dates are provenance, not a substitute for the first shipped tag.
Tests/examples establish available verification material, not customer use. Review ownership
remains with the anonymized project maintainer; no promotion is inferred from missing reports.

| Capability / interface | Introduction or history evidence | Reason to defer / next evidence needed |
|---|---|---|
| Affected discovery: `list affected`, shared diff traversal | Announced 2025-12-20; selectors added 2026-10-08; closed #3203/#3199 fixed built-in/deleted types | Core custom-type omission #3208; also review open cross-stack/context reports. |
| Local-only workdir provisioning | `v1.203.0`, #1876; path encoding fixed 2026-08-26; closed #3192/#3197 address races/global defaults | Source staging and management commands graduate above. Independent field-use evidence for local-only provisioning remains unverified. |
| Devcontainers: `devcontainer` lifecycle | `v1.201.0`, announced 2025-12-06; few recent direct changes | No directly matching core report found in the retrieved issue titles/bodies; native Atmos lifecycle use is unverified. A `.devcontainer` file alone is not evidence of using Atmos's lifecycle. |
| Backend provisioning: `terraform backend`, provision hooks | Announced 2025-12-12 (`v1.202.0` per roadmap); Azure support added 2026-08-11 and S3 namespace option 2026-10-07 | Provider-specific bootstrap evidence and state-safety checks needed; no independent adoption attestation. |
| Explicit stack names: stack `name` | Announced 2025-12-10; closed #2782 concerns dependency ordering with explicit names | Promising small interface; verify naming/dependency combinations and establish field use. |
| File generation: inherited `generate` blocks | `v1.204.0`, #1878; closed #2326 concerns global generation | Promising candidate; needs usage and overwrite/inheritance compatibility evidence. Do not conflate this with Terraform varfile or scaffold generation. |
| Terraform CLI configuration: `components.terraform.rc` | Announced 2026-06-14 | No directly matching report found in the retrieved issue search; field use unverified. |
| Automatic provider caching | Announced 2026-01-03 | Needs shared-cache concurrency/platform evidence and field use. Distinct from registry proxy caching. |
| Terraform registry cache: `terraform cache` and proxy | Announced 2026-06-14 | Needs provider/module, offline and concurrency evidence; field use unverified. |
| Locals: file-scoped locals and references | Announced 2025-12-17 | Core reports #2374, #2343 and #2344 remain open. |
| YAML key delimiter: `settings.yaml.key_delimiter` | Announced 2026-07-05 | No field-use attestation; retain the setting-level gate. |
| Editions: `edition`, date-pinned defaults | Announced 2026-07-16; reusable behavior gates changed 2026-10-14 | Active compatibility work; retain setting-level experimental gate and all current edition values/defaults. |
| AI: `ai` commands and assistant interface | Roadmap: Q1 2026; exact introduction tag not verified | No separate usage/interface-stability evidence for graduation. |
| MCP: `mcp` management/client/server integration | Roadmap: Q1 2026; #3269 recently fixed config-fragment writes | No capability-specific adoption evidence; retain command and docs status. |
| LSP: `lsp`, client/server interfaces | Roadmap: shipped milestones; exact introduction tag not verified | No capability-specific adoption evidence. |
| Native Helm: `helm`, lifecycle and GitOps delivery | Announced 2026-07-02; substantial lifecycle and runtime-override additions | Interface expanding; no separate production attestation. |
| Native Kubernetes: `kubernetes`, manifests/Kustomize/GitOps | Announced 2026-07-01 | CLI is experimental despite missing roadmap flags; correct status metadata, do not silently graduate. |
| Secrets: `secret`, declared secrets and `!secret` | Announced 2026-06-14 | Needs backend-specific use, masking and lifecycle evidence. |
| Store CRUD: `store` and store workflow step | Roadmap: Q3 2026, #2858; recent backend extensions | Separate from longstanding store reads; no write-lifecycle adoption evidence. |
| SBOM: `sbom generate` | Announced 2026-07-17 | Needs provenance completeness and downstream consumption evidence. |
| Git: `git`, repository and hook configuration | Announced 2026-06-14; closed #3254 fixed history-preserving initialization | No separate supported-interface/adoption review. |
| Initialization: `init` | Implementation #1687 merged 2026-07-18; older announcement predates implementation | Template lifecycle still evolving; do not use announcement age as implementation age. |
| Scaffolding: `scaffold` and merge/update policy | Implementation #1687; additions through 2026-10-07; multiple closed merge/hook/path regressions | Needs merge-safety, upgrade and field-use evidence. |
| Terraform migrations: `terraform migrate` | #2534 merged 2026-08-04 | State-mutating interface needs its own adoption and safety review. |

## Implementation and verification

- Remove `IsExperimental` status for the four graduated command families, including inherited
  annotations, and remove the separate native-CI hook experimental gate.
- Remove the source-vendoring documentation/roadmap experimental markers; its commands
  and automatic provisioning already have no separate runtime gate.
- Preserve `ci.enabled`, normal CI detection, and all remaining command/configuration gates.
- Keep docs, roadmap flags, sidebar status, current experimental tables, and automation function
  references consistent. Preserve historical announcements. Correct missing deferred-feature
  roadmap flags without adding/removing/reordering curated highlights.
- Publish one anonymized [changelog post](../../website/blog/2026-10-15-core-features-graduate-to-stable.mdx).
  It contains no invented release number or customer identities.

Verification covers real registered command trees and remaining experimental annotations;
Terraform CI hooks writing outputs under all experimental modes and remaining off for disabled
CI/local invocation; storage selection precedence and actual destination; oversized Unicode
comment creation/update; exact-tag preservation; shared tool manifests; install/lock checksum
verification after version fallback; source staging/cache reuse/refresh under restrictive modes;
query-ref cache identity across metadata writes; and the existing Emulator/Devcontainer suites.

The version-fallback checksum test already passed before changes: existing sidecar alignment
handles that scenario, so no installer behavior change was needed. This distinguishes a
verified existing fix from the tag-normalization defect reproduced and fixed here.

Recorded local verification:

- `go build ./...` passed. Changed-code lint passed after the source fix (zero issues).
- Focused command, CI storage/comment, hook, Auth, Toolchain registry/installer, dependency,
  plan-diff, Emulator, source/workdir and Devcontainer tests passed. The source query-ref
  regression failed before the fix and passed afterward. The workdir component-clean
  dry-run regression also failed before its fix and passed afterward.
- Real CLI invocations for Auth, Toolchain, Emulator, CI cache paths, and Terraform source
  list/pull passed under `disable`, `error`, and `warn` without experimental notices.
  Deferred Devcontainer commands remained blocked under restrictive modes. Workdir
  list/show/describe/clean (including dry-run preservation) passed with actual Terraform
  source staging under `disable` and `error`. The documented config set/get examples
  passed in the same isolated fixture.
- Website production build and navigation tests passed, including the final build after
  the source-graduation updates.
- All 53 regenerated CLI casts passed color, error-output, and local-path checks. The global
  cast validator still rejects three unchanged config/stack-get recordings for missing
  color; the same recordings fail at the baseline. No unrelated recordings were rewritten.
- `atmos test` exercised the repository's short suite. Two environment-dependent failures
  (`TestSetupColorProfileIgnoresScriptFlags` and `TestPrintStartupStatus_PrintsWhenInCI`)
  reproduce on the unchanged baseline with `NO_COLOR` and the inherited startup-notice
  sentinel respectively. Both packages pass with those environment variables cleared.
  The large `tests` package exceeded the runner's five-minute limit. A longer-timeout
  run completed and identified 47 snapshot/CLI failures plus two native-CI workflow
  failures. The label snapshots were regenerated with `-regenerate-snapshots`;
  normal verification passed afterward. The CI failures exposed unavailable remote
  stores being selected from priority lists; credential-availability checks fixed
  that regression. Both affected CLI cases and both real Terraform plan/upload/list
  workflow tests then passed. All failures from that longer run were addressed and
  rerun individually; the entire suite was not repeated again. Do not describe the
  original repository-wide run as passing.

Recheck open core-functionality reports at the actual release cutoff. Credentialed customer
deployments and every live emulator/provider combination were not rerun locally. Do not
convert these limits into claims of comprehensive production QA.
