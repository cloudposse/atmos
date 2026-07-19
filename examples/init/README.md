---
title: Init
tags: [DX]
description: >-
  Bootstrap a brand-new Atmos project from a built-in template in one
  command — no existing atmos.yaml required.
cast:
  file: /casts/examples/init/init-basic.cast
  title: atmos init
---

# Example: Init

Bootstrap a brand-new Atmos project from a built-in template.

Learn more in the [Init Command Documentation](https://atmos.tools/cli/commands/init).

## What You'll See

- Interactive and non-interactive `atmos init` usage
- Generating `atmos.yaml`, stacks, and components from the `basic` template
- Provisioning the generated project for real — `atmos terraform apply` on
  the `greeting` component, a local-only resource that needs no cloud
  account or emulator
- Where to find the fuller catalog templates (`aws/app`, `aws/landing-zone`,
  `gcp/landing-zone`, `azure/landing-zone`)

## Try It

```shell
# Interactive mode: prompts for a template and target directory
atmos init

# Non-interactive: generate the minimal "basic" template into ./my-project
atmos init basic ./my-project --set project_name=my-project

# See every available template, including remote and atmos.yaml-defined ones
atmos scaffold list
```

## Key Templates

| Template | Purpose |
|----------|---------|
| `basic` | Minimal, cloud-agnostic layout — `atmos.yaml`, one stack, and a real local `greeting` component (no cloud account needed) |
| `simple` | A slightly fuller starter project |
| `atmos` | Convention-following full project skeleton |
| `aws/app` | Application SDLC repository for AWS (see `examples/scaffolds/aws/app`) |
| `aws/landing-zone` | AWS landing zone environments (see `examples/scaffolds/aws/landing-zone`) |
| `gcp/landing-zone` | GCP landing zone environments (see `examples/scaffolds/gcp/landing-zone`) |
| `azure/landing-zone` | Azure landing zone environments (see `examples/scaffolds/azure/landing-zone`) |

## How `init` relates to `scaffold`

`atmos init` is a thin, project-scoped specialization of the generic
`atmos scaffold` code-generation engine — it always targets a whole new
project directory and is meant to run once. For generating individual
components, configs, or any other repeatable boilerplate inside an existing
project, see [`examples/scaffolding`](../scaffolding) and
`atmos scaffold generate`.

## Learn More

- [Init Command Documentation](https://atmos.tools/cli/commands/init)
- [Scaffold Command Documentation](https://atmos.tools/cli/commands/scaffold/generate)
