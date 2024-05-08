---
title: Stack Manifest Templating
sidebar_position: 7
sidebar_label: Templating
id: templating
---

Atmos supports [Go templates](https://pkg.go.dev/text/template) in stack manifests.

[Sprig Functions](https://masterminds.github.io/sprig/), [Gomplate Functions](https://docs.gomplate.ca/functions/)
and [Gomplate Datasources](https://docs.gomplate.ca/datasources/)
are supported as well.

## Important Note

Atmos supports two different ways of configuring and using `Go` templates:

- In [Imports](/core-concepts/stacks/imports)
- In [Stack Manifests](/core-concepts/stacks)

These templates are processed in different phases and use different context:

- [`Go` Templates in Imports](/core-concepts/stacks/imports#go-templates-in-imports) are used in imported stack 
  manifests to make them DRY and reusable. The context (variables) for the `Go` templates is provided via the static 
  `context` section. Atmos processes `Go` templates in imports as the very first phase of the stack processing pipeline.
  When executing the [CLI commands](/cli/commands), Atmos parses and executes the templates using the provided static 
  `context`, processes all imports, and finds stacks and components

- `Go` templates in Atmos stack manifests, on the other hand, are processed as the very last phase of the stack processing 
  pipeline (after all imports are processed, all stack configurations are deep-merged, and the stack and component are found).
  For the context (template variables), it uses all the component's attributes returned from the 
  [`atmos describe component`](/cli/commands/describe/component) command

These two mechanisms, although both using `Go` templates, serve different purposes, use different contexts, and are executed 
in different phases of the stack processing pipeline.

For more details, refer to:

- [`Go` Templates in Imports](/core-concepts/stacks/imports#go-templates-in-imports)
- [Excluding templates in imports from processing by Atmos](#excluding-templates-in-stack-manifest-from-processing-by-atmos)


## Stack Manifest Templating Configuration

Templating in Atmos stack manifests can be configured in the following places:

- In the `templates.settings` section in `atmos.yaml` [CLI config file](/cli/configuration)

- In the `settings.templates.settings` section in [Atmos stack manifests](/core-concepts/stacks).
  The `settings.templates.settings` section can be defined globally per organization, tenant, account, or per component.
  Atmos deep-merges the configurations from all scopes into the final result using [inheritance](/core-concepts/components/inheritance).

### Configuring Templating in `atmos.yaml` CLI Config File

Templating in Atmos stack manifests is configured in the `atmos.yaml` [CLI config file](/cli/configuration) in the
`templates.settings` section:

```yaml title="atmos.yaml"
# https://pkg.go.dev/text/template
templates:
  settings:
    # Enable `Go` templates in Atmos stack manifests
    enabled: true
    # Number of steps/passes to process `Go` templates
    # If not defined, `num_steps` is automatically set to `1`
    num_steps: 2
    # Optional steps configuration
    steps:
      1:
        # Left delimiter for step #1
        left_delimiter: "${"
        # Right delimiter for step #1
        right_delimiter: "}"
      2:
        # Left delimiter for step #2
        left_delimiter: "{{"
        # Right delimiter for step #2
        right_delimiter: "}}"
    # Environment variables to use when executing templates
    # https://docs.gomplate.ca/datasources/#using-awssmp-datasources
    # https://docs.gomplate.ca/functions/aws/#configuring-aws
    # https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html
    env:
      AWS_PROFILE: "<AWS profile>"
      AWS_TIMEOUT: 2000
    # https://masterminds.github.io/sprig
    sprig:
      # Enable Sprig functions in `Go` templates in Atmos stack manifests
      enabled: true
    # https://docs.gomplate.ca
    # https://docs.gomplate.ca/functions
    gomplate:
      # Enable Gomplate functions and datasources in `Go` templates in Atmos stack manifests
      enabled: true
      # Timeout in seconds to execute the datasources
      timeout: 5
      # https://docs.gomplate.ca/datasources
      datasources:
        # 'http' datasource
        # https://docs.gomplate.ca/datasources/#using-file-datasources
        ip:
          url: "https://api.ipify.org?format=json"
          # https://docs.gomplate.ca/datasources/#sending-http-headers
          # https://docs.gomplate.ca/usage/#--datasource-header-h
          headers:
            accept:
              - "application/json"
        # This `random` datasource uses `Go` templates in the `url`
        # and will be processed in two steps/passes:
        # 1) process the template tokens using the delimiters `${ }` configured in step #1
        # 2) execute the datasource itself using the delimiters `{{ }}` configured in step #2
        random:
          url: "http://www.randomnumberapi.com/api/v1.0/randomstring?min=${ .settings.random.min }&max=${ .settings.random.max }&count=1"
        # 'file' datasources
        # https://docs.gomplate.ca/datasources/#using-file-datasources
        config-1:
          url: "./config1.json"
        config-2:
          url: "file:///config2.json"
        # `aws+smp` AWS Systems Manager Parameter Store datasource
        # https://docs.gomplate.ca/datasources/#using-awssmp-datasources
        secret-1:
          url: "aws+smp:///path/to/secret"
        # `aws+sm` AWS Secrets Manager datasource
        # https://docs.gomplate.ca/datasources/#using-awssm-datasource
        secret-2:
          url: "aws+sm:///path/to/secret"
        # `s3` datasource
        # https://docs.gomplate.ca/datasources/#using-s3-datasources
        s3-config:
          url: "s3://mybucket/config/config.json"
```

- `templates.settings.enabled` - a boolean flag to enable/disable the processing of `Go` templates in Atmos stack manifests.
  If set to `false`, Atmos will not process `Go` templates in stack manifests

- `templates.settings.env` - a map of environment variables to use when executing the templates

- `templates.settings.num_steps` - number of steps/passes to process `Go` templates. If not defined, `num_steps` 
  is automatically set to `1`

- `templates.settings.steps` - a map of step configurations to process `Go` templates. The keys of the map are the step
  numbers. The values of the map are objects with the following schema:

  - `left_delimiter` - the left delimiter to use to process the templates in this step/pass. If not defined, the default
    delimiter `{{` will be used
  - `right_delimiter` - the right delimiter to use to process the templates in this step/pass. If not defined, the default
    delimiter `}}` will be used

  If `templates.settings.num_steps` is not configured, all steps/passes (defined by `templates.settings.num_steps`) will 
  use the default delimiters `{{ }}`.

- `templates.settings.sprig.enabled` - a boolean flag to enable/disable the [Sprig Functions](https://masterminds.github.io/sprig/)
  in Atmos stack manifests

- `templates.settings.gomplate.enabled` - a boolean flag to enable/disable the [Gomplate Functions](https://docs.gomplate.ca/functions/) 
  and [Gomplate Datasources](https://docs.gomplate.ca/datasources) in Atmos stack manifests

- `templates.settings.gomplate.timeout` - timeout in seconds to execute [Gomplate Datasources](https://docs.gomplate.ca/datasources)

- `templates.settings.gomplate.datasources` - a map of [Gomplate Datasource](https://docs.gomplate.ca/datasources) definitions:

  - The keys of the map are the datasource names (aliases), which are used in `Go` templates in Atmos stack manifests.
    For example:

    ```yaml
     terraform:
       vars:
         tags:
           provisioned_by_ip: '{{ (datasource "ip").ip }}'
           config1_tag: '{{ (datasource "config-1").tag }}'
           config2_service_name: '{{ (datasource "config-2").service.name }}'
    ```

  - The values of the map are the datasource definitions with the following schema:

    - `url` - the [Datasource URL](https://docs.gomplate.ca/datasources/#url-format)

    - `headers` - a map of [HTTP request headers](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers) for
      the [`http` datasource](https://docs.gomplate.ca/datasources/#sending-http-headers).
      The keys of the map are the header names. The values of the map are lists of values for the header.

      The following configuration will result in the
      [`accept: application/json`](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Accept) HTTP header
      being sent with the HTTP request to the datasource:

         ```yaml
         headers:
           accept:
             - "application/json"
        ```

<br/>

:::warning

Some functions are present in both [Sprig](https://masterminds.github.io/sprig/) and [Gomplate](https://docs.gomplate.ca/functions/).

For example, the `env` function has the same name in [Sprig](https://masterminds.github.io/sprig/os.html) and
[Gomplate](https://docs.gomplate.ca/functions/env/), but has different syntax and accept different number of arguments.

If you use the `env` function from one templating engine and enable both [Sprig](https://masterminds.github.io/sprig/)
and [Gomplate](https://docs.gomplate.ca/functions/), it will be invalid in the other templating engine, and an error will be thrown.

To be able to use the `env` function from both templating engines, you can do one of the following:

- Use the `env` function from one templating engine, and disable the other templating engine by using the
  `templates.settings.sprig.enabled` and `templates.settings,gomplate.enabled` settings

- Enable both engines and use the Gomplate's `env` function via its 
  [`getenv`](https://docs.gomplate.ca/functions/env/#examples) alias

:::

<br/>

### Configuring Templating in Atmos Stack Manifests

Templating in Atmos can also be configured in the `settings.templates.settings` section in stack manifests.

The `settings.templates.settings` section can be defined globally per organization, tenant, account, or per component.
Atmos deep-merges the configurations from all scopes into the final result using [inheritance](/core-concepts/components/inheritance).

The schema is the same as `templates.settings` in the `atmos.yaml` [CLI config file](/cli/configuration),
except the following settings are not supported in the `settings.templates.settings` section:

- `settings.templates.settings.enabled`
- `settings.templates.settings.sprig.enabled`
- `settings.templates.settings.gomplate.enabled`
- `settings.templates.settings.num_steps`
- `settings.templates.settings.steps`

The reasons these settings are not supported are:

- You can't disable templating in the stack manifests which Atmos needs to process as `Go` templates 

- If you define the `left_delimiter` and `right_delimiter` in the `settings.templates.settings` section in stack manifests, 
  the `Go` templating engine will think that the delimiters specify the beginning and the end of template strings, will 
  try to evaluate it, which will result in an error

As an example, let's define [Gomplate Datasources](https://docs.gomplate.ca/datasources/) for the entire organization in the
`stacks/orgs/acme/_defaults.yaml` stack manifest:

```yaml title="stacks/orgs/acme/_defaults.yaml"
settings:
  templates:
    settings:
      # Environment variables to use when executing templates
      # https://docs.gomplate.ca/datasources/#using-awssmp-datasources
      # https://docs.gomplate.ca/functions/aws/#configuring-aws
      # https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html
      env:
        AWS_PROFILE: "<AWS profile>"
        AWS_TIMEOUT: 2000
      gomplate:
        # 7 seconds timeout to execute the datasources
        timeout: 7
        # https://docs.gomplate.ca/datasources
        datasources:
          # 'file' datasources
          # https://docs.gomplate.ca/datasources/#using-file-datasources
          config-1:
            url: "./my-config1.json"
          config-3:
            url: "file:///config3.json"
```

Atmos deep-merges the configurations from the `settings.templates.settings` section in [Atmos stack manifests](/core-concepts/stacks)
with the `templates.settings` section in `atmos.yaml` [CLI config file](/cli/configuration) using [inheritance](/core-concepts/components/inheritance).

The `settings.templates.settings` section in [Atmos stack manifests](/core-concepts/stacks) takes precedence over 
the `templates.settings` section in `atmos.yaml` [CLI config file](/cli/configuration), allowing you to define the global
`datasources` in `atmos.yaml` and then add or override `datasources` in Atmos stack manifests for the entire organization,
tenant, account, or per component.

For example, taking into account the configurations described above in `atmos.yaml` [CLI config file](/cli/configuration) 
and in the `stacks/orgs/acme/_defaults.yaml` stack manifest, the final `datasources` map will look like this:

```yaml
gomplate:
  timeout: 7
  datasources:
    ip:
      url: "https://api.ipify.org?format=json"
      headers:
        accept:
          - "application/json"
    random:
      url: "http://www.randomnumberapi.com/api/v1.0/randomstring?min=${ .settings.random.min }&max=${ .settings.random.max }&count=1"
    secret-1:
      url: "aws+smp:///path/to/secret"
    secret-2:
      url: "aws+sm:///path/to/secret"
    s3-config:
      url: "s3://mybucket/config/config.json"
    config-1:
      url: "./my-config1.json"
    config-2:
      url: "file:///config2.json"
    config-3:
      url: "file:///config3.json"
```

Note that the `config-1` datasource from `atmos.yaml` was overridden with the `config-1` datasource from the 
`stacks/orgs/acme/_defaults.yaml` stack manifest. The `timeout` attribute was overridden as well.

You can now use the `datasources` in `Go` templates in all Atmos sections that support `Go` templates.

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
          # Examples of using the Sprig and Gomplate functions and datasources
          # https://masterminds.github.io/sprig/os.html
          provisioned_by_user: '{{ env "USER" }}'
          # https://docs.gomplate.ca/functions/strings
          atmos_component_description: "{{ strings.Title .atmos_component }} component {{ .vars.name | strings.Quote }} provisioned in the stack {{ .atmos_stack | strings.Quote }}"
          # https://docs.gomplate.ca/datasources
          provisioned_by_ip: '{{ (datasource "ip").ip }}'
          config1_tag: '{{ (datasource "config-1").tag }}'
          config2_service_name: '{{ (datasource "config-2").service.name }}'
          config3_team_name: '{{ (datasource "config-3").team.name }}'
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
    config1_tag: test1
    config2_service_name: service1
    config3_team_name: my-team
    description: vpc component provisioned in plat-ue2-dev stack by assuming IAM role <role-arn>
    provisioned_by_user: <user>
    provisioned_by_ip: 167.38.132.237
    region: us-east-2
    terraform_workspace: plat-ue2-dev
```

## Environment Variables

Environment variables to use when processing and executing templates can be defined in the `env` map.
It's supported in both the `templates.settings` section in `atmos.yaml` [CLI config file](/cli/configuration) and in the 
`settings.templates.settings` section in [Atmos stack manifests](/core-concepts/stacks).

For example:

```yaml
settings:
  templates:
    settings:
      # Environment variables to use when executing templates
      # https://docs.gomplate.ca/datasources/#using-awssmp-datasources
      # https://docs.gomplate.ca/functions/aws/#configuring-aws
      # https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html
      env:
        AWS_PROFILE: "<AWS profile>"
        AWS_TIMEOUT: 2000
```

This is useful when executing the `datasources` that need to authenticate to cloud APIs. 

For more details, refer to:

- [Configuring AWS](https://docs.gomplate.ca/functions/aws/#configuring-aws)
- [Configuring GCP](https://docs.gomplate.ca/functions/gcp/#configuring-gcp)

## Datasources

Currently, Atmos supports all the [Gomplate Datasources](https://docs.gomplate.ca/datasources).
More datasources will be added in the future (and this doc will be updated).

The [Gomplate Datasources](https://docs.gomplate.ca/datasources) can be configured in the 
`templates.settings.gomplate.datasources` section in `atmos.yaml` 
[CLI config file](/cli/configuration) and in the `settings.templates.settings` section in 
[Atmos stack manifests](/core-concepts/stacks).

The `templates.settings.gomplate.datasources` section is a map of objects.

The keys of the map are the datasource names (aliases).

The values of the map are the datasource definitions with the following schema:

  - `url` - the [Datasource URL](https://docs.gomplate.ca/datasources/#url-format)

  - `headers` - a map of [HTTP request headers](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers) for
    the [`http` datasource](https://docs.gomplate.ca/datasources/#sending-http-headers).
    The keys of the map are the header names. The values of the map are lists of values for the header.

    The following configuration will result in the
    [`accept: application/json`](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Accept) HTTP header
    being sent with the HTTP request to the datasource:

       ```yaml
       headers:
         accept:
           - "application/json"
      ```

For example, let's define the following Gomplate datasources in the global `settings` section (this will apply to all
components in all stacks in the infrastructure):

```yaml
settings:
  templates:
    settings:
      gomplate:
        # Timeout in seconds to execute the datasources
        timeout: 5
        # https://docs.gomplate.ca/datasources
        datasources:
          # 'http' datasource
          # https://docs.gomplate.ca/datasources/#using-file-datasources
          ip:
            url: "https://api.ipify.org?format=json"
            # https://docs.gomplate.ca/datasources/#sending-http-headers
            # https://docs.gomplate.ca/usage/#--datasource-header-h
            headers:
              accept:
                - "application/json"
          # This `random` datasource uses `Go` templates in the `url`
          # and will be processed in two steps/passes:
          # 1) process the template tokens using the delimiters `${ }` configured in step #1
          # 2) execute the datasource itself using the delimiters `{{ }}` configured in step #2
          random:
            url: "http://www.randomnumberapi.com/api/v1.0/randomstring?min=${ .settings.random.min }&max=${ .settings.random.max }&count=1"
          # 'file' datasources
          # https://docs.gomplate.ca/datasources/#using-file-datasources
          config-1:
            url: "./config1.json"
          config-2:
            url: "file:///config2.json"
          # `aws+smp` AWS Systems Manager Parameter Store datasource
          # https://docs.gomplate.ca/datasources/#using-awssmp-datasources
          secret-1:
            url: "aws+smp:///path/to/secret"
          # `aws+sm` AWS Secrets Manager datasource
          # https://docs.gomplate.ca/datasources/#using-awssm-datasource
          secret-2:
            url: "aws+sm:///path/to/secret"
          # `s3` datasource
          # https://docs.gomplate.ca/datasources/#using-s3-datasources
          s3-config:
            url: "s3://mybucket/config/config.json"
```

After the above `datasources` are defined, you can use them in Atmos stack manifests like this:

```yaml
terraform:
 vars:
   tags:
     tag1: '{{ (datasource "config-1").tag }}'
     service_name2: '{{ (datasource "config-2").service.name }}'
     service_name3: '{{ (datasource "s3-config").config.service_name }}'

components:
  terraform:
    my-component-1:
     settings:
       provisioned_by_ip: '{{ (datasource "ip").ip }}'
       secret-1: '{{ (datasource "secret-1").secret1.value }}'
     vars:
       enabled: '{{ (datasource "config-2").config.enabled }}'
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

## Excluding templates in stack manifest from processing by Atmos

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

## Excluding templates in imports from processing by Atmos

If you are using [`Go` Templates in Imports](/core-concepts/stacks/imports#go-templates-in-imports) and `Go` templates
in stack manifests in the same Atmos manifest, take into account that in this case Atmos will do `Go` 
template processing two times (two passes):

  - When importing the manifest and processing the template tokens using the variables from the provided `context` object
  - After finding the component in the stack as the final step in the processing pipeline

<br/>

For example, we can define the following configuration in the `stacks/catalog/eks/eks_cluster.tmpl` template file:

```yaml title="stacks/catalog/eks/eks_cluster.tmpl"
components:
  terraform:
    eks/cluster:
      metadata:
        component: eks/cluster
      vars:
        enabled: "{{ .enabled }}"
        name: "{{ .name }}"
        tags:
          atmos_component: "{{ .atmos_component }}"
          atmos_stack: "{{ .atmos_stack }}"
          terraform_workspace: "{{ .workspace }}"
```

<br/>

Then we import the template into a top-level stack providing the context variables for the import in the `context` object:

```yaml title="stacks/orgs/acme/plat/prod/us-east-2.yaml"
import:
  - path: "catalog/eks/eks_cluster.tmpl"
    context:
      enabled: true
      name: prod-eks
```

Atmos will process the import and replace the template tokens using the variables from the `context`.
Since the `context` does not provide the variables for the template tokens in `tags`, the following manifest will be
generated:

```yaml
components:
  terraform:
    eks/cluster:
      metadata:
        component: eks/cluster
      vars:
        enabled: true
        name: prod-eks
        tags:
          atmos_component: <no value>
          atmos_stack: <no value>
          terraform_workspace: <no value>
```

<br/>

The second pass of template processing will not replace the tokens in `tags` because they are already processed in the 
first pass (importing) and the values `<no value>` are generated.

To deal with this, use **double curly braces + backtick + double curly braces** instead of just **double curly braces**
in `tags` to prevent Atmos from processing the templates in the first pass and instead process them in the second pass:

```yaml title="stacks/catalog/eks/eks_cluster.tmpl"
components:
  terraform:
    eks/cluster:
      metadata:
        component: eks/cluster
      vars:
        enabled: "{{ .enabled }}"
        name: "{{ .name }}"
        tags:
          atmos_component: "{{`{{ .atmos_component }}`}}"
          atmos_stack: "{{`{{ .atmos_stack }}`}}"
          terraform_workspace: "{{`{{ .workspace }}`}}"
```

<br/>

Atmos will first process the import and replace the template tokens using the variables from the `context`.
Then in the second pass the tokens in `tags` will be replaced with the correct values.

It will generate the following manifest:

```yaml
components:
  terraform:
    eks/cluster:
      metadata:
        component: eks/cluster
      vars:
        enabled: true
        name: prod-eks
        tags:
          atmos_component: eks/cluster
          atmos_stack: plat-ue2-prod
          terraform_workspace: plat-ue2-prod
```


## Template Steps and Template Processing Pipelines
