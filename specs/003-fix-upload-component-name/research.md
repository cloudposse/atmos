# Research: Full Component Name in Atmos Pro Uploads

**Feature**: `specs/003-fix-upload-component-name` | **Date**: 2026-09-10

All findings come from direct code inspection of the working tree (branch base `main` at `0d3ce6fee`) during the review of GitHub issue #3102. There were no unresolved NEEDS CLARIFICATION items.

## Decision 1: The issue's "single call site" claim is wrong — there are two independent truncation sites

- **Decision**: Fix both `internal/exec/pro.go` `uploadStatus` (~line 254, `InstanceStatusUploadRequest.Component`) and `cmd/terraform/utils.go` `terraformNodeHooks.recordExecResult` (~line 581, `buildTerraformExecData` argument). Do not treat `recordExecResult` as covering single-component runs.
- **Rationale**: `recordExecResult` fires only when `info.NodeHooks` is wired, which happens exclusively in `wirePerComponentHook` for `--affected`/`--all`/`--components`/`--query` dispatch (`cmd/terraform/utils.go:1636–1666`). `captureExecMetadataSync` (`internal/exec/terraform.go:257`) explicitly skips when `NodeHooks != nil`. The single-component execution record instead flows through `terraformExecMetadataParserFunc` (`cmd/terraform/utils.go:958`), whose `component` is `execComponent = args[0]` — the raw, full CLI argument (`cmd/terraform/plan.go:91–95`, same in `apply.go`/`deploy.go`) — already correct. Therefore the issue's exact repro (`atmos terraform plan "foo/bar/baz" -s <stack> --upload-status`) is truncated only via the **instance-status** upload (`uploadStatus`), which the issue's proposed one-line fix would not touch.
- **Alternatives considered**: (a) Apply only the issue's proposed `recordExecResult` change — rejected: leaves the reported repro broken. (b) Fix only `uploadStatus` — rejected: leaves multi-component execution records truncated (same bug class, confirmed at the same field).

## Decision 2: Use `info.ComponentFromArg` as the identity source at both sites

- **Decision**: Substitute `info.ComponentFromArg` for `info.Component` in both payloads; change nothing else.
- **Rationale**: `ComponentFromArg` always holds the full logical name at both call sites:
  - Single-component: set from the CLI argument; slash-containing logical names do **not** trigger path resolution (`IsExplicitComponentPath` in `internal/exec/cli_utils.go:441–450` only fires on `.`, `./`, `../`, `/` prefixes), and path-style args are rewritten to the resolved logical name before execution (`cmd/terraform/utils.go:1411`).
  - Multi-component: the scheduler adapter sets `nodeInfo.ComponentFromArg = node.Component` — the full logical graph-node name — at `pkg/scheduler/adapters/terraform.go:681–682`, and nothing downstream truncates it (`ProcessStacks` only truncates `Component`).
- **Alternatives considered**: (a) Re-join `ComponentFolderPrefix + "/" + Component` — rejected: fragile reconstruction of a value that already exists intact, and `ComponentFolderPrefix` is overwritten by the base-component path when `metadata.component` inheritance is in play (`internal/exec/utils.go:1120–1129`). (b) Thread a new dedicated field — rejected: YAGNI, `ComponentFromArg` already carries the documented contract ("from the CLI's own component argument", `specs/002-pro-exec-metadata/data-model.md`).

## Decision 3: The splitting logic is intentional and must not change

- **Decision**: `internal/exec/utils.go` `ProcessStacks` ~lines 1106–1116 (split of `ComponentFromArg` into `ComponentFolderPrefix` + leaf `Component`) is read-only for this feature.
- **Rationale**: `Component`/`ComponentFolderPrefix`/`FinalComponent` feed Terraform working-directory resolution (`components/terraform/<prefix>/<leaf>/`) across the codebase (e.g. `u.GetComponentPath` at `cmd/terraform/utils.go:357`). Note the split also runs *after* `postProcessTemplatesAndYamlFunctions` may set `Component` from the component section's `component:` attribute (`internal/exec/utils.go:1475–1477`), so the split is the final author of `Component` — confirming flat-named components are byte-identical before/after this fix (spec FR-006).
- **Alternatives considered**: none seriously — the issue itself correctly flags this as out of bounds. (Ticket nit: it attributes the split to `ProcessCommandLineArgs`; it actually lives in `ProcessStacks`, same file.)

## Decision 4: Defer the path-style-argument quirk to a follow-up issue (clarified with user)

- **Decision**: The pre-existing quirk — `execComponent = args[0]` is captured in RunE *before* `resolveComponentPath` rewrites a path-style argument (`./components/terraform/vpc`) to its logical name, so the raw path lands in the single-component execution record — is out of scope. Open a follow-up GitHub issue and link it by number in the PR description before merge (spec FR-008; repo Follow-up Tracking policy).
- **Rationale**: User-confirmed in `/speckit-clarify` (Session 2026-09-10). Different trigger, different fix shape (capture-ordering change in three files), rare in CI where uploads matter; bundling it would widen an otherwise surgical diff.
- **Alternatives considered**: fix-in-passing (rejected by user); document-only without an issue (rejected — violates repo follow-up policy).

## Decision 5: Existing test fixtures and pact contracts are compatible with the fix

- **Decision**: No pact fixture changes expected; existing unit fixtures keep passing. New tests add nested-name cases.
- **Rationale**: Verified: `cmd/terraform/utils_exec_metadata_test.go` fixtures (lines 437, 517–518, 559) all set `Component` and `ComponentFromArg` to identical flat values (`"myapp"`, `"other"`) — the substitution is invisible to them. `pkg/pro/consumer_pact_test.go` uses flat names (`"vpc"`) throughout; the contract constrains shape, not this value.
- **Alternatives considered**: N/A (verification, not a choice).

## Decision 6: Server-side identity shift is accepted; no client-side compatibility shim

- **Decision**: After the fix, Atmos Pro instances previously recorded under the leaf name (`baz`) will report under the full name (`foo/bar/baz`). No dual-reporting or migration logic on the client.
- **Rationale**: The full name is the correlation key every other channel already uses (locks: user-supplied full name in `executeProUnlock`; affected uploads: logical names; single-component execution records: full CLI arg). The mismatch is the bug; converging is the point. Historical-row reconciliation is a server concern (spec Assumptions).
- **Alternatives considered**: report both names during a transition window — rejected: payload-shape change for a case the server side (owned by the same stakeholder who filed #3102) doesn't need.

## Decision 7: Release process

- **Decision**: PR label `patch`; no changelog blog post, no roadmap update; PR closes #3102 and corrects its root-cause narrative; `/pull-request` skill runs before opening the PR.
- **Rationale**: Bug fix with no new commands, flags, or config surface — the repo's label decision tree maps this to `patch`, which requires neither blog post nor roadmap entry.
- **Alternatives considered**: `no-release` — rejected: this changes shipped binary behavior.
