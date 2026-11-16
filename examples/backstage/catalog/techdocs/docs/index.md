# Atmos service handbook

This portal connects a service catalog to infrastructure managed by Atmos.
The **payments** component illustrates environment configuration: `dev` has two
replicas and `prod` has four, both owned by `platform-team`. It does not deploy a
payments application.

Use **Create → Inspect an Atmos stack** to see that inheritance in action.
Use **Create → Request a service with an S3 bucket** to provision real resources
in the local AWS emulator.

## What runs locally?

| Tool | Role |
| --- | --- |
| Backstage | Service catalog, request form, and this handbook |
| Atmos | Stack configuration, scaffolding, Git publication, and Terraform execution |
| Gitea | Infrastructure repository, review, Actions jobs, and logs |
| Gitea runner | PR plans and post-merge provisioning |
| Floci | Local S3-compatible AWS emulator |

The Git and AWS services use throwaway local data. No cloud account is needed.
[Continue with a service request](provisioning.md).
