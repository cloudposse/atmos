# `atlantis` integration

## Atlantis Repo Config Generation

`atmos` supports generating [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html) for `atmos` components
and stacks.

The following `atmos` commands will first generate the varfiles for all components in all stacks,
then generate the `atlantis.yaml` repo config file:

```bash
  atmos terraform generate varfiles --file-template=varfiles/{tenant}-{environment}-{stage}-{component}.tfvars.json
  atmos atlantis generate repo-config --config-template config-template-1 --project-template project-template-1 --workflow-template workflow-template-1
```

Supported context tokens: `{namespace}`, `{tenant}`, `{environment}`, `{region}`, `{stage}`, `{component}`, `{component-path}`.

You can run these commands manually and commit the generated varfiles and `atlantis.yaml` repo config.

If you want to generate `atlantis.yaml` on the server dynamically,
you can add the following run commands to [pre_workflow_hooks](https://www.runatlantis.io/docs/pre-workflow-hooks.html#pre-workflow-hooks).
The `atlantis.yaml` repo config file will be generated right before Atlantis parses it.

```yaml
repos:
  - id: /.*/
    pre_workflow_hooks:
      - run: |
          atmos terraform generate varfiles --file-template=varfiles/{tenant}-{environment}-{stage}-{component}.tfvars.json
          atmos atlantis generate repo-config --config-template config-template-1 --project-template project-template-1 --workflow-template workflow-template-1
```

Note that the `-file-template` parameter in the `atmos terraform generate varfiles` command must match the following two settings in `atmos.yaml`:

- `when_modified` must use the same template with the context tokens - this will allow Atlantis to check if any of the generated variables were
  modified
- workflow `extra_args` must use the same template with the context tokens - this will allow Atlantis to run Terraform commands with the
  correct `-var-file` parameters

```yaml
# atmos.yaml CLI config

# Integrations
integrations:

  # Atlantis integration
  # https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html
  atlantis:
    # Path and name of the Atlantis config file `atlantis.yaml`
    # Can be overridden on the command line by using `--output-path` command-line argument in `atmos atlantis generate repo-config` command
    # If not specified (can be empty here, or set to an empty string on the command line), the content of the file will be dumped to `stdout`
    path: "atlantis.yaml"

    # Config templates
    # Select a template by using the `--config-template <config_template>` command-line argument in `atmos atlantis generate repo-config` command
    config_templates:
      config-template-1:
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
      project-template-1:
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
            - "varfiles/{tenant}-{environment}-{stage}-{component}.tfvars.json"
          apply_requirements:
            - "approved"

    # Workflow templates
    # Select a template by using the `--workflow-template <workflow_template>` command-line argument in `atmos atlantis generate repo-config` command
    workflow_templates:
      # generate a workflow entry for each component in every stack
      workflow-template-1:
        plan:
          steps:
            - init
            - plan:
                extra_args:
                  - "-var-file"
                  - "varfiles/{tenant}-{environment}-{stage}-{component}.tfvars.json"
        apply:
          steps:
            - apply:
                extra_args:
                  - "-var-file"
                  - "varfiles/{tenant}-{environment}-{stage}-{component}.tfvars.json"
```

Using the config, project and workflow templates, `atmos` generates a separate `atlantis` project for each `atmos` component in every stack:

```yaml
version: 3
automerge: true
delete_source_branch_on_merge: true
parallel_plan: true
parallel_apply: true
projects:
  - name: tenant1-ue2-staging-test-test-component-override-3
    workspace: test-component-override-3-workspace
    workflow: workflow-tenant1-ue2-staging-test-test-component-override-3
    dir: examples/complete/components/terraform/test/test-component
    terraform_version: v1.2
    delete_source_branch_on_merge: true
    autoplan:
      enabled: true
      when_modified:
        - '**/*.tf'
        - varfiles/tenant1-ue2-staging-test-test-component-override-3.tfvars.json
      apply_requirements:
        - approved
  - name: tenant1-ue2-staging-infra-vpc
    workspace: tenant1-ue2-staging
    workflow: workflow-tenant1-ue2-staging-infra-vpc
    dir: examples/complete/components/terraform/infra/vpc
    terraform_version: v1.2
    delete_source_branch_on_merge: true
    autoplan:
      enabled: true
      when_modified:
        - '**/*.tf'
        - varfiles/tenant1-ue2-staging-infra-vpc.tfvars.json
      apply_requirements:
        - approved
workflows:
  workflow-tenant1-ue2-staging-test-test-component-override-3:
    apply:
      steps:
        - apply:
            extra_args:
              - -var-file
              - varfiles/tenant1-ue2-staging-test-test-component-override-3.tfvars.json
    plan:
      steps:
        - init
        - plan:
            extra_args:
              - -var-file
              - varfiles/tenant1-ue2-staging-test-test-component-override-3.tfvars.json
    workflow-tenant1-ue2-staging-infra-vpc:
      apply:
        steps:
          - apply:
              extra_args:
                - -var-file
                - varfiles/tenant1-ue2-staging-infra-vpc.tfvars.json
      plan:
        steps:
          - init
          - plan:
              extra_args:
                - -var-file
                - varfiles/tenant1-ue2-staging-infra-vpc.tfvars.json
```

## References

- [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html)
- [Pre-workflow Hooks](https://www.runatlantis.io/docs/pre-workflow-hooks.html#pre-workflow-hooks)
- [Dynamic Repo Config Generation](https://www.runatlantis.io/docs/pre-workflow-hooks.html#dynamic-repo-config-generation)
