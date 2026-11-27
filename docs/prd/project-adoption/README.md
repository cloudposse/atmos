# Adopting existing infrastructure projects

**Status** — Proposed shared CLI contract. Each source has its own release criteria. This branch implements the CloudFormation initializer, single-root Terraform/OpenTofu registration with ordered variable files, single-unit Terragrunt adoption, local Helm and Packer registration, and explicit Atlantis v3 project inventory and built-in plan import.

A developer should be able to point Atmos at a deployment they already run, recognize its output, and save the configuration for a teammate. At the November 18 Office Hours, one Rain user wanted to run the same template under several names. Another had tried Terraform and built something smaller than the minimal tutorial to understand what Atmos added. The first-run guide for each source should start with files the developer owns and end with a command they can repeat. [Office Hours recording](https://www.youtube.com/watch?v=UfLGcAM4N5g)

| Source | First useful result | PRD |
| --- | --- | --- |
| Terraform / OpenTofu | Reproduce the plan for an existing root and its variable files | [Terraform](terraform.md) |
| Terragrunt | Discover existing units and plan one with its current execution behavior | [Terragrunt](terragrunt.md) |
| Atlantis | Select an existing project and determine which workflow stages Atmos can reproduce | [Atlantis](atlantis.md) |
| AWS CloudFormation / Rain | Register one template under N AWS stack names | [CloudFormation](cloudformation.md) |
| Helm | Render an existing chart with its release name and ordered values files | [Helm](helm.md) |
| Packer | Validate and build an existing template with its original inputs | [Packer](packer.md) |

## CLI contract

```shell
atmos terraform plan ./infra/network
atmos stack add ./infra/network -s dev --dry-run
atmos stack add ./infra/network -s dev
atmos terraform plan network -s dev
```

These commands work for a single local-state Terraform or OpenTofu root with native defaults and auto-loaded files. The broader contract below describes the remaining adapters and cases. A trial computes an in-memory deployment description. A native command can still produce its usual local artifacts. Saving materializes the same description as ordinary Atmos configuration. The two operations must agree on the source, working directory, inputs, execution tool and deployment target. Keep native Terraform and Packer initialization semantics.

- **Selection.** The positional path identifies a source. Show candidates when it contains multiple deployable roots or points at a deployment inventory. An explicit tool or project selection resolves ambiguity. Display where each consequential inferred choice came from.
- **Interaction.** Explicit flags win. Infer unambiguous values, then ask only for remaining required inputs. Selecting an Atlantis project fills all values that project supplies. Give every answer a flag equivalent and show a replayable command with secret references in place of values. Use existing conditional prompts and stack-create TTY conventions. With CI, a non-TTY, or `--interactive=false`, report unresolved choices and the flags that resolve them. Cancellation before save leaves project files untouched. [Interactive prompt PRD](../terraform-interactive-prompts.md)
- **Naming.** Use an existing Atmos stack or a filename-derived name in a new project. A preview may suggest `dev` for a new sample. An Atmos stack name alone supplies no evidence about Terraform state, a Kubernetes namespace, AWS account or region.
- **Persistence.** Preview relative paths, the proposed manifest diff, source evidence, and any unresolved work. Reuse stack creation, YAML editing, and the validation and preview facilities behind doctor where applicable. Existing `stack create` creates disabled stubs in new manifests. Adoption needs a runnable entry and must also support adding one to an existing manifest. Resolve configured paths, preserve unrelated entries and comments, and identify inherited-name conflicts before editing a shared catalog.
- **Retries and collisions.** An identical import produces no diff. Conflicting definitions identify both sources. Recheck destinations before writing, serialize concurrent additions, and stage a selected batch so cancellation or a failed write can be retried without losing existing configuration. Import and preview never deploy. An explicitly requested execution runs after registration succeeds.
- **First-run documentation.** For each source, show the familiar command, the corresponding Atmos command, the files added, one recognizable result and how to address a second deployment. Use a plan, render, validation result or build output as evidence. Keep `describe component` for readers who want to inspect the underlying representation.

## Working with the saved stack

In a new project, create one `atmos.yaml` to locate component sources and stack manifests. For a named stack, propose `stacks/dev.yaml` and derive `dev` from the filename. In an established project, show the actual manifest selected by its existing configuration. If several manifests could own the new entry, prompt for the destination or require it explicitly in automation.

After saving, `atmos stack instances -s dev` should list each runnable component the user added. Use the source-specific command in the corresponding PRD to act on one instance. Adding Helm to a stack that already contains Terraform extends the same manifest under `components.helm`. Adding another deployment to a different environment creates or extends that environment's manifest. A second rollout in the *same* environment gets its own component address. The preview names both the Atmos address and the native target before writing.

Test the complete saved experience from a fresh checkout: register a first instance, extend the stack with a second, select each by name, and repeat the registration with an empty diff. For Terragrunt tree discovery, verify that the same inventory is available from the plugin configuration alone. For Atlantis, check that unresolved inventory-only projects stay out of runnable stack selections.

## Delivery and evidence

| Source | Smallest useful release | Main dependency or stop condition | Evidence available today |
| --- | --- | --- | --- |
| [CloudFormation](cloudformation.md#what-can-ship-first) | Register one plain template under N names in dev | Required parameters, Rain preprocessing and physical-name collisions can prevent deployment | Working initializer, two addressable components and a verified render, emulator deployment |
| [Terraform and OpenTofu](terraform.md#what-can-ship-first) | Plan one selected root, save it with its verified existing workspace and native auto-files | Implemented for one local-state root in a standalone manifest. Explicit files retain source order and numeric precision | The importer reproduced prior resource IDs and no-op actions in two local workspaces per engine. Native and named plans matched with ordered HCL and JSON inputs |
| [Helm](helm.md#what-can-ship-first) | Register and render one unpacked chart with a release, namespace and ordinary local values files | Shared writer and native overlay/resource-set checks | Helm 4 and Atmos rendered matching manifests with multi-document values files in either order |
| [Packer](packer.md#what-can-ship-first) | Register and validate one HCL2 build target with defaults or auto-files | One build source per selected template or directory | Local Docker example built. Native and named Packer builds produced matching marker contents with ordered inputs in two stacks |
| [Terragrunt](terragrunt.md#what-can-ship-first) | Import one explicit unit into a named stack using the existing plugin | Preserve the project-wide execution mode and verify the native state target with the pinned executable | Focused plugin discovery and execution tests passed. Single-unit registration and native execution are covered by the Terragrunt adoption regression |
| [Atlantis](atlantis.md#what-can-ship-first) | Inventory explicit v3 projects and explain each project's plan readiness | Runnable translation depends on effective workflow and a verified source adapter | Upstream fields mapped. The demo generated one bound project and applied its reviewed plan after inputs changed. Inbound registration reproduced a native plan against populated state |

**Delivery order.** Release the CloudFormation initializer on its own. Build the first shared writer around a single Terraform root and a plain local manifest. Start with one writable destination, a filename-derived name and an explicit component directory. Exercise adding a second component and a second stack before generalizing discovery. Adopt Helm and Packer independently once the writer works. Terragrunt can reuse explicit plugin imports. Atlantis inventory can ship independently, while its runnable import follows the relevant source adapter.

**Cost drivers.** File-marker detection and emitting existing component fields are bounded CLI work. Selecting the writable owner in an inherited stack, preserving effective state, and recovering a multi-file save require integration with the existing engine. Ordered `vars` requires reader, schema and merge changes shared by Terraform, Atlantis and Packer. Whole-repository inference, CI behavior migration and cross-tool remote execution each need their own compatibility evidence. There is no measured basis here for promising a percentage of repositories supported or a delivery date.

**Release check.** Start with a checkout containing only the developer’s existing project. Compare the native and Atmos result for that adapter, preview the diff, save, and repeat from a second checkout. Add a second address and prove which native deployment it selects. Exercise an idempotent retry, a conflicting destination and one unresolved input in CI. In a first-run tutorial, the user should be able to name the Atmos stack, identify the native target, and repeat the operation without editing YAML. Each source PRD distinguishes the bounded first release from later acceptance cases.
