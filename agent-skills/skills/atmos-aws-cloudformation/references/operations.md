# CloudFormation Operational Commands

Detailed command guidance for [the CloudFormation skill](../SKILL.md).

## Changesets

`diff`/`plan` and `apply`/`deploy` manage changesets implicitly. For manual, two-phase control (e.g.
create + review in one CI job, execute in another), use the explicit verb group:

```shell
atmos aws cloudformation changeset create vpc -s dev
atmos aws cloudformation changeset list vpc -s dev
atmos aws cloudformation changeset execute vpc -s dev --changeset-name=<name>
atmos aws cloudformation changeset delete vpc -s dev --changeset-name=<name>
```

`changeset execute` and `changeset delete` prompt for confirmation (skip with `--auto-approve`), same
as top-level `apply`/`delete`. Change-set creation expands macros/transforms (`Fn::Transform`, SAM);
`CAPABILITY_AUTO_EXPAND` has no effect here. It is required for direct macro-based `CreateStack`/
`UpdateStack` calls without change-set review. Declare applicable `CAPABILITY_IAM`/`CAPABILITY_NAMED_IAM`
acknowledgments in `capabilities:` independently.

## Observability: tree, logs, watch

```shell
atmos aws cloudformation tree vpc -s dev              # nested-stack/resource dependency graph
atmos aws cloudformation logs vpc -s dev [--chart] [--follow]   # combined event log across nested stacks
atmos aws cloudformation watch vpc -s dev             # attach to an in-progress (or terminal) operation
```

`logs --chart` renders a per-resource timeline instead of a flat chronological list; `--follow`
tails new events continuously and is mutually exclusive with `--chart`. `watch` is for *attaching* to
an operation already in progress — including one started outside Atmos — distinct from the live
streaming `apply`/`deploy`/`delete` already show while they run.

## Deployed-Stack Retrieval

```shell
atmos aws cloudformation get template vpc -s dev [--original]
atmos aws cloudformation get policy vpc -s dev
```

`get template` answers "what's actually deployed right now" via `GetTemplate` (`--original` fetches
the user-submitted body instead of the post-transform one); `get policy` fetches the live stack
policy via `GetStackPolicy`. This is the inverse of `render` (local-only) — useful for drift
investigation and for inspecting a stack before adopting it into Atmos management (stack
import/adoption itself is not supported).
