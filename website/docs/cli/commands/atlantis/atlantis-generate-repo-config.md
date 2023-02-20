---
title: atmos atlantis generate repo-config
sidebar_label: generate repo-config
sidebar_class_name: command
id: generate-repo-config
description: Use this command to generate a repository configuration for Atlantis.
---

:::info Purpose
Use this command to generate a repository configuration for Atlantis.
:::

<br/>

```shell
atmos atmos atlantis generate repo-config [options]
```

<br/>

:::tip
Run `atmos atlantis generate repo-config --help` to see all the available options
:::

## Examples

```shell
atmos atlantis generate repo-config

atmos atlantis generate repo-config --output-path /dev/stdout

atmos atlantis generate repo-config --config-template config-1 --project-template project-1

atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --stacks <stack1, stack2>

atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --components <component1, component2>

atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --stacks <stack1> --components <component1, component2>

atmos atlantis generate repo-config --affected-only=true

atmos atlantis generate repo-config --affected-only=true --output-path /dev/stdout

atmos atlantis generate repo-config --affected-only=true --verbose=true

atmos atlantis generate repo-config --affected-only=true --output-path /dev/stdout --verbose=true

atmos atlantis generate repo-config --affected-only=true --repo-path <path_to_cloned_target_repo>

atmos atlantis generate repo-config --affected-only=true --ref refs/heads/main

atmos atlantis generate repo-config --affected-only=true --ref refs/tags/v1.1.0

atmos atlantis generate repo-config --affected-only=true --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073

atmos atlantis generate repo-config --affected-only=true --ref refs/tags/v1.2.0 --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073

atmos atlantis generate repo-config --affected-only=true --ssh-key <path_to_ssh_key>

atmos atlantis generate repo-config --affected-only=true --ssh-key <path_to_ssh_key> --ssh-key-password <password>
```

## Flags

| Flag                 | Description                                                                                                                                                      | Required |
|:---------------------|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------|:---------|
| `--config-template`  | Atlantis config template name                                                                                                                                    | no       |
| `--project-template` | Atlantis project template name                                                                                                                                   | no       |
| `--output-path`      | Output path to write `atlantis.yaml` file                                                                                                                        | no       |
| `--stacks`           | Generate Atlantis projects for the specified stacks only (comma-separated values)                                                                                | no       |
| `--components`       | Generate Atlantis projects for the specified components only (comma-separated values)                                                                            | no       |
| `--affected-only`    | Generate Atlantis projects only for the Atmos components changed<br/>between two Git commits                                                                     | no       |
| `--ref`              | [Git Reference](https://git-scm.com/book/en/v2/Git-Internals-Git-References) with which to compare the current working branch                                    | no       |
| `--sha`              | Git commit SHA with which to compare the current working branch                                                                                                  | no       |
| `--ssh-key`          | Path to PEM-encoded private key to clone private repos using SSH                                                                                                 | no       |
| `--ssh-key-password` | Encryption password for the PEM-encoded private key if the key contains<br/>a password-encrypted PEM block                                                       | no       |
| `--repo-path`        | Path to the already cloned target repository with which to compare the current branch.<br/>Conflicts with `--ref`, `--sha`, `--ssh-key` and `--ssh-key-password` | no       |
| `--verbose`          | Print more detailed output when cloning and checking out the target<br/>Git repository and processing the result                                                 | no       |

## Atlantis Workflows

Atlantis workflows can be defined in two places:

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

:::info

Refer to [Atlantis Integration](/integrations/atlantis.md) for more details on the Atlantis integration in Atmos

:::

<br/>

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

<br/>

:::info

For more information, refer to:

- [Configuring Atlantis](https://www.runatlantis.io/docs/configuring-atlantis.html)
- [Server Side Config](https://www.runatlantis.io/docs/server-side-repo-config.html)
- [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html)
- [Server Configuration](https://www.runatlantis.io/docs/server-configuration.html)
- [Atlantis Custom Workflows](https://www.runatlantis.io/docs/custom-workflows.html)

:::
