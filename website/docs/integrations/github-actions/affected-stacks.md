---
title: Affected Stacks
sidebar_position: 30
sidebar_label: Affected Stacks
---

**Streamline Your Change Management Process**

The [Atmos Affected Stacks GitHub Action](https://github.com/cloudposse/github-action-atmos-affected-stacks) empowers you to easily identify the affected [atmos stacks](/core-concepts/stacks/) for a pull request so you can gain better insights into the impact of your pull requests. It works by installing Atmos and running [`atmos describe affected`](/cli/commands/describe/affected), and outputs a comprehensive list of affected stacks, both as raw output and as a matrix to be used in subsequent GitHub action jobs.

Discover more details, including the full list of `inputs` and `outputs`, in the [GitHub Action repository](https://github.com/cloudposse/github-action-atmos-affected-stacks) on GitHub.

The [`describe affected`](/cli/commands/describe/affected) command works by comparing two different Git commits to generate a list of affected Atmos components and stacks. It assumes that the current repo root is a Git checkout and accepts a parameter to specify the second commit.

Overall Process:
1.  Clone the target branch (`--ref`), check out the commit, or use the pre-cloned target repository
2.  Deep merge all stack configurations for the current working and remote target branches.
3.  Identify changes in the component directories.
4.  Compare each section of the stack configuration to detect differences.
5.  Output a matrix containing a list of affected components and stacks

Atmos checks component folders for changes first, marking all related components and stacks as affected when changes are detected. It then skips evaluating those stacks for differences, streamlining the process.

## Usage Example

### Config

The action expects the Atmos configuration file `atmos.yaml` to be present in the repository. 
Usually, the configuration placed in `./rootfs/usr/local/etc/atmos/atmos.yaml`.
The config should have the following structure:

```yaml
# ./rootfs/usr/local/etc/atmos/atmos.yaml
integrations:
  github:
    gitops:
      terraform-version: 1.5.2
      infracost-enabled: false
      artifact-storage:
        region: us-east-2
        bucket: cptest-core-ue2-auto-gitops
        table: cptest-core-ue2-auto-gitops-plan-storage
        role: arn:aws:iam::xxxxxxxxxxxx:role/cptest-core-ue2-auto-gitops-gha
      role:
        plan: arn:aws:iam::yyyyyyyyyyyy:role/cptest-core-gbl-identity-gitops
        apply: arn:aws:iam::yyyyyyyyyyyy:role/cptest-core-gbl-identity-gitops
      matrix:
        sort-by: .stack_slug
        group-by: .stack_slug | split("-") | [.[0], .[2]] | join("-")
```

:::tip Important!

**Please note!** This GitHub Action only works with `atmos >= 1.63.0`. If you are using `atmos < 1.63.0` please use [`v2` version](https://github.com/cloudposse/github-action-atmos-affected-stacks/tree/v2).

:::

### Workflow example

```yaml
# .github/workflows/atmos-terraform-plan.yaml
name: Pull Request
on:
  pull_request:
    branches: [ 'main' ]
    types: [opened, synchronize, reopened, closed, labeled, unlabeled]

jobs:
  context:
    runs-on: ubuntu-latest
    steps:
      - id: affected
        uses: cloudposse/github-action-atmos-affected-stacks@v3
        with:
          atmos-config-path: ./rootfs/usr/local/etc/atmos/
          atmos-version: 1.63.0
          nested-matrices-count: 1

    outputs:
      affected: ${{ steps.affected.outputs.affected }}
      matrix: ${{ steps.affected.outputs.matrix }}

  # This job is an example how to use the affected stacks with the matrix strategy
  atmos-plan:
    needs: ["atmos-affected"]
    if: ${{ needs.atmos-affected.outputs.has-affected-stacks == 'true' }}
    name: ${{ matrix.stack_slug }}
    runs-on: ['self-hosted']
    strategy:
      max-parallel: 10
      fail-fast: false # Don't fail fast to avoid locking TF State
      matrix: ${{ fromJson(needs.atmos-affected.outputs.matrix) }}
    ## Avoid running the same stack in parallel mode (from different workflows)
    concurrency:
      group: ${{ matrix.stack_slug }}
      cancel-in-progress: false
    steps:
      - name: Plan Atmos Component
        uses: cloudposse/github-action-atmos-terraform-plan@v2
        with:
          component: ${{ matrix.component }}
          stack: ${{ matrix.stack }}
          atmos-config-path: ./rootfs/usr/local/etc/atmos/
          atmos-version: 1.63.0
```
