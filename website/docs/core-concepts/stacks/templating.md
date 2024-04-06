---
title: Stack Manifest Templating
sidebar_position: 7
sidebar_label: Templating
id: templating
---

Atmos supports [Go templates](https://pkg.go.dev/text/template) in stack manifests.
[Sprig Functions](https://masterminds.github.io/sprig/) and [Gomplate Functions](https://docs.gomplate.ca/functions/)
are supported as well.

You can use `Go` templates in the following Atmos section to refer to values in the same or other sections:

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
          # `provisioned_at` uses the Sprig functions
          # https://masterminds.github.io/sprig/date.html
          # https://pkg.go.dev/time#pkg-constants
          provisioned_at: '{{ dateInZone "2006-01-02T15:04:05Z07:00" (now) "UTC" }}'
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
    atmos_manifest: orgs/acme/plat/dev/us-east-2
    atmos_stack: plat-ue2-dev
    description: vpc component provisioned in plat-ue2-dev stack by assuming IAM role <role-arn>
    provisioned_at: "2024-03-12T16:18:24Z"
    region: us-east-2
    terraform_workspace: plat-ue2-dev
```

<br/>

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
      provisioned_at: '{{ dateInZone "2006-01-02T15:04:05Z07:00" (now) "UTC" }}'
```

the tags will be processed and automatically added to all the resources provisioned in the infrastructure.
