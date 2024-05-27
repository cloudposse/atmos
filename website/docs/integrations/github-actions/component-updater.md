---
title: Component Updater
sidebar_position: 20
sidebar_label: Component Updater
---

**Efficient Automation for Component Updates**

Use Cloud Posse's GitHub Action for updating [Atmos components](/core-concepts/components/) (e.g. [like the ones provided by Cloud Posse](https://github.com/cloudposse/terraform-aws-components/)) to streamline your infrastructure management.

Keep your infrastructure repositories current with the latest versions of components using the [Cloud Posse GitHub Actions for Updating Atmos Components](https://github.com/cloudposse/github-action-atmos-component-updater). This powerful action simplifies and accelerates the management of component updates using pull requests, ensuring that updates are processed quickly, accurately, and without hassle.

With its customizable features, you can design an automated workflow tailored to your needs, making infrastructure repository maintenance more efficient and less error-prone.

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
name: "Atmos Component Updater"

on:
  workflow_dispatch: {}

  schedule:
    - cron:  '0 8 * * *'  # Every day at 8am UTC

jobs:
  update:
    environment: atmos
    runs-on:
      - "self-hosted"
    steps:
      - name: Checkout Repository
        uses: actions/checkout@v4
        with:
          fetch-depth: 1

      - name: Generate a token
        id: github-app
        uses: actions/create-github-app-token@v1
        with:
          app-id: ${{ secrets.ATMOS_APP_ID }}
          private-key: ${{ secrets.ATMOS_PRIVATE_KEY }}

      - name: Update Atmos Components
        uses: cloudposse/github-action-atmos-component-updater@v2
        env:
          # https://atmos.tools/cli/configuration/#environment-variables
          ATMOS_CLI_CONFIG_PATH: ${{ github.workspace }}/rootfs/usr/local/etc/atmos/
        with:
          github-access-token: ${{ steps.github-app.outputs.token }}
          log-level: INFO
          vendoring-enabled: true
          max-number-of-prs: 10

      - name: Delete abandoned update branches
        uses: phpdocker-io/github-actions-delete-abandoned-branches@v2
        with:
          github_token: ${{ steps.github-app.outputs.token }}
          last_commit_age_days: 0
          allowed_prefixes: "component-update/"
          dry_run: no
```

### Requirements

This action will automatically open pull requests for updated components in your repository.
To use it, we recommend installing a GitHub App to allow GitHub Actions to create pull requests within your GitHub Organization.

- [ ] Create and install a GitHub App, following [Using a GitHub App](#using-a-github-app)
- [ ] Grant GitHub Actions workflows read and write permission

1. Go to `https://github.com/organizations/YOUR_ORG/settings/actions`

If you're using GitHub Enterprise, update this setting under "enterprises": `https://github.com/enterprises/YOUR_ORG/settings/actions`

2. Find the section called "Workflow permissions"
3. Select "Read and write permissions"

- [ ] Allow GitHub Actions to create and approve pull requests

1. Go to `https://github.com/organizations/YOUR_ORG/settings/actions`

Or if you're using GitHub Enterprise, go to `https://github.com/enterprises/YOUR_ORG/settings/actions`

2. Check "Allow GitHub Actions to create and approve pull requests"

:::tip Repository-Level Action Settings

Enabling these action settings at an Organization level will enable the same settings for all repositories in your Organization. Confirm the same settings have been enabled for your infrastructure repository and optionally disable these settings for other repositories in your Organization at your own discretion.

:::

### Using a GitHub App

You may notice that we pass a generated token from a GitHub App to `github-access-token` instead of using the native `GITHUB_TOKEN`. We do this because Pull Requests will only trigger other GitHub Action Workflows if the Pull Request is created by a GitHub App or PAT. For reference, see [Triggering a workflow from a workflow](https://docs.github.com/en/actions/using-workflows/triggering-a-workflow#triggering-a-workflow-from-a-workflow).

1. Create a new GitHub App
2. Name this new app whatever you prefer. For example, `Atmos Component Updater [<YOUR NAMESPACE>]`.

:::tip GitHub Requires Unique App Names

GitHub requires all GitHub Apps to have unique names. The name you select for this GitHub App can be whatever you'd prefer. For example, an `acme` organization might name their GitHub App `Atmos Component Updater [ACME]`.

:::

3. List a Homepage URL of your choosing. This is required by GitHub, but you can use any URL. For example use our documentation page: `https://atmos.tools/integrations/github-actions/component-updater/`
4. (Optional) Add an icon for your new app (See below)

<details>
<summary>Feel free to download and use our Atmos icon with your GitHub App!</summary>

![App Icon](/img/github-actions/github-app.png)

</details>

5. Assign only the following Repository permissions:

```diff
+ Contents: Read and write
+ Pull Requests: Read and write
+ Metadata: Read-only
```

6. Generate a new private key [following the GitHub documentation](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/managing-private-keys-for-github-apps#generating-private-keys).

:::tip Private Key Token Value

The private key token should start and end with `-----BEGIN RSA PRIVATE KEY-----` and `-----END RSA PRIVATE KEY-----` respectively, and include everything in between.
When you click "Generate Private Key", a `.pem` file is automatically downloaded. This file's content is the token value.

:::

7. Finally, save both the App ID and the new private key as secrets for GitHub Actions with `ATMOS_APP_ID` and `ATMOS_PRIVATE_KEY` respectively. Note, if using GitHub Enterprise, we recommend using "GitHub Environments" to scope the Secrets to protected branches. If that's not available, use repository-scoped GitHub Secrets instead.

:::tip App ID

The App ID is shown under GitHub App settings under `General > About > App ID`. The App ID is _not_ the same as the Installation ID that is given in the URL when you install the App.

`https://github.com/settings/apps/<YOUR APP NAME>`

:::

### Using GitHub Environments

We recommend creating a new GitHub environment for Atmos (requires GitHub Enterprise). With environments, the Atmos Component Updater workflow will be required to follow any branch protection rules before running or accessing the environment's secrets. Plus, GitHub natively organizes these Deployments separately in the GitHub UI.
To configure your environment, perform the following:
1. Open "Settings" for your repository
1. Navigate to "Environments"
1. Select "New environment"
1. Name the new environment, "`atmos`"
1. In the drop-down next to "Deployment branches and tags", select "Protected branches only"
1. In "Environment secrets", create the two required secrets for App ID (`ATMOS_APP_ID`) and App Private Key (`ATMOS_PRIVATE_KEY`) from [Using a GitHub App](#using-a-github-app) step.

Now the Atmos Component Updater workflow will create a new Deployment in the `atmos` environment for each workflow run, easily accessible from the GitHub UI.

![Example Environment](/img/github-actions/github-deployment-environment.png)

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
