# Adopt an existing Terraform or OpenTofu root

**Status** — Single-root path planning, engine selection and local-state stack registration implemented on this branch. Registration saves ordered HCL and JSON variable files. Repository-wide import remains proposed. Uses the [shared adoption contract](README.md).

## Outcome

An engineer who runs Terraform in an existing directory can obtain the same plan through Atmos before creating an Atmos file. A teammate can later run that plan by component and stack name, against the same state.

```shell
# Today's command from infra/network
terraform plan -var-file=vars/common.tfvars -var-file=vars/dev.tfvars

# Proposed trial and save from the repository root
atmos terraform plan ./infra/network -var-file=vars/common.tfvars -var-file=vars/dev.tfvars
atmos stack add ./infra/network -s dev --var-file=vars/common.tfvars --var-file=vars/dev.tfvars
atmos terraform plan network -s dev
```

## Implemented commands

```shell
atmos terraform plan ./infra/network
atmos stack add ./infra/network -s dev --dry-run
atmos stack add ./infra/network -s dev
atmos terraform plan network -s dev
```

A bare project's trial uses the native engine in the selected directory. The save writes a minimal project configuration with Terraform base path `.` and an ordinary component with `metadata.component: infra/network`, `metadata.terraform_workspace` and `command`. A version selected through an exact project pin is also recorded. A named local workspace must already exist. The writer supports a standalone manifest and rejects remote backends or cached local state paths it cannot reproduce. Explicit variable files can be forwarded at plan time. Pass repeated `--var-file` flags on registration to save live references.

## What can ship first

Start with one explicitly selected root, the original working directory, a local backend and an existing workspace. Offer a path-based plan using the selected native binary and its original ordered arguments. A root with only defaults or native auto-loaded files can also be saved as a named component once the writer and state checks below pass. In that case omit `vars` entirely. Explicit variable files are stored as ordered component-level includes.

| Case | What the importer can establish | Release decision |
| --- | --- | --- |
| Explicit directory with `.tf` or `.tf.json` | A configuration directory. A user selection supplies evidence that it is a deployment root. | Offer a plan. Use a tool pin or explicit choice for shared syntax. Include `.tofu` and `.tofu.json` in OpenTofu discovery and respect OpenTofu’s file selection. |
| Native defaults, environment and auto-files | Leaving these to the binary preserves their ordering. Inherited Atmos vars can still override them through a generated var file. | Enable save only after checking the effective component, including inherited variables, backend generation and workspace selection. |
| One or more explicit files | Native argument forwarding exists. An existing single HCL include parses values before the generated JSON var file reaches the binary. | Trial can forward the files. Save ordered include references. File order comes from the invocation or user. |
| Several environments, partial backend or remote execution | A directory or tfvars basename alone cannot identify the state object, account or remote workspace. | Show the candidates and missing evidence. Enable each backend family separately after reproducing an existing nonempty state from a fresh checkout. |

**Original regression.** With Terraform 1.9.8, a provider-free root outputting `var.ratio` and `terraform.workspace` produced `1.75` and `default` with native `-var-file=dev.tfvars`. A named Atmos plan using a single `!include` produced `1` and `default`. Forwarding the original file to the named plan restored `1.75`. All three plans succeeded. The regression was fixed in the HCL reader and the stack decoder. Include fractional and large numbers, nulls, empty collections and nested values in the regression fixture. [HCL reader](../../../pkg/filetype/filetype.go) · [Terraform argument handling](../../../pkg/component/terraform/execute_helpers_args.go)

**New engine work.** Normalize a sequence after resolving its local includes and before stack inheritance or map-only consumers see it. Update manifest validation and diagnostic locations together. Set `tags = { owner = "ops", region = "west" }` in the common file and `tags = { region = "east" }` in dev. Verify that the later assignment removes `owner`, then reverse the files and compare again. Test a trailing native `-var` and a saved-plan apply. For mixed, interleaved `-var` and `-var-file` arguments, offer native trial only until the writer can reproduce the full order without persisting secrets. Existing typed flags can express a subset of defaults. They cannot encode this ordered argument stream. [Instance model](../../../pkg/schema/instance.go) · [Variable-file generation](../../../pkg/component/terraform/generate_varfile.go)

**State evidence.** With hand-authored minimal manifests, Terraform 1.9.8 and OpenTofu 1.10.6 each matched the native prior resource ID, no-op action and workspace output for both `dev → default` and `prod → blue` on local state. The OpenTofu fixture used only a `.tofu` root. The CLI regression now repeats all four comparisons using manifests created by `stack add` and checks that dev and prod have different prior resource IDs. [CLI state parity test](../../../tests/cli_terraform_adoption_test.go)

**State gate.** Seed a local state with a known resource, use Atmos stack `dev` with native workspace `default`, and compare the prior resource and actions in both plan JSON files. Repeat with a second, already-existing workspace containing different state. Missing or contradictory workspace evidence must produce a diagnostic before a plan can select or create another workspace. The current executor has workspace creation and init recovery paths that require explicit handling for adoption. [Workspace execution tests](../../../pkg/component/terraform/execute_helpers_workspace_test.go) · [Init behavior](../../../pkg/component/terraform/execute_init.go)

