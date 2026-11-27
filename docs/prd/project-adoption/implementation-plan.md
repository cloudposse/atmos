# Existing-project onboarding implementation plan

**Status** — Implemented on the adoption feature branch. The November 27 newcomer field test at `04b9247a93` established the baseline. The acceptance checks below apply to the guided registration flow.

## 1. Make the second deployment target explicit

**Field tasks** `0b00895d`, `8012d8e2`. Two successful registrations under different Atmos stack names produced plans with the same prior resource ID. Planning an instance bound to workspace `blue` also left the developer's checkout on `blue`.

- Extend the existing local-backend resolver in `pkg/terraform/backend` to return the effective state location for the default and named workspaces. Account for a custom local backend path, workspace directory, initialization settings and symlinks. Two roots or engines can address the same state file.
- In `pkg/terraform/adoption`, compare the proposed target with other registered instances in the supported project layout. Show the existing component, Atmos stack and native workspace when another registration points at the same state. A same-instance retry stays idempotent. For a new instance, offer to choose another existing workspace or explicitly share this target. In CI, report the conflict and the exact flag for intentional sharing. Recheck under the project lock before saving.
- Scope workspace selection to the native process for an adopted named plan. Carry the verified workspace through init and plan using the per-command environment, and teach the executor's implicit workspace setup to honor it. Preserve explicit native workspace commands. Check previously generated manifests as well as fresh registrations.

**Accept when** Terraform and OpenTofu each pass a populated-state test. Registering `network/dev` and `network/prod` against different workspaces yields distinct prior IDs. An accidental alias is caught before writing. Intentional sharing is reproducible from a script. After a successful, failed or cancelled named plan, native `workspace show` still returns its original value. Concurrent plans for two workspaces select the correct state independently.

## 2. Make the no-flags path work in a terminal

**Field task** `e87c7536`. Change the CLI signature to `atmos stack add [path]`. Use existing Atmos TTY, CI and explicit-flag handling. Start in `cmd/stack/add.go`; keep source recognition in the registered adoption adapters.

| Invocation | Behavior |
|---|---|
| No path, interactive terminal | Discover candidates from the working directory. Show relative paths and source types. Enter accepts the sole suggestion. Multiple candidates get a picker. With no matches, offer manual path entry and an example. |
| Partial input, interactive terminal | Each supplied flag answers its own question. Ask for unresolved choices. Offer the component name and destination stack with editable defaults. Show the native target and inputs together before saving. |
| Fully specified invocation | Honor the supplied values directly. An identical retry reports the saved component and stack. |
| CI, redirected stdin or `--interactive=false` | Keep documented optional defaults. For a missing required or ambiguous choice, exit promptly with a copyable command naming the relevant flag. |

Keep explicit-path detection fast. For guided discovery, skip VCS metadata, generated files, dependency caches and symlink loops. Group files into deployment roots, give wrapper formats precedence within a root, and allow entry of an explicit path outside the scan. Discover by reading filenames and configuration, before taking a write lock or running tools and hooks. Escape or Ctrl-C before save should leave the project untouched.

Reuse the existing component and stack selectors for `helm template` and `packer validate`. Resolve a valid **component–stack pair**. A single compatible stack completes the selection; several compatible stacks produce a second question. In a bare project, `atmos terraform plan` should surface the existing-root trial with a usable path example.

**Accept when** a real PTY test starts at `atmos stack add`, finds a nested root, registers it under a chosen stack, and reaches the named plan. Repeat with one supplied flag, several candidate roots, cancellation, and closed stdin. Preserve the working Terraform/OpenTofu engine and Atlantis project pickers.

## 3. Preserve the inputs of the first native run

**Field task** `9d0b857b`. A native Terraform plan using two explicit varfiles was a no-op. Following the quick start proposed changing an existing value from green to gray.

