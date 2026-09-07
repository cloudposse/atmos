# Fix: Address field-test findings on the terraform `flags:` block (PR #2992)

**Date:** 2026-08-24

## Summary

A hands-on field-test pass of PR #2992 (declarative defaults for terraform CLI execution flags — `lock_timeout`, `lock`, `parallelism`, `refresh`, `compact_warnings`) found one real functional bug, one documentation contradiction, one pre-existing visibility gap this feature ran into, and zero integration/acceptance test coverage. This fix addresses all four plus a bundle of smaller documentation gaps, and records one investigated-but-not-fixed finding for the historical record.

## Context

The field test built a disposable local fixture with a mock `terraform` binary that logs its real argv, then exercised the feature end-to-end: full 3-layer precedence (atmos.yaml global < stack-level `terraform: flags:` < component `flags:`), env var overrides, CLI-typed-flag wins, quoted-YAML-string coercion, multi-level `metadata.inherits` propagation, and the per-subcommand flag support matrix. Most of this was confirmed working correctly with no prior coverage at all (every existing test was an isolated Go-level unit test with hand-built inputs — nothing exercised the real end-to-end wiring). Full findings are in the field-test conversation transcript; this record covers only what was fixed.

## Changes

1. **Typo'd flags key silently ignored (functional bug).** A mistyped key like `lock_timout` (missing the second `e`) decoded successfully with the typo'd field simply absent, and the intended override silently never took effect — no error or warning anywhere in the default `terraform plan`/`apply`/`destroy`/`refresh`/`import` workflow. `atmos validate stacks` did catch it, but that's a separate, non-default-invoked command.

    The first fix attempt (adding `ErrorUnused: true` to the mapstructure decoder in `pkg/schema/terraform_flags_decode.go`) was reverted: that decoder is also called from `internal/exec/utils.go`'s `findComponentInStacks`, a tolerant stack-name-candidate search that treats *any* error from a candidate as "wrong candidate, try the next one" and silently swallows it — so the typo error surfaced as a confusing "Could not find the component `vpc` in the stack" instead of the real problem, verified live before landing on the final approach.

    Final fix: a new `schema.ValidateTerraformFlagsKeys` function, called separately from `internal/exec/terraform_execute_helpers_args.go`'s five subcommand builders (plan/apply/destroy/refresh/import) at argv-build time — after the component is definitively resolved, mirroring exactly where an invalid `lock_timeout` duration is already caught. Decoding itself (`DecodeTerraformFlags`) still ignores unknown keys, unchanged.

2. **Documentation contradiction.** `website/docs/cli/configuration/components/terraform.mdx`'s `flags.parallelism` entry listed supported subcommands as "plan, apply, and destroy," omitting `refresh` — contradicting both `terraform-refresh.mdx` (which correctly listed it) and the actual code. Confirmed live that `atmos terraform refresh` does inject `-parallelism`; fixed the doc to match.

3. **`describe component` hides `flags:` by default.** `internal/exec/describe_component.go`'s `FilterComputedFields` allowlist (used by the default, `describe.component.filter: schema` mode) didn't include `"flags"`, so a component's fully resolved terraform CLI flag defaults were invisible via the most natural inspection command. Added `"flags"` to the allowlist. This allowlist is *already* missing several other real sections (`retry`, `generate`, `auth`, `secrets`, `command`, `backend_type`, `workspace`) — a pre-existing, broader gap, left as a code comment pointing here rather than expanded in this pass (narrower fix, per explicit scope decision).

4. **Zero integration/acceptance coverage.** Added `tests/fixtures/scenarios/invalid-terraform-flags/` plus `tests/test-cases/terraform-flags-declarative.yaml`, a `--dry-run` regression test proving the typo case (finding 1) fails loudly with the right error text through the real CLI, not just an isolated Go function call. Also added a `flags:` block to the public `examples/native-terraform` example (atmos.yaml global `lock_timeout`, dev-stack `parallelism` override) plus matching assertions in `tests/test-cases/native-terraform-example.yaml` and regenerated golden snapshots — proves the full merge chain through a real, working example that also serves as user-facing documentation.

5. **Doc-gaps bundle**, all confirmed against actual code/behavior before writing: `website/docs/cli/environment-variables.mdx` gained the 5 `ATMOS_COMPONENTS_TERRAFORM_FLAGS_*` vars, which that page claims to comprehensively list but omitted. `agent-skills/skills/atmos-stacks/SKILL.md` gained a `### flags` entry in the "Configuration Sections Reference," matching the existing `command`/`backend`/`providers` entries. `agent-skills/skills/atmos-terraform/SKILL.md` gained `flags` in the "Configuration in atmos.yaml" key-settings list and a pointer from "Common Flags." `website/blog/2026-08-24-terraform-lock-timeout-flags.mdx`'s "How to Use It" walkthrough only mentioned the env-var override layer once, in passing, in the closing line; added it to the main example.

    One flagged item from the field test was investigated and found to be a **false positive, not fixed**: `lock_timeout`'s JSON Schema entry lacks the `$ref: yamlFunction` alternative that `lock`/`parallelism`/`refresh`/`compact_warnings` have. Traced to `pkg/config/schema/overrides.go`'s `rejectsYamlFunctionString`: an unconstrained `string`-typed field (no enum/pattern/format) already accepts any string value, including a YAML-function-tag-shaped one, so the explicit alternative is genuinely unnecessary — confirmed by reading the generator logic and the generated schema. No code change.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/schema/... ./pkg/config/...` — pass, including new `TestValidateTerraformFlagsKeys_*` and updated `TestDecodeTerraformFlags_UnknownFieldsIgnored` tests.
- `go test ./internal/exec/...` targeted runs (`TestBuildTerraformCommandArgs_*`, `TestFilterComputedFields`, `TestDescribeComponentFilter`) — pass, including the new `TestBuildTerraformCommandArgs_Flags_UnknownKey_Errors` regression test.
- `go test ./tests -run 'TestCLICommands/...'` — targeted runs against every test this change touches (the new `invalid-terraform-flags` dry-run test, all `native-terraform-example` tests including regenerated snapshots) — pass. A full unfiltered `TestCLICommands` run was also started and its filtered output (native-terraform + adjacent tests) showed all passing; the full suite's raw exit code wasn't independently re-verified beyond that filtered view, given the narrow set of files this change touched and the targeted passes already covering all of them.
- Patch-scoped lint (`./custom-gcl run --new-from-rev=origin/main`) — 0 issues.
- `cd website && npm run build` — succeeds; no new broken anchors (the two pre-existing ones are unrelated to this change).
- Live-verified against a real build (`atmos build`) and the field-test fixture: typo now produces a clear error instead of "component not found"; the working `dev`/`typo` stacks behave identically to before.

## Follow-ups

None tracked. Finding #4 from the field test (`internal/exec/utils.go`'s `postProcessTemplatesAndYamlFunctions` silently swallows a post-template `flags:` decode error via `log.Warn` + stale-value-retention) was investigated and found to match the existing, pre-existing `retry` decode restoration's identical behavior a few lines above it — not a one-off inconsistency introduced by this feature. Left unfixed by explicit user decision; not tracked as a GitHub issue per the same decision. The broader `describe component` default-filter gap noted in Changes item 3 (missing `retry`/`generate`/`auth`/`secrets`/`command`/`backend_type`/`workspace`) is similarly informational only, not tracked.
