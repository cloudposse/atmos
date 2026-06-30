---
title: Deploy Everything
sidebar_position: 8
sidebar_label: Deploy Everything
---

Having configured the Terraform components, the Atmos components catalog, all the mixins and defaults, and the Atmos top-level stacks, we can now
deploy the backend. Everything runs against the [local sandbox](/quick-start/advanced/start-sandbox), so no cloud account or credentials are involved.

The components depend on each other — the bucket and topic are encrypted by the KMS key, the queue subscribes to the topic, and `app-config` publishes
the stack's resolved coordinates and two secrets. Atmos uses the component dependency graph to deploy and destroy them in the right order.

## 1. Start the sandbox

If it isn't running already, start the sandbox for the stack you're deploying into:

```shell
atmos emulator up aws -s plat-ue2-dev
```

## 2. Validate the stacks

Before deploying, validate the stack manifests:

```shell
atmos validate stacks
```

You can also orient yourself with the built-in list commands:

```shell
atmos list stacks --identity=false --process-functions=false --process-templates=false
atmos list components -s plat-ue2-dev --identity=false --process-functions=false --process-templates=false
atmos list instances -s plat-ue2-dev --identity=false --process-functions=false --process-templates=false
```

## 3. Set the required secrets

Let Atmos guide you through the two secrets `app-config` requires (see [Manage Secrets](/quick-start/advanced/configure-secrets)):

```shell
atmos secret -s plat-ue2-dev -c app-config
```

You can also set them explicitly:

```shell
atmos secret set API_KEY=sk-quickstart-example -s plat-ue2-dev -c app-config
atmos secret set 'DB_CONFIG={"username":"app","password":"s3cr3t"}' -s plat-ue2-dev -c app-config
```

:::tip Let Atmos prompt you
In an interactive terminal, you can omit the stack or component on single-component commands and Atmos will prompt you to choose one.

```shell
# Explicit
atmos terraform plan app-config -s plat-ue2-dev

# Prompt for the missing component and stack
atmos terraform plan
```

For bulk operations, use explicit selectors such as `--all`, `--components`, or `--query` instead of the prompt.
:::

## 4. Deploy everything

Deploy every Terraform component in the stack with one graph-backed command:

```shell
atmos terraform deploy --all -s plat-ue2-dev
```

Atmos applies prerequisites before dependents. In this example, the KMS key is applied before the S3 bucket, DynamoDB table, SNS topic, and SQS queue;
then `app-config` runs after the infrastructure it consumes is available.

:::note Secrets reach Terraform as environment variables
When `app-config` is applied, Atmos delivers the two secrets to Terraform as `TF_VAR_api_key` and `TF_VAR_db_password` **environment variables** — they
are never written into the `.tfvars` file on disk. See [Manage Secrets](/quick-start/advanced/configure-secrets) for details.
:::

## 5. Inspect the result

Once `app-config` is applied, inspect its outputs to confirm everything was wired together:

```shell
atmos terraform output app-config -s plat-ue2-dev
```

To see where stack configuration values came from, use provenance:

```shell
atmos list stacks --format tree --provenance --identity=false --process-functions=false --process-templates=false
atmos list instances --format tree --provenance --identity=false --process-functions=false --process-templates=false
atmos describe component app-config -s plat-ue2-dev --provenance
```

### See how environments differ

The whole point of the layered configuration is that the three environments deploy the **same services with different settings**. The catalog defines
each component once; each account's `_defaults.yaml` overrides only what differs. You can see this without deploying anything — compare the resolved
config for the same component across stages:

```shell
atmos describe component s3-bucket -s plat-ue2-dev  --process-functions=false --process-templates=false
atmos describe component s3-bucket -s plat-ue2-prod --process-functions=false --process-templates=false
```

`dev` resolves to `force_destroy: true` / `versioning_enabled: false` (cheap and ephemeral), while `prod` resolves to `force_destroy: false` /
`versioning_enabled: true` (durable and protected) — and likewise the KMS key uses a 7-day deletion window with rotation off in `dev` versus 30 days with
rotation on in `prod`. Add `--provenance` to see exactly which file each value came from:

```shell
atmos describe component kms-key -s plat-ue2-prod --provenance
```

The `deletion_window_in_days: 30` traces back to `orgs/acme/plat/prod/_defaults.yaml` — the one file that captures "what makes prod different."

## 6. Tear down

When you're done, destroy the components through the dependency graph and stop the sandbox:

```shell
atmos terraform destroy --all -s plat-ue2-dev -auto-approve
atmos emulator down aws -s plat-ue2-dev
```

Destroy reverses the dependency graph so dependents are removed before the components they depend on.

## Stack Search Algorithm

Looking at the commands above, you might have a question "How does Atmos find the component in the stack and all the variables?"

Let's consider what Atmos does when executing the command `atmos terraform plan app-config -s plat-ue2-prod`:

- Atmos uses the [CLI config](/quick-start/advanced/configure-project) `stacks.name_template: "{{ .vars.tenant }}-{{ .vars.environment }}-{{ .vars.stage }}"` to find the stack whose variables render to `plat-ue2-prod`.

- Atmos searches for the stack configuration file (in the `stacks` folder and all sub-folders) where `tenant: plat`, `environment: ue2`
  and `stage: prod` are defined (inline or via imports). During the search, Atmos processes all parent (top-level) config files and compares the
  context variables specified in the command (`-s` flag) with the context variables defined in the stack configurations, finally finding the matching
  stack.

- Atmos finds the component `app-config` in the stack, processing all the inline configs and all the configs from the imports.

- Atmos deep-merges all the catalog imports for the `app-config` component and then deep-merges all the variables for the component defined in all
  sections (global `vars`, terraform `vars`, base components `vars`, component `vars`), producing the final variables for the component.

- Lastly, Atmos writes the final deep-merged variables into a `.tfvar` file in the component directory and then executes the Terraform command. Any
  [secrets](/quick-start/advanced/configure-secrets) are excluded from that file and passed as `TF_VAR_*` environment variables instead.

---

**That's the whole backend — deployed, inspected, and torn down, entirely on your laptop.** From here, three optional chapters go deeper: orient operators with [workflows](/quick-start/advanced/create-workflows), extend the CLI with [custom commands](/quick-start/advanced/add-custom-commands), and pull in shared modules with [vendoring](/quick-start/advanced/vendor-components). Or jump to the [Final Notes](/quick-start/advanced/final-notes) and [Next Steps](/quick-start/advanced/next-steps).
