# GitOps Demo

This example shows the Atmos Git command lifecycle for a managed deployment repository.

The repository is named `deploy` and intentionally omits `workdir`, so Atmos uses the automatic XDG cache location:

```yaml
git:
  repositories:
    deploy:
      uri: https://github.com/cloudposse-sandbox/empty.git
```

## Try It

```shell
cd examples/gitops

atmos git clone deploy
atmos git status deploy
atmos git diff deploy
atmos git clean deploy --dry-run
```

The custom commands wrap the same Atmos Git operations:

```shell
atmos gitops reconcile
atmos gitops review
atmos gitops clean
```

`atmos gitops publish` commits pending changes in the managed workdir. Its push step is commented out in `atmos.yaml` so the example cannot publish to the sandbox repository by accident.
