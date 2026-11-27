# Adopt an existing Packer build

**Status** — Proposed own-project import. The [existing Docker builder example](../../../examples/packer-docker/README.md) supplies a runnable sample. Uses the [shared adoption contract](README.md).

## Outcome

A developer can register the Packer template they build today, validate it, and build the same artifact through Atmos. Packer's existing init command continues to install the plugins for the selected build.

```shell
# Proposed registration from the project root
atmos stack add ./images/alpine -s dev --var-file=inputs/dev.pkrvars.hcl

# Existing Packer operations on the registered component
atmos packer init alpine -s dev
atmos packer validate alpine -s dev
atmos packer build alpine -s dev
```

## What can ship first

Register one explicitly selected HCL2 build directory, or one explicitly selected HCL2 template file. Start with defaults or auto-loaded Packer inputs and a single intended build. Save a filename-derived Atmos stack without injecting `vars.stage`. In the repository example, Packer declares a `stage` variable specifically because Atmos passes inherited stack vars into the generated file. Adopting someone’s template should work without adding such a declaration. In a configured project, report any effective Atmos input the target does not declare. [Docker build fixture](../../../examples/packer-docker)

| Case | First delivery | Proof or dependency |
| --- | --- | --- |
| One selected HCL2 build | Reuse the component working directory, `settings.packer.template` for a file and Packer’s own plugin init. | Compare native and named validation on a split directory and a selected file. The directory must load companion variable and source declarations. |
| Explicit `.pkrvars.hcl` or `.pkrvars.json` | Preview the ordered paths. Enable saving once each format passes value and precedence tests. | Packer 1.14.2 validated a template requiring `ratio == 1.75` with the original HCL var file. Named Atmos validation failed that rule with a single include. Reuse the common lossless include fix, then test Packer’s auto-files, `PKR_VAR_*` and explicit overrides independently. |
| Multiple builds or `-only` / `-except` | Show the selected target if explicit. Keep a filtered invocation unresolved until the filter survives registration and a fresh named build. | A saved component that runs all builders can publish extra artifacts. Compare the selected build names as well as the resulting marker content. |
| Legacy JSON templates, wrappers and credentialed cloud builders | Inventory explicit paths. Defer an equivalence claim for these cases. | Legacy JSON uses another template dialect. Arbitrary JSON and `.pkrvars.json` are insufficient evidence of a build. Require a fixture for each supported dialect and invocation. |

The current executor chooses the template, writes a JSON var file and places it before additional CLI arguments. This gives a concrete place to preserve native overrides. Check whether concurrently validating the same instance can share a generated filename and exercise cleanup on error before supporting concurrent batch builds. An earlier run of the checked-in Docker example completed a local image build. The independent gate for adoption is validating an unmodified user template and reproducing its artifact content by the new saved address. [Packer executor](../../../internal/exec/packer.go) · [Native variable precedence](https://developer.hashicorp.com/packer/docs/templates/hcl_templates/variables)

## Working with the saved stack

For the build directory `images/alpine`, a new project's `atmos.yaml` sets `components.packer.base_path` to `images` and includes the stack manifests under `stacks/`. The target output of the proposed registration is:

```yaml
# stacks/dev.yaml
components:
  packer:
    alpine:
      vars:
        - !include ../images/alpine/inputs/dev.pkrvars.hcl
```

Use the [proposed ordered vars syntax](terraform.md). Resolve CLI file paths from the build directory and generated include paths from the stack manifest. Verify that the include reader preserves Packer variable-file values and precedence for each supported format. Report unsupported expressions or formats before saving a runnable entry. Keep native auto-loaded files implicit. For single-file builds, the existing `settings.packer.template` selects the template.

Run `atmos stack instances -s dev`, then `atmos packer validate alpine -s dev`. Register prod against the same source to create or extend `stacks/prod.yaml` with its own input references. Register a second build in dev as a sibling component referencing the original build directory through `metadata.component`. Preserve any selected builder filter and verify it against the native invocation before offering the named build. The preview shows the build target and any known artifact destination so the user can see when two entries publish to the same place.

## Requirements

- Recognize Packer HCL templates and Packer JSON configuration in the selected path. Preserve whether the native invocation selects a single file or a directory. A variable file alone supplies no build target. For ambiguous directories, show candidates before registration.
- Preserve the template's required plugins, sources, builders, version constraints, native variable loading, and the order of explicit var-file arguments. In the proposed directory example the var-file path is relative to the selected build directory. Retain file references so later edits feed the next build.
- Carry forward source or build filters when the existing invocation selects one of several builds. If a wrapper supplies unresolved variables, credentials, or filters, identify them before offering an equivalent build. Keep sensitive values out of generated YAML and previews.
- Registration writes project configuration locally. Validation and plugin initialization are distinct from a build, which can create images and use cloud credentials. Display the next command and what it will do. Use a local container build for the first exercise.

## Acceptance

Starting with a Docker-builder template, register and validate the component. Build with a supplied var file and verify the expected provisioned file or image content against the native build. Image digests may vary between equivalent builds. Repeat after changing the var file and after selecting one build from a multi-build fixture. Verify init still installs plugins, registration performs no build, and rerunning registration produces no diff. Clearly report the missing prerequisite when the local container runtime is unavailable.
