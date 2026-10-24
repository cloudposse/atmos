---
title: Debugging
sidebar_position: 4
sidebar_label: Debugging
description: Trace Atmos configuration, imports, templates, and functions before troubleshooting a component command.
---

Start by inspecting the configuration Atmos resolves for the failing component and stack. This separates configuration problems from errors returned by Terraform, CloudFormation, or another underlying tool.

## Inspect Effective CLI Configuration

Run from the same directory, with the same profile and environment, as the failing command:

```shell
atmos version
atmos describe config --format yaml
```

[`describe config`](/cli/commands/describe/config) shows the merged CLI configuration. Check `base_path`, component and stack paths, included/excluded stack paths, and the selected [profile](/cli/configuration/profiles). A CLI configuration file (`atmos.yaml`) and a stack manifest serve different purposes; inspect each separately.

## Trace a Component Value

```shell
atmos describe component vpc -s dev --provenance
```

[`describe component`](/cli/commands/describe/component) shows the resolved component configuration. Provenance identifies the source of values so you can find which imported defaults or component overrides contributed to the result. Check `vars`, `settings`, `metadata`, and, for Terraform, `backend` and `workspace`.

Output inspection can evaluate functions that read state, contact services, or run commands such as `!exec`. When investigating parsing or configuration without resolving those functions, disable processing as shown below. Review output before sharing it: ordinary variables and store values may contain sensitive information.

## Check Imports and Merging

An `_defaults.yaml` file is a naming convention, not an automatic parent-directory inheritance rule. Follow the actual [imports](/stacks/imports) and [component inheritance](/stacks/components/component-metadata) in the selected stack.

By default, maps merge and lists replace. For example:

```yaml
# Imported defaults
vars:
  tags:
    owner: platform
    environment: dev
  zones: [a, b]
```

```yaml
# Values in the importing stack
vars:
  tags:
    environment: production
  zones: [c]
```

The result retains `tags.owner: platform`, sets `tags.environment: production`, and replaces `zones` with `[c]`. An empty map does not clear inherited map keys. Check [`settings.list_merge_strategy`](/cli/configuration/settings) if the project changes the default list behavior, and use provenance to verify the actual result. [Locals](/stacks/locals) are file-scoped and do not inherit across imports like `vars`.

## Isolate Template and Function Evaluation

Compare the resolved component with each processing stage disabled:

```shell
atmos describe component vpc -s dev --process-templates=false
atmos describe component vpc -s dev --process-functions=false
atmos describe component vpc -s dev --process-templates=false --process-functions=false
```

These flags are diagnostic controls for the describe command, not a way to make an invalid deployment valid. Inspect template inputs, YAML quoting, and the referenced component and stack names. A fallback in a value expression is not a guarantee that a missing stack or failed authentication will be ignored. For deployed-output lookups, confirm the producer exists and the reader has access; see [sharing data and outputs](/stacks/sharing-state).

## Validate and Narrow the Failure

```shell
atmos validate stacks
```

[Stack validation](/cli/commands/validate/stacks) checks configuration; it does not prove that remote resources, credentials, or outputs are available. If configuration resolves but the underlying tool fails, inspect that tool's diagnostic and the [common errors guide](/troubleshoot/errors). Use the [profiling guide](/troubleshoot/profiling) for slow operations.

## Prepare a Minimal Reproduction

Include the Atmos and underlying tool versions, exact command, working directory relative to the repository, selected profile/identity names, error text, and the smallest relevant `atmos.yaml` and stack manifests. Keep the imports and dependency relationships that trigger the problem. State the expected result and whether it happens locally, in CI, or both.

Remove credentials, private state, and organization-specific values before sharing. Preserve the structure and value types so others can reproduce the configuration behavior without access to your infrastructure.
