# Gitea CI/CD

[Gitea Actions](http://localhost:3001/atmos/deployments/actions) executes the workflow
in `.gitea/workflows/atmos.yaml`.

| Trigger | Job | Behavior |
| --- | --- | --- |
| Pull request | plan | Validate stacks and plan the requested service against the local emulator |
| Push to main | deploy | Apply changed, merged service stacks and register their catalog entries |

The runner uses the `atmos-local` label. Jobs execute inside the runner container,
which is attached to the Atmos stack network. It has no Docker socket mount.
Only this throwaway infrastructure repository should use this runner.

The runner processes one job at a time. Terraform state persists across job
checkouts in a mounted directory, with separate Atmos workspaces for each service.
Plans are previews: the deploy job creates a fresh plan from the merged commit.

## Inspect a run

Open the repository's **Actions** page to see queued, running, successful, and failed
jobs. Expand the Atmos step for validation, Terraform output, and catalog registration.
The Backstage CI/CD tab links to these real jobs; it is not a simulated build dashboard.

## Beyond the demo

Use protected branches and required checks, separate plan/apply permissions, a shared
remote state backend with locking, and production authentication. CI can also build
TechDocs and publish them to object storage. This demo builds the handbook into the
Backstage image and serves it through the local TechDocs publisher.

- [Gitea Actions documentation](https://docs.gitea.com/1.22/usage/actions/overview/)
- [Backstage TechDocs](https://backstage.io/docs/features/techdocs/)
