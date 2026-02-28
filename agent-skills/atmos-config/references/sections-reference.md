# atmos.yaml Sections Reference

Complete reference for every top-level section in `atmos.yaml`.

## base_path

Root directory for all relative paths in the configuration. All other `base_path` settings
(stacks, components, workflows, schemas) are resolved relative to this value.

```yaml
base_path: ""                              # Current directory (default)
base_path: "/opt/atmos/project"            # Absolute path
base_path: !repo-root                      # Git repository root
```

Environment variable: `ATMOS_BASE_PATH`.

## stacks

Controls how Atmos discovers and names stack manifests.

```yaml
stacks:
  base_path: "stacks"                      # Directory containing stack manifests
  included_paths:                          # Globs for deployable stacks
    - "orgs/**/*"
  excluded_paths:                          # Globs to exclude (catalogs, defaults)
    - "**/_defaults.yaml"
    - "catalog/**/*"
    - "mixins/**/*"
  name_template: "{{ .vars.tenant }}-{{ .vars.environment }}-{{ .vars.stage }}"
  name_pattern: "{tenant}-{environment}-{stage}"  # Legacy (superseded by name_template)
```

| Field | Description |
|-------|-------------|
| `base_path` | Root directory for stack manifests. |
| `included_paths` | Glob patterns identifying deployable stack files. |
| `excluded_paths` | Glob patterns to skip (catalog, mixin, defaults files). |
| `name_template` | Go template computing stack names from merged vars. Recommended. |
| `name_pattern` | Legacy token-based naming pattern. Use `name_template` instead. |

Environment variables: `ATMOS_STACKS_BASE_PATH`, `ATMOS_STACKS_INCLUDED_PATHS`, `ATMOS_STACKS_EXCLUDED_PATHS`,
`ATMOS_STACKS_NAME_TEMPLATE`, `ATMOS_STACKS_NAME_PATTERN`.

For complete stack configuration details, see the `atmos-stacks` skill.

## components

Configures component types and their settings.

```yaml
components:
  terraform:
    base_path: "components/terraform"
    command: "terraform"
    apply_auto_approve: false
    deploy_run_init: true
    init_run_reconfigure: true
    auto_generate_backend_file: true
    backend_type: s3
    backend:
      s3:
        encrypt: true
        bucket: "my-terraform-state"
        dynamodb_table: "my-terraform-state-lock"
        region: "us-east-1"
        key: "terraform.tfstate"
    shell:
      shell: "/bin/bash"

  helmfile:
    base_path: "components/helmfile"
    command: "helmfile"
    cluster_name_pattern: "{namespace}-{tenant}-{environment}-{stage}-eks-cluster"

  packer:
    base_path: "components/packer"
    command: "packer"

  ansible:
    base_path: "components/ansible"
    command: "ansible"
```

| Field | Description |
|-------|-------------|
| `<type>.base_path` | Directory containing component directories for this type. |
| `<type>.command` | Executable to run (supports absolute paths). |
| `terraform.backend_type` | Default backend type for all Terraform components. |
| `terraform.backend` | Backend-specific configuration (s3, gcs, azurerm, etc.). |
| `terraform.apply_auto_approve` | Auto-approve applies without confirmation. |
| `terraform.deploy_run_init` | Run `init` before `deploy`. |
| `terraform.init_run_reconfigure` | Pass `-reconfigure` to `init`. |
| `terraform.auto_generate_backend_file` | Generate `backend.tf.json` automatically. |
| `terraform.shell` | Configuration for `atmos terraform shell`. |
| `helmfile.cluster_name_pattern` | Pattern for EKS cluster name resolution. |

For Terraform details, see the `atmos-terraform` skill. For Helmfile, see `atmos-helmfile`.
For Packer, see `atmos-packer`. For Ansible, see `atmos-ansible`.

## workflows

Configures workflow file discovery.

```yaml
workflows:
  base_path: "stacks/workflows"
```

