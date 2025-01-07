# `atlantis` integration

## Atlantis Repo Config Generation

`atmos` supports generating [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html) for `atmos` components
and stacks.

The following `atmos` commands will first generate the varfiles for all components in all stacks,
then generate the `atlantis.yaml` repo config file:

```bash
atmos terraform generate varfiles --file-template=varfiles/{tenant}-{environment}-{stage}-{component}.tfvars.json
atmos atlantis generate repo-config --config-template config-1 --project-template project-1
```

__NOTE:__ All paths, `--file-template` in the `atmos terraform generate varfiles` command, and in the `atlantis` config in `atmos.yaml`,
should be relative to the root of the repo. 

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
          atmos atlantis generate repo-config --config-template config-1 --project-template project-1
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
            # You must output the plan using `-out $PLANFILE` because Atlantis expects plans to be in a specific location
            - run: terraform plan -input=false -refresh -out $PLANFILE -var-file varfiles/$PROJECT_NAME.tfvars.json
        apply:
          steps:
            - run: terraform apply $PLANFILE
```

Using the config, project and workflow templates, `atmos` generates a separate `atlantis` project for each `atmos` component in every stack:

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
    dir: tests/fixtures/scenarios/complete/components/terraform/test/test-component
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
    workflow: workflow-1
    dir: tests/fixtures/scenarios/complete/components/terraform/infra/vpc
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

## References

- [Repo Level atlantis.yaml Config](https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html)
- [Pre-workflow Hooks](https://www.runatlantis.io/docs/pre-workflow-hooks.html#pre-workflow-hooks)
- [Dynamic Repo Config Generation](https://www.runatlantis.io/docs/pre-workflow-hooks.html#dynamic-repo-config-generation)
- [Custom init/plan/apply Commands](https://www.runatlantis.io/docs/custom-workflows.html#custom-init-plan-apply-commands)
