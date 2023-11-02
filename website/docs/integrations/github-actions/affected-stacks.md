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

```yaml
name: Pull Request

on:
  pull_request:
    branches: [ 'main' ]
    types: [opened, synchronize, reopened, closed, labeled, unlabeled]

jobs:
  context:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - id: affected
        uses: cloudposse/github-action-atmos-affected-stacks@v0.0.1

    outputs:
      affected: ${{ steps.affected.outputs.affected }}
      matrix: ${{ steps.affected.outputs.matrix }}
```
