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

## Apply, diff, and delete behavior

- **apply/deploy order**: create changeset, print predicted changes (stderr), then confirm, then execute. A
  no-op changeset (CloudFormation's `FAILED` "didn't contain changes") is deleted and reported as `No changes`;
  the stack-policy, termination-protection, and Outputs follow-ups still run. Changeset names are
  `atmos-<stack>-<timestamp>` (the `atmos-` prefix is not repeated for stacks already named `atmos-...`).
- **Never-deployed stacks**: the first CREATE changeset makes CloudFormation register an empty
  `REVIEW_IN_PROGRESS` stack. `diff`/`plan`, a declined `apply`, a failed changeset, and a changeset that
  cannot be executed delete that stub (only when this run created it and it is still `REVIEW_IN_PROGRESS`);
  a pre-existing stack is never deleted. `changeset create` keeps the changeset and its stub on success
  (execute needs them) but deletes a no-op or failed one (`No changes; changeset not kept`).
- **ROLLBACK_COMPLETE**: the first create failed and CloudFormation cannot update the stack. `apply`/`diff`/
  `changeset create` fail with `aws/cloudformation stack is in ROLLBACK_COMPLETE and cannot be updated` and a
  hint: `atmos aws cloudformation delete <component> -s <stack>`, then apply again. Atmos never deletes it.
- **Targets**: publish-only `aws/s3` prints `Published template to s3://... (TemplateURL: https://...)` and
  records `package_s3_uri`/`package_url`; external targets print the delivered file and target; an `aws/stackset`
  target is rejected (`apply` cannot deliver to it) with a hint to use `stackset create|update --target <name>`.
- **delete**: prints `Deleted stack <name>` after the stream ends with the stack's `DELETE_COMPLETE`; a missing
  stack (also with `--retain-resources`) prints `<name> does not exist; nothing to delete` and exits 0; a
  `DELETE_FAILED` result hints `--retain-resources=<failed logical IDs>`; termination protection and a
  `--retain-resources` outside `DELETE_FAILED` each have their own error. `stackset delete` of a missing
  StackSet is the same idempotent no-op; `changeset delete --changeset-name <unknown>` is a not-found error
  with a hint to `changeset list`.
- **`output --all --format=json|yaml`** prints one document keyed by stack, then component; `table` prints a
  `<component> in stack <stack>:` title above each table.

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
the user-submitted body and prints it byte-for-byte as returned; the default processed body is
re-serialized as YAML); `get policy` fetches the live stack policy via `GetStackPolicy`. Both map a
missing stack to a stack-not-found error with a hint. This is the inverse of `render` (local-only) — useful for drift
investigation and for inspecting a stack before adopting it into Atmos management (stack
import/adoption itself is not supported).

## Output details

- `output <component> <key>` prints only that Output value, bare and pipeable. With `--format=json` or `yaml`, the value is encoded. A missing key fails and lists the available keys. The key cannot be combined with `--all`, `--affected`, `--tags`, or `--labels`.
- A stack that is not deployed is an error, for `output`, `!aws.cloudformation.output`, and `atmos.Component(...).outputs`. Not deployed means `REVIEW_IN_PROGRESS`, `ROLLBACK_*`, `CREATE_FAILED`, or `DELETE_*`.
- A deployed stack with no Outputs prints `Stack <name> has no outputs` for the table format. Structured formats still print an empty document, so stdout stays parseable.
- An unsupported `--format` error names the value and lists the valid formats. JSON is written without HTML escaping, so `<MASKED>` appears literally.
- In bulk runs, JSON and YAML print one document keyed by stack, then component. Table output titles each component.

## Confirmation

**Confirmation**: `delete` prompts for interactive confirmation on a TTY; pass `--auto-approve` to
skip it. `apply` creates its changeset first, prints the predicted changes, then asks (`--auto-approve`
skips only the question, not the preview); declining deletes the changeset and the empty
`REVIEW_IN_PROGRESS` stack Atmos created for a never-deployed component. Without a TTY and without
`--auto-approve`, `apply` fails before creating anything (`confirmation required`, not `user aborted`).
Publish-only (`aws/s3`) and external (`git`) targets change no stack and never ask. `deploy` defaults
`--auto-approve` to `true`. See [apply flow](#apply-diff-and-delete-behavior).


## Output Formats and Masking

`output` supports the full standard format set shared with `atmos terraform output`: `json`, `yaml`,
`hcl`, `env`, `dotenv`, `bash`, `csv`, `tsv`, `table` (default on a TTY), and `github` (GitHub
Actions `$GITHUB_OUTPUT` syntax via `atmos aws cloudformation output vpc -s dev --format=github`),
plus `--flatten` and `--uppercase` key options. An unsupported `--format` lists the valid ones. A
stack with no Outputs prints `Stack <name> has no outputs` only in table format; JSON and YAML
print an empty document. The `key` argument cannot be combined with bulk selection.

With masking enabled, standalone output and apply summaries read the deployed template
(`cloudformation:GetTemplate`) and redact outputs that reference NoEcho parameters, including
intrinsics and indirect resource/condition dependencies. Known parameter/default values are also
registered with the masker. Missing or invalid sensitivity metadata fails before output is printed.
This works without a local template or source download, including when configured values are stale.
`--mask=false` explicitly disables presentation masking. Internal component output lookups retain
real values. Arbitrary transformed secrets without a detectable dependency cannot be recognized.
