---
title: CLI Configuration
sidebar_position: 1
id: configuration
description: Use the `atmos.yaml` configuration file to control the behavior of the `atmos` CLI.
---

:::note Purpose
Use the `atmos.yaml` configuration file to control the behavior of the `atmos` CLI
:::

Everything in the `atmos` CLI is configurable. The defaults are established in the `atmos.yaml` configuration file. The CLI configuration should not
be confused with [Stack configurations](/core-concepts/stacks/), which have a different schema.

# Configuration File (`atmos.yaml`)

The CLI config is loaded from the following locations (from lowest to highest priority):

- System directory (`/usr/local/etc/atmos/atmos.yaml` on Linux, `%LOCALAPPDATA%/atmos/atmos.yaml` on Windows)
- Home directory (`~/.atmos/atmos.yaml`)
- Current directory (`./atmos.yaml`)
- Environment variable `ATMOS_CLI_CONFIG_PATH` (the ENV var should point to a folder without specifying the file name)

Each configuration file discovered is deep-merged with the preceeding configurations.

<br/>

:::tip Pro-Tip
Atmos supports [POSIX-style greedy Globs](https://en.wikipedia.org/wiki/Glob_(programming)) for all file
names/paths (double-star/globstar `**`)
:::

<br/>

What follows are all the sections of the `atmos.yaml` configuration file.

## Base Path

The base path for components, stacks and workflows configurations.
It can also be set using `ATMOS_BASE_PATH` environment variable, or by passing the `--base-path` command-line argument.
It supports both absolute and relative paths.

If not provided or is an empty string, `components.terraform.base_path`, `components.helmfile.base_path`, `stacks.base_path` and `workflows.base_path`
are independent settings (supporting both absolute and relative paths).

If `base_path` is provided, `components.terraform.base_path`, `components.helmfile.base_path`, `stacks.base_path`, `workflows.base_path`,
`schemas.jsonschema.base_path` and `schemas.opa.base_path` are considered paths relative to `base_path`.

```yaml
base_path: "."
```

## Components

Specify the default behaviors for components.

```yaml
components:
  terraform:
    # Can also be set using 'ATMOS_COMPONENTS_TERRAFORM_BASE_PATH' ENV var, or '--terraform-dir' command-line argument
    # Supports both absolute and relative paths
    base_path: "components/terraform"

    # Can also be set using 'ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE' ENV var
    apply_auto_approve: false

    # Can also be set using 'ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT' ENV var, or '--deploy-run-init' command-line argument
    deploy_run_init: true

    # Can also be set using 'ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE' ENV var, or '--init-run-reconfigure' command-line argument
    init_run_reconfigure: true

    # Can also be set using 'ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE' ENV var, or '--auto-generate-backend-file' command-line argument
    auto_generate_backend_file: false

  helmfile:
    # Can also be set using 'ATMOS_COMPONENTS_HELMFILE_BASE_PATH' ENV var, or '--helmfile-dir' command-line argument
    # Supports both absolute and relative paths
    base_path: "components/helmfile"

    # Can also be set using 'ATMOS_COMPONENTS_HELMFILE_USE_EKS' ENV var
    # If not specified, defaults to 'true'
    use_eks: true

    # Can also be set using 'ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH' ENV var
    kubeconfig_path: "/dev/shm"

    # Can also be set using 'ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN' ENV var
    helm_aws_profile_pattern: "{namespace}-{tenant}-gbl-{stage}-helm"

    # Can also be set using 'ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN' ENV var
    cluster_name_pattern: "{namespace}-{tenant}-{environment}-{stage}-eks-cluster"
```

## Stacks

Specify where to find stacks.

```yaml
stacks:
  # Can also be set using 'ATMOS_STACKS_BASE_PATH' ENV var, or '--config-dir' and '--stacks-dir' command-line arguments
  # Supports both absolute and relative paths
  base_path: "stacks"

  # Can also be set using 'ATMOS_STACKS_INCLUDED_PATHS' ENV var (comma-separated values string)
  included_paths:
    - "orgs/**/*"

  # Can also be set using 'ATMOS_STACKS_EXCLUDED_PATHS' ENV var (comma-separated values string)
  excluded_paths:
    - "**/_defaults.yaml"

  # Can also be set using 'ATMOS_STACKS_NAME_PATTERN' ENV var
  name_pattern: "{tenant}-{environment}-{stage}"
```

## Workflows

```yaml
workflows:
  # Can also be set using 'ATMOS_WORKFLOWS_BASE_PATH' ENV var, or '--workflows-dir' command-line arguments
  # Supports both absolute and relative paths
  base_path: "stacks/workflows"
```

## Custom CLI Sub-commands

You can extend the Atmos CLI and add as many subcommands as you want. This is a great way to increase DX by exposing a consistent CLI interface to
developers.

For example, one great way to use subcommands is to tie all the miscellaneous scripts into one consistent CLI interface. Then we can kiss those ugly,
inconsistent arguments to bash scripts goodbye! Just wire up the commands in atmos to call the script. Then developers can just run `atmos help` and
discover all available commands.

Here are some examples to play around with to get started.

```yaml
# Custom CLI commands
commands:
  - name: tf
    description: Execute 'terraform' commands

    # subcommands
    commands:
      - name: plan
        description: This command plans terraform components
        arguments:
          - name: component
            description: Name of the component
        flags:
          - name: stack
            shorthand: s
            description: Name of the stack
            required: true
        env:
          - key: ENV_VAR_1
            value: ENV_VAR_1_value
          - key: ENV_VAR_2
            # 'valueCommand' is an external command to execute to get the value for the ENV var
            # Either 'value' or 'valueCommand' can be specified for the ENV var, but not both
            valueCommand: echo ENV_VAR_2_value
        # steps support Go templates
        steps:
          - atmos terraform plan {{ .Arguments.component }} -s {{ .Flags.stack }}

  - name: terraform
    description: Execute 'terraform' commands

    # subcommands
    commands:
      - name: provision
        description: This command provisions terraform components
        arguments:
          - name: component
            description: Name of the component

        flags:
          - name: stack
            shorthand: s
            description: Name of the stack
            required: true

        # ENV var values support Go templates
        env:
          - key: ATMOS_COMPONENT
            value: "{{ .Arguments.component }}"
          - key: ATMOS_STACK
            value: "{{ .Flags.stack }}"
        steps:
          - atmos terraform plan $ATMOS_COMPONENT -s $ATMOS_STACK
          - atmos terraform apply $ATMOS_COMPONENT -s $ATMOS_STACK

  - name: play
    description: This command plays games
    steps:
      - echo Playing...

    # subcommands
    commands:
      - name: hello
        description: This command says Hello world
        steps:
          - echo Hello world

      - name: ping
        description: This command plays ping-pong

        # If 'verbose' is set to 'true', atmos will output some info messages to the console before executing the command's steps
        # If 'verbose' is not defined, it implicitly defaults to 'false'
        verbose: true
        steps:
          - echo Playing ping-pong...
          - echo pong

  - name: show
    description: Execute 'show' commands

    # subcommands
    commands:
      - name: component
        description: Execute 'show component' command
        arguments:
          - name: component
            description: Name of the component
        flags:
          - name: stack
            shorthand: s
            description: Name of the stack
            required: true

        # ENV var values support Go templates and have access to {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables
        env:
          - key: ATMOS_COMPONENT
            value: "{{ .Arguments.component }}"
          - key: ATMOS_STACK
            value: "{{ .Flags.stack }}"
          - key: ATMOS_TENANT
            value: "{{ .ComponentConfig.vars.tenant }}"
          - key: ATMOS_STAGE
            value: "{{ .ComponentConfig.vars.stage }}"
          - key: ATMOS_ENVIRONMENT
            value: "{{ .ComponentConfig.vars.environment }}"
          - key: ATMOS_IS_PROD
            value: "{{ .ComponentConfig.settings.config.is_prod }}"

        # If a custom command defines 'component_config' section with 'component' and 'stack', 'atmos' generates the config for the component in the stack
        # and makes it available in {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables,
        # exposing all the component sections (which are also shown by 'atmos describe component' command)
        component_config:
          component: "{{ .Arguments.component }}"
          stack: "{{ .Flags.stack }}"
        # Steps support using Go templates and can access all configuration settings (e.g. {{ .ComponentConfig.xxx.yyy.zzz }})
        # Steps also have access to the ENV vars defined in the 'env' section of the 'command'
        steps:
          - 'echo Atmos component from argument: "{{ .Arguments.component }}"'
          - 'echo ATMOS_COMPONENT: "$ATMOS_COMPONENT"'
          - 'echo Atmos stack: "{{ .Flags.stack }}"'
          - 'echo Terraform component: "{{ .ComponentConfig.component }}"'
          - 'echo Backend S3 bucket: "{{ .ComponentConfig.backend.bucket }}"'
          - 'echo Terraform workspace: "{{ .ComponentConfig.workspace }}"'
          - 'echo Namespace: "{{ .ComponentConfig.vars.namespace }}"'
          - 'echo Tenant: "{{ .ComponentConfig.vars.tenant }}"'
          - 'echo Environment: "{{ .ComponentConfig.vars.environment }}"'
          - 'echo Stage: "{{ .ComponentConfig.vars.stage }}"'
          - 'echo settings.spacelift.workspace_enabled: "{{ .ComponentConfig.settings.spacelift.workspace_enabled }}"'
          - 'echo Dependencies: "{{ .ComponentConfig.deps }}"'
          - 'echo settings.config.is_prod: "{{ .ComponentConfig.settings.config.is_prod }}"'
          - 'echo ATMOS_IS_PROD: "$ATMOS_IS_PROD"'

  - name: list
    description: Execute 'atmos list' commands
    # subcommands
    commands:
      - name: stacks
        description: |
          List all Atmos stacks.
        steps:
          - >
            atmos describe stacks --sections none | grep -e "^\S" | sed s/://g
      - name: components
        description: |
          List all Atmos components in all stacks or in a single stack.

          Example usage:
            atmos list components
            atmos list components -s tenant1-ue1-dev
            atmos list components --stack tenant2-uw2-prod
        flags:
          - name: stack
            shorthand: s
            description: Name of the stack
            required: false
        steps:
          - >
            {{ if .Flags.stack }}
            atmos describe stacks --stack {{ .Flags.stack }} --format json --sections none | jq ".[].components.terraform" | jq -s add | jq -r "keys[]"
            {{ else }}
            atmos describe stacks --format json --sections none | jq ".[].components.terraform" | jq -s add | jq -r "keys[]"
            {{ end }}

  - name: set-eks-cluster
    description: |
      Download 'kubeconfig' and set EKS cluster.

      Example usage:
        atmos set-eks-cluster eks/cluster -s tenant1-ue1-dev -r admin
        atmos set-eks-cluster eks/cluster -s tenant2-uw2-prod --role reader
    verbose: false  # Set to `true` to see verbose outputs
    arguments:
      - name: component
        description: Name of the component
    flags:
      - name: stack
        shorthand: s
        description: Name of the stack
        required: true
      - name: role
        shorthand: r
        description: IAM role to use
        required: true
    # If a custom command defines 'component_config' section with 'component' and 'stack',
    # Atmos generates the config for the component in the stack
    # and makes it available in {{ .ComponentConfig.xxx.yyy.zzz }} Go template variables,
    # exposing all the component sections (which are also shown by 'atmos describe component' command)
    component_config:
      component: "{{ .Arguments.component }}"
      stack: "{{ .Flags.stack }}"
    env:
      - key: KUBECONFIG
        value: /dev/shm/kubecfg.{{ .Flags.stack }}-{{ .Flags.role }}
    steps:
      - >
        aws
        --profile {{ .ComponentConfig.vars.namespace }}-{{ .ComponentConfig.vars.tenant }}-gbl-{{ .ComponentConfig.vars.stage }}-{{ .Flags.role }}
        --region {{ .ComponentConfig.vars.region }}
        eks update-kubeconfig
        --name={{ .ComponentConfig.vars.namespace }}-{{ .Flags.stack }}-eks-cluster
        --kubeconfig="${KUBECONFIG}"
        > /dev/null
      - chmod 600 ${KUBECONFIG}
      - echo ${KUBECONFIG}
```

## Integrations

```yaml
# Integrations
integrations:

  # Atlantis integration
  # https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html
  atlantis:
    # Path and name of the Atlantis config file 'atlantis.yaml'
    # Supports absolute and relative paths
    # All the intermediate folders will be created automatically (e.g. 'path: /config/atlantis/atlantis.yaml')
    # Can be overridden on the command line by using '--output-path' command-line argument in 'atmos atlantis generate repo-config' command
    # If not specified (set to an empty string/omitted here, and set to an empty string on the command line), the content of the file will be dumped to 'stdout'
    # On Linux/macOS, you can also use '--output-path=/dev/stdout' to dump the content to 'stdout' without setting it to an empty string in 'atlantis.path'
    path: "atlantis.yaml"

    # Config templates
    # Select a template by using the '--config-template <config_template>' command-line argument in 'atmos atlantis generate repo-config' command
    config_templates:
      config-1:
        version: 3
        automerge: true
        delete_source_branch_on_merge: true
        parallel_plan: true
        parallel_apply: true
        allowed_regexp_prefixes:
          - dev/
          - staging/
          - prod/

    # Project templates
    # Select a template by using the '--project-template <project_template>' command-line argument in 'atmos atlantis generate repo-config' command
    project_templates:
      project-1:
        # generate a project entry for each component in every stack
        name: "{tenant}-{environment}-{stage}-{component}"
        workspace: "{workspace}"
        dir: "{component-path}"
        terraform_version: v1.2
        delete_source_branch_on_merge: true
        autoplan:
          enabled: true
          when_modified:
            - "**/*.tf"
            - "varfiles/$PROJECT_NAME.tfvars.json"
        apply_requirements:
          - "approved"

    # Workflow templates
    # https://www.runatlantis.io/docs/custom-workflows.html#custom-init-plan-apply-commands
    # https://www.runatlantis.io/docs/custom-workflows.html#custom-run-command
    workflow_templates:
      workflow-1:
        plan:
          steps:
            - run: terraform init -input=false
            # When using workspaces, you need to select the workspace using the $WORKSPACE environment variable
            - run: terraform workspace select $WORKSPACE || terraform workspace new $WORKSPACE
            # You must output the plan using '-out $PLANFILE' because Atlantis expects plans to be in a specific location
            - run: terraform plan -input=false -refresh -out $PLANFILE -var-file varfiles/$PROJECT_NAME.tfvars.json
        apply:
          steps:
            - run: terraform apply $PLANFILE
```

## Schemas

Configure the paths where to find OPA, JSON Schema, and CUE files.

```yaml
# Validation schemas (for validating atmos stacks and components)
schemas:

  # https://json-schema.org
  jsonschema:

    # Can also be set using 'ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH' ENV var, or '--schemas-jsonschema-dir' command-line arguments
    # Supports both absolute and relative paths
    base_path: "stacks/schemas/jsonschema"

  # https://www.openpolicyagent.org
  opa:
    # Can also be set using 'ATMOS_SCHEMAS_OPA_BASE_PATH' ENV var, or '--schemas-opa-dir' command-line arguments
    # Supports both absolute and relative paths
    base_path: "stacks/schemas/opa"

  # https://cuelang.org
  cue:
    # Can also be set using 'ATMOS_SCHEMAS_CUE_BASE_PATH' ENV var, or '--schemas-cue-dir' command-line arguments
    # Supports both absolute and relative paths
    base_path: "stacks/schemas/cue"
```

## Logs

Atmos logs are configured in the `logs` section:

```yaml
logs:
  file: "/dev/stdout"
  # Supported log levels: Trace, Debug, Info, Warning, Off
  level: Info
```

- `logs.file` - the file to write Atmos logs to. Logs can be written to any file or any standard file descriptor,
  including `/dev/stdout`, `/dev/stderr` and `/dev/null`). If omitted, `/dev/stdout` will be used. The ENV variable `ATMOS_LOGS_FILE` can also be used
  to specify the log file

