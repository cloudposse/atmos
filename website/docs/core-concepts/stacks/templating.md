---
title: Stack Manifest Templating
sidebar_position: 7
sidebar_label: Templating
id: templating
---

Atmos supports [Go templates](https://pkg.go.dev/text/template) in stack manifests.
[Sprig Functions](https://masterminds.github.io/sprig/) and [Gomplate Functions](https://docs.gomplate.ca/functions/)
are supported as well.

## Configuration

Templating in Atmos stack manifests is configured in the `atmos.yaml` [CLI config file](/cli/configuration) in the
`templates` section:

```yaml title="atmos.yaml"
# https://pkg.go.dev/text/template
templates:
  settings:
    enabled: true
    # https://masterminds.github.io/sprig
    sprig:
      enabled: true
    # https://docs.gomplate.ca
    gomplate:
      enabled: true
```

- `templates.settings.enabled` - a boolean flag to enable/disable the processing of `Go` templates in Atmos stack manifests. 
  If set to `false`, Atmos will not process `Go` templates in stack manifests

- `templates.settings.sprig.enabled` - a boolean flag to enable/disable the [Sprig Functions](https://masterminds.github.io/sprig/)
  in Atmos stack manifests

- `templates.settings.gomplate.enabled` - a boolean flag to enable/disable the [Gomplate Functions](https://docs.gomplate.ca/functions/)
  in Atmos stack manifests

<br/>

:::warning

Some functions are present in both [Sprig](https://masterminds.github.io/sprig/) and [Gomplate](https://docs.gomplate.ca/functions/).

For example, the `env` function has the same name in [Sprig](https://masterminds.github.io/sprig/os.html) and
[Gomplate](https://docs.gomplate.ca/functions/env/), but has different syntax and accept different number of arguments.

If you use the `env` function from one templating engine and enable both [Sprig](https://masterminds.github.io/sprig/)
and [Gomplate](https://docs.gomplate.ca/functions/), it will be invalid in the other templating engine, and an error will be thrown.

For this reason, you can use the `templates.settings.sprig.enabled` and `templates.settings,gomplate.enabled` settings to selectively
enable/disable the [Sprig](https://masterminds.github.io/sprig/) and [Gomplate](https://docs.gomplate.ca/functions/)
functions.

:::

<br/>

## Atmos sections supporting `Go` templates

You can use `Go` templates in the following Atmos sections to refer to values in the same or other sections:

  - `vars`
  - `settings`
  - `env`
  - `metadata`
  - `providers`
  - `overrides`
  - `backend`
  - `backend_type`

<br/>

:::tip
In the template tokens, you can refer to any value in any section that the Atmos command 
[`atmos describe component <component> -s <stack>`](/cli/commands/describe/component) generates
:::

<br/>

For example, let's say we have the following component configuration using `Go` templates:

```yaml
component:
  terraform:
    vpc:
      settings:
        setting1: 1
        setting2: 2
        setting3: "{{ .vars.var3 }}"
        setting4: "{{ .settings.setting1 }}"
        component: vpc
        backend_type: s3
        region: "us-east-2"
        assume_role: "<role-arn>"
      backend_type: "{{ .settings.backend_type }}"
      metadata:
        component: "{{ .settings.component }}"
      providers:
        aws:
          region: "{{ .settings.region }}"
          assume_role: "{{ .settings.assume_role }}"
      env:
        ENV1: e1
        ENV2: "{{ .settings.setting1 }}-{{ .settings.setting2 }}"
      vars:
        var1: "{{ .settings.setting1 }}"
        var2: "{{ .settings.setting2 }}"
        var3: 3
        # Add the tags to all the resources provisioned by this Atmos component
        tags:
          atmos_component: "{{ .atmos_component }}"
          atmos_stack: "{{ .atmos_stack }}"
          atmos_manifest: "{{ .atmos_stack_file }}"
          region: "{{ .vars.region }}"
          terraform_workspace: "{{ .workspace }}"
          assumed_role: "{{ .providers.aws.assume_role }}"
          description: "{{ .atmos_component }} component provisioned in {{ .atmos_stack }} stack by assuming IAM role {{ .providers.aws.assume_role }}"
          # Examples of using the Gomplate and Sprig functions
          # https://docs.gomplate.ca/functions/strings
          atmos_component_description: "{{ strings.Title .atmos_component }} component {{ .vars.name | strings.Quote }} provisioned in the stack {{ .atmos_stack | strings.Quote }}"
          # https://masterminds.github.io/sprig/os.html
          provisioned_by_user: '{{ env "USER" }}'
```

When executing Atmos commands like `atmos describe component` and `atmos terraform plan/apply`, Atmos processes all the template tokens 
in the manifest and generates the final configuration for the component in the stack:

```yaml title="atmos describe component vpc -s plat-ue2-dev"
settings:
  setting1: 1
  setting2: 2
  setting3: 3
  setting4: 1
  component: vpc
  backend_type: s3
  region: us-east-2
  assume_role: <role-arn>
backend_type: s3
metadata:
  component: vpc
providers:
  aws:
    region: us-east-2
    assume_role: <role-arn>
env:
  ENV1: e1
  ENV2: 1-2
vars:
  var1: 1
  var2: 2
  var3: 3
  tags:
    assumed_role: <role-arn>
    atmos_component: vpc
    atmos_component_description: Vpc component "common" provisioned in the stack "plat-ue2-dev"
    atmos_manifest: orgs/acme/plat/dev/us-east-2
    atmos_stack: plat-ue2-dev
    description: vpc component provisioned in plat-ue2-dev stack by assuming IAM role <role-arn>
    provisioned_by_user: <user>
    region: us-east-2
    terraform_workspace: plat-ue2-dev
```

## Use-cases

While `Go` templates in Atmos stack manifests offer great flexibility for various use-cases, one of the obvious use-cases
is to add a standard set of tags to all the resources in the infrastructure.

For example, by adding this configuration to the `stacks/orgs/acme/_defaults.yaml` Org-level stack manifest:

```yaml title="stacks/orgs/acme/_defaults.yaml"
terraform:
  vars:
    tags:
      atmos_component: "{{ .atmos_component }}"
      atmos_stack: "{{ .atmos_stack }}"
      atmos_manifest: "{{ .atmos_stack_file }}"
      terraform_workspace: "{{ .workspace }}"
      # Examples of using the Gomplate and Sprig functions
      # https://docs.gomplate.ca/functions/strings
      atmos_component_description: "{{ strings.Title .atmos_component }} component {{ .vars.name | strings.Quote }} provisioned in the stack {{ .atmos_stack | strings.Quote }}"
      # https://masterminds.github.io/sprig/os.html
      provisioned_by_user: '{{ env "USER" }}'
```

The tags will be processed and automatically added to all the resources provisioned in the infrastructure.

## Excluding templates from processing by Atmos

If you need to provide `Go` templates to external systems (e.g. ArgoCD or Datadog) verbatim and prevent Atmos from
processing the templates, use **double curly braces + backtick + double curly braces** instead of just **double curly braces**:

```console
{{`{{  instead of  {{

}}`}}  instead of  }}
```

For example:

```yaml
components:
  terraform:

    eks/argocd:
      metadata:
        component: "eks/argocd"
      vars:
        enabled: true
        name: "argocd"
        chart_repository: "https://argoproj.github.io/argo-helm"
        chart_version: 5.46.0

        chart_values:
          template-github-commit-status:
            message: |
              Application {{`{{ .app.metadata.name }}`}} is now running new version.
            webhook:
              github-commit-status:
                method: POST
                path: "/repos/{{`{{ call .repo.FullNameByRepoURL .app.metadata.annotations.app_repository }}`}}/statuses/{{`{{ .app.metadata.annotations.app_commit }}`}}"
                body: |
                  {
                    {{`{{ if eq .app.status.operationState.phase "Running" }}`}} "state": "pending"{{`{{end}}`}}
                    {{`{{ if eq .app.status.operationState.phase "Succeeded" }}`}} "state": "success"{{`{{end}}`}}
                    {{`{{ if eq .app.status.operationState.phase "Error" }}`}} "state": "error"{{`{{end}}`}}
                    {{`{{ if eq .app.status.operationState.phase "Failed" }}`}} "state": "error"{{`{{end}}`}},
                    "description": "ArgoCD",
                    "target_url": "{{`{{ .context.argocdUrl }}`}}/applications/{{`{{ .app.metadata.name }}`}}",
                    "context": "continuous-delivery/{{`{{ .app.metadata.name }}`}}"
                  }
```

When Atmos processes the templates in the manifest shown above, it renders them as raw strings allowing sending
the templates to the external system for processing:

```yaml
chart_values:
  template-github-commit-status:
    message: |
      Application {{ .app.metadata.name }} is now running new version.
    webhook:
      github-commit-status:
        method: POST
        path: "/repos/{{ call .repo.FullNameByRepoURL .app.metadata.annotations.app_repository }}/statuses/{{ .app.metadata.annotations.app_commit }}"
        body: |
          {
            {{ if eq .app.status.operationState.phase "Running" }} "state": "pending"{{end}}
            {{ if eq .app.status.operationState.phase "Succeeded" }} "state": "success"{{end}}
            {{ if eq .app.status.operationState.phase "Error" }} "state": "error"{{end}}
            {{ if eq .app.status.operationState.phase "Failed" }} "state": "error"{{end}},
            "description": "ArgoCD",
            "target_url": "{{ .context.argocdUrl }}/applications/{{ .app.metadata.name }}",
            "context": "continuous-delivery/{{ .app.metadata.name }}"
          }
```

<br/>

The `printf` template function is also supported and can be used instead of **double curly braces + backtick + double curly braces**.

The following examples produce the same result:

```yaml
chart_values:
  template-github-commit-status:
    message: >-
      Application {{`{{ .app.metadata.name }}`}} is now running new version.
```

```yaml
chart_values:
  template-github-commit-status:
    message: "Application {{`{{ .app.metadata.name }}`}} is now running new version."
```

```yaml
chart_values:
  template-github-commit-status:
    message: >-
      {{ printf "Application {{ .app.metadata.name }} is now running new version." }}
```

```yaml
chart_values:
  template-github-commit-status:
    message: '{{ printf "Application {{ .app.metadata.name }} is now running new version." }}'
```
