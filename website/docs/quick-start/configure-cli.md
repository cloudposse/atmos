---
title: Configure CLI
sidebar_position: 4
sidebar_label: Configure CLI
---

In the previous step, we've decided on the following:

- Use a monorepo to configure and provision two Terraform components into three AWS accounts and two AWS regions
- The filesystem layout for the infrastructure monorepo
- To be able to use [Component Remote State](/core-concepts/components/remote-state), we put the `atmos.yaml` CLI config file
  into `/usr/local/etc/atmos/atmos.yaml` folder and set the ENV var `ATMOS_BASE_PATH` to point to the absolute path of the root of the repo

Next step is to configure `atmos.yaml`.

`atmos.yaml` configuration file is used to control the behavior of the `atmos` CLI. The file supports many features that are configured in different
sections of the `atmos.yaml` file. For the description of all the sections, refer to [CLI Configuration](/cli/configuration).

For the purpuse of this Quck Start, below is the minimum configuration required for Atmos to work with Terraform and to
configure [Atmos components](/core-concepts/components) and [Atmos stacks](/core-concepts/stacks). Copy the YAML config below into your `atmos.yaml`
file.

<br/>

```yaml
# CLI config is loaded from the following locations (from lowest to highest priority):
# system dir ('/usr/local/etc/atmos' on Linux, '%LOCALAPPDATA%/atmos' on Windows)
# home dir (~/.atmos)
# current directory
# ENV vars
# Command-line arguments
#
# It supports POSIX-style Globs for file names/paths (double-star '**' is supported)
# https://en.wikipedia.org/wiki/Glob_(programming)

# Base path for components, stacks and workflows configurations.
# Can also be set using 'ATMOS_BASE_PATH' ENV var, or '--base-path' command-line argument.
# Supports both absolute and relative paths.
# If not provided or is an empty string, 'components.terraform.base_path', 'components.helmfile.base_path', 'stacks.base_path' and 'workflows.base_path'
# are independent settings (supporting both absolute and relative paths).
# If 'base_path' is provided, 'components.terraform.base_path', 'components.helmfile.base_path', 'stacks.base_path' and 'workflows.base_path'
# are considered paths relative to 'base_path'.
base_path: ""

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
    auto_generate_backend_file: true

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

workflows:
  # Can also be set using 'ATMOS_WORKFLOWS_BASE_PATH' ENV var, or '--workflows-dir' command-line arguments
  # Supports both absolute and relative paths
  base_path: "stacks/workflows"

logs:
  verbose: false

# Custom CLI commands
commands: [ ]

# Integrations
integrations: { }

# Validation schemas (for validating atmos stacks and components)
schemas: { }
```

<br/>

The `atmos.yaml` configuration file has the following sections:

- `base_path` - the base path for components, stacks and workflows configurations. We set it to an empty string because we've decided to use the ENV
  var `ATMOS_BASE_PATH` to point to the absolute path of the root of the repo

- `components.terraform.base_path` - the base path to the Terraform components (Terraform root modules). As we've described in
  [Configure Repository](/quick-start/configure-repository), we've decided to put the Terraform components into the `components/terraform` directory,
  and this setting tells Atmos where to find them. Atmos will join the base path (set in the `ATMOS_BASE_PATH` ENN var)
  with `components.terraform.base_path` to calculate the final path to the Terraform components

- `components.terraform.apply_auto_approve` - if set to `true`, Atmos automatically adds `-auto-approve` option to instruct Terraform to apply the
  plan without asking for confirmation when executing `atmos terraform apply` command

- `components.terraform.deploy_run_init` - if set to `true`, Atmos runs `terraform init` before
  executing [`atmos terraform deploy`](/cli/commands/terraform/deploy) command

- `components.terraform.init_run_reconfigure`

- `components.terraform.auto_generate_backend_file`

