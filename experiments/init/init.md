# Atmos Init Command

The `atmos init` command initializes configurations and examples for Atmos scaffold templates.

## Usage

```bash
atmos init [configuration] [target path]
```

## Examples

### Interactive Mode
Initialize a scaffold template with an interactive menu to select configuration and target path:

```bash
atmos init
```

### Specific Configurations
Initialize specific configuration types:

```bash
# Initialize a typical scaffold template for atmos
atmos init default

# Initialize an 'atmos.yaml' CLI configuration file
atmos init atmos.yaml

# Initialize the atmos.yaml in the ./ location
atmos init atmos.yaml

# Initialize the atmos.yaml as /tmp/atmos.yaml
atmos init atmos.yaml /tmp/atmos.yaml
```

### Demo and Example Scaffold Templates
Initialize demo scaffold templates and examples:

```bash
# Initialize the Localstack demo
atmos init examples/demo-localstack

# Or, simply install it into the current path
atmos init examples/demo-localstack ./demo
```

## Flags

### File Management
- `--force, -f`: Force overwrite existing files
- `--update, -u`: Attempt 3-way merge for existing files

### Template Values
- `--values, -V`: Set template values (format: key=value, can be specified multiple times)
- `--use-defaults`: Use default values without prompting

### Merge Configuration
- `--threshold`: Percentage threshold for 3-way merge (0-100, 0 = use default 50%)

## Advanced Examples

### Force Overwrite Existing Files
```bash
atmos init default --force
```

### Update Existing Files with 3-way Merge
```bash
atmos init default --update
```

### Set Template Values via Command Line
```bash
atmos init rich-project --values author=John --values year=2024 --values license=MIT
```

### Set Template Values and Skip Prompts
```bash
atmos init rich-project --values name=my-project --values cloud_provider=aws --values enable_monitoring=true
```

### Use Default Values Without Prompting
```bash
atmos init rich-project --use-defaults
```

### Custom Merge Threshold
```bash
atmos init default --update --threshold 75
```

## Available Configurations

The `init` command supports various configuration types:

- **default**: Basic Atmos scaffold template setup
- **rich-project**: Comprehensive scaffold template with monitoring, CI/CD, etc.
- **atmos.yaml**: Atmos CLI configuration file
- **.editorconfig**: Editor configuration file
- **.gitignore**: Git ignore file
- **examples/demo-***: Various demo scaffold templates (LocalStack, Helmfile, etc.)

## Template Delimiters

Scaffold templates support custom delimiters to avoid conflicts with Atmos's own Go template syntax. By default, templates use `{{` and `}}` delimiters, but you can specify custom delimiters in the `scaffold.yaml` configuration:

```yaml
name: "My Template"
description: "A template with custom delimiters"
template_id: "my-template"
delimiters: ["[[", "]]"]
fields:
  project_name:
    type: string
    label: "Project Name"
    default: "my-project"
```

This allows you to use different delimiters in your template files:

```markdown
# [[ .Config.project_name ]]

This project was created by [[ .Config.author ]] in [[ .Config.year ]].
```

**Note**: If no delimiters are specified, the default `{{` and `}}` delimiters are used.

## Interactive Features

When run without arguments, the command provides an interactive menu that:

1. **Configuration Selection**: Choose from available scaffold templates and configurations
2. **Target Path**: Specify where files should be created with smart defaults
3. **Template Values**: Prompt for scaffold template-specific values (author, project name, etc.)

The interactive mode makes it easy to get started with Atmos without memorizing configuration names or paths.
