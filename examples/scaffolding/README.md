# Example: Scaffold Templates

Scaffold templates for generating new Atmos projects and components.

Learn more in the [Scaffold Command Documentation](https://atmos.tools/cli/commands/scaffold/generate).

## What You'll See

- Scaffold template configuration with `scaffold.yaml`
- Interactive prompts for customizing generated projects
- Template files with Go templating support

## Try It

```shell
# List available scaffold templates
atmos scaffold list

# Generate a new project from a template
atmos scaffold generate simple ./my-project

# Generate with custom values
atmos scaffold generate simple ./my-project --vars project_name=my-app
```

## Key Files

| File | Purpose |
|------|---------|
| `scaffold.yaml` | Template configuration with prompts and metadata |
| `atmos.yaml` | Template for generated Atmos configuration |

## Creating Custom Templates

Scaffold templates use Go templates with access to:
- `.Config` - Values from prompts and `--vars` flags
- Sprig functions for string manipulation
- Gomplate functions for advanced templating

See the [Scaffold Templates Guide](https://atmos.tools/core-concepts/scaffold-templates) for more details.
