# CloudFormation `list`, `fmt`, and Native CI details

Detail for `atmos aws cloudformation list`, `fmt`, and the native CI integration. The main
[SKILL.md](../SKILL.md) links here.

## `list`

`atmos aws cloudformation list [--stack <stack>] [--status <status,...>] [--region <region>] [--identity <name>]`
calls `ListStacks` account-wide and prints a table with a header row (`MANAGED`, `STATUS`,
`STACK NAME`).

- **Managed vs. unmanaged.** A deployed stack is `managed` when its name equals the `stack_name` of an
  `aws/cloudformation` component. With `--stack`, only that Atmos stack's components count. Without
  it, every Atmos stack's components count. An unknown `--stack` is an `ErrStackNotFound` error, not an
  rc-0 listing of everything as `unmanaged`.
- **Identity.** An explicit `--identity` wins. Otherwise `list` scans the stack manifests for a single
  `default: true` identity (`list` has no component, so it cannot read one component's `auth`
  section). With no resolvable identity and no SDK credentials, the credential error carries a hint to
  pass `--identity`.
- **`--status`.** Matched case-insensitively against the SDK's `StackStatus` values (`create_complete`
  works) and validated before any auth or AWS call. An unknown value is an `ErrInvalidFlag` error that
  names it and lists every valid status. The default excludes `DELETE_COMPLETE`.

## `fmt`

`fmt --check` prints `<path>: already formatted` for a clean template and `<path>: not formatted` for
a dirty one (plain `formatted` is printed only after a write). With `--all`, `--affected`, `--tags`,
or `--labels`, `--check` checks every selected template, prints each result, and exits non-zero once
at the end with `ErrAwsCloudFormationFmtNotClean` listing every unformatted file. Components sharing
one template file check it once per run, keyed by the resolved file path.

## Native CI

The plugin (`pkg/ci/plugins/cloudformation`) fires on `after.aws/cloudformation.{diff,apply,delete,drift-detect,drift-describe}`.

- **Summary titles and commands** use the verb the user ran: `plan` is not reported as `diff`, and
  `deploy` is not reported as `apply`. The reproduce command includes `--fail-on-drift` when the drift
  run used it.
- **"CloudFormation output" section.** `apply`/`deploy` list the stack Outputs; `diff`/`plan` list the
  changeset's resource changes (action, type, logical ID, replacement); `drift describe` lists drifted
  resources with property differences. Outputs come from the same masked set `output` presents, so
  NoEcho-dependent values are redacted.
- **"No changes".** A no-op `diff`/`apply` changeset says so instead of showing an empty summary.
- **Output variables** (`$GITHUB_OUTPUT`, honoring `ci.output.enabled` and `ci.output.variables`):
  `stack`, `component`, `command`, `stack_name`, `exit_code`, `changeset_name`, `has_changes`
  (`diff`/`apply`), `stack_status` (`delete`), `drifted`, `drift_status`, `drifted_resource_count`
  (drift verbs), and `output_<Key>` for each stack Output after `apply` (masked, never filtered by the
  allow-list). There are still no commit statuses, PR comments, or artifacts.
- **Hooks.** Only these five verb pairs fire events; `--skip-hooks` (no value skips all,
  `--skip-hooks=a,b` skips named hooks) and `ATMOS_SKIP_HOOKS` skip them.

See [atmos-ci](../../atmos-ci/SKILL.md) for the native-CI plumbing this rides on.