- `logs.level` - Log level. Supported log levels are `Trace`, `Debug`, `Info`, `Warning`, `Off`. If the log level is set to `Off`, Atmos will not log
  any messages (note that this does not prevent other tools like Terraform from logging). The ENV variable `ATMOS_LOGS_LEVEL` can also be used
  to specify the log level

To prevent Atmos from logging any messages, you can do one of the following:

- Set `logs.file` or the ENV variable `ATMOS_LOGS_FILE` to `/dev/null`

- Set `logs.level` or the ENV variable `ATMOS_LOGS_LEVEL` to `Off`

## Environment Variables

Most YAML settings can also be defined by environment variables. This is helpful while doing local development. For example,
setting `ATMOS_STACKS_BASE_PATH` to a path in `/localhost` to your local development folder, will enable you to rapidly iterate.

| Variable                                              | YAML Path                                       | Description                                                                                                                                                                                                                 |
|:------------------------------------------------------|:------------------------------------------------|:----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| ATMOS_CLI_CONFIG_PATH                                 | N/A                                             | Where to find `atmos.yaml`. Path to a folder where `atmos.yaml` CLI config file is located (e.g. `/config`)                                                                                                                 |
| ATMOS_BASE_PATH                                       | base_path                                       | Base path to `components` and `stacks` folders                                                                                                                                                                              |
| ATMOS_COMPONENTS_TERRAFORM_BASE_PATH                  | components.terraform.base_path                  | Base path to Terraform components                                                                                                                                                                                           |
| ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE         | components.terraform.apply_auto_approve         | If set to `true`, auto-generate Terraform backend config files when executing `atmos terraform` commands                                                                                                                    |
| ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT            | components.terraform.deploy_run_init            | Run `terraform init` when executing `atmos terraform deploy` command                                                                                                                                                        |
| ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE       | components.terraform.init_run_reconfigure       | Run `terraform init -reconfigure` when executing `atmos terraform` commands                                                                                                                                                 |
| ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE | components.terraform.auto_generate_backend_file | If set to `true`, auto-generate Terraform backend config files when executing `atmos terraform` commands                                                                                                                    |
| ATMOS_COMPONENTS_HELMFILE_BASE_PATH                   | components.helmfile.base_path                   | Path to helmfile components                                                                                                                                                                                                 |
| ATMOS_COMPONENTS_HELMFILE_USE_EKS                     | components.helmfile.use_eks                     | If set to `true`, download `kubeconfig` from EKS by running `aws eks update-kubeconfig` command before executing `atmos helmfile` commands                                                                                  |
| ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH             | components.helmfile.kubeconfig_path             | Path to write the `kubeconfig` file when executing `aws eks update-kubeconfig` command                                                                                                                                      |
| ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN    | components.helmfile.helm_aws_profile_pattern    | Pattern for AWS profile to use when executing `atmos helmfile` commands                                                                                                                                                     |
| ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN        | components.helmfile.cluster_name_pattern        | Pattern for EKS cluster name to use when executing `atmos helmfile` commands                                                                                                                                                |
| ATMOS_STACKS_BASE_PATH                                | stacks.base_path                                | Base path to Atmos stack manifests                                                                                                                                                                                          |
| ATMOS_STACKS_INCLUDED_PATHS                           | stacks.included_paths                           | List of paths to use as top-level stack manifests                                                                                                                                                                           |
| ATMOS_STACKS_EXCLUDED_PATHS                           | stacks.excluded_paths                           | List of paths to not consider as top-level stacks                                                                                                                                                                           |
| ATMOS_STACKS_NAME_PATTERN                             | stacks.name_pattern                             | Stack name pattern to use as Atmos stack names                                                                                                                                                                              |
| ATMOS_WORKFLOWS_BASE_PATH                             | workflows.base_path                             | Base path to Atmos workflows                                                                                                                                                                                                |
| ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH                    | schemas.jsonschema.base_path                    | Base path to JSON schemas for component validation                                                                                                                                                                          |
| ATMOS_SCHEMAS_OPA_BASE_PATH                           | schemas.opa.base_path                           | Base path to OPA policies for component validation                                                                                                                                                                          |
| ATMOS_LOGS_FILE                                       | logs.file                                       | The file to write Atmos logs to. Logs can be written to any file or any standard file descriptor, including `/dev/stdout`, `/dev/stderr` and `/dev/null`). If omitted, `/dev/stdout` will be used                           |
| ATMOS_LOGS_LEVEL                                      | logs.level                                      | Log level. Supported log levels are `Trace`, `Debug`, `Info`, `Warning`, `Off`. If the log level is set to `Off`, Atmos will not log any messages (note that this does not prevent other tools like Terraform from logging) |
