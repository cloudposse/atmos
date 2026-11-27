# Adopt Terragrunt units and stacks

**Status** — Proposed onboarding for the [Terragrunt component plugin](../terragrunt-support.md). Follows the [shared adoption contract](README.md).

## Outcome

A Terragrunt user can select a unit directory or a live repository and see the units they recognize. Saving the selection gives the team ordinary Atmos component addresses while retaining its HCL, module sources, dependencies and state layout.

```shell
atmos terraform plan ./live/dev/vpc
atmos stack add ./live/dev/vpc -s dev --dry-run
atmos stack add ./live/dev/vpc -s dev
atmos terraform plan vpc -s dev
```

## What can ship first

Register one explicitly selected `terragrunt.hcl` with a `terragrunt://` import. For a team that already invokes Terragrunt, propose the plugin’s executable mode, pin the established binaries and preview the project configuration change. Executable mode is project-wide. If an existing Atmos project uses native mode, changing that setting could switch every unit. Keep the current mode and report the conflict when the requested unit needs another. Record the plugin package and resulting lockfile for a fresh checkout. [Plugin setup](../../../website/docs/components/terraform/terragrunt.mdx) · [Execution mode and limitations](../../../website/docs/migration/terragrunt.mdx#executable-mode)

| Stage | Reuse | Boundary for first release |
| --- | --- | --- |
| Identify | Look for unit or stack filenames before plain Terraform discovery. | A missing plugin can still yield an identified path and setup instructions. Marker detection must work without evaluating HCL. |
| Resolve one unit | The plugin already discovers units, follows includes and reports per-unit failures. | Start with a local source, resolvable includes and existing state. If identity depends on an unavailable environment value, external catalog or blocked `run_cmd`, show the unit path and unresolved expression. |
| Save and plan | A named YAML import works with tree discovery disabled. Executable mode delegates execution to Terragrunt. | Verify the original unit CWD, selected binary, workspace, inputs and backend key. Exclude Atmos-managed saved-plan handoff and stack variable overrides in executable mode. |
| Expand a tree | The plugin can expand stacks in an overlay and deduplicate their generated children. | Enable registration of a tree after pinning the catalog and verifying the complete inventory, partial failures and duplicate addresses. Schedule bulk plan/apply separately. |

Three focused plugin tests passed in this audit. They cover discovery reading the Terraform binary without invoking it, skipping materialized nested stacks, and rejecting unsupported hook semantics. They exercise the engine below the new CLI. The existing executable-mode integration scenarios include an invocation probe and a real pinned binary. Reuse the real-binary setup for the importer test. [Executable-mode scenarios](../../../plugins/terragrunt/scripts/modes.star)

The release test still needs a side-by-side single-unit plan with the pinned Terragrunt executable from two clean checkouts. Verify hook invocation count on explicit plan and zero during preview. Use Gruntwork’s prod/non-prod stack declarations as the later expansion test. [Execution tests](../../../plugins/terragrunt/internal/engine/execution_test.go) · [Discovery tests](../../../plugins/terragrunt/internal/engine/discovery_paths_test.go) · [Pinned stack layout](https://github.com/gruntwork-io/terragrunt-infrastructure-live-stacks-example/tree/753aa3b43fd36a874c33ea67cb042a9fe51ca2e5)

## Working with the saved stack

**One chosen unit in an Atmos stack.** For the proposed `stack add ./live/dev/vpc -s dev`, enable the Terragrunt plugin in `atmos.yaml`, preserve the established engine and toolchain, and include the named stack manifest. Use the existing unit import syntax:

```yaml
# stacks/dev.yaml
import:
  - terragrunt://live/dev/vpc
```

The file names the Atmos stack `dev`; the imported unit supplies component `vpc`. Check it with `atmos stack instances -s dev`, then run `atmos terraform plan vpc -s dev`. Adding `./live/dev/database` to dev appends a second import. Selecting `./live/prod/vpc -s prod` creates or extends `stacks/prod.yaml` with the prod unit. A shared basename across environments is expected. Resolve a collision within the same destination stack before writing.

**Whole live tree.** When the user chooses plugin discovery for `live/**`, save its scope in `atmos.yaml` and list the plugin-derived addresses. In the documented layout the unit above is `vpc` in stack `live/dev`, so the named command is `atmos terraform plan vpc -s live/dev`. This mode needs no per-unit stack files. Show which mode and addresses will be saved. Verify that subsequent imports or discovery do not create a second runnable address for the same deployment without an explicit user choice.

## Source rules

- Recognize configured unit directories containing `terragrunt.hcl` and explicit stack definitions containing `terragrunt.stack.hcl`. A directory containing both Terraform code and a Terragrunt unit goes through Terragrunt discovery first. An include-only file such as `root.hcl` is insufficient evidence of a deployment.
- Reuse the plugin's unit and stack discovery, includes, inputs, remote state and dependencies. Ignore `.terragrunt-cache`. Deduplicate generated units with their declaring stack. Show the source stack when expansion requires an external catalog or generation step.
- For a selected unit, preserve the working directory and the established Terragrunt execution mode. Evidence that the team runs the Terragrunt binary selects executable mode. Respect an existing explicit native-mode setting. Surface unresolved mode or toolchain choices before planning. Preserve path-relative backend keys and external dependencies.
- Inspect a directory tree without running hooks or executing arbitrary configuration as a side effect of identifying a source. Report when resolution needs a plugin, environment value, generation step or external source. Explain those requirements at the point of trial.
- For an existing named Atmos stack, reuse a `terragrunt://` unit import where appropriate. For a whole live tree, offer plugin discovery with scoped include paths. Preview plugin configuration and the resulting addresses. Avoid generating a parallel copy of each unit's inputs. [Current Terragrunt integration](../../../website/docs/components/terraform/terragrunt.mdx)

## Acceptance

- A conventional unit with inherited inputs and path-relative remote state selects the same source, backend key, workspace and effective inputs as its native invocation.
- A mixed directory resolves to the Terragrunt unit. A repository containing three units offers all three once. Choosing one in the picker can be replayed by path in CI.
- A stack whose units live in a catalog can identify the declaration before expansion. An unavailable catalog reports the unresolved source and never substitutes an empty successful inventory.
- Initial trial and saved execution agree on the selected unit and plan. Re-importing the same unit produces no manifest or plugin changes. A pre-existing executable workflow remains executable through adoption.

Use both the [classic live repository](https://github.com/gruntwork-io/terragrunt-infrastructure-live-example) and the [explicit stacks repository](https://github.com/gruntwork-io/terragrunt-infrastructure-live-stacks-example) for compatibility fixtures.
