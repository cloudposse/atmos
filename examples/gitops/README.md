---
title: GitOps Publishing
tags: [Kubernetes, Automation]
cast:
  file: /casts/examples/gitops/reconcile.cast
  title: atmos git clone, status, and diff
---

# GitOps Publishing Demo

Use the built-in [`atmos git` commands](https://atmos.tools/cli/commands/git/usage) to clone a deployment repository and review its changes by name. In a GitOps pipeline, you can then commit and push generated manifests for Argo CD or Flux to deploy. This example demonstrates the repository operations; it does not generate manifests or run a deployment controller.

## Configure the repository

The [managed repository configuration](https://atmos.tools/cli/configuration/git) names the repository `deploy`. With no `workdir` set, Atmos keeps its checkout in the XDG cache. The optional `init.from` setting supplies a template for [`atmos git init`](https://atmos.tools/cli/commands/git/init); cloning an existing repository does not use that template.

```yaml
git:
  repositories:
    deploy:
      uri: https://github.com/cloudposse-sandbox/empty.git
      init:
        from: https://github.com/cloudposse/terraform-aws-components.git
        keep_history: false
```

## Try It

```shell
cd examples/gitops

# Clone or update the managed checkout.
atmos git clone deploy

# Inspect the checkout and review changes.
atmos git status deploy
atmos git diff deploy

# Preview cleanup, then remove the demo checkout.
atmos git clean deploy --dry-run
atmos git clean deploy
```

An unchanged checkout has no diff. In your own project, configure `git.repositories.deploy.uri` to point to a repository you control, then write your generated manifests into its managed workdir. Review and publish those changes with:

```shell
atmos git status deploy
atmos git diff deploy
atmos git commit deploy --message "Update generated deployment artifacts"
atmos git push deploy
```

Run the publishing commands against your own repository with the appropriate authentication, rather than the sandbox URL included here.

To start a new repository from the configured template instead of cloning existing content, preview the operation with `atmos git init deploy --dry-run` and omit `--dry-run` when ready to initialize it.
