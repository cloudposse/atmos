# Atmos Scaffold Command

The `atmos scaffold` command manages scaffold template scaffolding from templates defined in your `atmos.yaml` configuration.

## Usage

```bash
atmos scaffold <subcommand> [options]
```

## Subcommands

### `list`
List all available scaffold templates configured in your `atmos.yaml`:

```bash
atmos scaffold list
```

This command reads the `scaffold.templates` section from your `atmos.yaml` file and displays them in a formatted table showing:
- Template name
- Source repository
- Version/ref
- Description

### `generate`
Generate a scaffold template from a template:

```bash
atmos scaffold generate [template] [target-directory]
```

## Examples

### Interactive Mode
Generate a scaffold template with an interactive menu to select template and target directory:

```bash
atmos scaffold generate
```

### Specific Template Generation
Generate a scaffold template from a specific template:

```bash
# Generate using a template from atmos.yaml config
atmos scaffold generate terraform-module

# Generate to a specific target directory
atmos scaffold generate terraform-module ./my-project
```

### Local and Remote Templates
Generate from local filesystem or remote repositories:

```bash
# Generate from a local filesystem path
atmos scaffold generate ./my-template ./my-project

# Generate from a remote Git repository
atmos scaffold generate https://github.com/user/template.git ./my-project

# Generate from a remote Git repository with specific ref
atmos scaffold generate https://github.com/user/template.git?ref=v1.0.0 ./my-project
```

## Flags

### File Management
- `--force, -f`: Force overwrite existing files
- `--update, -u`: Update existing files with 3-way merge

### Template Values
- `--value, -v`: Set a configuration value (format: key=value, can be specified multiple times)
- `--use-defaults, -d`: Use default values without prompting

### Merge Configuration
- `--threshold, -t`: Percentage threshold for merge changes (0-100, default 50)

## Advanced Examples

### Force Overwrite Existing Files
```bash
atmos scaffold generate terraform-module ./my-project --force
```

### Update Existing Files with 3-way Merge
```bash
atmos scaffold generate terraform-module ./my-project --update
```

### Use Default Values Without Prompting
```bash
atmos scaffold generate terraform-module ./my-project --use-defaults
```

### Set Template Values via Command Line
```bash
atmos scaffold generate terraform-module ./my-project --value project_name=my-project --value environment=prod
```

### Custom Merge Threshold
```bash
atmos scaffold generate terraform-module ./my-project --update --threshold 75
```

## Configuration

### atmos.yaml Scaffold Section
Configure available templates in your `atmos.yaml` file:

```yaml
scaffold:
  templates:
    terraform-module:
      source: "github.com/cloudposse/terraform-aws-s3-bucket"
      ref: "v1.0.0"
      target_dir: "./components/terraform/{{ .Config.project_name }}"
      description: "Basic Terraform module template for S3 buckets"
      values:
        bucket_name: "{{ .Config.project_name }}-{{ .Config.environment }}"
        environment: "{{ .Config.environment }}"
        author: "{{ .Config.author }}"

    component-config:
      source: "github.com/cloudposse/terraform-aws-vpc"
      ref: "v2.0.0"
      target_dir: "./components/terraform/{{ .Config.project_name }}-vpc"
      description: "VPC component configuration template"
      values:
        vpc_name: "{{ .Config.project_name }}-vpc"
        environment: "{{ .Config.environment }}"
```

### Template Configuration Fields
- **source**: Go-getter compatible source URL (GitHub, GitLab, etc.)
- **ref**: Git reference (branch, tag, or commit)
- **target_dir**: Target directory with Go template variables
- **description**: Human-readable description of the template
- **values**: Default values for template variables
- **delimiters**: Custom template delimiters (array of two strings, e.g., `["[[", "]]"]`)

### Template Delimiters

Scaffold templates support custom delimiters to avoid conflicts with Atmos's own Go template syntax. You can specify custom delimiters in both the `atmos.yaml` configuration and the template's `scaffold.yaml`:

**In atmos.yaml:**
```yaml
scaffold:
  templates:
    my-template:
      source: "github.com/user/template"
      ref: "v1.0.0"
      delimiters: ["[[", "]]"]
      description: "Template with custom delimiters"
```

**In scaffold.yaml:**
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

**Note**: If no delimiters are specified, the default `{{` and `}}` delimiters are used. The delimiters specified in the template's `scaffold.yaml` take precedence over those in `atmos.yaml`.

## Interactive Features

When run without arguments, the `generate` command provides an interactive menu that:

1. **Template Selection**: Choose from available scaffold templates configured in `atmos.yaml`
2. **Target Directory**: Specify where the scaffold should be generated with smart defaults
3. **Template Processing**: Automatically downloads and processes the selected template

The interactive mode makes it easy to scaffold scaffold templates without memorizing template names or paths.

## Error Handling

The command provides helpful error messages for common scenarios:

- **No atmos.yaml**: "No atmos.yaml configuration file found"
- **No scaffold section**: "No scaffold templates configured in atmos.yaml"
- **No templates**: "No scaffold templates configured in atmos.yaml"
- **Template not found**: "scaffold template 'name' not found"
