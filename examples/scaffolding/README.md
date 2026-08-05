# Example: Scaffold Templates

Scaffold templates for generating new Atmos projects and components.

Learn more in the [Scaffold Command Documentation](https://atmos.tools/cli/commands/scaffold/generate).

## What You'll See

- Scaffold template configuration with `scaffold.yaml`
- Interactive prompts for customizing generated projects
- Template files with Go templating support
- **Conditional prompts** — `vendor_version` is only asked `when:`
  `enable_vendoring` was answered `true`
- **Conditional file generation** — `vendor.yaml` is only generated `when:`
  `enable_vendoring` is `true`, and is skipped entirely otherwise
- **Dynamic file generation (`matrix`)** — one `stacks/<env>.yaml` file is
  generated per environment selected in the `environments` answer (e.g.
  selecting `dev` and `staging` generates `stacks/dev.yaml` and
  `stacks/staging.yaml`); selecting none (the default) generates none

## Try It

```shell
# List available scaffold templates
atmos scaffold list

# Generate a new project from a template
atmos scaffold generate example ./my-project

# Generate with a pinned component version
atmos scaffold generate example ./my-project --set project_name=my-app --set enable_vendoring=true --set vendor_version=1.536.0

# Skip vendoring entirely — vendor_version is never asked and vendor.yaml is never generated
atmos scaffold generate example ./my-project --set enable_vendoring=false

# Generate one stacks/<env>.yaml file per environment via matrix
atmos scaffold generate example ./my-project --defaults --set project_name=my-app --set environments=dev,staging
```

## Key Files

| File | Purpose |
|------|---------|
| `scaffold.yaml` | Template configuration with prompts, conditional `when:` rules, `matrix` file expansion, and metadata |
| `atmos.yaml` | Template for generated Atmos configuration |
| `vendor.yaml` | Conditionally-generated vendor manifest (only when `enable_vendoring: true`) |
| `environment.yaml` | Discovered template expanded by `matrix` into one `stacks/<env>.yaml` per selected environment |

## Learn More: Dynamic File Generation (`matrix`)

`spec.files[].matrix` expands a single discovered template file into one
generated file per resolved combination, reusing the same axis shape (a map
of axis name to a list of values) the workflow `matrix:` step uses. Each
axis's values can be a literal list declared right in `scaffold.yaml`, or a
dot-path into `answers.*` for a dynamic source, like `environments` here.
`target:` (a Go template overriding the discovered `path:`) and the file's
own content both see the resolved combination as `.matrix.<axis>`, and an
entry's `when:` can prune specific combinations via the `matrix` CEL
variable — for example, restricting a `region` axis to only the regions a
given `environment` actually uses:

```yaml
spec:
  files:
    - path: deploy.yaml
      target: "deploy/{{"{{"}} .matrix.environment {{"}}"}}/{{"{{"}} .matrix.region {{"}}"}}.yaml"
      matrix:
        environment: [dev, staging, production]
        region: [us-east-1, us-west-2]
      when: "matrix.region in answers.regions_by_env[matrix.environment]"
```

See the [Scaffold Command Documentation](https://atmos.tools/cli/commands/scaffold/generate)
for the full `matrix` design.

## Learn More: Generation Hooks

Templates can also declare hooks that run automatically before or after
generation — for example, formatting generated files or running a linter.
Hooks reuse the same `when:` condition engine as Atmos workflows and CI
hooks, and support `--skip-hooks` to opt out per invocation. This example
doesn't wire one up, but the syntax looks like:

```yaml
hooks:
  format:
    events:
      - after.scaffold.generate
    kind: step
    type: shell
    with:
      command: "terraform fmt"
```

## Creating Custom Templates

Scaffold templates use Go templates with access to:
- `.Config` - Values from prompts and `--vars` flags
- Sprig functions for string manipulation
- Gomplate functions for advanced templating

See the [Scaffold Templates Guide](https://atmos.tools/core-concepts/scaffold-templates) for more details.