Environment variable: `ATMOS_WORKFLOWS_BASE_PATH`.

For complete workflow configuration, see the `atmos-workflows` skill.

## commands

Defines custom CLI commands. This is an array of command definitions.

```yaml
commands:
  - name: plan-all
    description: "Plan all components in a stack"
    steps:
      - command: terraform plan vpc
      - command: terraform plan rds

  - name: info
    description: "Show project info"
    arguments:
      - name: component
        description: "Component name"
    flags:
      - name: stack
        shorthand: s
        description: "Stack name"
        required: true
    steps:
      - 'echo "Component: {{ .Arguments.component }}, Stack: {{ .Flags.stack }}"'
    env:
      - key: ENV
        value: "{{ .Flags.stack }}"
```

For complete custom command syntax, see the `atmos-custom-commands` skill.

## aliases

Maps command aliases to full commands.

```yaml
aliases:
  tp: terraform plan
  ta: terraform apply
  dc: describe component
```

## templates

Configures Go template and Gomplate processing for stack manifests.

```yaml
templates:
  settings:
    enabled: true
    sprig:
      enabled: true
    gomplate:
      enabled: true
      timeout: 5
      datasources: {}
    delimiters:
      - "{{"
      - "}}"
    evaluations: 1
    env:
      enabled: true
```

| Field | Description |
|-------|-------------|
| `settings.enabled` | Enable/disable template processing globally. |
| `settings.sprig.enabled` | Enable Sprig template functions. |
| `settings.gomplate.enabled` | Enable Gomplate template functions. |
| `settings.gomplate.timeout` | Timeout in seconds for Gomplate datasources. |
| `settings.gomplate.datasources` | Named datasources for Gomplate. |
| `settings.delimiters` | Custom template delimiters (default: `{{ }}`). |
| `settings.evaluations` | Number of template evaluation passes. |
| `settings.env.enabled` | Allow `{{ env "VAR" }}` in templates. |

For complete template configuration, see the `atmos-templates` skill.

## schemas

Configures base paths for validation schemas.

```yaml
schemas:
  jsonschema:
    base_path: "stacks/schemas/jsonschema"
  opa:
    base_path: "stacks/schemas/opa"
  cue:
    base_path: "stacks/schemas/cue"
```

For schema management details, see the `atmos-schemas` skill.
For validation configuration, see the `atmos-validation` skill.

## validate

Configures validation behavior.

```yaml
validate:
  editorconfig:
    enabled: true
    base_path: "."
    format: text
    dry_run: false
    exclude:
      - "vendor/**"
      - "node_modules/**"
```

## auth

Configures the multi-provider authentication system.

```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://company.awsapps.com/start

  identities:
    dev-admin:
      kind: aws/permission-set
      default: true
      via:
        provider: company-sso
      principal:
        name: AdminAccess
        account:
          name: development

  integrations:
    ecr:
      kind: aws/ecr
      via:
        identity: dev-admin
      spec:
        registry:
          account_id: "123456789012"
          region: us-east-1

  keyring:
    type: system

  logs:
    level: Info
```

For complete auth configuration, see the `atmos-auth` skill.

## stores

Configures external key-value store backends.

```yaml
stores:
  ssm/dev:
    type: aws-ssm-parameter-store
    options:
      region: us-east-1
      identity: dev-admin

  secrets:
    type: azure-key-vault
    options:
      vault_url: https://my-vault.vault.azure.net/
```

Supported types: `aws-ssm-parameter-store`, `azure-key-vault`, `google-secret-manager` (alias: `gsm`),
`redis`, `artifactory`.

For complete store configuration, see the `atmos-stores` skill.

## vendor

Configures vendoring behavior.

```yaml
vendor:
  base_path: "vendor.d"
  retry:
    max_attempts: 3
    delay: 5s
```

For complete vendoring configuration, see the `atmos-vendoring` skill.

## toolchain

Configures CLI tool version management.

