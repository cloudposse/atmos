# Move from Atlantis to Atmos Native CI

This reference gives steps to move a project from [Atlantis](https://runatlantis.io) to Atmos
Native CI. The steps change the CI system only. The steps cover automatic plans, plan comments on
pull requests, approval before an apply, and locks on components.

This reference does not give the full config schema for Native CI. This reference does not give
full GitHub Actions workflow files. For this data, read [atmos-ci](../../atmos-ci/SKILL.md) and
[references/native-ci.md](../../atmos-ci/references/native-ci.md). This reference tells you what
config to add and why. The atmos-ci reference tells you how the config works.

The command `atmos atlantis generate repo-config` is still a supported command. The config
sections `integrations.atlantis` and `settings.atlantis` are still supported. Atmos does not
remove this feature. When you move a project away from Atlantis, the project does not need this
feature. Atmos keeps the feature for other projects.

## Identify the User's Shape

| Shape                                                                              | Recipe                     |
|--------------------------------------------------------------------------------------|----------------------------|
| The project uses Atmos now. It uses `integrations.atlantis`/`settings.atlantis` and `atmos atlantis generate repo-config` to send config to an Atlantis server. | [Shape A](#shape-a-the-project-uses-atmos-now) |
| The project uses plain Terraform and Atlantis. The project does not use Atmos yet.   | [Shape B](#shape-b-the-project-uses-plain-terraform-and-atlantis) |

## Shape A: The Project Uses Atmos Now

The project has components and stacks now. Only the CI layer changes. Do not change components,
stacks, or Terraform code.

**Recipe:**

1. Read the [Concept Mapping](#concept-mapping) table. Find which Atlantis behaviors the project
  uses. Most `atlantis.yaml` files use a small set of these behaviors.
2. Add the `ci:` block to `atmos.yaml`. Add the GitHub Actions workflows from
  [atmos-ci](../../atmos-ci/SKILL.md). Do not remove the Atlantis integration yet. Run both
  systems on real pull requests. Compare the plan output from each system.
3. Confirm the output from Native CI matches the output from Atlantis. Then, follow the
  [Checklist to Remove Atlantis](#checklist-to-remove-atlantis).

## Shape B: The Project Uses Plain Terraform and Atlantis

The project has no Atmos components or stacks. The `atlantis.yaml` file points to Terraform root
modules, not Atmos components. This move has two parts. First, move to Atmos. Then, move to
Native CI.

**Recipe:**

1. Move the Terraform code to Atmos first. Find the layout of the Terraform code. Follow
  [from-native-terraform.md](from-native-terraform.md). If the Atlantis projects use
  `terraform.workspace`, follow [from-terraform-workspaces.md](from-terraform-workspaces.md)
  instead. Confirm `atmos terraform plan` gives correct output for each former Atlantis project
  before you change the CI system.
2. After you create the components and stacks, treat the CI move as
  [Shape A](#shape-a-the-project-uses-atmos-now). The project did not use
  `integrations.atlantis`. Go directly to the Concept Mapping table and the Checklist to Remove
  Atlantis.

Do not do both moves in one step. Complete the move to Atmos. Confirm the output of `atmos
terraform plan` matches the output of the old `terraform plan` command. Then, start the move to
Native CI.

## Concept Mapping

This table shows each Atlantis feature and the matching Atmos Native CI feature.

| Atlantis concept | Atmos Native CI equivalent | Notes |
|---|---|---|
| `projects[]` and `autoplan.when_modified` | `atmos describe affected --format=matrix` | Atmos does not need a fixed list of projects. Atmos finds affected components through `dependencies.components`. See [atmos-ci](../../atmos-ci/SKILL.md#component-dependencies). |
| `workflow_templates.<name>.plan/apply.steps[].run` (custom init, plan, and apply commands) | [Hooks](../../atmos-hooks/SKILL.md) | A hook runs custom steps before or after almost any Atmos command. Examples: `before.terraform.plan`, `after.terraform.apply`. Atmos also has hook events for other component types, such as helmfile and packer. A hook gives the same result as an Atlantis workflow template. Both add custom steps at a fixed point in the command. The command `atmos terraform plan`, `apply`, or `deploy` already runs init, plan, and apply. Use a hook for most custom steps. Use a [custom command](../../atmos-custom-commands/SKILL.md) only when the step is not linked to a command lifecycle. |
| `pre_workflow_hooks` and `post_workflow_hooks` (server-side, global steps that build repo config at run time) | CI job steps before or after the `atmos` command, or Atmos hooks with a broad `when:` condition | The scope is not the same. An Atlantis server-side hook runs once for each pull request. An Atmos hook runs at the component or stack level by default. Use a workflow job step for any action that must run once for each pull request. |
| `apply_requirements: [approved]` | GitHub Environment protection rules (required reviewers, wait timers) | Do not build custom approval logic in Atmos config. See [atmos-ci Workflow Guidance](../../atmos-ci/SKILL.md#workflow-guidance). |
| Plan comment on the pull request | `ci.comments` (`behavior: create`, `update`, or `upsert`) | This comment covers plan output only. See [Known Gaps](#known-gaps). |
| Commit status or check on the pull request | `ci.checks` (`context_prefix` and status toggles) | This uses the Commit Status API. Atlantis uses the same API. |
| The `$PLANFILE` passed from the plan step to the apply step | `components.terraform.planfiles` storage (S3, GitHub Artifacts, or a local path), with `--verify-plan` on `deploy` | The `deploy` command creates a new plan. The `deploy` command compares the new plan to the stored plan before it runs the apply step. |
| `terraform_version` set for each project | `dependencies.tools.terraform` set on the component, stack, or workflow | The Atmos toolchain manages the version. You do not set the version as a separate project field. |
| A lock on a project during plan or apply | `atmos pro lock` and `atmos pro unlock` | These commands need Atmos Pro. See [Known Gaps](#known-gaps). |
| `automerge`, `parallel_plan`, `parallel_apply` | A GitHub Actions matrix job, with branch protection rules or a merge queue | The matrix job runs affected components at the same time. You do not need a separate flag for this. |
| `allowed_regexp_prefixes` | Not needed | The command `atmos describe affected` finds the correct components. You do not need a fixed list of path patterns. |

## Checklist to Remove Atlantis

1. Add the `ci:` block to `atmos.yaml`. Set `enabled`, `output`, `summary`, `checks`, and
  `comments`. See [atmos-ci Native CI First](../../atmos-ci/SKILL.md#native-ci-first) for the
  full schema.
2. Add two GitHub Actions workflows. Add a pull request workflow that finds the affected matrix
  and runs a plan. Add a merge or manual workflow that runs a deploy. Use the examples in
  [atmos-ci references/native-ci.md](../../atmos-ci/references/native-ci.md). Grant only the
  permissions each enabled `ci.*` feature needs, such as `statuses: write`, `checks: write`, or
  `pull-requests: write`.
3. Run both systems on real pull requests. Compare the plan output, the resource counts, and the
  pass or fail result. Confirm the results match before you continue.
4. Remove the `integrations.atlantis` section from `atmos.yaml`. Remove any `settings.atlantis`
  overrides from the stack config files.

**Note:** Read the [Known Gaps](#known-gaps) section before you continue. If the team uses
Atlantis project locks or apply-time pull request comments, make a plan for this gap before you
remove Atlantis.

5. Delete the `atlantis.yaml` file from the project.
6. Stop the Atlantis server. Remove the Atlantis webhook.

## Known Gaps

Read this section before you remove Atlantis. Native CI does not yet match every Atlantis
feature.

- **No native project lock in the open-source CLI.** Atlantis locks a project by default during a
  plan or an apply. The open-source Atmos CLI has no equivalent. The commands `atmos pro lock` and
  `atmos pro unlock` give this feature, but they need Atmos Pro.
- **Pull request comments cover plan output only.** The `ci.comments` feature posts a comment
  after a plan. An apply or a deploy does not post a pull request comment. The commit status and
  the checks pane show the apply result instead. If a team needs an apply comment on the pull
  request thread, there is no direct replacement yet.
- **The Azure Blob and GCS planfile stores are not built yet.** The
  `components.terraform.planfiles.stores` config supports S3, GitHub Artifacts, and a local path
  today.
- **There is no GitLab CI provider yet.** The GitHub provider is complete. The Atmos team has not
  built the GitLab provider yet.

## Actions to Avoid

- Do not write large custom shell scripts to copy `workflow_templates` steps. Use a hook instead.
  A hook runs custom steps before or after almost any Atmos command, not only a plan or an apply
  step. Use a custom command only for a step that is not linked to a command lifecycle. See the
  [Concept Mapping](#concept-mapping) table.
- Do not build custom approval logic in Atmos workflow YAML to replace
  `apply_requirements: [approved]`. GitHub Environment protection rules give this feature.
- Do not keep both `integrations.atlantis` and the Native CI `ci:` config active after you confirm
  Native CI gives correct results. Use only one CI system. Two active systems can give different
  plan or apply results for the same pull request.
- Do not skip the [Known Gaps](#known-gaps) review before you remove Atlantis. If the team uses
  project locks or apply-time pull request comments, make a plan for this gap first.
