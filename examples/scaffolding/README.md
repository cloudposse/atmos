# Example: Scaffold Templates

Scaffold templates for generating new Atmos projects and components.

Learn more in the [Scaffold Command Documentation](https://atmos.tools/cli/commands/scaffold/generate).

## What You'll See

- Scaffold template configuration with `scaffold.yaml`
- Interactive prompts for customizing generated projects
- Template files with Go templating support
- **Conditional prompts** — `vendor_source` is only asked `when:`
  `enable_vendoring` was answered `true`
- **Conditional file generation** — `vendor.yaml` is only generated `when:`
  `enable_vendoring` is `true`, and is skipped entirely otherwise

## Try It

```shell
# List available scaffold templates
atmos scaffold list

# Generate a new project from a template
atmos scaffold generate example ./my-project

# Generate with custom values, including the conditional vendor_source prompt
atmos scaffold generate example ./my-project --set project_name=my-app --set enable_vendoring=true --set vendor_source=github.com/acme/terraform-modules.git

# Skip vendoring entirely — vendor_source is never asked and vendor.yaml is never generated
atmos scaffold generate example ./my-project --set enable_vendoring=false
```

## Key Files

| File | Purpose |
|------|---------|
| `scaffold.yaml` | Template configuration with prompts, conditional `when:` rules, and metadata |
| `atmos.yaml` | Template for generated Atmos configuration |
| `vendor.yaml` | Conditionally-generated vendor manifest (only when `enable_vendoring: true`) |

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
