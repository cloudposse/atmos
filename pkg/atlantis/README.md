# atlantis integration

## Dynamic Repo Config Generation

If you want to generate `atlantis.yaml` before Atlantis can parse it,
you can add the following run command to [pre_workflow_hooks](https://www.runatlantis.io/docs/pre-workflow-hooks.html#pre-workflow-hooks).
The `atlantis.yaml` repo config file will be generated right before Atlantis will parse it.

```yaml
repos:
  - id: /.*/
    pre_workflow_hooks:
      - run: ./repo-config-generator.sh
```

In `repo-config-generator.sh`, you need to first generate the varfiles for all components in all stacks,
then generate `atlantis.yaml` repo config file using the following `atmos` commands:

```shell
atmos terraform generate varfiles --file-template=varfiles/{tenant}-{environment}-{stage}-{component}.tfvars.json

atmos atlantis generate repo-config --config-template config-template-1 --project-template project-template-1 --workflow-template workflow-template-1
```

You can also run these commands manually and commit the generated varfiles and `atlantis.yaml` repo config.

Note that the `-file-template` parameter in the `atmos terraform generate varfiles` command must correspond to the two settings in `atmos.yaml`:

- `when_modified` must use the same template with the context tokens - this will allow Atlantis to check if any of the generated variables were
  modified
- workflow `extra_args` must use the same template with the context tokens - this will allow Atlantis to run Terraform commands with the
  correct `-var-file` parameters

Supported context tokens: `{namespace}`, `{tenant}`, `{environment}`, `{region}`, `{stage}`, `{component}`, `{component-path}`.

```yaml
# Integrations
integrations:

  # Atlantis
  # https://www.runatlantis.io/docs/repo-level-atlantis-yaml.html
  atlantis:
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

Using the config, project and workflow templates, `atmos` generates a separate workflow for each `atlantis` project (which corresponds to an `atmos`
component in a stack):

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