```yaml
toolchain:
  install_path: ".atmos/tools"
  file_path: ".tool-versions"
  use_lock_file: true
  lock_file: ".tool-versions.lock"
  registries:
    - type: aqua
      ref: "v4.332.0"
    - type: atmos
      tools:
        - name: tflint
          version: "0.54.0"
  aliases:
    tf: hashicorp/terraform
```

For complete toolchain configuration, see the `atmos-toolchain` skill.

## devcontainer

Configures development container environments. This feature is experimental.

```yaml
devcontainer:
  default:
    settings:
      runtime: docker
    spec:
      name: "Atmos Dev"
      image: "cloudposse/geodesic:latest"
      workspaceFolder: "/workspace"
      workspaceMount: "type=bind,source=${WORKSPACE},target=/workspace"
```

For complete devcontainer configuration, see the `atmos-devcontainer` skill.

## integrations

Configures external platform integrations.

```yaml
integrations:
  atlantis:
    config_templates: {}
    project_templates: {}
    workflow_templates: {}

  github:
    gitops:
      terraform-version: "1.9.8"
      infracost-enabled: false
      artifact-storage:
        region: us-east-1
        bucket: "gitops-plan-storage"
```

For complete integration configuration, see the `atmos-gitops` skill.

## settings

Global CLI behavior and feature settings.

```yaml
settings:
  # List merge strategy for stack deep-merging
  list_merge_strategy: replace

  # Terminal/UI settings
  terminal:
    pager: false
    max_width: 120
    colors: true
    unicode: true
    syntax_highlighting:
      enabled: true
      theme: dracula
    masking:
      enabled: true

  # Telemetry collection
  telemetry:
    enabled: true

  # Experimental features
  experimental:
    enabled: false
```

| Field | Description |
|-------|-------------|
| `list_merge_strategy` | How lists are merged during deep-merge (`replace` or `append`). |
| `terminal.pager` | Enable pager for long output. |
| `terminal.max_width` | Maximum output width. |
| `terminal.colors` | Enable colored output. |
| `terminal.syntax_highlighting` | Code syntax highlighting in output. |
| `terminal.masking.enabled` | Enable secret masking in output. |
| `telemetry.enabled` | Enable anonymous usage telemetry. |
| `experimental.enabled` | Enable experimental features. |

## logs

Configures logging behavior.

```yaml
logs:
  file: "/dev/stderr"
  level: Info                              # Debug, Info, Warn, Error
```

Environment variables: `ATMOS_LOGS_LEVEL`, `ATMOS_LOGS_FILE`.

## errors

Configures error handling and reporting.

```yaml
errors:
  format:
    verbose: false
  sentry:
    enabled: false
    dsn: ""
    environment: "production"
```

## env

Global environment variables injected into all component executions.

```yaml
env:
  TF_CLI_ARGS_plan: "-lock=false"
  AWS_SDK_LOAD_CONFIG: "true"
```

## import

Import other atmos.yaml files for modular configuration.

```yaml
import:
  - atmos.d/stacks.yaml
  - atmos.d/auth.yaml
  - atmos.d/integrations.yaml
```

Imported files are deep-merged into the main configuration.

## version

Specifies Atmos version constraints for the project.

```yaml
version:
  check:
    enabled: true
    constraints: ">= 1.100.0"
```

## profiles

Configures named configuration profiles.

```yaml
profiles:
  base_path: "profiles"
```

Profile directories contain `atmos.yaml` overrides activated with `--profile` or `ATMOS_PROFILE`.

## describe

Configures `describe` command behavior.

```yaml
describe:
  settings:
    include_empty: false
```

## docs

Configures documentation generation settings.

```yaml
docs:
  generate:
    base_dir: "docs"
    template: "README.md.gotmpl"
    output: "README.md"
```

## metadata

Project metadata for identification and organization.

```yaml
metadata:
  name: "my-infrastructure"
  description: "Core infrastructure project"
  version: "1.0.0"
  tags:
    team: platform
    env: production
```
