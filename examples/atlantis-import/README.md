---
title: Import an Atlantis project
tags: [Terraform, Atlantis, Stacks]
related_docs:
  - label: "Adopt an Atlantis project"
    url: /cli/commands/stack/usage#adopt-an-atlantis-project
---

# Import an Atlantis project

Try a local plan from an existing `atlantis.yaml` with Atmos and Terraform installed. The module uses local state and an output. You can run it without an Atlantis server or cloud credentials.

From this example directory, initialize the Terraform root and list the projects.

```shell
terraform -chdir=infra/network init
atmos stack add ./atlantis.yaml --dry-run
```

`network-dev` uses Atlantis's built-in init and plan steps. Its plan reads `common.tfvars`, followed by `env/default.tfvars` for the native workspace `default`. The second project, `network-review`, needs manual translation because its first plan step runs a shell command. The inventory identifies that step.

Save the supported project in the Atmos stack `dev` and plan by name.

```shell
atmos stack add ./atlantis.yaml --project network-dev -s dev
atmos terraform plan network-dev -s dev
```

The plan shows `message = "Hello from dev"`. Change `stage` in `infra/network/env/default.tfvars` and repeat the named plan to see the new message. Atlantis continues to handle pull-request approvals, locking, and apply.
