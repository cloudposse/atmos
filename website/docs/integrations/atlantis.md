---
title: Atlantis Integration
sidebar_position: 11
sidebar_label: Atlantis
---

Atmos natively supports [Atlantis](https://runatlantis.io) for Terraform Pull Request Automation.

## How it Works

With Atmos, all of your configuration is neatly defined in YAML. This makes transformations of that data very easy.

Atmos supports three commands that, when combined, make it easy to use Atlantis:

1. Generate the `atlantis.yaml` repo configuration: [`atmos atlantis generate repo-config`](/cli/commands/atlantis/generate-repo-config)

2. Generate the backend configuration for all
   components: [`atmos terraform generate backends --format=backend-config|hcl`](/cli/commands/terraform/generate-backends)

3. Generate the full deep-merged configurations of all stacks for each
   component: [`atmos terraform generate varfiles`](/cli/commands/terraform/generate-varfiles)

## Configuration

Atlantis Integration can be configured in two different ways (or a combination of them):

- In the `integrations.atlantis` section in `atmos.yaml`
- In the `settings.atlantis` sections in the stack config files

### Configure Atlantis integration in `integrations.atlantis` section in `atmos.yaml`

To configure Atmos to generate the Atlantis repo configurations, update the `integrations.atlantis` section in `atmos.yaml`.

Here's an example to get you started. As with *everything* in Atmos, it supports deep-merging. Anything under the `integrations.atlantis` section
in `atmos.yaml` can be overridden in the stack config section `settings.atlantis` at any level of the inheritance chain.

```yaml title=atmos.yaml
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

Using the config and project templates, Atmos generates a separate atlantis project for each Atmos component in every stack.

By running:

```shell
atmos atlantis generate repo-config --config-template config-1 --project-template project-1
```

The following Atlantis repo-config would be generated:

```yaml title=atlantis.yaml
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

<br/>

## Atlantis Workflows

Atlantis workflows can be defined in two different ways:

- In [Server Side Config](https://www.runatlantis.io/docs/server-side-repo-config.html) using the `workflows` section and `workflow` attribute

  ```yaml title=server.yaml
  repos:
    - id: /.*/
      branch: /.*/

      # 'workflow' sets the workflow for all repos that match.
      # This workflow must be defined in the workflows section.
      workflow: custom

      # allowed_overrides specifies which keys can be overridden by this repo in
      # its atlantis.yaml file.
      allowed_overrides: [apply_requirements, workflow, delete_source_branch_on_merge, repo_locking]

      # allowed_workflows specifies which workflows the repos that match
      # are allowed to select.
      allowed_workflows: [custom]

      # allow_custom_workflows defines whether this repo can define its own
      # workflows. If false (default), the repo can only use server-side defined
      # workflows.
      allow_custom_workflows: true  

  # workflows lists server-side custom workflows
  workflows:
    custom:
      plan:
        steps:
          - init
          - plan
      apply:
        steps:
          - run: echo applying
          - apply  
  ```

- In [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html) using the `workflows` section and the `workflow`
  attribute in each Atlantis project in `atlantis.yaml`

  ```yaml title=atlantis.yaml
  version: 3
  projects:
    - name: my-project-name
      branch: /main/
      dir: .
      workspace: default
      workflow: myworkflow
  workflows:
    myworkflow:
      plan:
        steps:
          - init
          - plan
      apply:
        steps:
          - run: echo applying
          - apply
  ```

<br/>

If you use [Server Side Config](https://www.runatlantis.io/docs/server-side-repo-config.html) to define Atlantis workflows, you don't need to define
workflows in the [CLI Config Atlantis Integration](/cli/configuration#integrations) section in `atmos.yaml` or in
the `settings.atlantis.workflow_templates` section in the stack configurations. When you defined the workflows in the server config `workflows`
section, you can reference a workflow to be used for each generated Atlantis project in the project templates.

On the other hand, if you use [Repo Level workflows](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html),
you need to provide at least one workflow template in the `workflow_templates` section in the [Atlantis Integration](/cli/configuration#integrations)
or in the `settings.atlantis.workflow_templates` section in the stack configurations.

For example, after executing the following command:

```console
atmos atlantis generate repo-config --config-template config-1 --project-template project-1
```

the generated `atlantis.yaml` file would look like this:

```yaml title=atlantis.yaml
version: 3
projects:
  - name: tenant1-ue2-dev-infra-vpc
    workspace: tenant1-ue2-dev
    workflow: workflow-1

workflows:
  workflow-1:
    apply:
      steps:
        - run: terraform apply $PLANFILE
    plan:
      steps:
        - run: terraform init -input=false
        - run: terraform workspace select $WORKSPACE || terraform workspace new $WORKSPACE
        - run: terraform plan -input=false -refresh -out $PLANFILE -var-file varfiles/$PROJECT_NAME.tfvars.json
```

<br/>

## Dynamic Repo Config Generation

If you want to generate the `atlantis.yaml` file before Atlantis can parse it, you can add a run command to `pre_workflow_hooks`. The repo config
will be generated right before Atlantis can parse it.

```yaml
repos:
  - id: /.*/
    pre_workflow_hooks:
      - run: ./repo-config-generator.sh
        description: Generating configs
```

To help with dynamic repo config generation, the `atmos atlantis generate repo-config` command accepts the `--affected-only` flag.
If set to `true`, Atmos will generate Atlantis projects only for the Atmos components changed between two Git commits.

See also [Pre Workflow Hooks](https://www.runatlantis.io/docs/pre-workflow-hooks.html)
and [Post Workflow Hooks](https://www.runatlantis.io/docs/post-workflow-hooks.html) for more information.

## Working with Private Repositories

If the flag `--affected-only=true` is passed on the command line (e.g. `atmos atlantis generate repo-config --affected-only=true`), the command
will clone and checkout the remote target repo (which can be the default `refs/heads` reference, or specified by the command-line
flags `--ref`, `--sha` or `--repo-path`). If the remote target repo is private, special attention needs to be given to how to work with private
repositories.

There are a few ways to work with private repositories with which the current local branch is compared to detect the changed files and affected Atmos
stacks and components:

- Using the `--ssh-key` flag to specify the filesystem path to a PEM-encoded private key to clone private repos using SSH, and
  the `--ssh-key-password` flag to provide the encryption password for the PEM-encoded private key if the key contains a password-encrypted PEM block

- Execute the `atmos atlantis generate repo-config --affected-only=true --repo-path <path_to_cloned_target_repo>` command in
  a [GitHub Action](https://docs.github.com/en/actions). For this to work, clone the remote target repository using
  the [checkout](https://github.com/actions/checkout) GitHub action. Then use the `--repo-path` flag to specify the path to the already cloned
  target repository with which to compare the current branch

## Using with GitHub Actions

If the `atmos atlantis generate repo-config --affected-only=true` command is executed in a [GitHub Action](https://docs.github.com/en/actions), and
you don't want to store or generate a long-lived SSH private key on the server, you can do the following:

- Create a GitHub
  [Personal Access Token (PAT)](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-personal-access-token)
  with scope permissions to clone private repos

- Add the created PAT as a repository or GitHub organization [secret](https://docs.github.com/en/actions/security-guides/encrypted-secrets) with the
  name [`GITHUB_TOKEN`](https://docs.github.com/en/actions/security-guides/automatic-token-authentication)

- In your GitHub action, clone the remote repository using the [checkout](https://github.com/actions/checkout) GitHub action

- Execute `atmos atlantis generate repo-config --affected-only=true --repo-path <path_to_cloned_target_repo>` command with the `--repo-path` flag set
  to the cloned repository path using the [`GITHUB_WORKSPACE`](https://docs.github.com/en/actions/learn-github-actions/variables) ENV variable (which
  points to the default working directory on the GitHub runner for steps, and the default location of the repository when using
  the [checkout](https://github.com/actions/checkout) action). For example:

    ```shell
    atmos atlantis generate repo-config --affected-only=true --repo-path $GITHUB_WORKSPACE
    ```

## Example GitHub Action

Here's an example GitHub Action to use Atlantis with Atmos.

The action executes the `atmos generate varfiles/backends` commands to generate Terraform varfiles and backend config files for all Atmos stacks,
then executes the `atmos atlantis generate repo-config` command to generate the Atlantis repo config file (`atlantis.yaml`) for all Atlantis projects,
then commits all the generated files and calls Atlantis via a webhook.

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
  generate-atlantis-yaml:
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
          atmos atlantis generate repo-config --config-template config-1 --project-template project-1

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
    if: ${{ always() }}
    needs: generate-atlantis-yaml
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

<br/>

## Next Steps

Generating the Atlantis repo-config is only part of what's needed to use Atmos with Atlantis. The rest will depend on your organization's
preferences for generating the Terraform `.tfvars` files and backends.

We suggest using pre-commit hooks and/or GitHub Actions (or similar), to generate the `.tfvars` files and state backend configurations, which are
necessarily derived from the atmos stack configuration.

The following commands will generate those files.

1. `atmos terraform generate backends --format=backend-config|hcl`
2. `atmos terraform generate varfiles`

Make sure that the resulting files are committed back to VCS (e.g. `git add -A`) and push'd upstream. That way Atlantis will trigger on the "affected
files" and propose a plan.

## References

For more information, refer to:

- [Configuring Atlantis](https://www.runatlantis.io/docs/configuring-atlantis.html)
- [Server Side Config](https://www.runatlantis.io/docs/server-side-repo-config.html)
- [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html)
- [Server Configuration](https://www.runatlantis.io/docs/server-configuration.html)
- [Atlantis Custom Workflows](https://www.runatlantis.io/docs/custom-workflows.html)
- [Pre Workflow Hooks](https://www.runatlantis.io/docs/pre-workflow-hooks.html)
- [Post Workflow Hooks](https://www.runatlantis.io/docs/post-workflow-hooks.html)
