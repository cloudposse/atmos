# Migrating from Rain / Raw CloudFormation

This reference is a scenario-keyed decision guide for moving a Rain-managed or raw-CloudFormation
repo onto Atmos's native `aws/cloudformation` component type. For the full user-facing prose
tutorial, see [atmos.tools/migration/rain](https://atmos.tools/migration/rain).

**Framing matters**: this is migrating *off* Rain / raw CloudFormation *onto* Atmos, not "how to
write Rain syntax in Atmos." There is no Rain CLI or config-file compatibility layer, and no
`!Rain::` directive preprocessing. A `path:` template containing `!Rain::*` tags is not valid
CloudFormation and Atmos will not accept it as-is — every directive must be resolved to a
plain-CloudFormation or Atmos-native equivalent first. Atmos now fails locally (at
`render`/`validate`/`plan`/`apply`) with `ErrAwsCloudFormationRainDirective`: the error lists the
directives found, gives one hint per directive naming its replacement, and links
https://atmos.tools/migration/rain. (Previously only AWS rejected the template, with an opaque
"YAML not well-formed (line N, column M)".) This is the single most likely blocker for a real
migration; walk the directive table below with the user before touching anything else.

Atmos's `aws/cloudformation` component type is currently **experimental**
(`atmos aws cloudformation`, alias `atmos aws cfn`) — SDK-native, no `aws`/`cfn`/`sam`/`rain`
binary dependency. See `website/docs/cli/commands/aws/cloudformation/` for the full verb surface
and `website/docs/stacks/components/aws-cloudformation.mdx` for the stack-config field reference.

## Core Principles (Rain-Specific)

1. **`path:` templates are not preprocessed.** `aws/cloudformation` reads a component's `path:`
  file as raw bytes (`os.ReadFile`) and submits it to the CloudFormation API — Atmos never rewrites,
  merges, or macro-expands that template body. This is a deliberate design boundary, not a gap: it
  means every `!Rain::*` directive (which Rain resolves by *preprocessing* the template before
  submission) has no drop-in replacement at the `path:` layer. The fix is to move the substitution
  into a layer Atmos or CloudFormation already handles: Atmos stack config (`parameters:`, `!env`,
  `vars`, inheritance) for anything CloudFormation Parameters can express, or an Atmos-evaluated
  template source — an inline `template:` map, or `template: !include <file> | eval`:
  - Inline `template:` maps and `template: !include <file>` accept CloudFormation short-form
    intrinsics (`!Ref`, `!Sub`, `!GetAtt`, all 18) and rewrite them to long form; do not hand-convert
    to `Ref:`/`Fn::GetAtt:`.
  - `!include <file> | eval` (opt-in) evaluates Atmos YAML functions (`!env`, nested `!include`)
    inside the included file. Without `| eval`, included content is data: tags are dropped and the
    argument kept.
2. **Reuse templates and parameter files.** Point `path:` at the existing `.yaml`/`.json`
  template file unchanged (after resolving any `!Rain::` directives per the table below).
  Atmos `parameters:` accepts a map from parameter names to values, or a list of AWS CLI
  `ParameterKey`/`ParameterValue` entries, so an existing parameters JSON file needs no conversion:
  include it with `!include`, which loads data without reshaping it. A Rain config file
  (`{Parameters: {...}, Tags: {...}}`) is not a parameter list: select its map with
  `!include <file> .Parameters`. Keep the original parameter file for the existing workflow.
  Migration is opt-in — see [from-native-terraform.md](from-native-terraform.md) Core Principle 2
  for the same stance.
3. **No 1:1 CLI compatibility.** `atmos aws cloudformation` verbs are Atmos-native — see the verb
  cross-reference table below. Do not tell a user to alias `rain` to `atmos aws cfn`; flag names,
  output shape, and confirmation semantics differ.
