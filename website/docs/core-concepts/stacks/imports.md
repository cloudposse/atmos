---
title: Stack Imports
sidebar_position: 7
sidebar_label: Imports
id: imports
---

Imports are how we reduce duplication of configurations by creating reusable baselines. The imports should be thought of almost like blueprints. Once
a reusable catalog of Stacks exists, robust architectures can be easily created simply by importing those blueprints.

Imports may be used in Stack configuratinos together with [inheritance](/core-concepts/components/inheritance)
and [mixins](/core-concepts/stacks/mixins) to produce an exceptionally DRY configuration in a way that is logically organized and easier to maintain
for your team.

:::info
The mechanics of mixins and inheritance apply only to the [Stack](/core-concepts/stacks) configurations. Atmos knows nothing about the underlying
components (e.g. terraform), and does not magically implement inheritance for HCL. However, by designing highly reusable components that do one thing
well, we're able to achieve many of the same benefits.
:::

## Configuration

To import any stack configuration from the `catalog/`, simply define an `import` section at the top of any [Stack](/core-concepts/stacks)
configuration. Technically, it can be placed anywhere in the file, but by convention we recommend putting it at the top.

```yaml
import:
  - catalog/file1
  - catalog/file2
  - catalog/file2
```

s
The base path for imports is specified in the [`atmos.yaml`](/cli/configuration) in the `stacks.base_path` section.

If no file extension is used, a `.yaml` extension is automatically appended.

It's also possible to specify file extensions, although we do not recommend it.

```yaml
import:
  - catalog/file1.yml
  - catalog/file2.yaml
  - catalog/file2.YAML
```

## Conventions

We recommend placing all baseline "imports" in the `stacks/catalog` folder, however, they can exist anywhere.

Use [mixins](/core-concepts/stacks/mixins) for reusable snippets of configurations that alter the behavior of Stacks in some way.

## Imports Schema

The `import` section supports the following two formats (note that only one format is supported in a stack config file, but different stack
configs can use one format or the other):

- a list of paths to the imported files, for example:

  ```yaml title=stacks/orgs/cp/tenant1/test1/us-east-2.yaml
  import:
    - mixins/region/us-east-2
    - orgs/cp/tenant1/test1/_defaults
    - catalog/terraform/top-level-component1
    - catalog/terraform/test-component
    - catalog/terraform/vpc
    - catalog/helmfile/echo-server
  ```

- a list of objects with the following schema:

  ```yaml
  import:
    - path: "<path_to_imported_file>"
      context: {}
    - path: "<path_to_imported_file>"
      context: {}
  ```

where:

- `path` - the path to the imported file
- `context` - an optional freeform map of context variables that are applied as template variables to the imported file (if the imported file is
  a [Go template](https://pkg.go.dev/text/template))

## Go Templates in Imported Stacks

Atmos supports all the functionality of the [Go templates](https://pkg.go.dev/text/template) in imported stack configurations (including
[functions](https://pkg.go.dev/text/template#hdr-Functions)). Stack configurations can be templatized and then reused with different
settings provided via the import `context` section.

For example, we can define the following configuration for EKS Atmos components in the `catalog/terraform/eks_cluster_tmpl.yaml` template file:

```yaml title=stacks/catalog/terraform/eks_cluster_tmpl.yaml
# Imports can also be parameterized using `Go` templates
import: []

components:
  terraform:
    "eks/cluster-{{ .color }}":
      metadata:
        component: "test/test-component"
      vars:
        enabled: "{{ .enabled }}"
        name: "eks-{{ .color }}"
        service_1_name: "{{ .service_1_name }}"
        service_2_name: "{{ .service_2_name }}"
        tags:
          color: "{{ .color }}"
```

<br/>

:::note

Since `Go` processes templates as text files, we can parameterize the Atmos component name `eks/cluster-{{ .color }}` and any values in any
sections (`vars`, `settings`, `env`, `backend`, etc.), and even the `import` section in the imported file (if the file imports other configurations).

:::

<br/>

Then we can import the template into a top-level stack multiple times providing different context variables to each import:

```yaml title=stacks/orgs/cp/tenant1/test1/us-west-2.yaml
import:
  - path: mixins/region/us-west-2
  - path: orgs/cp/tenant1/test1/_defaults

  # This import with the provided context will dynamically generate 
  # a new Atmos component `eks/cluster-blue` in the stack
  - path: catalog/terraform/eks_cluster_tmpl
    context:
      color: "blue"
      enabled: true
      service_1_name: "blue-service-1"
      service_2_name: "blue-service-2"

  # This import with the provided context will dynamically generate 
  # a new Atmos component `eks/cluster-green` in the stack
  - path: catalog/terraform/eks_cluster_tmpl
    context:
      color: "green"
      enabled: false
      service_1_name: "green-service-1"
      service_2_name: "green-service-2"
```

Now we can execute the following Atmos commands to describe and provision the dynamically generated EKS components into the stack:

```shell
atmos describe component eks/cluster-blue -s tenant1-uw2-test-1
atmos describe component eks/cluster-green -s tenant1-uw2-test-1

atmos terraform apply eks/cluster-blue -s tenant1-uw2-test-1
atmos terraform apply eks/cluster-green -s tenant1-uw2-test-1
```

All the parameterized variables will get their values from the `context`:

```yaml title="atmos describe component eks/cluster-blue -s tenant1-uw2-test-1"
vars:
  enabled: true
  environment: uw2
  name: eks-blue
  namespace: cp
  region: us-west-2
  service_1_name: blue-service-1
  service_2_name: blue-service-2
  stage: test-1
  tags:
    color: blue
  tenant: tenant1
```

```yaml title="atmos describe component eks/cluster-green -s tenant1-uw2-test-1"
vars:
  enabled: true
  environment: uw2
  name: eks-green
  namespace: cp
  region: us-west-2
  service_1_name: green-service-1
  service_2_name: green-service-2
  stage: test-1
  tags:
    color: green
  tenant: tenant1
```

<br/>

Using imports with context and parameterized config files will help you make the configurations extremely DRY,
and is very useful when creating stacks and components
for [EKS blue-green deployment](https://aws.amazon.com/blogs/containers/kubernetes-cluster-upgrade-the-blue-green-deployment-strategy/).

## Related

- [Configure CLI](/quick-start/configure-cli)
- [Create Atmos Stacks](/quick-start/create-atmos-stacks)
