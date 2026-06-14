# GitOps Publishing Demo

This example shows the **publishing half of a GitOps pipeline** — reconcile, review, and publish artifacts to a managed deployment repository using `atmos git`. A reconciler such as Argo CD or Flux (or your CI) consumes what gets published; Atmos is the producer side.

The repository is named `deploy` and intentionally omits `workdir`, so Atmos uses the automatic XDG cache location. It also configures an `init.from` template, so `atmos git init deploy` bootstraps the repository from a starting point (the `--from` / `--keep-history` flags override these defaults):

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

# Bootstrap the deploy repository from the configured init.from template.
# Add --dry-run to preview, or --force to re-initialize an existing workdir.
atmos git init deploy --dry-run

atmos git clone deploy
atmos git status deploy
atmos git diff deploy
atmos git clean deploy --dry-run
```

The custom commands wrap the same Atmos Git operations — showing how to compose your own GitOps publishing workflow from the `atmos git` primitives:

```shell
atmos gitops reconcile
atmos gitops review
atmos gitops clean
```

`atmos gitops publish` commits pending changes in the managed workdir. Its push step is commented out in `atmos.yaml` so the example cannot publish to the sandbox repository by accident.
