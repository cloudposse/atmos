---
title: Atlantis Integration
sidebar_position: 11
sidebar_label: Atlantis
---

Atmos natively supports [Atlantis](https://runatlantis.io) for Terraform Pull Request Automation.

## How it Works

With `atmos`, all of your configuration is neatly defined in YAML. This makes transformations of that data very easy.

The `atmos` tool supports (3) commands that when combined, make it easy to use `atlantis`.

1. Generate the `atlantis.yaml` repo configuration: `atmos atlantis generate repo-config`
2. Generate the backend configuration for all components: `atmos terraform generate backends --format=hcl`
3. Generate the full deep-merged configurations of all stacks for each components: `atmos terraform generate varfiles`

## Configuration

To configure Atmos to generate the Atlantis repo configurations, update the `atmos.yaml` configuration.

Here's an example to get you started. As with *everything* in atmos, it supports deep-merging. Anything under the `integrations.atlantis` section can
be overridden in the `components.terraform._name_.settings.atlantis` section at any level of the inheritance chain.

```yaml
# atmos.yaml CLI config

# Integrations
integrations:

  # Atlantis integration
  # https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html
  atlantis:
    # Path and name of the Atlantis config file `atlantis.yaml`
    # Supports absolute and relative paths
    # All the intermediate folders will be created automatically (e.g. `path: /config/atlantis/atlantis.yaml`)
    # Can be overridden on the command line by using `--output-path` command-line argument in `atmos atlantis generate repo-config` command
    # If not specified (set to an empty string/omitted here, and set to an empty string on the command line), the content of the file will be dumped to `stdout`
    # On Linux/macOS, you can also use `--output-path=/dev/stdout` to dump the content to `stdout` without setting it to an empty string in `atlantis.path`
    path: "atlantis.yaml"

    # Config templates
    # Select a template by using the `--config-template <config_template>` command-line argument in `atmos atlantis generate repo-config` command
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
    # Select a template by using the `--project-template <project_template>` command-line argument in `atmos atlantis generate repo-config` command
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
            - "varfiles/$PROJECT_NAME.tfvars"
          apply_requirements:
            - "approved"

    # Workflow templates
    # https://www.runatlantis.io/docs/custom-workflows.html#custom-init-plan-apply-commands
    # https://www.runatlantis.io/docs/custom-workflows.html#custom-run-command
    # Select a template by using the `--workflow-template <workflow_template>` command-line argument in `atmos atlantis generate repo-config` command
    workflow_templates:
      workflow-1:
        plan:
          steps:
            - run: terraform init -input=false
            # When using workspaces, you need to select the workspace using the $WORKSPACE environment variable
            - run: terraform workspace select $WORKSPACE
            # You must output the plan using `-out $PLANFILE` because Atlantis expects plans to be in a specific location
            - run: terraform plan -input=false -refresh -out $PLANFILE -var-file varfiles/$PROJECT_NAME.tfvars
        apply:
          steps:
            - run: terraform apply $PLANFILE
```

Using the config, project and workflow templates, atmos generates a separate atlantis project for each atmos component in every stack:

By running:

```shell
atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --workflow-template workflow-1
```

The following Atlantis repo-config would be generated.

```yaml
version: 3
automerge: true
delete_source_branch_on_merge: true
parallel_plan: true
parallel_apply: true
allowed_regexp_prefixes:
  - dev/
  - staging/
  - prod/
projects:
  - name: tenant1-ue2-staging-test-test-component-override-3
    workspace: test-component-override-3-workspace
    workflow: workflow-1
    dir: examples/complete/components/terraform/test/test-component
    terraform_version: v1.2
    delete_source_branch_on_merge: true
    autoplan:
      enabled: true
      when_modified:
        - '**/*.tf'
        - varfiles/$PROJECT_NAME.tfvars
      apply_requirements:
        - approved
  - name: tenant1-ue2-staging-infra-vpc
    workspace: tenant1-ue2-staging
    workflow: workflow-1
    dir: examples/complete/components/terraform/infra/vpc
    terraform_version: v1.2
    delete_source_branch_on_merge: true
    autoplan:
      enabled: true
      when_modified:
        - '**/*.tf'
        - varfiles/$PROJECT_NAME.tfvars
      apply_requirements:
        - approved
workflows:
  workflow-1:
    apply:
      steps:
        - run: terraform apply $PLANFILE
    plan:
      steps:
        - run: terraform init -input=false
        - run: terraform workspace select $WORKSPACE
        - run: terraform plan -input=false -refresh -out $PLANFILE -var-file varfiles/$PROJECT_NAME.tfvars
```

## Next Steps

Generating the Atlantis repo-config is only part of what's needed to use `atmos` with `atlantis`. The rest will depend on your organization's
preferences for generating the Terraform `.tfvars` files and backends.

We suggest using pre-commit hooks and/or GitHub Actions (or similar), to generate the `.tfvars` files and state backend configurations, which are
necessarily derived from the atmos stack configuration.

The following commands will generate those files.

1. `atmos terraform generate backends --format=backend-config|hcl`
2. `atmos terraform generate varfiles`

Make sure that the resulting files are committed back to VCS (e.g. `git add -A`) and push'd upstream. That way Atlantis will trigger on the "affected
files" and propose a plan.

### Example GitHUb Action

Here's an example GitHub Action to use Atlantis with Atmos.

You can adopt and modify it to your own needs.

```yaml
name: atmos

on:
  workflow_dispatch:

  issue_comment:
    types:
      - created

  pull_request:
    types:
      - opened
      - edited
      - synchronize
      - closed
    branches: [ main ]

env:
  ATMOS_VERSION: 1.14.0
  ATMOS_CLI_CONFIG_PATH: ./

jobs:
  generate_atlantis-yaml:
    name: Generate varfiles, backend config and atlantis.yaml
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v3
        if: github.event.pull_request.state == 'open' || ${{ github.event.issue.pull_request }}
        with:
          ref: ${{ github.event.pull_request.head.ref }}
          fetch-depth: 2

      # Install Atmos and generate tfvars and backend config files
      - name: Generate TF var files and backend configs
        if: github.event.pull_request.state == 'open' || ${{ github.event.issue.pull_request }}
        shell: bash
        run: |
          wget -q https://github.com/cloudposse/atmos/releases/download/v${ATMOS_VERSION}/atmos_${ATMOS_VERSION}_linux_amd64 && \
          mv atmos_${ATMOS_VERSION}_linux_amd64 /usr/local/bin/atmos && \
          chmod +x /usr/local/bin/atmos
          atmos terraform generate varfiles --file-template={component-path}/varfiles/{namespace}-{environment}-{component}.tfvars.json
          atmos terraform generate backends --format=backend-config --file-template={component-path}/backends/{namespace}-{environment}-{component}.backend

      # Commit changes (if any) to the PR branch
      - name: Commit changes to the PR branch
        if: github.event.pull_request.state == 'open' || ${{ github.event.issue.pull_request }}
        shell: bash
        run: |
          untracked=$(git ls-files --others --exclude-standard)
          changes_detected=$(git diff --name-only)
          if [ -n "$untracked" ] || [ -n "$changes_detected" ]; then
            git config --global user.name github-actions
            git config --global user.email github-actions@github.com
            git add -A *
            git commit -m "Committing generated autogenerated var files"
            git push
          fi

      # Generate atlantis.yaml with atmos
      - name: Generate Dynamic atlantis.yaml file
        if: github.event.pull_request.state == 'open' || ${{ github.event.issue.pull_request }}
        shell: bash
        run: |
          atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --workflow-template workflow-1

      # Commit changes (if any) to the PR branch
      - name: Commit changes to the PR branch
        if: github.event.pull_request.state == 'open' || ${{ github.event.issue.pull_request }}
        shell: bash
        run: |
          yaml_changes=$(git diff --name-only)
          untracked=$(git ls-files --others --exclude-standard atlantis.yaml)
          if [ -n "$yaml_changes" ] || [ -n "$untracked" ]; then
            git config --global user.name github-actions
            git config --global user.email github-actions@github.com
            git add -A *
            git commit -m "Committing generated atlantis.yaml"
            git push
          fi

  call-atlantis:
    needs: generate_atlantis-yaml
    name: Sending data to Atlantis
    runs-on: ubuntu-latest
    steps:
      - name: Invoke deployment hook
        uses: distributhor/workflow-webhook@v2
        env:
          webhook_type: 'json-extended'
          webhook_url: ${{ secrets.WEBHOOK_URL }}
          webhook_secret: ${{ secrets.WEBHOOK_SECRET }}
          verbose: false
```
