# Landing-page casts

The homepage plays committed native Atmos asciicasts using the same recorder and
CastPlayer as the examples. Its terminal window styling is preserved by DemoCast.

All landing fixtures live in `demo/landing/fixtures/`. Each generation command
copies its fixture into `.context/cast-fixtures/landing/<name>` using a `workdir`
step before running real commands. Recordings never execute against or copy from
`examples/`, and never modify the source fixture.

From the repository root:

```sh
cd demo/casts
atmos casts generate demo landing hero
atmos casts validate demo landing hero
atmos casts generate demo landing
atmos casts validate demo landing
```

The commands build this branch's Atmos into `.context/bin` automatically. Outputs
are committed at `website/static/casts/demo/landing/<name>.cast`. They travel with
the website deployment and PR previews; no separate media publishing is needed.
The manual/label-gated `landing-demos` workflow generates and validates all nine
casts and makes them downloadable as an artifact for review and commit.

The command definitions live under `demo/casts/atmos.d/demo/landing/`. Shared
terminal dimensions and environment settings live in `demo/casts/landing-defaults.yaml`.
Use `mode: steps` for real commands with typed introductions, and nested `session`
steps for interactive prompts. Session waits match actual output; command failures
abort the recording. Validation rejects error output, machine paths, unmasked demo
secrets, and missing scenario-specific results.

## Prerequisites

- Go, Python 3, Git, and Terraform on PATH.
- Docker or Podman for Kubernetes, emulators, and DX.
- Internet access for container images and Terraform providers.

No VHS, ttyd, FFmpeg, or recording fonts are required.

| Cast | Demonstration |
| --- | --- |
| `hero` | Discover environments and workload types, get/set configuration, inspect affected components, then preview the change with `--ui`. |
| `how-it-works` | Discover mixed workloads and plan stack-resolved configuration. |
| `terraform` | Inspect component instances and their dependency graph. |
| `kubernetes` | Deploy a namespace with native Kubernetes and a ready Helm application on k3s. |
| `emulators` | Start local cloud emulators and provision an AWS resource. |
| `local-ci` | Read/set/read a stack value, inspect the diff, and apply only affected instances. |
| `secrets` | Initialize, rotate, mask, and inject declared secrets. |
| `dx` | Select a component and stack interactively and deploy it. |
| `extensibility` | Start the MCP server and show readiness for an agent. |

Keep every recording focused: prefer tables, getters, concise diffs, and results.
Avoid full YAML dumps; any YAML shown must be a small, clean snippet explaining
one action.

Review every changed cast in the homepage at desktop and mobile sizes. The player
pauses offscreen, supports manual pause/play, and respects reduced motion. Commit
both the scenario definition and its generated cast; no videos or posters are needed.

The hero tells the platform story before showing the UI: discover stacks and mixed
workloads, read and change one component's configuration, identify the affected
instance, then preview the change with a brief `--ui` plan. Its cloud-free
`terraform_data` resource is seeded before recording so the plan shows the real
replica change from 2 to 3. The 64-column recording keeps text readable in the
homepage frame; screen clears separate the short chapters without YAML dumps.
The recorder unsets CI detection only for the local UI command. The Local = CI
cast keeps `atmos list affected` and `atmos terraform apply --affected --ci`
without `--ui`.
