# Gitea CI/CD

[Gitea Actions](http://localhost:3001/atmos/deployments/actions) executes the workflow
in `.gitea/workflows/atmos.yaml`.

| Trigger | Job | Behavior |
| --- | --- | --- |
| Pull request | plan | Validate stacks and plan the requested service against the local emulator |
| Push to main | deploy | Apply affected, merged service stacks and register their catalog entries |

The runner uses the `atmos-local` label. Jobs execute inside the runner container,
which is attached to the Atmos stack network. It has no Docker socket mount.
Only this throwaway infrastructure repository should use this runner.

The runner processes one job at a time. Terraform state persists across job
checkouts in a mounted directory, with separate Atmos workspaces for each service.
Atmos native CI writes Terraform summaries and step outputs. Native `describe affected`
compares the event's base commit to the checked-out revision, including shared Terraform
source and inherited stack changes. Initial pushes select all current service stacks.
Service deletion fails before provisioning: destroy a service explicitly while its stack
still exists, then remove its configuration through a separate reviewed procedure.

Plans are previews: the deploy job creates a fresh plan from the merged commit.
The native local planfile store retains previews under `/state/planfiles`, keyed by
stack, component, and commit SHA. It does not replay a PR's plan after merge or claim
that the merged plan was approved. The state volume also holds these plan artifacts;
remove them when resetting the demo and treat them as sensitive infrastructure data.

Commit statuses and PR comments are explicitly disabled in the seed configuration.
To enable them, provision a repository-scoped Gitea token with the required status
and issue/comment write permissions as a repository Actions secret, expose it as
`ATMOS_CI_GITEA_TOKEN` on the Terraform steps, and enable `ci.checks.enabled` and
`ci.comments.enabled` in `atmos.yaml`. Comment behavior is `upsert`. Ordinary Actions
job status remains available without these API features.

The deployment repository is seeded once. Existing repositories keep their reviewed
configuration: copy these changes into a branch of that repository and review/merge
it to adopt the new workflow; running `portal up` does not overwrite it.

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
