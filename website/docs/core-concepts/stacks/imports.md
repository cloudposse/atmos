---
title: Stack Imports
sidebar_position: 7
sidebar_label: Imports
id: imports
---

Imports are how we reduce duplication of configurations by creating reusable baselines. The imports should be thought of almost like blueprints. Once
a reusable catalog of Stacks exists, robust architectures can be easily created simply by importing those blueprints.

Imports may be used in Stack configurations together with [inheritance](/core-concepts/components/inheritance)
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

## `Go` Templates in Imports

Atmos supports all the functionality of the [Go templates](https://pkg.go.dev/text/template) in imported stack configurations (including
[functions](https://pkg.go.dev/text/template#hdr-Functions)). Stack configurations can be templatized and then reused with different
settings provided via the import `context` section.

For example, we can define the following configuration for EKS Atmos components in the `catalog/terraform/eks_cluster_tmpl.yaml` template file:

```yaml title=stacks/catalog/terraform/eks_cluster_tmpl.yaml
# Imports can also be parameterized using `Go` templates
import: []

components:
  terraform:
    "eks/cluster-{{ .flavor }}":
      metadata:
        component: "test/test-component"
      vars:
        enabled: "{{ .enabled }}"
        name: "eks-{{ .flavor }}"
        service_1_name: "{{ .service_1_name }}"
        service_2_name: "{{ .service_2_name }}"
        tags:
          flavor: "{{ .flavor }}"
```

<br/>

:::note

Since `Go` processes templates as text files, we can parameterize the Atmos component name `eks/cluster-{{ .flavor }}` and any values in any
sections (`vars`, `settings`, `env`, `backend`, etc.), and even the `import` section in the imported file (if the file imports other configurations).

:::

<br/>

Then we can import the template into a top-level stack multiple times providing different context variables to each import:

```yaml title=stacks/orgs/cp/tenant1/test1/us-west-2.yaml
import:
  - path: "mixins/region/us-west-2"
  - path: "orgs/cp/tenant1/test1/_defaults"

  # This import with the provided context will dynamically generate 
  # a new Atmos component `eks/cluster-blue` in the current stack
  - path: "catalog/terraform/eks_cluster_tmpl"
    context:
      flavor: "blue"
      enabled: true
      service_1_name: "blue-service-1"
      service_2_name: "blue-service-2"

  # This import with the provided context will dynamically generate 
  # a new Atmos component `eks/cluster-green` in the current stack
  - path: "catalog/terraform/eks_cluster_tmpl"
    context:
      flavor: "green"
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

All the parameterized variables get their values from the `context`:

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
    flavor: blue
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
    flavor: green
  tenant: tenant1
```

<br/>

## Hierarchical Imports with Context

Atmos supports hierarchical imports with context.
This will allow you to parameterize the entire chain of stack configurations and dynamically generate components in stacks.

For example, let's create the configuration `stacks/catalog/terraform/eks_cluster_tmpl_hierarchical.yaml` with the following content:

```yaml title=stacks/catalog/terraform/eks_cluster_tmpl_hierarchical.yaml
import:
  # Use `region_tmpl` `Go` template and provide `context` for it.
  # This can also be done by using `Go` templates in the import path itself.
  # - path: "mixins/region/{{ .region }}"
  - path: "mixins/region/region_tmpl"
    # `Go` templates in `context`
    context:
      region: "{{ .region }}"
      environment: "{{ .environment }}"

  # `Go` templates in the import path
  - path: "orgs/cp/{{ .tenant }}/{{ .stage }}/_defaults"

components:
  terraform:
    # Parameterize Atmos component name
    "eks/cluster-{{ .flavor }}":
      metadata:
        component: "test/test-component"
      vars:
        # Parameterize variables
        enabled: "{{ .enabled }}"
        name: "eks-{{ .flavor }}"
        service_1_name: "{{ .service_1_name }}"
        service_2_name: "{{ .service_2_name }}"
        tags:
          flavor: "{{ .flavor }}"
```

<br/>

Then we can import the template into a top-level stack multiple times providing different context variables to each import and to the imports for
the entire inheritance chain (which `catalog/terraform/eks_cluster_tmpl_hierarchical` imports itself):

```yaml title=stacks/orgs/cp/tenant1/test1/us-west-1.yaml
import:

  # This import with the provided hierarchical context will dynamically generate
  # a new Atmos component `eks/cluster-blue` in the `tenant1-uw1-test1` stack
  - path: "catalog/terraform/eks_cluster_tmpl_hierarchical"
    context:
      # Context variables for the EKS component
      flavor: "blue"
      enabled: true
      service_1_name: "blue-service-1"
      service_2_name: "blue-service-2"
      # Context variables for the hierarchical imports
      # `catalog/terraform/eks_cluster_tmpl_hierarchical` imports other parameterized configurations
      tenant: "tenant1"
      region: "us-west-1"
      environment: "uw1"
      stage: "test1"

  # This import with the provided hierarchical context will dynamically generate
  # a new Atmos component `eks/cluster-green` in the `tenant1-uw1-test1` stack
  - path: "catalog/terraform/eks_cluster_tmpl_hierarchical"
    context:
      # Context variables for the EKS component
      flavor: "green"
      enabled: false
      service_1_name: "green-service-1"
      service_2_name: "green-service-2"
      # Context variables for the hierarchical imports
      # `catalog/terraform/eks_cluster_tmpl_hierarchical` imports other parameterized configurations
      tenant: "tenant1"
      region: "us-west-1"
      environment: "uw1"
      stage: "test1"
```

<br/>

In the case of hierarchical imports, Atmos performs the following steps:

- Processes all the imports in the `import` section in the current configuration in the order they are specified providing the `context` to all
  imported files

- For each imported file, Atmos deep-merges the parent `context` with the current context. Note that the current `context` (in the current file) takes
  precedence over the parent `context` and will override items with the same keys. Atmos does this hierarchically for all imports in all files,
  effectively processing a graph of imports and deep-merging the contexts on all levels

For example, in the `stacks/orgs/cp/tenant1/test1/us-west-1.yaml` configuration above, we first import
the `catalog/terraform/eks_cluster_tmpl_hierarchical` and provide it with the `context` which includes the context variables for the EKS component
itself, as well as the context variables for all the hierarchical imports. Then, when processing
the `stacks/catalog/terraform/eks_cluster_tmpl_hierarchical` configuration, Atmos deep-merges the parent `context` (from
`stacks/orgs/cp/tenant1/test1/us-west-1.yaml`) with the current `context` and processes the `Go` templates.

We are now able to dynamically generate the components `eks/cluster-blue` and `eks/cluster-green` in the stack `tenant1-uw1-test1` and can
execute the following Atmos commands to provision the components into the stack:

```shell
atmos terraform apply eks/cluster-blue -s tenant1-uw1-test-1
atmos terraform apply eks/cluster-green -s tenant1-uw1-test-1
```

All the parameterized variables get their values from the all the hierarchical `context` settings:

```yaml title="atmos describe component eks/cluster-blue -s tenant1-uw1-test-1"
vars:
  enabled: true
  environment: uw1
  name: eks-blue
  namespace: cp
  region: us-west-1
  service_1_name: blue-service-1
  service_2_name: blue-service-2
  stage: test-1
  tags:
    flavor: blue
  tenant: tenant1
```

## Summary

Using imports with context (and hierarchical imports with context) with parameterized config files will help you make the configurations
extremely DRY. It's very useful in many cases, for example, when creating stacks and components
for [EKS blue-green deployment](https://aws.amazon.com/blogs/containers/kubernetes-cluster-upgrade-the-blue-green-deployment-strategy/).

## Related

- [Configure CLI](/quick-start/configure-cli)
- [Create Atmos Stacks](/quick-start/create-atmos-stacks)