- Have the Terraform and Packer adapters report candidate input files and identify files their native tool loads automatically. In a terminal, offer additional files, allow an explicit empty selection, and show the final order. A basename such as `prod.tfvars` is a suggestion only. User-supplied repeated `--var-file` flags establish the complete order and skip the picker.
- Use the existing ordered `vars` sequence with `!include`. Preserve native path bases and precedence. Surface known missing Packer inputs by name, account for native environment or other runtime inputs, and keep secret values out of the summary. If the user chooses to supply an input at runtime, say what the subsequent command requires.
- Support correcting a saved selection. Show a diff and require explicit intent to replace the input references for that instance. Preserve unrelated manifest edits, hand-authored variables, and file contents. Supply the equivalent scripted invocation in the diagnostic.

**Accept when** the native and named Terraform plans are both no-ops against the same populated state, and changing the last selected file changes the next plan. Run the equivalent Packer validation and compare the local build artifact. Cover a previously saved entry with missing files, an intentional empty selection, automatic files, and an invalid path.

## 4. Repair the CloudFormation first run

**Field tasks** `cdcdde98`, `a5b6b8be`, `0ea3f4b8`. This work can land independently of the Terraform changes.

- In `cmd/aws/cloudformation/init_prompt.go`, display the suggested name and stack as defaults. Typing begins a replacement value; pressing Enter on an empty field accepts the suggestion. Validate the resolved value. Exercise the normal terminal renderer as well as accessible mode.
- Validate explicitly supplied template files before manifest writes using the same top-level template recognition as discovery. Preserve supported YAML tags, JSON, and explicit paths outside the scan limits.
- When `init --apply` needs confirmation in a noninteractive session, report which registrations were saved and print the supported standalone `atmos aws cfn apply <name> -s <stack> --auto-approve` command for each requested name. Keep apply approval explicit. Test cancellation and the real binary's CI and interactive flags.

**Accept when** typing the guide's literal `orders-blue, orders-green` and pressing Enter for `dev` makes both documented renders work. Typing `prod` creates `stacks/prod.yaml`. An unrelated YAML file produces an actionable error before writing. Repeat the two-queue local-emulator test and update just one queue.

## 5. Put the working path in front of newcomers

**Field tasks** `328c5d63`, `a9006d1a`, `60bc061b`. Land the public examples with the behavior they demonstrate.

- Add **Use an existing project** beside the current next steps on the installation and Get Started pages. Add `stack add` to the stack reference's Usage and Subcommands sections. Give `--help=usage` one interactive example and one fully specified example, with a tool-neutral documentation link.
- After `--dry-run`, print the command to **save** those exact choices, including ordered flags. After saving, print the command to **run** the component. Resolve a retried path through its registered instances; use the unique match or prompt for a stack when several match. In a script, show the matching names and an explicit `-s` command.
- In the Terraform quick start, show where existing varfiles enter the first plan and registration. Show one root deployed twice with two distinct native state targets, and label the Atmos stack separately from the native workspace. Add a copyable inbound Atlantis example with one importable project and one custom-workflow project whose unsupported steps appear in inventory.

**Accept when** someone following the rendered homepage through either primary entry point can reach the existing-project guide, copy the displayed preview → save → run commands, and reach the expected native result. Exercise that sequence with paths containing spaces and the inbound Atlantis example.

## Merge criteria

1. Run the PTY and noninteractive matrix against the built CLI in fresh repositories. Assert prompt text, supplied keystrokes, exit status, file side effects and the result of the printed next command.
2. Repeat the successful field cases for Terraform, OpenTofu, Terragrunt, Atlantis, Helm, Packer and CloudFormation. Compare state IDs, rendered resources or local artifacts as appropriate. Retain preview, cancellation, identical-retry and conflicting-write coverage.
3. Require at least 90% coverage of changed Go statements and run the race tests for the affected packages. Re-run the rendered-doc walkthrough and the no-docs terminal journey against the final branch head.
