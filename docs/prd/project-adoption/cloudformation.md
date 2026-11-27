# Adopt an existing AWS CloudFormation template

**Status** — The source-specific initializer is implemented on this branch. Integration with a common stack writer is proposed. Builds on the [CloudFormation component](../aws-cloudformation-component.md) and [shared adoption contract](README.md).

## Outcome

A Rain or native CloudFormation user with one template can deploy it N times in one AWS account and region. In a terminal, running `atmos aws cfn init` discovers local templates and prompts for the AWS stack names and the destination Atmos stack. Supplied paths and flags skip their corresponding questions. Scripts can pass the template path and use defaults or explicit names. Atmos writes the local configuration without asking for a naming convention.

```shell
atmos aws cfn init templates/queue.yaml --name orders-blue --name orders-green
atmos aws cfn render orders-blue -s dev
atmos aws cfn apply orders-green -s dev
```

## What can ship first

The branch already registers a regular template file under several AWS names, reuses it by path, selects an existing manifest where possible, and reports each apply command. On the queue fixture, `stack instances -s dev` listed `orders-blue` and `orders-green`. Rendering green returned the expected SQS queue. The branch’s emulator exercise covers deployment of two instances. Keep this first tutorial on a plain template with no required external inputs and service-generated physical resource names.

| Case | Current behavior | Release decision |
| --- | --- | --- |
| Local template, N AWS names, one Atmos stack | The initializer prepares the selected entries together, checks name conflicts and locks registration. | Ship this independently of the generic importer. Re-run the same-name retry and later addition to a named stack from a fresh checkout. |
| Arbitrary file, Rain directive or required template parameters | Init checks for a regular file. A dry run accepted a plain-text `queue.txt`. The component loader detects Rain directives when reading the template. Init has no per-instance parameter-file flag. | Distinguish “registered” from “ready to deploy”. For generic detection, inspect the template structure including `Resources`, accept CloudFormation intrinsic tags, and surface directives or missing parameters before suggesting apply. Defer translation of Rain packaging. |
| Two names with the same explicit physical resource name | Distinct AWS stack names still allow a template to request the same bucket or named IAM resource twice. | Use parameterized or service-generated names for the N-instance promise. Render each instance and compare change sets before claiming a particular template can coexist. |
| New project plus new manifest | Starter configuration and manifest use two individual atomic writes. The current dry run lists names and paths. | The shared writer needs a proposed diff and recovery from failure between writes. Test retry, concurrent import and a conflicting pre-existing manifest before reusing it across sources. |

The local fixture establishes naming and rendering. AWS identity, region, required parameters, IAM capabilities and existing deployed state can still affect a live change set. A repository registration alone cannot reconstruct a prior Rain invocation. Treat an existing AWS stack with that name as a potential update and show its target in the existing preview/apply flow. [Initializer](../../../cmd/aws/cloudformation/init.go) · [Manifest writer](../../../cmd/aws/cloudformation/init_manifest.go) · [Template loader](../../../pkg/component/aws/cloudformation/template.go)

## Working with the saved stack

The command above produces this manifest in a new project. The initializer sets the CloudFormation base path to the project root in `atmos.yaml`, so both entries refer to the original `templates/queue.yaml`.

```yaml
# stacks/dev.yaml
components:
  aws/cloudformation:
    orders-blue:
      metadata:
        component: templates
      path: queue.yaml
      stack_name: orders-blue
    orders-green:
      metadata:
        component: templates
      path: queue.yaml
      stack_name: orders-green
```

Run `atmos stack instances -s dev` to see both entries. In `atmos aws cfn apply orders-green -s dev`, `dev` selects this Atmos stack and `orders-green` selects the component targeting the AWS stack of that name. Registering `--name orders-canary` adds a sibling. Registering with `-s prod` selects a different Atmos manifest. To obtain an independent AWS stack in the same account and region, give it a distinct AWS stack name there as well. Verify each named deployment from a fresh checkout.

## Requirements

- In a new directory create ordinary project configuration and one filename-named Atmos stack, defaulting to dev. Keep the original template at its existing path. Honor the layout of an existing project and accept an explicit stack selection.
- Make each AWS name an individually addressable component. Default an omitted AWS name to the template filename stem when valid. An invalid stem requires an explicit valid name. Show the template path, Atmos stack and AWS names together.
- Repeating registration for additional names extends the same manifest. An identical registration is idempotent. Conflicting component definitions fail before any selected entry is committed. Preserve existing per-instance configuration.
- Registration and rendering work locally. Optional `--apply` previews changes and confirms each requested instance. An existing `apply --all -s dev` selects all CloudFormation components in that Atmos stack, including any that predate this registration.
- Use a plain template with service-generated or parameterized resource names in the first exercise. A Rain template with preprocessing directives needs the relevant migration steps. Surface that requirement before promising equivalent deployment. [Rain migration guide](../../../website/docs/migration/rain.mdx)

## Acceptance

Start with one queue template. Register two names, inspect the human-readable result, deploy both to an AWS emulator in the same region, and update one independently. Confirm the template bytes remain identical. Add a third name without hand editing. Test retry, conflict, dry run and cancellation at an apply prompt. Declining deployment leaves the saved configuration available. When sharing the stack writer, rerun this sequence through the existing cfn command.
