# Migrating from Rain / Raw CloudFormation

This reference is a scenario-keyed decision guide for moving a Rain-managed or raw-CloudFormation
repo onto Atmos's native `aws/cloudformation` component type. For the full user-facing prose
tutorial, see [atmos.tools/migration/from-rain](https://atmos.tools/migration/from-rain).

**Framing matters**: this is migrating *off* Rain / raw CloudFormation *onto* Atmos, not "how to
write Rain syntax in Atmos." There is no Rain CLI or config-file compatibility layer, and no
`!Rain::` directive preprocessing. A template containing `!Rain::*` tags is not valid
CloudFormation and Atmos will not accept it as-is — every directive must be resolved to a
plain-CloudFormation or Atmos-native equivalent before the template is handed to
`aws/cloudformation`. This is the single most likely blocker for a real migration; walk the
directive table below with the user before touching anything else.

Atmos's `aws/cloudformation` component type is currently **experimental**
(`atmos aws cloudformation`, alias `atmos aws cfn`) — SDK-native, no `aws`/`cfn`/`sam`/`rain`
binary dependency. See `website/docs/cli/commands/aws/cloudformation/` for the full verb surface
and `website/docs/stacks/components/aws-cloudformation.mdx` for the stack-config field reference.

## Core Principles (Rain-Specific)

1. **Templates are not preprocessed.** `aws/cloudformation` reads a component's `template:` file
   as raw bytes (`os.ReadFile`) and submits it to the CloudFormation API — Atmos never rewrites,
   merges, or macro-expands the template body. This is a deliberate design boundary, not a gap: it
   means every `!Rain::*` directive (which Rain resolves by *preprocessing* the template before
   submission) has no direct drop-in replacement at the same layer. The fix in every case is to
   move the substitution **out of the template body** and into a layer Atmos or CloudFormation
   itself already handles — Atmos stack config (`parameters:`, `!env`, `!template`, inheritance)
   for anything CloudFormation Parameters can express, or CloudFormation's own native
   `Fn::Transform`/`AWS::Include` intrinsic for template-fragment reuse.
2. **Existing templates and parameter files are `!include`d or pointed at, never rewritten.**
   Point `template:` at the existing `.yaml`/`.json` template file unchanged (after resolving any
   `!Rain::` directives per the table below). If the user has a Rain/CFN parameters JSON file
   (`--parameters` flag or `Parameters.json`), pull it into the component's `parameters:` section
   with `!include`. Migration is opt-in, matching the Terraform migration guide's philosophy — see
   [from-native-terraform.md](from-native-terraform.md) Core Principle 2 for the identical stance.
3. **No 1:1 CLI compatibility.** `atmos aws cloudformation` verbs are Atmos-native — see the verb
   cross-reference table below. Do not tell a user to alias `rain` to `atmos aws cfn`; flag names,
   output shape, and confirmation semantics differ.
4. **Template packaging is size-triggered, not full `aws cloudformation package`/Rain `pkg`
   parity.** `apply`/`deploy` auto-uploads the **template body itself** to the component's
   `kind: aws/s3` provision target when it exceeds CloudFormation's 51,200-byte inline limit
   (`pkg/component/aws/cloudformation/packaging.go`). It does **not** currently rewrite local-asset
   references inside the template (Lambda source zips, nested-stack templates referenced by
   relative path) the way `aws cloudformation package` or Rain's `pkg`/`!Rain::S3` do. Tell users
   who rely on that: pre-upload those assets to S3 out-of-band (a hook, a build step, or a
   pre-existing pipeline) and reference the resulting S3 location directly in the template until a
   future phase closes this gap. Do not claim full asset-packaging parity — it does not exist yet.
5. **Crawl → walk → run**, same arc as every other migration reference: get to a working
   `atmos aws cloudformation plan`/`deploy` first, defer `provision:` targets, StackSets,
   inheritance, and catalogs until the user has a concrete need.

## `!Rain::` Directive Mapping

Resolve every directive before the template is handed to Atmos — there is no compatibility layer,
so a template still containing `!Rain::*` tags will fail as invalid CloudFormation.

| Rain directive | What it did | Atmos-native replacement |
|---|---|---|
| `!Rain::Constant` | Injects a named constant's value into the template at preprocess time | Move the value to a CloudFormation `Parameters:` entry, referenced via `!Ref` in the template; feed the value from the component's `parameters:` section in stack config (which itself supports inheritance, Go templates, and `!env`/`!template`) |
| `!Rain::Env` | Injects an environment variable's value into the template at preprocess time | Same as `Constant`: turn it into a `Parameters:` entry, and set the component's `parameters:` value with `!env VAR_NAME` in the stack manifest |
| `!Rain::Include` | Merges an external JSON/YAML fragment into the template at preprocess time | No direct Atmos-side equivalent (templates aren't preprocessed). For genuine fragment reuse, use CloudFormation's own native `Fn::Transform`/`AWS::Include` intrinsic (resolved by CloudFormation itself at deploy time, from a fragment already in S3) — this is a CloudFormation feature, not Rain- or Atmos-specific. For anything more structural, see `Module` below |
| `!Rain::Embed` | Inlines a local file's contents as a string literal (e.g. Lambda inline code, `UserData` scripts) at preprocess time | For small scripts: inline the content directly using a YAML block scalar (`|` / `>`) in the template by hand — this is a one-time manual flatten, not an ongoing process. For larger assets: pre-upload to S3 out-of-band and reference the S3 location directly (same gap noted in Core Principle 4) |
| `!Rain::S3` | Uploads a local file/directory to S3 and rewrites the reference (e.g. Lambda `S3Bucket`/`S3Key`, nested-stack `TemplateURL`) at preprocess time | **Partial today**: the template body itself auto-packages via the component's `kind: aws/s3` provision target when it exceeds the inline size limit. Arbitrary local assets (Lambda zips, nested templates by relative path) are **not** auto-rewritten yet — pre-upload them out-of-band and reference the resulting S3 URL directly in the template |
| `!Rain::Module` | Client-side, multi-file template composition (Rain's own docs mark this experimental) | Not supported — use AWS CDK for real modular/reusable template composition. This mirrors the PRD's own Non-Goal: Rain's module system is not a design Atmos is replicating |

### Before/After: `Constant`/`Env` → Parameters

Before (Rain):

```yaml
# template.yaml (Rain-preprocessed)
Resources:
  Bucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: !Rain::Constant BucketNamePrefix-bucket
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
      template: template.yaml
      parameters:
        BucketNamePrefix: acme-plat
        DeployOwner: !env DEPLOY_OWNER
```

### Before/After: `Embed` → inline block scalar

Before (Rain):

```yaml
Resources:
  Function:
    Type: AWS::Lambda::Function
    Properties:
      Code:
        ZipFile: !Rain::Embed handler.py
```

After (Atmos — content flattened into the template once, by hand):

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
zip) instead — see the `S3` row.

## Rain → Atmos Verb Cross-Reference

Every Atmos verb below is confirmed registered in `cmd/aws/cloudformation/cloudformation.go`'s
`init()` — do not invent verb names beyond this table. Verbs marked "not supported" are confirmed
PRD Non-Goals (`docs/prd/aws-cloudformation-component.md`); don't imply a workaround exists beyond
what's stated.

| Rain command | Atmos-native equivalent | Notes |
|---|---|---|
| `rain deploy` | `atmos aws cloudformation deploy <component> -s <stack>` | `deploy` = `apply` with `--auto-approve` implied, matching `atmos terraform deploy` |
| `rain diff` | `atmos aws cloudformation diff <component> -s <stack>` (or `plan`) | `plan`/`diff` are the same operation under different names; both create-and-render a changeset without executing it |
| `rain fmt` | `atmos aws cloudformation fmt <component> -s <stack> [--check]` | Native comment-preserving YAML round-trip — there is no `cfn-format` binary to shell out to, since Rain's own formatter was archived with Rain |
| `rain cat` | `atmos aws cloudformation get template <component> -s <stack> [--original]` | `get template` fetches the deployed stack's template via `GetTemplate`; the verb shape follows `helm get manifest`, not Unix `cat` |
| `rain ls` | `atmos aws cloudformation list -s <stack>` | Account-wide `ListStacks`, annotated with whether each stack matches a configured component's `stack_name`; not scoped to a single component |
| `rain log` | `atmos aws cloudformation logs <component> -s <stack> [--chart]` | Combined event log across a stack and its nested stacks |
| `rain watch` | `atmos aws cloudformation watch <component> -s <stack>` | Attaches to a stack's in-progress (or already-terminal) operation and streams events |
| `rain tree` | `atmos aws cloudformation tree <component> -s <stack>` | Nested-stack/resource dependency graph |
| `rain rm` | `atmos aws cloudformation delete <component> -s <stack>` | Respects `termination_protection`; never silently disables it (`--disable-termination-protection` is an explicit escape hatch) |
| `rain bootstrap` | `atmos aws cloudformation backend create <component> -s <stack>` | Provisions the S3 artifact bucket the component's `kind: aws/s3` provision target references — same registered S3 backend provisioner `atmos terraform backend create` uses. Opt into automatic provisioning with `provision.backend.enabled: true` instead of an explicit bootstrap step, if preferred |
| `rain build` | Not supported — use [`atmos scaffold`](https://atmos.tools) | Skeleton-template generation is dev-time code generation, not a component-type concern (PRD Non-Goal) |
| `rain forecast` | Not supported — use `plan`/`diff` + `validate` | Changeset review plus server-side `ValidateTemplate` cover the practical "will this deploy work" question (PRD Non-Goal) |
| `rain cc` (Cloud Control API passthrough) | Not supported | Niche even within Rain; no equivalent planned (PRD Non-Goal) |
| (no Rain equivalent) | `atmos aws cloudformation output <component> -s <stack> [key]` | Renders deployed stack Outputs — the most-requested Rain gap per this PRD's user research. Also available as `!aws.cloudformation.output` for cross-component consumption |
| (no Rain equivalent) | `atmos aws cloudformation drift detect` / `drift describe` | Native `DetectStackDrift`/`DescribeStackResourceDrifts` — Rain only implied drift support through its stack-monitoring workflow, with no dedicated commands |
| (no Rain equivalent) | `atmos aws cloudformation changeset create/execute/list/delete` | Manual changeset control for two-phase deploy pipelines; `apply`/`deploy` already do this implicitly |
| (no Rain equivalent) | `atmos aws cloudformation stackset create/update/delete/instances` | Multi-account/multi-region orchestration |

**Confidence note for the agent**: the left column above is limited to Rain verbs this PRD's own
Non-Goals/verb-surface sections explicitly name (`fmt`, `cat`, `ls`, `bootstrap`, `log`, `rm`,
`deploy`, `diff`, `watch`, `tree`, `build`, `forecast`, `cc`). Rain's full historical CLI may have
had additional niche subcommands (e.g. an `info`/`merge`-style command) — do not assert a mapping
for a Rain verb not confirmed here; tell the user "I'm not certain that verb existed / what it
mapped to" rather than guessing.

## Identifying the User's Shape

| Shape | Recipe |
|---|---|
| Raw CloudFormation, no Rain (hand-written templates + `aws cloudformation deploy`/console) | Skip the directive table — go straight to [The Minimum-Viable Migration](#the-minimum-viable-migration) |
| Rain-managed templates with `!Rain::*` directives | Resolve every directive per the [mapping table](#rain-directive-mapping) first, then migrate |
| Rain + a `rain.yaml`/pkg config for multi-stack orchestration | Config-file orchestration has no Atmos compatibility layer — each stack becomes one `aws/cloudformation` component in stack config; `depends_on`/DAG ordering replaces Rain's own ordering logic |

## The Minimum-Viable Migration

1. **Install Atmos.** See `atmos.tools/install`.
2. **Resolve `!Rain::` directives** in every template being migrated, per the table above. A
   template still containing `!Rain::*` tags is not valid CloudFormation and will fail at
   `render`/`validate`.
3. **Create `atmos.yaml`** pointing `components."aws/cloudformation".base_path` at wherever the
   templates already live — no forced reorganization, same stance as
   [from-native-terraform.md](from-native-terraform.md).
4. **Create one stack file** for one environment, pointing `template:` at the existing (now
   directive-free) template file:
   ```yaml
   # stacks/dev.yaml
   components:
     "aws/cloudformation":
       vpc:
         template: template.yaml
         stack_name: acme-plat-dev-vpc
         parameters: !include ../params/dev-parameters.json
         capabilities:
           - CAPABILITY_IAM
   ```
5. **Run `atmos aws cloudformation plan vpc -s dev`** and compare the predicted changeset against
   what `rain diff`/`aws cloudformation deploy --no-execute-changeset` produced before.
6. **Run `atmos aws cloudformation deploy vpc -s dev`** and confirm the end-of-deploy Outputs
   summary matches what the stack already had deployed (via `rain cat`/console) before the
   migration.

## Common Gotchas

### The component type string has a slash

Stack config uses the literal key `components."aws/cloudformation".<name>:` — the quotes around
`"aws/cloudformation"` are required YAML syntax because the key contains a `/`. This is unlike
every other built-in component type (`terraform`, `helmfile`, etc.), which are flat, unquoted
keys.

### Secrets go into `parameters:`, not `env:`

SDK-native execution means there is no subprocess to receive `env:` the way Rain (which shells out
to the `aws` CLI) does. `NoEcho` template parameters fed with `!secret` in `parameters:` are the
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
components get. A packaged `apply`/`deploy` against a missing bucket fails with an actionable hint
rather than a raw S3 error either way — never a surprise resource creation.

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
  rewriting — it currently auto-packages only the template body past the inline size limit.
- Do not invent Atmos verb names beyond the [cross-reference table](#rain--atmos-verb-cross-reference) — verify against `cmd/aws/cloudformation/cloudformation.go`'s `init()` if in doubt.
- Do not present `aws/cloudformation` as stable/GA — it is explicitly experimental.
