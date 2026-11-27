---
title: Atlantis
tags: [Automation]
cast:
  file: /casts/examples/demo-atlantis/repo-config.cast
  title: atmos Atlantis repo config
related_docs:
  - label: "Atlantis commands"
    url: /cli/commands/atlantis/usage
  - label: "Atlantis configuration"
    url: /cli/configuration/integrations/atlantis
---

# Example: Demo Atlantis

Generate one Atlantis project from the Atmos stack `demo` and Terraform component `hello`. Its Terraform output is `Hello from Atlantis (demo)`. The example uses local state and requires no cloud account.

## Try It

```shell
cd examples/demo-atlantis
atmos atlantis build-all
```

Open `atlantis.yaml` and look for project `demo-hello`. It selects Terraform workspace `default` in `components/terraform/hello`, binds `workflow-1`, and requires approval before apply. The same command writes both inputs used by that workflow under the component directory:

- `backends/demo-hello.backend` points Terraform to the demo's state file.
- `varfiles/demo-hello.tfvars.json` supplies the stage and greeting from `stacks/deploy/demo.yaml`.

To generate only the repo configuration, run `atmos atlantis generate repo-config --config-template config-1 --project-template project-1`.

For an Atlantis checkout, run `atmos atlantis build-all` before project discovery or provide the three generated files in the checkout. The Atlantis server must permit this repository's custom workflow and approval setting. [Atlantis custom workflows](https://www.runatlantis.io/docs/custom-workflows.html) · [Server-side repo configuration](https://www.runatlantis.io/docs/server-side-repo-config.html)

## How the workflow uses the files

In the project directory, the plan stage initializes Terraform with the generated backend, selects `$WORKSPACE`, and plans with the generated var file. It saves the result to Atlantis's `$PLANFILE`. After approval, the apply stage applies that saved plan. The local backend keeps the demo state in `components/terraform/hello/demo-hello.tfstate`.
