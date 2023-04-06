---
title: Component Updater
sidebar_position: 20
sidebar_label: Component Updater
---

**Efficient Automation for Component Updates**

Use Cloud Posse's GitHub Action for updating [Atmos components](/core-concepts/components/) (e.g. [like the one's provided by Cloud Posse](https://github.com/cloudposse/terraform-aws-components/)) to streamline your infrastructure management.

Keep your infrastructure repositories current with the latest versions of components using the [Cloud Posse GitHub Actions for Updating Atmos Components](https://github.com/cloudposse/github-action-atmos-component-updater). This powerful tool simplifies and accelerates the management of pull requests, ensuring that updates are processed quickly, accurately, and without hassle.

Leverage the [Atmos components GitHub Action](https://github.com/cloudposse/terraform-aws-components/) to automate the creation and management of pull requests for component updates. With its customizable features, you can design an automated workflow tailored to your needs, making infrastructure repository maintenance more efficient and less error-prone.

## Key Features:

- **Selective Component Processing:** Configure the action to `exclude` or `include` specific components using wildcards, ensuring that only relevant updates are processed.
- **PR Management:** Limit the number of PRs opened at a time, making it easier to manage large-scale updates without overwhelming the system. Automatically close old component-update PRs, so they don't pile up.
- **Material Changes Focus:** Automatically open pull requests only for components with significant changes, skipping minor updates to `component.yaml` files to reduce unnecessary PRs and maintain a streamlined system.
- **Informative PRs:** Link PRs to release notes for new components, providing easy access to relevant information, and use consistent naming for easy tracking.
- **Scheduled Updates:** Run the action on a cron schedule tailored to your organization's needs, ensuring regular and efficient updates.

Discover more details and a comprehensive list of `inputs` and `outputs` in the [GitHub Action repository](https://github.com/cloudposse/github-action-atmos-component-updater) on GitHub. 

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
