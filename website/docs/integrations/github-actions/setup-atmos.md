---
title: Setup Atmos
sidebar_position: 10
sidebar_label: Setup Atmos
---

The Cloud Posse GitHub Action to "Setup Atmos" simplifies your GitHub Action Workflows.

Easily integrate Atmos into your GitHub Action workflows using [`github-action-setup-atmos`](https://github.com/cloudposse/github-action-setup-atmos). To simplify your workflows, we offer a [GitHub Action](https://github.com/cloudposse/github-action-setup-atmos) that streamlines the process of [installing Atmos](/quick-start/install-atmos).

We provide a [GitHub Action](https://github.com/cloudposse/github-action-setup-atmos) to make that easier for CI/CD applications.

## Usage Example

```yaml
on:
  workflow_dispatch:
  pull_request:

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Atmos
        uses: cloudposse/github-action-setup-atmos
        with:
          # Make sure to pin to the latest version of atmos
          atmos_version: 1.78.0
  ```