4. **Template packaging is template-body only; assets use hooks.** The template body itself is
  uploaded to the component's `kind: aws/s3` provision target when it exceeds CloudFormation's
  51,200-byte inline limit, or whenever a `kind: aws/s3` target is selected
  (`pkg/component/aws/cloudformation/packaging.go`). Packaging runs on
  plan/diff/validate/changeset create/apply/deploy; only apply/deploy may create the bucket. It does
  **not** rewrite local-asset references inside the template (Lambda source zips, nested-stack
  templates by relative path) the way `aws cloudformation package` or Rain's `!Rain::S3` do. Asset
  parity is the `archive` + `publish` hook-step pattern (see the `S3` row below): build and upload
  the asset in a `before.aws/cloudformation.*` hook, derive the key from stack vars, and pass it
  into the template through `parameters:`. There is no upload YAML function by design — YAML
  functions never mutate state.
5. **Crawl → walk → run**, same arc as every other migration reference: get to a working
  `atmos aws cloudformation plan`/`deploy` first, defer `provision:` targets, StackSets,
  inheritance, and catalogs until the user has a concrete need.

## `!Rain::` Directive Mapping

Resolve every directive before the template is handed to Atmos — there is no compatibility layer,
so a template still containing `!Rain::*` tags will fail as invalid CloudFormation.

| Rain directive | What it did | Atmos-native replacement |
|---|---|---|
| `!Rain::Constant` (plus the `Rain: Constants:` section and `${Rain::Name}` inside `!Sub`) | Injects a named constant's value into the template at preprocess time | Stack `vars` + `{{ .vars.x }}` (in `parameters:` or an inline `template:`), or a CloudFormation `Parameters:` entry referenced via `!Ref` and fed from `parameters:` |
| `!Rain::Env` | Injects an environment variable's value into the template at preprocess time | `template: !include template.yaml \| eval` with `!env NAME` inside the file; or a `Parameters:` entry fed from `parameters:` with `!env NAME` in the stack manifest |
| `!Rain::Include` | Merges an external JSON/YAML fragment into the template at preprocess time | `!include f.yaml` inside the template file, with `\| eval` on the outer `template: !include`. Alternative: CloudFormation's own `Fn::Transform`/`AWS::Include` (resolved by CloudFormation from a fragment already in S3) |
| `!Rain::Embed` | Inlines a local file's contents as a string literal (e.g. Lambda inline code, `UserData`) | `!include f.py` inside the template file (a non-YAML file loads as a string), with `\| eval` on the outer include; or flatten into a YAML block scalar by hand |
| `!Rain::S3` | Uploads a local file/directory to S3 and rewrites the reference (e.g. Lambda `S3Bucket`/`S3Key`) at preprocess time | `archive` + `publish` steps in a `kind: steps` hook on `before.aws/cloudformation.apply` (add `before.aws/cloudformation.changeset-create` for the `--no-exec` workflow, and `diff` to preview). Derive the key from stack vars and pass bucket/key via `parameters:` (`Code.S3Bucket`/`Code.S3Key` use `!Ref`) |
| `!Rain::S3Http` | Like `S3`, but yields the object's https URL (e.g. nested-stack `TemplateURL`) | Same `archive` + `publish` hook pattern; build the https URL from the bucket/key parameters in the template (`!Sub`) |
| `!Rain::Module` | Client-side, multi-file template composition (experimental in Rain; needs `--experimental`) | No equivalent. The PRD points to AWS CDK for modular template composition; Rain's module system is not a design Atmos replicates |

An unset `!env` variable with no default logs a warning and resolves to `""`. Set the variable or
pass a default; `--dry-run` skips YAML functions and will not show the problem.

### Before/After: `Constant`/`Env` → Parameters

Before (Rain):

```yaml
# template.yaml (Rain-preprocessed)
Resources:
  Bucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: !Sub "${Rain::BucketNamePrefix}-bucket"
      Tags:
        - Key: Owner
          Value: !Rain::Env DEPLOY_OWNER
```

After (Atmos):

```yaml
# template.yaml (plain CloudFormation, no directives)
Parameters:
  BucketNamePrefix:
    Type: String
  DeployOwner:
    Type: String
Resources:
  Bucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: !Sub "${BucketNamePrefix}-bucket"
      Tags:
        - Key: Owner
          Value: !Ref DeployOwner
```

