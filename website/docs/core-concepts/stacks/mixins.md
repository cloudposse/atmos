---
title: Stack Mixins
sidebar_position: 7
sidebar_label: Mixins
---

Mixins are a special kind of "import". It's simply a convention we recommend to distribute reusable snippets of configuration that alter the behavior in some deliberate way. Mixins are not handled in any special way. They are technically identical to all other imports. 

## Conventions

Here are some examples of how we recommend using Mixins.

### Mixins by Region

Mixins organized by region will make it very easy to configure where a stack is deployed by simply changing the imported mixin.

Consider naming them after the canonical region name for the cloud provider you're using.

For example, here's what it would look like for AWS. Let's name this file `mixins/region/us-east-1.yaml`.
Now, anytime we want a Parent Stack deployed in the `us-east-1` region, we just need to specify this import, and we'll automatically inherit all the settings for that region.

```yaml
imports:
- mixins/region/us-east-1
```

### Mixins by Stage

Provide a the default settings for operating in a particular stage (e.g. Dev, Staging, Prod) to enforce consistency.

For example, let's define the stage name for production in the mixin file named `mixins/stage/prod.yaml`

```yaml
stage: prod
```

Now, anytime we want to provision a parent stack in production, we'll want to add this to the imports:

```yaml
imports:
- mixins/stage/prod
```

While this is a fleetingly simple example, it helps an organization impart consistency. There are many ways developers will define "production".

- e.g. `prd`
- e.g. `prod`
- e.g. `production`
- e.g. `Production`
- e.g. `Prod`
- e.g. `PROD`
- etc

To avoid this situation, use the mixin `mixings/stage/prod` to always use the appropriate naming convention.
