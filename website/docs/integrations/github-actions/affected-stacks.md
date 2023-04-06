---
title: Affected Stacks
sidebar_position: 30
sidebar_label: Affected Stacks
---

This is a GitHub Action to get a list of affected atmos stacks for a pull request. It optionally installs 
`atmos`, `terraform` and `jq` and runs `atmos describe affected` to get the list of affected stacks. It provides the 
raw list of affected stacks as an output as well as a matrix that can be used further in GitHub action jobs.

Details and a full list of `inputs` and `outputs` might be found in [GitHub Action](https://github.com/cloudposse/github-action-atmos-affected-stacks) repository on GitHub.

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
        uses: cloudposse/github-action-atmos-affected-stacks@feature/initial-implementation

    outputs:
      affected: ${{ steps.affected.outputs.affected }}
      matrix: ${{ steps.affected.outputs.matrix }}
```
