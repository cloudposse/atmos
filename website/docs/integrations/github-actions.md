---
title: GitHub Actions Integration
sidebar_position: 10
sidebar_label: GitHub Actions
---

Atmos works anywhere you can [install and run the CLI](/quick-start/install).

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
          atmos_version: 1.12.2
  ```
