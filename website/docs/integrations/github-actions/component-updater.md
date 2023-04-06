---
title: Component Updater
sidebar_position: 20
sidebar_label: Component Updater
---

This is GitHub Action that can be used as the workflow for automatic updates via Pull Requests on the infrastructure repository according to versions of components sources.

Details and a full list of `inputs` and `outputs` might be found in [GitHub Action](https://github.com/cloudposse/github-action-atmos-component-updater) repository on GitHub.

## Usage Example

```yaml
name: "atmos-components"

on:
  workflow_dispatch: {}

  schedule:
    - cron:  '0 8 * * 1'         # Execute every week on Monday at 08:00

permissions:
  contents: write
  pull-requests: write

jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - name: Update Atmos Components
        uses: cloudposse/github-action-atmos-component-updater@v1
        with:
          github-access-token: ${{ secrets.GITHUB_TOKEN }}
          max-number-of-prs: 5
          includes: |
            aws-*
            eks/*
            bastion
          excludes: aws-sso,aws-saml
```