## Choosing Terraform or OpenTofu

**Named components.** A named `atmos terraform` operation uses the effective component command, which defaults to `terraform`. The existing `--command=tofu` override selects OpenTofu for one invocation. Atmos can classify the selected executable by basename or version output. That helper answers which binary Atmos selected. Adoption also needs to establish which binary this deployment normally runs. Its existing fallback to Terraform on a failed version probe is insufficient evidence for a saved choice. [Executable override](../../../website/docs/cli/commands/terraform/usage.mdx#executable-override) · [Current detector](../../../pkg/component/terraform/detection.go)

**Engine selection for an unregistered root.** Apply the same selection to path-based plan and `stack add`. First honor an explicit `--command`. Next use an explicit effective command for the deployment, such as a selected Atlantis project's `terraform_distribution` or a literal plan invocation tied to this root. An implicit Atmos default supplies no project evidence. When those declarations disagree, show the sources and ask for an explicit choice. Preserve an explicitly configured wrapper by its command.

| Remaining evidence | Proposed behavior |
| --- | --- |
| A single applicable project tool pin, or a root containing only OpenTofu configuration files (`.tofu` / `.tofu.json`) | Select that engine and say why. If independent evidence conflicts, ask. Scope pins to the selected directory and honor a more local configuration. |
| Plain `.tf` / `.tf.json`, tfvars, `required_version` or `.terraform.lock.hcl` | These are shared conventions. Check which engines are available through the applicable toolchain and PATH. |
| One available engine, compatible with a shared-syntax root, and no contrary project evidence | Use it for the trial. Identify availability as the reason and show the choice in the save preview. |
| Both available, with no decisive project evidence | Ask once in a TTY, showing the two resolved binaries and versions. In CI or with prompts disabled, exit before init, plan or writes and print both replay commands using `--command=terraform` and `--command=tofu`. PATH order has no bearing on this choice. |
| A dual-engine root or two tool pins | A `main.tofu` beside `main.tf` can be an intentional OpenTofu alternative. Two pins can support a compatibility matrix. Use a deployment-specific declaration or explicit choice. |
| Selected engine unavailable, or neither engine available | Keep any project-declared selection and version. Show the applicable toolchain resolution or missing executable. When the engine is also unknown, require a choice to save it. |

On save, reuse the existing `command` setting and any project version pin. Store the portable command name when the choice came from availability or the picker. A teammate with both installed then gets the same engine without a second prompt. Restrict a mixed project's new selection to the selected component. [OpenTofu file selection](https://opentofu.org/docs/language/files/)

**Regression fixture.** With both real binaries on PATH and the OpenTofu directory first, a named Atmos plan with no command setting used Terraform 1.9.8 and loaded `main.tf`. Adding `--command=tofu` used OpenTofu 1.10.6 and loaded its same-basename `main.tofu` alternative. The importer was exercised with both binaries installed, an ambiguous path trial, an explicit OpenTofu trial with a var file, and a saved OpenTofu run after reversing PATH. The selector tests cover interactive choice, conflicting declarations and a missing pinned engine. [Engine selection tests](../../../pkg/terraform/adoption/engine_test.go)

## Stack files created by adoption

In a new repository, `stack add` creates a minimal `atmos.yaml` and a manifest for the chosen deployment. The path-based trial creates neither. For the network example above, the resulting project looks like this:

```text
atmos.yaml                 # added once
stacks/dev.yaml            # added for this deployment
infra/network/             # existing Terraform root
  main.tf
  vars/common.tfvars
  vars/dev.tfvars
```

The project configuration points Atmos at the existing root modules and preserves the module's backend declaration:

```yaml
# atmos.yaml
base_path: ./
components:
  terraform:
    base_path: .
    auto_generate_backend_file: false
stacks:
  base_path: stacks
  included_paths:
    - "*.yaml"
```

The generated manifest below assumes the existing deployment uses Terraform workspace `default`. The filename supplies the Atmos stack name. The component key resolves to `infra/network`.

```yaml
# stacks/dev.yaml
components:
  terraform:
    network:
      command: terraform
      metadata:
        component: infra/network
        terraform_workspace: default
      vars:
        - !include ../infra/network/vars/common.tfvars
        - !include ../infra/network/vars/dev.tfvars
```

Component `vars` accepts a sequence of mapping-valued inputs alongside the existing mapping and single-include forms. Resolve each include from the declaring stack file. Walk the sequence from first to last. Within the sequence, each later assignment replaces the complete value of that top-level variable, including objects and lists. Normalize to a single map before ordinary stack inheritance. Validate every entry and report its file and index on failure. Existing mapping-form behavior stays intact.

Keep native automatic file loading and explicit CLI override precedence. Applying a saved plan uses its recorded inputs. If an included source cannot be read with equivalent native semantics, identify it before offering a reproducible saved plan.

OpenTofu uses the same `components.terraform` manifest layout. The writer stores `command: tofu` on the adopted component. A second root in the same project can select Terraform independently.

Adding prod for the same root creates `stacks/prod.yaml` with another `network` entry, its own ordered inputs and its verified state selection. Reuse the original HCL directory. Adding a second root to dev extends the existing dev manifest. In a configured project, resolve the writable manifest using its actual stack layout and logical names before suggesting a new filename. Preview the files and changes together, including any required project configuration edit.

### Using the saved stack

After registration, run `atmos stack instances -s dev` to find `network`, then `atmos terraform plan network -s dev`. For OpenTofu the Atmos command is the same. A teammate can switch to `-s prod` after registering the prod inputs. If dev and prod resolve to the same backend and workspace, show that shared state target in the preview so the user can check whether it is intentional.

## Discovery and execution

1. Keep the existing path-to-component resolver first in an established project. A path already bound to multiple stack instances uses the existing picker or stack selector. An explicitly requested stack that fails to resolve must report that failure. Try unregistered-root discovery for an intentional path when there is no configured match. [Existing path behavior](../../../website/docs/cli/commands/terraform/usage.mdx#path-based-component-resolution)
2. Treat an explicit directory path, including `.`, as a filesystem selection. Preserve existing bare component names, including names containing slashes such as `network/vpc`. Inspect Terraform configuration files in the chosen directory, including `.tf.json`. A valid root can lack `main.tf`, a provider block or a backend block. Check Terragrunt markers before selecting the plain Terraform adapter.
3. Treat a user-selected root as intentional. On a repository-wide scan, directories containing reusable modules, examples and tests are candidates only when there is deployment evidence. Skip caches and downloaded modules. A directory containing only var files needs a root selection.
4. Read project tool settings, an explicit invocation or deployment metadata to identify Terraform versus OpenTofu and its version. Common HCL syntax cannot distinguish them. Preserve the working directory, backend, state location, selected workspace, provider lockfile and execution context. A remote workspace or partial backend can depend on information outside Git. Identify the missing input before offering an equivalent plan. Avoid automatic workspace creation or backend reconfiguration during discovery.
5. Allow the native tool to load `terraform.tfvars`, JSON variants and auto-loaded files in its usual order. Resolve explicit CLI var-file paths from the selected root, then write equivalent manifest-relative include paths in their original order. A later assignment can replace an entire object. Deep-merging files or copying their contents into stack YAML could change the plan or persist secrets. Honor explicit CLI overrides and native environment precedence. [Terraform variable rules](https://developer.hashicorp.com/terraform/language/values/variables)
6. Offer ordered inputs when a declarative project record or literal CI invocation supplies them. When filenames are the only evidence, show candidates and let the user choose and order files. Carry references to backend arguments separately from plan arguments. A stack called dev can use Terraform workspace default.

## Execution flags

Reuse the existing Terraform `flags` defaults for `lock_timeout`, `lock`, `parallelism`, `refresh` and `compact_warnings`. Explicit native CLI flags take precedence. Adopt a default only when its scope matches the source deployment; a flag specific to one CI plan must retain that scope. For trial plans, forward repeated native `-var-file` arguments in order. The declarative `flags` block has no var-file key. For saved inputs use the ordered `vars` syntax above. [Terraform flag defaults](../../../website/docs/cli/configuration/components/terraform.mdx)

## Acceptance

- Single root with local state and auto-loaded variables yields equivalent native, path-based and saved plans. Compare normalized resource addresses, actions, outputs and workspace. Run the same checks for Terraform and OpenTofu.
- Two explicit var files override the same object. Swapping their order changes the result exactly as it does in Terraform. Editing a referenced file affects the next named plan.
- Two environments share a root and use different external backend settings. Detect or request the missing backend input before planning. Preserve backend type, effective state address and workspace through save and retry.
- Directory-per-environment and shared-root repositories both work. A scan offers deployable candidates without silently treating every child module as a deployment. CI reports ambiguity without writing files.
- Starting with only the HCL and var files, inspect the dry-run diff, save dev and prod, and run both by name from a fresh checkout. Verify prod registration leaves the dev entry intact, both use the intended native workspace and state, and repeat registration produces an empty diff.
- Existing name-based commands and `atmos terraform init` retain their current interpretation.

Two upstream layouts give different acceptance cases. Google's development, nonproduction and production directories each have their own backend declaration and auto-loaded `terraform.tfvars`. Discover three candidates and keep each working directory intact. The AWS GitLab pipeline generates a backend file, selects an environment workspace and computes the var-file path. Discovery can identify the root there, but requires the resolved inputs before saving a reproducible plan. [Google environment roots](https://github.com/terraform-google-modules/terraform-example-foundation/tree/39a543f8fec0c769b74e87849102f89960eff259/2-environments/envs) · [AWS pipeline](https://github.com/aws-samples/sample-terraform-gitlab-infra-orchestrator/blob/a224359b83932a967e8570ef4a0826046ed6b6be/.gitlab-ci.yml)