```yaml
# stacks/dev.yaml
components:
  "aws/cloudformation":
    my-bucket:
      path: template.yaml
      parameters:
        BucketNamePrefix: acme-plat
        DeployOwner: !env DEPLOY_OWNER
```

Parity for `!Rain::Env` inside the template file itself uses an Atmos-evaluated template source:

```yaml
# stacks/dev.yaml
components:
  "aws/cloudformation":
    my-bucket:
      template: !include template.yaml | eval
```

```yaml
# template.yaml: !env is evaluated because of "| eval"
Resources:
  Bucket:
    Type: AWS::S3::Bucket
    Properties:
      Tags:
        - Key: Owner
          Value: !env DEPLOY_OWNER
```

### Before/After: `Embed` → `!include` or inline block scalar

Before (Rain):

```yaml
Resources:
  Function:
    Type: AWS::Lambda::Function
    Properties:
      Code:
        ZipFile: !Rain::Embed handler.py
```

After (Atmos), option 1: `ZipFile: !include handler.py` inside the template file, with
`template: !include template.yaml | eval` in stack config. Option 2: content flattened into the
template once, by hand:

```yaml
Resources:
  Function:
    Type: AWS::Lambda::Function
    Properties:
      Code:
        ZipFile: |
          def handler(event, context):
              return {"statusCode": 200}
```

For anything beyond a few lines, package the function as a real deployment artifact (S3-hosted
zip) instead.

### Before/After: `S3` → `archive` + `publish` hook

```yaml
components:
  "aws/cloudformation":
    fn:
      path: template.yaml          # Code.S3Bucket: !Ref ArtifactBucket, Code.S3Key: !Ref ArtifactKey
      vars:
        artifact_bucket: acme-lambda-artifacts
        artifact_key: lambda/releases/v1/handler.zip
      parameters:
        ArtifactBucket: "{{ .vars.artifact_bucket }}"
        ArtifactKey: "{{ .vars.artifact_key }}"
      hooks:
        package-lambda:
          events:
            - before.aws/cloudformation.diff
            - before.aws/cloudformation.apply
            - before.aws/cloudformation.changeset-create
          kind: steps
          on_failure: fail
          with:
            - type: archive
              source: src/
              destination: handler.zip
              mtime: epoch
            - type: publish
              source: handler.zip
              destination: "{{ .vars.artifact_key }}"
              target:
                kind: aws/s3
                bucket: "{{ .vars.artifact_bucket }}"
                region: us-east-1
```

Use a new artifact key per release so a code change changes the parameter. `--dry-run` skips hooks.
Details: `website/docs/stacks/components/aws-cloudformation.mdx` (asset-packaging hooks).

## Rain → Atmos Verb Cross-Reference

Every Atmos verb below is confirmed registered in `cmd/aws/cloudformation/cloudformation.go`'s
`init()` — do not invent verb names beyond this table. Verbs marked "no equivalent" are confirmed
PRD Non-Goals (`docs/prd/aws-cloudformation-component.md`); don't imply a workaround exists beyond
what's stated.

