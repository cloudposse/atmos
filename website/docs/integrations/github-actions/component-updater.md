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

## Usage Examples

### Workflow example

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
            include: |
              aws-*
              eks/*
              bastion
            exclude: aws-sso,aws-saml
```

### Using a Custom Atmos CLI Config Path (`atmos.yaml`)

If your [`atmos.yaml` file](https://atmos.tools/cli/configuration) is not located in the root of the infrastructure repository, you can specify the path to it using [`ATMOS_CLI_CONFIG_PATH` env variable](https://atmos.tools/cli/configuration/#environment-variables).

```yaml
  # ...
  - name: Update Atmos Components
    uses: cloudposse/github-action-atmos-component-updater@v1
    env:
      # Directory containing the `atmos.yaml` file
      ATMOS_CLI_CONFIG_PATH: ${{ github.workspace }}/rootfs/usr/local/etc/atmos/
    with:
      github-access-token: ${{ secrets.GITHUB_TOKEN }}
      max-number-of-prs: 5
```

### Customize Pull Request labels, title and body

```yaml
  # ...
  - name: Update Atmos Components
    uses: cloudposse/github-action-atmos-component-updater@v1
    with:
      github-access-token: ${{ secrets.GITHUB_TOKEN }}
      max-number-of-prs: 5
      pr-title: 'Update Atmos Component \`{{ component_name }}\` to {{ new_version }}'
      pr-body: |
        ## what
        Component \`{{ component_name }}\` was updated [{{ old_version }}]({{ old_version_link }}) → [{{ old_version }}]({{ old_version_link }}).

        ## references
        - [{{ source_name }}]({{ source_link }})
      pr-labels: |
        component-update
        automated
        atmos
```

**IMPORTANT:** The backtick symbols must be escaped in the GitHub Action parameters. This is because GitHub evaluates whatever is in the backticks and it will render as an empty string.

#### For `title` template these placeholders are available:
- `component_name`
- `source_name`
- `old_version`
- `new_version`

#### For `body` template these placeholders are available:
- `component_name`
- `source_name`
- `source_link`
- `old_version`
- `new_version`
- `old_version_link`
- `new_version_link`
- `old_component_release_link`
- `new_component_release_link`