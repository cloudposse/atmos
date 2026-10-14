---
related_docs:
  - label: Lifecycle hooks
    url: /stacks/hooks
  - label: Starlark hook context
    url: /steps/type/script#lifecycle-hook-context
cast:
  file: /casts/examples/starlark-hooks/owner-check.cast
  title: Check ownership before Terraform plans
---

# A lifecycle hook with Atmos Automation

Check component ownership before Terraform plans a module using Atmos Automation
Language, a Python-like language built on Starlark and embedded in Atmos. The hook reads
resolved stack variables directly through `ctx.component`; it needs no shell
parsing or extra `atmos describe` call.

Install Terraform and run from this directory:

```shell
atmos terraform plan api -s dev
```

Atmos prints `Owner check passed: api belongs to platform (dev).`, then Terraform
reports no changes. The module has no providers or resources, so the command
requires no credentials, network access, or provider installation.

The `unowned` stack inherits the same component but clears its owner. To see the
guard stop the plan before Terraform runs:

```shell
atmos terraform plan api -s unowned
```

The hook fails with `Set an owner before planning api`.

The stack names the component `api` while its implementation directory is `service`.
The hook receives the component's stack identity automatically.

See [hooks](https://atmos.tools/stacks/hooks) and
[Starlark lifecycle context](https://atmos.tools/steps/type/script#lifecycle-hook-context).