| Rain command | Atmos-native equivalent | Notes |
|---|---|---|
| `rain deploy` | `atmos aws cloudformation deploy <component> -s <stack>` | `deploy` = `apply` with `--auto-approve` implied, matching `atmos terraform deploy`. `rain deploy --no-exec` → `changeset create`; `rain deploy --changeset <stack> <cs>` → `changeset execute --changeset-name=<cs>`. Flag mapping: see below |
| `rain diff <from> <to>` | Not the same operation. Atmos `diff` (alias `plan`) | `rain diff` compares two templates and never creates a changeset. Atmos `diff`/`plan` creates a changeset against the deployed stack and renders it without executing |
| `rain fmt` | `atmos aws cloudformation fmt <component> -s <stack> [--check]` | `rain fmt --verify` → `fmt --check`. Atmos `fmt` writes in place by default. It strips blank lines between sections, so a Rain-formatted template shows as unformatted once. Native comment-preserving YAML round-trip; no formatter binary |
| `rain cat` | `atmos aws cloudformation get template <component> -s <stack> [--original]` | Fetches the deployed stack's template. Default = processed template (like `rain cat --transformed`); `--original` = what was submitted (like `rain cat --unformatted`) |
| `rain ls` | `atmos aws cloudformation list [-s <stack>]` | One region only (`--region`), annotated managed/unmanaged against configured components; `-s` is optional. Rain's `--all` (all regions) has no equivalent. Rain's `--changeset` → `changeset list` |
| `rain logs` (not `rain log`) | `atmos aws cloudformation logs <component> -s <stack> [--chart]` | Both have `--chart` (Rain: HTML Gantt) and include nested stacks by default |
| `rain watch` | `atmos aws cloudformation watch <component> -s <stack>` | Attaches to a stack's in-progress (or already-terminal) operation and streams events |
| `rain tree` | Not equivalent (Atmos `tree` is different) | Rain `tree` is a local-template dependency graph of Parameters/Resources/Outputs (`--dot`). Atmos `tree <component> -s <stack>` is a *deployed* nested-stack tree (recurses `AWS::CloudFormation::Stack` only) |
| `rain rm` | `atmos aws cloudformation delete <component> -s <stack>` | Respects `termination_protection`; never silently disables it (`--disable-termination-protection` is an explicit escape hatch). A stack that no longer exists reports "nothing to delete" even with `termination_protection: true` |
| `rain stackset deploy` | `atmos aws cloudformation stackset create\|update <component> -s <stack>` | Needs a `kind: aws/stackset` provision target (`accounts`, `regions`, `permission_model`). Rain's `--accounts`/`--regions` → those target fields. `--admin` (delegated admin, `CallAs`) is not supported by Atmos today |
| `rain stackset ls` | `atmos aws cloudformation stackset instances <component> -s <stack>` | |
| `rain stackset rm` | `atmos aws cloudformation stackset delete <component> -s <stack>` | |
| `rain bootstrap` | `atmos aws cloudformation backend create <component> -s <stack>` | Creates the S3 artifact bucket the `kind: aws/s3` target references (same provisioner as `atmos terraform backend create`). Rain also auto-creates `rain-artifacts-<account>-<region>` on deploy; Atmos auto-creates only with `provision.backend.enabled: true` (apply/deploy only) |
| `rain console` | `atmos auth console` | Credentialed console access; no stack-scoped deep link (PRD Non-Goal) |
| `rain info` | `atmos auth whoami` / `atmos describe config` | |
| `rain build` | No equivalent | Schema/Bedrock template codegen. Do NOT map to `atmos scaffold`: scaffold ships no CloudFormation template (PRD Non-Goal) |
| `rain forecast` | No equivalent | Closest practical check: `plan`/`diff` + `validate` (PRD Non-Goal) |
| `rain merge`, `rain module` | No equivalent | Template composition; see the `Module` row above |
| `rain cc` (Cloud Control API passthrough) | No equivalent | PRD Non-Goal |
| (no Rain equivalent) | `atmos aws cloudformation output <component> -s <stack> [key]` | Renders deployed stack Outputs — the most-requested Rain gap per this PRD's user research. Also available as `!aws.cloudformation.output` for cross-component consumption |
| (no Rain equivalent) | `atmos aws cloudformation drift detect` / `drift describe` | Native `DetectStackDrift`/`DescribeStackResourceDrifts` |
| (no Rain equivalent) | `atmos aws cloudformation changeset create/execute/list/delete` | Manual changeset control for two-phase pipelines; `apply`/`deploy` already do this implicitly. `changeset create`/`execute` fire `before`/`after.aws/cloudformation.changeset-create` / `changeset-execute` hook events (list them in a packaging hook's `events:` for the `rain deploy --no-exec` workflow) |

### `rain deploy` flag mapping

| Rain flag | Atmos |
|---|---|
| `--keep` | `disable_rollback: true` |
| `--role-arn` | `role_arn` |
| `--termination-protection` | `termination_protection: true` |
| `--detach`, `--nested-change-set`, `--ignore-unknown-params` | No equivalent |

**Confidence note for the agent**: this table reflects Rain's own docs and a 2026-10-08 field test
(Rain was archived 2026-07-29): `deploy`, `diff`, `fmt`, `cat`, `ls`, `logs`, `tree`, `rm`,
`stackset deploy|ls|rm`, `bootstrap`, `console`, `info`, `build`, `forecast`, `merge`, `module`,
`cc`. Do not assert a mapping for a Rain verb not listed here (`watch` was carried over from the
PRD, not re-verified); tell the user "I'm not certain that verb existed / what it mapped to"
rather than guessing.

## Identifying the User's Shape

| Shape | Recipe |
|---|---|
| Raw CloudFormation, no Rain (hand-written templates + `aws cloudformation deploy`/console) | Skip the directive table — go straight to [The Minimum-Viable Migration](#the-minimum-viable-migration) |
| Rain-managed templates with `!Rain::*` directives | Resolve every directive per the [mapping table](#rain-directive-mapping) first, then migrate |
| Rain + a Rain config file (`{Parameters: {...}, Tags: {...}}`, `RAIN_VAR_*` / `RAIN_DEFAULT_TAG_*` env defaults) | No config-file compatibility layer — each stack becomes one `aws/cloudformation` component; select the file's maps with `!include <file> .Parameters` / `.Tags`. The `RAIN_*` env defaults have no Atmos equivalent: set `parameters:` / `tags:` in (inheritable) stack config. Use `dependencies.components` for cross-stack ordering |

## The Minimum-Viable Migration

1. **Install Atmos.** See `atmos.tools/install`.
2. **Resolve `!Rain::` directives** in every template being migrated, per the table above. A
  `path:` template still containing `!Rain::*` tags fails locally at
  `render`/`validate`/`plan`/`apply` with `ErrAwsCloudFormationRainDirective` (directives listed,
  one hint each).
3. **Create `atmos.yaml`** pointing `components."aws/cloudformation".base_path` at wherever the
  templates already live — no forced reorganization, same stance as
  [from-native-terraform.md](from-native-terraform.md).
4. **Create one stack file** for one environment, pointing `path:` at the existing (now
  directive-free) template file. Set the region with `settings.aws_cloudformation.region` (then the
  active identity's region, then the AWS SDK chain; a component `env` `AWS_REGION` does not
  override it):
  ```yaml
  # stacks/dev.yaml
  components:
    "aws/cloudformation":
      vpc:
        path: template.yaml
        stack_name: acme-plat-dev-vpc
        settings:
          aws_cloudformation:
            region: us-east-2
        parameters: !include ../params/dev-parameters.json
        capabilities:
          - CAPABILITY_IAM
  ```
  The included `params/dev-parameters.json` can be the existing AWS CLI parameter array as-is:

  ```json
  [
    {"ParameterKey": "CidrBlock", "ParameterValue": "10.0.0.0/16"},
    {"ParameterKey": "Environment", "ParameterValue": "dev"}
  ]
  ```

  A malformed entry (missing `ParameterKey`, unknown field, wrong type) fails with the entry's
  index instead of deploying template defaults. `UsePreviousValue: true` (without a
  `ParameterValue`) is accepted and keeps the stack's current value; it works only on updates. On a
  first deployment (CREATE) it fails locally with a hint. For a Rain config file, select the map
  instead of including the whole file:

  ```yaml
  parameters: !include ../params/rain-config.yaml .Parameters
  tags: !include ../params/rain-config.yaml .Tags
  ```

  Including a whole Rain config file as `parameters:` fails: `Parameters` and `Tags` would be read
  as two parameters with map values.

  `tags:` must be a map. An AWS CLI `[{Key, Value}]` list or a `!tags` string list is rejected with
  a hint. Reshape an AWS CLI tags file with:

  ```yaml
  tags: !include 'tags.json ".[] as $t ireduce ({}; .[$t.Key] = $t.Value)"'
  ```

  `tags: !labels` is the explicit bridge from `metadata.labels`; labels and tags never flow into
  each other automatically.

5. **Authenticate and dry-run.** Pass `--identity` (or configure a default identity;
  see the atmos-auth skill) in place of Rain's ambient credentials. Run
  `atmos aws cloudformation plan vpc -s dev --dry-run` first: no AWS calls, static rules enforced
  (unknown `--target`, S3 target missing `bucket`/`region`, oversize template with no packaging
  target). It defers YAML functions, templates, source downloads, authentication, and hooks.
6. **Run `atmos aws cloudformation plan vpc -s dev`** and compare the predicted changeset against
  what `rain deploy --no-exec`/`aws cloudformation deploy --no-execute-changeset` produced before
  (not `rain diff`, which compares two templates).
7. **Run `atmos aws cloudformation deploy vpc -s dev`** and confirm the end-of-deploy Outputs
  summary matches what the stack already had deployed (via `rain cat`/console) before the
  migration.

## Common Gotchas

### The component type string has a slash

Under `components:`, `aws/cloudformation` is one literal component-type key. The slash
does not create a nested mapping. Both `aws/cloudformation:` and `"aws/cloudformation":`
are valid YAML; quoting this key is optional.

### Secrets go into `parameters:`, not `env:`

Atmos is SDK-native: there is no subprocess, so a component `env:` cannot carry parameters into
anything. `NoEcho` template parameters fed with `!secret` in `parameters:` are the
delivery channel, and are masked in every rendered surface (changeset diffs, `describe`, logs) —
see [from-native-terraform.md](from-native-terraform.md)'s "Common Gotchas" for the equivalent
Terraform-side pattern, and the [Secrets skill](../../atmos-secrets/SKILL.md) for `!secret` usage.

### This component type is experimental

`atmos aws cloudformation` carries an experimental annotation today. Say so plainly when advising
a user to adopt it in a production pipeline — point them at the CLI docs
(`website/docs/cli/commands/aws/cloudformation/`) for the current verb surface, which may still
change.

### Rain's artifact bucket vs. Atmos's `backend`

Rain auto-creates its artifact bucket silently on first use. Atmos never does this by default —
either run `atmos aws cloudformation backend create` explicitly (mirroring `atmos terraform backend
create`), or set `provision.backend.enabled: true` for the same opt-in auto-provisioning Terraform
components get (apply/deploy only). Without it, a missing bucket fails on every packaging verb
(plan/diff/validate/changeset create/apply/deploy) with `ErrAwsCloudFormationBackendMissing` and a
hint naming `backend create` — never a surprise resource creation.

## When to Escalate to Other Skills

Same routing as [from-native-terraform.md](from-native-terraform.md)'s equivalent section, plus:

- **Cross-component Outputs consumption (CFN ↔ Terraform interop)** →
  [atmos-yaml-functions](../../atmos-yaml-functions/SKILL.md) for `!aws.cloudformation.output` and
  `!terraform.output`
- **Packaging destinations, StackSets, GitOps delivery targets** → the `provision:` section
  fields documented at `/stacks/components/aws-cloudformation`
- **Secrets flowing into `parameters:`** → [atmos-secrets](../../atmos-secrets/SKILL.md)

## What to NOT Do

- Do not tell a user `!Rain::*` directives "just work" in Atmos, or that there is any
  compatibility shim — there is none.
- Do not claim the packaging pipeline replicates full `aws cloudformation package`/Rain `pkg` asset
  rewriting — it packages only the template body; assets go through `archive` + `publish` hooks
  with the key passed via `parameters:`. Do not suggest an upload YAML function.
- Do not map `rain tree` to `atmos aws cloudformation tree`, `rain diff` to `plan`, or `rain build`
  to `atmos scaffold` as equivalents.
- Do not invent Atmos verb names beyond the [cross-reference table](#rain--atmos-verb-cross-reference) — verify against `cmd/aws/cloudformation/cloudformation.go`'s `init()` if in doubt.
- Do not present `aws/cloudformation` as stable/GA — it is explicitly experimental.
