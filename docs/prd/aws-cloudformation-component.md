# PRD: `aws/cloudformation` Component Type

## Summary

Add `aws/cloudformation` as a native, SDK-based Atmos component type: a stack-scoped CloudFormation
stack, deployed directly through the AWS SDK for Go v2, with full participation in the same registry
that backs `terraform`, `helmfile`, `packer`, `ansible`, `container`, `emulator`, `kubernetes`, and
`helm` — inheritance, catalogs, deep-merge, DAG dependencies, hooks, auth/identity, and provision
targets, all for free.

`aws/cloudformation` is also the first member of a new `aws/*` namespace of **AWS-native primitives**:
component types that let Atmos manage AWS resources directly through AWS's own APIs, with no
Terraform in the loop at all. CloudFormation is the natural first (and most general-purpose) member
of that family, since it covers nearly the full AWS resource surface, but the naming and registry
changes in this PRD are written to generalize to future siblings (see
[Naming & Registry Precedent](#naming--registry-precedent)), not to be a one-off special case.

CloudFormation today is reachable only through the documented **custom component type** escape hatch
(`website/docs/components/custom.mdx`, `website/docs/components/components-overview.mdx` — both name
CloudFormation as their flagship example of "beyond the native types"). That means hand-rolled
`steps:` glue per user, no first-class `vars`/inheritance/DAG, and no dedicated CLI verbs. This PRD
promotes CloudFormation out of that escape hatch and into a first-class type —
`docs/prd/component-registry-pattern.md` (the PRD for the registry pattern itself) already names
CloudFormation three times as the anticipated future plugin type; this document is that promise kept.

## Motivation

A prior attempt at CloudFormation support exists as PR #2536 (`osterman/rain-component-type`), which
wrapped the [`rain`](https://github.com/aws-cloudformation/rain) CLI — AWS's own CloudFormation
workflow tool — via shell-out, following the same pattern as the `ansible`/`packer` component types.
That PR was **closed the same day this PRD was written**, because AWS archived Rain upstream:

> "Rain is no longer actively maintained and has been deprecated. We are no longer accepting feature
> requests." — [aws-cloudformation/rain#801](https://github.com/aws-cloudformation/rain/pull/801)

AWS's own migration guidance points users to the AWS CLI, AWS CDK, `cfn-lint`, and `cfn-guard` instead
of Rain. This removes "shell out to `rain`" as a viable design entirely and is the deciding argument
for building `aws/cloudformation` **SDK-native** — architecturally in the same family as the native
`helm` (`helm.sh/helm/v4`) and `kubernetes` (`client-go`) components, not the shell-out family
(`ansible`, `packer`, and PR #2536's abandoned design).

This PRD draws on Rain's feature set for **capability inspiration only** — deploy/changesets, drift
detection, stack sets, nested-stack dependency trees, combined event logs, template packaging — not
for CLI or config syntax compatibility. Every capability below is expressed in Atmos-native verbs,
flags, and YAML conventions. There is no Rain compatibility layer.

### User Research

Direct feedback from a daily Rain user (community Slack, 2026-08-24) validates the verb surface and
sharpens one priority:

- **Daily drivers**: the `stack` command family — `deploy`, `logs`, `ls`, `rm`, `watch`, `cat`,
  `stackset` — "all day to deploy, update, and remove stacks," plus `fmt` and `diff` from the
  template family. Every one of these has a first-class equivalent in this PRD's
  [CLI surface](#cli-command-surface). `merge` was used "rarely" and `cc`/`forecast`/modules not at
  all — validating those Non-Goals.
- **The #1 dislike: no native way to get stack outputs.** The user's workaround is a shell alias
  around `aws cloudformation describe-stacks --query "Stacks[*].Outputs[*]" --stack-name ...` —
  and, in their words, "rain has nice output UI/UX at the end of a deployment or update, so I don't
  know why that couldn't include a native command to do the same." This is direct evidence for two
  decisions: the **`output` verb ships in Phase 1** (not Phase 2), and `apply`/`deploy` end with a
  rendered Outputs summary (see [Live Progress Streaming](#live-progress-streaming)) so the
  end-of-deploy UX and the standalone verb are the same view.

## Goals

- A first-class `aws/cloudformation` component type, addressable and composable like every other
  component type: `metadata.inherits`, catalogs, deep-merge, `depends_on` DAG ordering, hooks, `env`,
  `settings`, `auth`, and `provision` targets all work exactly as they do for Terraform/Helm/Kubernetes.
- Capability parity with Rain's feature set — deploy, changesets, drift detection, stack sets, nested
  stacks, rollback/stack-policy handling, template packaging — expressed through Atmos-native CLI verbs
  and first-class YAML config, not Rain's CLI/config shape.
- SDK-native execution (`github.com/aws/aws-sdk-go-v2/service/cloudformation` — a new module in the
  already-vendored `aws-sdk-go-v2` family), no external binary dependency, no toolchain resolution
  needed for the component type itself.
- Establish the `aws/*` namespace as Atmos's general home for AWS-native primitives that bypass
  Terraform, with `aws/cloudformation` as the reference implementation future siblings will follow.

## Non-Goals

- No Rain CLI or Rain config-file compatibility layer. Users coming from Rain get a
  [migration guide](#migration-guide-deferred-scope-only) (deferred, see below), not a syntax bridge.
- No Cloud Control API passthrough (Rain's `cc` — niche and experimental even within Rain).
- No `console` verb (Rain's stack-scoped console deep-link). `atmos auth console` already covers
  credentialed console access; a stack-scoped deep-link is not worth a dedicated verb.
- No `forecast` verb (Rain's experimental pre-deploy failure prediction). Changeset review
  (`plan`/`diff`) plus `validate` cover the practical "will this deploy work" question.
- No `!Rain::` directive preprocessing (`Embed`, `Include`, `S3`, `Module`, `Constant`, `Env`).
  Templates containing these directives are not valid CloudFormation and are not accepted as-is —
  there is no compatibility layer. The [migration guide](#migration-guide-deferred-scope-only) maps
  each directive to its Atmos-native equivalent (asset upload → the packaging pipeline;
  `Constant`/`Env` → `parameters:` fed from Atmos stack config; `Module` → CDK, per the modules
  non-goal below).
- No client-side "module" system (Rain's own docs mark this experimental; AWS CDK is the supported
  answer for reusable multi-file template composition).
- No bespoke `type: cloudformation` workflow step. Every existing component type — including the
  SDK-native `helm`/`kubernetes` — is invoked from workflows, hooks, and custom commands via the
  generic `type: atmos` step (e.g. `command: aws cloudformation deploy <component>`); CloudFormation
  follows the same pattern.
- No stack import/adoption (`ImportExistingResources`, adopting pre-existing unmanaged stacks or
  resources into an Atmos-managed stack). Real demand exists, but resource import is its own design
  surface (per-resource identifiers, drift-vs-import interplay); it is deliberately deferred to a
  future phase or follow-up PRD rather than left unaddressed.
- No skeleton-template-generation verb (Rain's `build`). Generating a new CloudFormation template from
  scratch is dev-time code generation — the job of `atmos scaffold`, not this component type. See
  [Template Scaffolding](#template-scaffolding-delegated-to-atmos-scaffold).
- No migration-guide content in this PRD. The guide's scope and location are recorded so follow-up work
  has a clear target, but authoring it is out of scope until this design is approved.

## Naming & Registry Precedent

The component type identifier is the literal, namespaced string **`aws/cloudformation`** — a
deliberate departure from every existing native type, which are all flat single words (`terraform`,
`helmfile`, `packer`, `ansible`, `container`, `emulator`, `kubernetes`, `helm` —
`pkg/config/const.go:75-82`). This is not cosmetic: it is the
first member of an `aws/*` namespace reserved for AWS-native primitives Atmos can operate directly,
without Terraform as an intermediary. Framing it as "one cloud vendor's IaC tool" would undersell the
intent — the namespace exists so Atmos can grow additional AWS-native primitives under `aws/*` later
without re-litigating the naming question each time.

How the namespaced form propagates through each layer:

| Layer | Value |
|---|---|
| Go type constant (`pkg/config/const.go`) | `CloudFormationComponentType = "aws/cloudformation"` |
| YAML section key | `components."aws/cloudformation".<name>:` |
| JSON Schema definition key | `aws_cloudformation_component_manifest` (definition *names* avoid `/`; the schema *property* name is the literal string `"aws/cloudformation"` — JSON Schema property names may contain `/` freely) |
| Go package path | `pkg/component/aws/cloudformation/` (namespaced to mirror the type, establishing the layout future `pkg/component/aws/...` primitives would also use) |
| CLI command path | `atmos aws cloudformation <verb>`, nested under the existing `cmd/aws/` group alongside `ecr`/`eks`/`security`/`compliance` |
| Provision target kind (direct deploy) | `kind: aws/cloudformation` — the type string itself, per the kubernetes precedent (`kind: kubernetes`) |

The CLI point deserves its own note: Cobra commands are space-separated tokens, not literal strings
containing `/`. `atmos aws cloudformation deploy` *is* the realization of the `aws/` namespace at the
CLI layer — there's no literal slash in a command name, and none is needed. The command lives
**under the `aws` group, never as a top-level `atmos cloudformation`** — this is what makes the
namespace real to users. This also means `cmd/aws/aws.go`'s existing group gains a new
subcommand-group member the same way `ecr`/`eks` are already structured, rather than requiring a new
top-level command tree. The subcommand group registers the Cobra alias **`cfn`**
(`Aliases: []string{"cfn"}`), so `atmos aws cfn deploy vpc -s dev` works everywhere
`atmos aws cloudformation deploy` does — matching how the community actually abbreviates it.

**Open engineering risk to spike early, before implementation starts:** every part of the registry
and whitelist machinery below was written assuming flat, `/`-free type strings, and needs to be
verified — not assumed — to tolerate `"aws/cloudformation"`:

- `pkg/component/registry.go`'s `GetGroup()`-based grouping in `ListProviders`, and any sorting or
  shell-completion logic keyed on `GetType()`.
- Every whitelist site in [Registry & Whitelist Wiring](#registry--whitelist-wiring) below —
  particularly string-based lookups, `switch` statements using the type as a `case`, and any place a
  type string is used to build a file path, flag name, or YAML key via naive concatenation.
- JSON Schema generation and IDE tooling that maps a schema property name to a definition/anchor name
  (the definition itself is named `aws_cloudformation_component_manifest`; the property it's
  referenced from is the literal `"aws/cloudformation"` string — these must not be conflated).
- **Two confirmed `/`-hostile sites already identified** (not hypothetical): the central path resolver
  `pkg/utils/component_path_utils.go:204-239` derives `ATMOS_COMPONENTS_<TYPE>_BASE_PATH` env-var names
  from the type string (a `/` has no sane env-var spelling), and the custom-component provider's
  default base path is built by naive concatenation, `fmt.Sprintf("components/%s", typeName)`
  (`pkg/component/custom/provider.go:55`), which turns a `/`-bearing type into a nested directory. Both
  need explicit handling (e.g. `/` → `_` mangling for env vars, `filepath.FromSlash` for paths) or they
  become arguments for the fallback below.

If this spike surfaces a hard blocker (e.g., a place in the codebase that truly cannot tolerate `/`
in a type string without a disproportionate refactor), the fallback is a flat `cloudformation` type
constant with the `aws/*` intent expressed instead through `GetGroup()` returning `"aws"` and the CLI
nesting under `atmos aws cloudformation` — that preserves the CLI-facing and product-facing "AWS
namespace" experience even if the internal type string stays flat. This PRD's default is the literal
namespaced string; the fallback is recorded here so it doesn't require a second design round if the
spike finds a blocker.

## Public Interface

`template`, `parameters`, `capabilities`, `tags`, and `stack_policy` are **first-class component
sections** — siblings of `vars`/`env`/`metadata`, not nested under `vars` (which is reserved for
arbitrary template variables). The structural precedent is **helm's section bag**
(`chart`/`values`/`repositories`/... via `helmComponentSectionKeys`,
`internal/exec/stack_processor_process_stacks_helpers_extraction.go:324-334`), *not* `container`:
container is not a built-in stack-processor type — it rides the custom pass-through branch
(`internal/exec/stack_processor_process_stacks.go:1250-1300`) that deep-merges every top-level key
with zero whitelisting, a privilege built-in types do not get. As a built-in type,
`aws/cloudformation`'s sections must be explicitly whitelisted at every site enumerated in
[Section-Whitelist Plumbing](#section-whitelist-plumbing) or they are silently dropped during stack
processing.

```yaml
components:
  "aws/cloudformation":
    vpc:
      stack_name: acme-plat-ue2-dev-vpc      # explicit; no legacy name-pattern interpolation
      template: template.yaml                # path relative to the component's base path

      parameters:
        CidrBlock: "10.0.0.0/16"
        Environment: dev
        DbPassword: !secret DB_PASSWORD      # secrets flow into parameters, not env — see Secrets & NoEcho

      capabilities:
        - CAPABILITY_IAM
        - CAPABILITY_NAMED_IAM

      tags:
        Team: platform
        Environment: dev

      stack_policy:
        file: stack-policy.json              # optional, protects specific resources from updates

      role_arn: "arn:aws:iam::123456789012:role/cfn-deploy"
      notification_arns: []
      disable_rollback: false
      termination_protection: true
      timeout_in_minutes: 30

      settings:
        aws_cloudformation:
          region: us-east-2

      provision:
        default: aws
        targets:
          aws:
            kind: aws/cloudformation          # deploy directly against the CloudFormation API
          artifacts:
            kind: aws/s3                      # packaging destination: large templates/nested stacks/assets
            bucket: acme-plat-cfn-artifacts   # `--target artifacts` = publish packaged output only, no deploy
            prefix: vpc/
          gitops:
            kind: git                         # publish the packaged template for a review pipeline
            repository: acme/infra-gitops
            path: stacks/dev/vpc

      secrets:
        vars:
          DB_PASSWORD:
            store: app-secrets
            required: true

      hooks:
        after:
          apply:
            - kind: command
              command: echo "vpc stack deployed"
```

Inheritance, catalogs, and deep-merge apply exactly as they do for every other component type
(`metadata.inherits` deep-merges `template`/`parameters`/`tags`/`capabilities` from abstract base
components). `provision.targets` reuses the existing `ProvisionSettings`/`ProvisionTarget` struct
(`pkg/schema/schema.go`) that already backs Helm/Kubernetes delivery — no new abstraction is
introduced for "deploy directly" vs. "publish for GitOps."

## CLI Command Surface

```bash
# Core lifecycle — mirrors the Helm/Kubernetes verb set
atmos aws cloudformation plan vpc -s dev
atmos aws cloudformation diff vpc -s dev
atmos aws cloudformation apply vpc -s dev
atmos aws cloudformation deploy vpc -s dev        # apply + provision-target delivery
atmos aws cloudformation delete vpc -s dev
atmos aws cloudformation validate vpc -s dev
atmos aws cloudformation render vpc -s dev         # client-side template render only, no API calls

# Outputs (Phase 1 — the most-cited Rain gap, see User Research & Cross-Component Outputs)
atmos aws cloudformation output vpc -s dev          # all Outputs; verb matches `atmos terraform output` (alias: outputs)
atmos aws cloudformation output vpc -s dev VpcId    # a single Output by key
atmos aws cloudformation output vpc -s dev --format=github  # json|yaml|hcl|env|dotenv|bash|csv|tsv|github

# Artifact bucket lifecycle — mirrors `atmos terraform backend` verb-for-verb
atmos aws cloudformation backend create vpc -s dev
atmos aws cloudformation backend describe vpc -s dev
atmos aws cloudformation backend update vpc -s dev
atmos aws cloudformation backend delete vpc -s dev
atmos aws cloudformation backend list

# Deployed-stack retrieval (Phase 2) — the `helm get manifest` analog
atmos aws cloudformation get template vpc -s dev   # GetTemplate: what is actually deployed right now
atmos aws cloudformation get policy vpc -s dev     # GetStackPolicy

# Template formatting (Phase 2) — the `atmos terraform fmt` analog
atmos aws cloudformation fmt vpc -s dev [--check]

# Account stack inventory (Phase 2)
atmos aws cloudformation list                      # ListStacks: managed + unmanaged stacks in the account/region

# JIT-vendored source management (Phase 2) — mirrors `atmos terraform source` / `atmos helmfile source`
atmos aws cloudformation source pull vpc -s dev
atmos aws cloudformation source list [vpc] [-s dev]
atmos aws cloudformation source describe vpc -s dev
atmos aws cloudformation source delete vpc -s dev

# Changesets
atmos aws cloudformation changeset create vpc -s dev
atmos aws cloudformation changeset execute vpc -s dev --changeset <name>
atmos aws cloudformation changeset list vpc -s dev
atmos aws cloudformation changeset delete vpc -s dev --changeset <name>

# Drift
atmos aws cloudformation drift detect vpc -s dev
atmos aws cloudformation drift describe vpc -s dev

# Stack sets (Phase 3, see Rollout Phases)
atmos aws cloudformation stackset create vpc -s dev
atmos aws cloudformation stackset update vpc -s dev
atmos aws cloudformation stackset delete vpc -s dev
atmos aws cloudformation stackset instances vpc -s dev

# Observability (Phase 3)
atmos aws cloudformation tree vpc -s dev           # nested-stack/resource dependency graph
atmos aws cloudformation logs vpc -s dev [--chart]  # combined nested-stack event log
atmos aws cloudformation watch vpc -s dev          # live stack-status monitor
```

Standard flags, matching the conventions established by `kubernetes-apply.mdx`/`helm-plan.mdx`:
`--stack/-s` (required), `--all`/`--affected`/`--include-dependents`, `--tags`/`--labels` (bulk
selection), `--target` (select a `provision.targets` entry). `diff`/`plan` additionally accept
`--against`/`--from-manifest`/`--context`, matching Helm's diff surface. Nothing here is inherited
from a shared base command: per the `cmd/helm/helm.go` precedent (flag registration, `--all`/
`--affected` mutual exclusion, `info.ComponentType` wiring), the `cloudformation` command group must
replicate the standard ~10-flag selection block itself, via `flags.NewStandardParser()`.

**Confirmation semantics**: `apply` and `delete` prompt for interactive confirmation on a TTY and
accept `--auto-approve` to skip it; `deploy` implies `--auto-approve`, matching the established
`terraform deploy` = "apply with auto-approve" convention.

## Design Deep-Dive

### Template Packaging (Rain's `pkg` equivalent — in scope)

Large templates, nested-stack templates, and local assets (e.g. Lambda source) that a CloudFormation
API call can't accept inline must be uploaded to S3 and the template rewritten to reference them
before a real deploy — this is what `aws cloudformation package` and Rain's `pkg` do. This is a
**deploy-time** concern owned by this component type's `apply`/`deploy` pipeline, and the packaging
destination is itself **a provision target, not a settings field**: a **`kind: aws/s3`** target carrying
its own destination config (`bucket`/`prefix`, optional `region`), exactly as `kind: git` carries
`repository`/`path`. There is no separate `settings.aws_cloudformation.s3_bucket` — one construct
declares every delivery destination. This extends the shared `ProvisionTarget` struct with the
S3-kind fields; no new abstraction is introduced, and Helm/Kubernetes delivery is untouched.

How the targets interact:

- **`kind: aws/cloudformation`** (deploy against the CloudFormation API): the direct-deploy kind is
  the component type string itself, following the kubernetes precedent (the kubernetes component's
  direct-deploy target registers `kind: kubernetes` — the kind names the delivery mechanism, and
  `aws` alone names a cloud, not a mechanism; a future `aws/lambda` sibling registers its own).
  When no `provision:` section is declared, an implicit default target of this kind applies,
  mirroring kubernetes' implicit `cluster` target — zero config for the common case. When packaging
  is needed, assets upload to the component's `kind: aws/s3` target before
  `CreateStack`/`ExecuteChangeSet`. Resolution is implicit when the component declares exactly one
  `kind: aws/s3` target; with several, the deploy-style target names its packaging store explicitly
  (`packaging: <target-name>`), and an ambiguous setup is an error with a hint — never a silent
  guess.
- **`kind: git`** (GitOps delivery): packages through the same `kind: aws/s3` target first (the
  published, rewritten template references S3 URIs), then publishes to the repository.
- **`kind: aws/s3` selected directly** (`--target artifacts`): publish-only — upload the packaged
  template + assets and stop, no deploy. This is the near-term, inline-config form of the
  object-store delivery idea; the future `kind: artifact` target (below) is its named-backend
  generalization once `artifact.repositories` exists.
- A packaged deploy with **no `kind: aws/s3` target declared** fails with an actionable hint (add the
  target, or see [Artifact Bucket Provisioning](#artifact-bucket-provisioning-backend) for
  provisioning the bucket). Small templates that fit inline need no `kind: aws/s3` target at all.

**Transport and convergence — no dependency on the Artifacts PRD.** Everything packaging needs in
Phase 1 already ships today: the native-CI artifact store (`pkg/ci/artifact`, `aws/s3` backend —
content upload, SHA-256 integrity, metadata sidecars, identity-based auth; see
`docs/prd/native-ci-artifact-storage.md`), the same machinery Terraform planfile storage already
rides, plus the backend provisioner for the bucket itself (see
[Artifact Bucket Provisioning](#artifact-bucket-provisioning-backend)). The Atmos Artifacts PRD
(`docs/prd/artifacts.md`, in progress on a parallel branch) defines the long-term `pkg/artifact`
repository framework that `pkg/ci/artifact` converges into — it is a **compatibility constraint on
this design, not a prerequisite of it**. Implementation of `aws/cloudformation` proceeds without
waiting for any part of that framework. Three rules keep the two from contradicting:

1. **Narrow seam**: packaging calls a small upload interface defined in this package (upload
    content, get back an S3 URL + digest), implemented in Phase 1 by the `pkg/ci/artifact` `aws/s3`
    store. When `pkg/artifact` lands, an adapter satisfies the same seam — a transport swap, not a
    redesign.
2. **No vocabulary squatting**: this component type introduces no `artifact:`/`repositories:`
    config namespace, no `publish`/`mirror`/`pull` verbs, and no artifact-kind registry of its own.
    Its packaging destination is declared inline on the `kind: aws/s3` provision target
    (`bucket`/`prefix`) — the target-kind string deliberately matches the artifacts PRD's `aws/s3`
    repository kind and the stores `aws/ssm`-style vocabulary — with an optional
    `repository: <name>` reference added only after `artifact.repositories` exists.
3. **Deferred integrations stay deferred**: the `kind: artifact` provision target (below) and the
    `cloudformation/template` artifact kind — which lets air-gap bundles carry CloudFormation
    deployments the same way they carry images, tool packages, and vendored sources — activate when
    the artifacts framework merges, and appear in none of this PRD's phases.

**Future `kind: artifact` provision target.** Once `artifact.repositories.<name>` exists (Artifacts
PRD), a third delivery target kind falls out naturally:
`{kind: artifact, repository: <name>, path: ...}` publishes the packaged template + assets to a
named artifact repository (S3, OCI, Git-backed) *instead of* deploying — the object-store sibling of
today's `kind: git` flow, useful for StackSets admin accounts, Service Catalog, or air-gap drops.
This mirrors exactly how `kind: git` references `git.repositories.<name>` (backend named once,
delivery intent per component — the pattern the Artifacts PRD codifies in its Prior Art). It is
deliberately **not** in this PRD's phases: it ships when the artifacts framework does, and nothing in
Phase 1 blocks on it.

### Artifact Bucket Provisioning (`backend`)

Rain auto-creates its artifact bucket on first use; Atmos instead follows the convention it already
established for Terraform state backends, reusing both halves of it:

- **Explicit lifecycle verbs**: an experimental **`backend` subcommand group**
  (`create`/`describe`/`update`/`delete`/`list`), mirroring `atmos terraform backend`
  (`cmd/terraform/backend/`) verb-for-verb. For this component type, "backend" means the S3 artifact
  bucket — declared by the component's `kind: aws/s3` provision target (`bucket`/`prefix`, see
  [Template Packaging](#template-packaging-rains-pkg-equivalent--in-scope)) — that packaged
  templates, nested-stack
  templates, and local assets are uploaded to — CloudFormation's own state is service-managed, so
  the artifact bucket is the type's only supporting infrastructure. The word `backend` is
  deliberately reused rather than inventing a synonym (`artifacts`, `bucket`), so the muscle memory
  transfers: `atmos terraform backend create` ↔ `atmos aws cloudformation backend create`.
- **Opt-in auto-provisioning**: the Terraform convention is not verbs-only — the Backend Provisioner
  (`docs/prd/backend-provisioner.md`, implemented in `pkg/provisioner/backend/`) auto-provisions the
  S3 bucket with secure defaults (encryption, versioning, public-access blocking) before
  `terraform init` when a component sets `provision.backend.enabled: true`, targeting dev/test
  cold-start friction. CloudFormation reuses the **same S3 provisioner and the same config key**:
  with `provision.backend.enabled: true`, the artifact bucket is provisioned before packaging;
  without it, a packaged `apply`/`deploy` against a missing bucket fails with an actionable hint
  pointing at `atmos aws cloudformation backend create` (or at the opt-in flag) — a guided path
  instead of a raw S3 error, and never surprise resource creation by default.

Both paths go through the registered S3 backend provisioner (`docs/prd/s3-backend-provisioner.md`)
rather than duplicating bucket-creation logic inside `pkg/component/aws/cloudformation/`.

### Template Scaffolding (delegated to `atmos scaffold`)

Generating a *new* skeleton CloudFormation template (Rain's `build`, optionally AI-assisted) is
dev-time code generation, not a deploy-time or even component-type concern — it's the same job
`atmos scaffold` already does for Terraform components (`pkg/generator`, `cmd/scaffold/`). Rather than
duplicating a templating engine inside this component type, this PRD's implementation should ship an
embedded or catalog `cloudformation-template` scaffold (prompted fields for stack name, resource type,
region, etc.) that generates a starting `template.yaml` plus a matching
`components.aws/cloudformation.<name>` stack-manifest stanza. This is explicitly **not** an
`aws/cloudformation` component verb — it's a cross-reference to an existing subsystem, and no new
scaffolding logic belongs in `pkg/component/aws/cloudformation/`.

### Changesets

`plan`/`diff` implicitly create a changeset (`CreateChangeSet` + `DescribeChangeSet`) and render the
predicted resource changes without executing them, mirroring how `terraform plan` and `helm diff`
behave. `apply`/`deploy` execute the changeset (`ExecuteChangeSet`) rather than calling `UpdateStack`
directly, giving every apply the same "review before mutate" semantics as changesets provide, without
requiring users to manage changesets by hand. The explicit `changeset create/execute/list/delete`
verbs exist for users who want manual control (e.g. a two-phase deploy pipeline: create + review in
one job, execute in another).

Templates using macros/transforms (`Fn::Transform`, `AWS::Serverless` a.k.a. SAM) work through this
same changeset flow with no special-cased handling: `CreateChangeSet` expands the macro as part of
computing the changeset given `CAPABILITY_AUTO_EXPAND` in `capabilities:`, the same as any other
required capability.

### Live Progress Streaming

`apply`/`deploy`/`delete` stream per-resource stack events live while the operation converges
(polling `DescribeStackEvents`, rendering `CREATE_IN_PROGRESS` → `CREATE_COMPLETE` transitions as
they happen) — the same "watch it happen" experience `terraform apply` gives and Rain's `deploy`
shows by default. Silent-poll-until-terminal-state is explicitly not acceptable for Phase 1.

On an interactive TTY, in-progress resources get **animated, theme-aware spinners** via the existing
`pkg/ui/spinner` component (Rain's polished deploy animation is the bar to meet, using Atmos's own
TUI system rather than a bespoke renderer). The standard I/O layer handles degradation automatically
(non-TTY/CI output collapses to plain event lines; colors, width, and animation follow the
zero-configuration TTY-detection pipeline).

Every successful `apply`/`deploy` **ends with a rendered summary: final stack status plus the
stack's Outputs** as a themed table — the same view the standalone `output` verb renders. This is a
direct response to the top Rain complaint (see [User Research](#user-research)): Rain showed a nice
end-of-deploy view but offered no way to get it back afterward; Atmos gives the same view both at
deploy time and on demand.

`watch` (Phase 3) remains distinct as the verb for *attaching* to an operation already in progress,
including one started outside Atmos.

### Drift Detection

Native AWS SDK drift detection (`DetectStackDrift`, `DescribeStackDriftDetectionStatus`,
`DescribeStackResourceDrifts`) exposed as first-class verbs — Rain only implied drift support through
its stack-monitoring workflow rather than dedicated commands. This is one of the areas where
`aws/cloudformation` should exceed Rain's UX, since drift detection is core to the "is my
infrastructure still what my config says it should be" question Atmos otherwise answers structurally.

### Cross-Component Outputs

CloudFormation stack **Outputs** are this type's primary consumable — the values (VPC IDs, ARNs,
endpoints) that other components exist to consume — and they get first-class treatment on par with
`!terraform.output`:

- **`atmos aws cloudformation output <component> -s <stack> [key]`** (**Phase 1** — promoted from
  Phase 2 on direct user evidence, see [User Research](#user-research)): renders the deployed
  stack's Outputs via `DescribeStacks` — all of them, or one by key — the same view `apply`/`deploy`
  render at completion. The verb is **`output`**, singular, deliberately matching
  `atmos terraform output` so muscle memory transfers across types; `outputs` is a Cobra alias
  (CloudFormation's own section name is plural). It supports the **full standard format set** the
  Terraform verb already ships: `json`, `yaml`, `hcl`, `env`, `dotenv`, `bash`, `csv`, `tsv`, and
  **`github`** (GitHub Actions `$GITHUB_OUTPUT` syntax via `pkg/github/actions`), plus the
  flatten/uppercase key options — by **lifting the generic formatter out of
  `pkg/terraform/output/format.go` into a shared package** rather than duplicating it: nothing in
  that formatter is Terraform-specific (it formats a `map[string]any`), and a second consumer is
  exactly the signal to extract it. The themed table remains the default on a TTY.
- **`!aws.cloudformation.output` YAML function** (Phase 2): the cross-component consumption story,
  sibling to `!terraform.output`/`!terraform.state`. Registered as a new tag in
  `pkg/utils/yaml_utils.go` with an implementation alongside the existing identity-only `!aws.*`
  functions (`internal/exec/yaml_func_aws.go` — `!aws.account_id`, `!aws.region`, etc., none of which
  read resource state today). This is also the **Terraform↔CloudFormation interop bridge**: a
  Terraform component's `vars` can consume a CFN stack's Outputs and vice versa
  (`!terraform.output` already works inside any component type's config).
- **`atmos.Component(...).outputs`**: the template function populates `outputs` only for Terraform
  today (`internal/exec/template_funcs_component.go:112-113` gates on
  `componentType == TerraformComponentType`). Phase 2 adds an `aws/cloudformation` branch backed by
  `DescribeStacks` → `Outputs`, so Go-template consumers get the same shape they get from Terraform.
- **Day-one fallback (Phase 1)**: a generic `after: apply` store hook can push Outputs to any
  configured store for `!store` consumption — no CFN-specific machinery, but it requires the type to
  be in `supportsComponentHooks` (see [Section-Whitelist Plumbing](#section-whitelist-plumbing)).

Outputs marked on the template as sensitive-by-convention (fed from `NoEcho` parameters) flow through
the standard secret-masking pipeline (see [Secrets & NoEcho](#secrets--noecho)).

### Deployed-Stack Retrieval (`get`)

`get template` answers "what template is actually deployed right now?" via the `GetTemplate` API
(`--original`/`--processed` to choose the pre- or post-transform body), and `get policy` fetches the
live stack policy via `GetStackPolicy`. This is the inverse of `render` (client-side, local-only) and
the Atmos equivalent of Rain's `cat` — the verb shape follows `helm get manifest` rather than Unix
`cat`, leaving room for future `get` siblings. Primary uses: drift investigation (compare
`get template` against `render`) and inspecting stacks before adopting them into Atmos management.

### Template Formatting (`fmt`)

`fmt` canonically formats a component's CloudFormation template — stable key ordering per the
CloudFormation section convention, comment preservation, and `--json`/`--yaml` conversion — with
`--check` for CI, following the `atmos terraform fmt` precedent (`cmd/terraform/fmt.go`). Unlike
`terraform fmt` this cannot shell out (Rain's `cfn-format` is archived along with Rain), so the
formatter is implemented natively on a comment-preserving YAML round-trip. This fills a real
migration hole: `rain fmt` is one of Rain's most-used commands and AWS's post-Rain guidance offers no
replacement.

### Account Stack Inventory (`list`)

`atmos aws cloudformation list` lists CloudFormation stacks in the active identity's account/region
via `ListStacks` — including stacks Atmos doesn't manage — annotating each with whether it maps to a
configured `aws/cloudformation` component (by `stack_name`) and supporting `--status` filtering. This
complements `atmos list components` (config-side inventory) with the account-side view, the Atmos
equivalent of Rain's `ls`, and is the discovery on-ramp for the deferred stack-adoption work (see
Non-Goals): find unmanaged stacks first, `get template` them second, adopt them later.

### Secrets & NoEcho

SDK-native execution means **there is no subprocess to receive `env:`** — the shell-out pattern of
exporting secrets as environment variables does not apply. Instead:

- Secrets flow into **`parameters:` values** via `!secret` (see the `DbPassword` line in
  [Public Interface](#public-interface)), resolved at stack-processing time and passed directly in the
  `CreateStack`/`CreateChangeSet` API call's parameter list.
- CloudFormation's own `NoEcho` only affects the AWS Console's parameter display — it does **not**
  redact the value from `DescribeStacks`/`DescribeChangeSet` API responses, and a NoEcho parameter's
  value can still resurface unmasked in a stack's `Outputs`/resource `Metadata` if the template wires
  it there. Atmos does its own masking on top: parameters declared `NoEcho` in the template are
  registered with the masker **by value**, so every rendered surface — `plan`/`diff` changeset
  rendering, `describe stacks`/`describe component` output, `output`/`atmos.Component()` results,
  logs — masks that value wherever it reappears, not just the original parameter field, through the
  existing Gitleaks-backed masking pipeline (the same guarantee the secrets subsystem provides
  elsewhere for resolved `!secret` values).
- The `env:` section remains supported for its normal cross-type uses (hooks, `!exec`, template
  functions), but is **not** a secret-delivery channel for the CloudFormation API itself.

### Dependencies & DAG Ordering

`aws/cloudformation` components participate fully in the Atmos dependencies construct:
`settings.depends_on` in stack config, DAG-ordered `--all`/`--affected` deploys,
`atmos describe dependents` / `--include-dependents`, dependency graphs, and the Atmos Pro
dependency upload. Most of this is genuinely free: the dependents index iterates every
`components.<type>` section generically (`internal/exec/describe_dependents_index.go:43`), and
`dependencies` is already on `FilterComputedFields`' keep-list — so once the
[section-whitelist plumbing](#section-whitelist-plumbing) lands, CFN components appear in the DAG
like any other type, on both sides: a Terraform component can `depends_on` a CFN stack and vice
versa.

Two type-specific sites are **not** free and join the wiring checklist:

1. `findComponentSectionInCachedStacks` (`internal/exec/describe_dependents.go:664-677`) scans only
    the `terraform` and `helmfile` sections when resolving a dependency's component config — a CFN
    dependency resolves to nothing. Add the `aws/cloudformation` section, or (preferred, since
    `helm`/`kubernetes`/`ansible` are missing here too) generalize the scan over registered types.
2. The `describe affected` deleted-component lists (`describe_affected_deleted.go`) already
    captured in [`describe affected` Wiring](#describe-affected-wiring) — dependents of a deleted
    CFN component are only detected once those lists include the type.

(The `if e.StackComponentType == "terraform"` branch at `describe_dependents.go:416` is
terraform-specific enrichment, not a gap — CFN needs no equivalent.)

### Stack Sets

Multi-account/multi-region deployment orchestration (`CreateStackSet`, `CreateStackInstances`, etc.).
Scoped to Phase 3 (see [Rollout Phases](#rollout-phases)) — the cross-account permission model and
target-account/region matrix add real design surface that shouldn't block the Phase 1 core lifecycle.

### Nested-Stack Dependency Tree

`tree` renders the parent/nested-stack and resource dependency graph for a component, the Atmos
equivalent of Rain's `tree`. Scoped to Phase 3 alongside `logs`/`watch` — useful observability, not
required for a working `plan`/`apply`/`delete` loop.

### Rollback & Stack Policy

`disable_rollback` and `stack_policy` are first-class config fields (see
[Public Interface](#public-interface)). Every deploy goes through `CreateChangeSet` +
`ExecuteChangeSet` (never a direct `CreateStack`/`UpdateStack`, see [Changesets](#changesets)), so
`disable_rollback: true` maps to whichever of `CreateChangeSet`'s `OnStackFailure` (stack creation)
or `ExecuteChangeSet`'s `DisableRollback` (stack update) applies to the changeset's detected type —
the two are mutually exclusive on a single changeset, so only one is ever set. `stack_policy` has no
`CreateChangeSet`/`ExecuteChangeSet` parameter at all; it's applied via a follow-up `SetStackPolicy`
call after a successful apply, the same "no changeset parameter, so it's a follow-up call" shape
[termination_protection](#delete-semantics) uses.

### Validate Semantics

`validate` calls the server-side `ValidateTemplate` API (syntax + capability discovery) — it is an
API-backed check, not a local linter. Local linting with `cfn-lint`/`cfn-guard` (AWS's recommended
post-Rain path) is **not** built into this component type; users who want it declare those tools via
the toolchain subsystem and run them from hooks or workflow steps. A deeper `cfn-guard` policy
integration, if ever wanted, would go through the existing `atmos validate` OPA/schema framework as
its own design, not through this verb.

### Delete Semantics

`delete` maps to `DeleteStack` with:

- `--retain-resources <logical-ids>` passed through to the API (only valid for `DELETE_FAILED`
  stacks, per AWS semantics — surfaced with a hint when misused).
- **Termination protection is respected, never auto-disabled**: deleting a stack with
  `termination_protection: true` fails with an actionable hint telling the user to flip the config
  field and re-apply first (or use an explicit `--disable-termination-protection` escape hatch that
  calls `UpdateTerminationProtection` before deleting). Silent auto-disable would defeat the point of
  the setting.
- **Applying `termination_protection`**: like `stack_policy`, neither `CreateChangeSet` nor
  `ExecuteChangeSet` has a termination-protection parameter, so `termination_protection` is
  reconciled via a follow-up `UpdateTerminationProtection` call after every successful apply —
  unconditionally, not just when `true`, so unsetting it in config actually disables it on the next
  apply rather than only stopping enforcement at `delete`.

### Parameter Typing

`parameters:` is a `map[string]any` in YAML, normalized at the API boundary: scalars are stringified,
and list values are joined comma-delimited to match CloudFormation's `List<Type>`/`CommaDelimitedList`
wire format (the API accepts only strings). `UsePreviousValue` is not expressible in config — Atmos
config is the source of truth for every parameter on every deploy, the same declarative stance the
Terraform component takes toward variables.

### Region & Account Resolution

Region precedence, most-specific wins: `settings.aws_cloudformation.region` → the active identity's
region → the AWS SDK default chain (`AWS_REGION`, shared config). The account is always the active
identity's account — there is no per-component account override outside stack sets (Phase 3), which
own the multi-account matrix explicitly. This precedence is documented at the field's reference docs.

### Source Provisioning & Vendoring

Pointing a component at a remote template is first-class, via the existing source-provisioner
(`docs/prd/source-provisioner.md`, `pkg/provisioner/source/`) — JIT vendoring from the component's
top-level `source:` section, gated on `supportsSourceProvision` (see
[Section-Whitelist Plumbing](#section-whitelist-plumbing)). Two shapes:

```yaml
components:
  "aws/cloudformation":
    # Point at a repo (subdirectory = the component dir: template + stack policy + assets)
    vpc:
      source:
        uri: github.com/acme/cfn-templates.git//vpc?ref={{ .Version }}
        version: 1.2.0
      template: template.yaml            # relative to the vendored component dir

    # Point at a single CloudFormation file — no repo checkout structure needed
    dns:
      source:
        uri: https://raw.githubusercontent.com/acme/cfn-templates/v1.2.0/dns.yaml
```

The repo/subdirectory case is the source-provisioner's existing behavior (any go-getter URI: Git,
HTTP archive, S3, OCI). The **single-file case is a stated requirement**: when the URI resolves to
one file, it is fetched as the component's `template:` directly — a CloudFormation component is
often exactly one file, and demanding a directory structure around it would be ceremony. Whether the
provisioner already handles single-file URIs or needs a small extension is an implementation detail
to verify; the behavior is the contract.

The per-type **`source` verb group** follows the `atmos terraform source` / `atmos helmfile source`
convention: `atmos aws cloudformation source pull|list|describe|delete` (+ `cache`), Phase 2 with
the other inspection verbs (the `source:` section itself provisioning during `plan`/`apply` is
Phase 1, since `supportsSourceProvision` is Phase 1 plumbing).

Manifest-driven vendoring is currently **hard-blocked for non-`terraform`/`helmfile`/`packer`
types** by `validateComponentType` (`pkg/vendoring/resolve.go:47-54`) — Phase 1 must add
`aws/cloudformation` there (and to `resolve.go:362`'s list) or explicitly document vendoring as
unsupported for the type; silently inheriting the rejection is not acceptable.

### Experimental Gating

The type ships **experimental** (Phase 1–3) and graduates in Phase 4. The registry-level
`IsExperimental()` gate is top-level-only (`cmd/internal/registry.go` applies it during
`RegisterAll`, and the `aws` group declares `IsExperimental() → false`), but a nested mechanism
already exists: the enforcement in `cmd/root.go:1049` walks the resolved command's annotations, and
`atmos terraform backend` marks itself experimental as a nested subcommand by setting
`Annotations["experimental"] = "true"` directly (`cmd/terraform/backend/backend.go:27`). The
`cloudformation` command group follows that exact precedent — no registry extension needed, and the
stable `ecr`/`eks` siblings are unaffected. Graduation is deleting one annotation line.

### Auth

Full `atmos auth` integration, at three layers, all through existing seams:

- **SDK client construction**: the primary seam is `pkg/aws/identity`'s `LoadConfigWithAuth` — the
  canonical in-process path that builds an `aws.Config` from the active Atmos Auth identity/auth
  context, already used by the `cmd/aws/*` family (`pkg/aws/organization/organization.go:32` shows
  the pattern). This handles identity chaining, emulator endpoint overrides (`aws/emulator`
  identities — how the Floci E2E path authenticates), and FIPS endpoints in one place, in-process,
  never via `os.Setenv`. This supersedes the helm-style `IdentityEnvironmentProvider`
  env-composition seam for the CloudFormation/S3 clients themselves; that interface
  (`pkg/provisioner/target/target.go`, satisfied by `auth.AuthManager`) still serves the delivery
  layer (e.g. the `kind: git` target). PR #2536's
  `auth.TerraformPreHook`/`SetupComponentAuthForCLI` pattern was built for subprocess-oriented
  shell-out components and does not apply here.
- **Component-level `auth:`**: the shared `auth` section is extracted and merged generically by the
  stack processor (`stack_processor_process_stacks_helpers_extraction.go:146`,
  `stack_processor_merge.go:533,651`) — no per-type whitelisting needed; a CFN component selects
  its identity exactly like a Terraform component does.
- **Per-target identity overrides**: `ProvisionTarget.Auth` (`pkg/schema/schema.go:571-572`)
  already exists; the `aws/cloudformation` and `aws/s3` target kinds honor it. This is what makes
  the cross-account packaging pattern work declaratively: the deploy target assumes the workload
  account's identity while the artifact-bucket target uses a shared-services account's identity,
  each declared on its own target.

Distinct from all of the above: the component's `role_arn` field is the **CloudFormation service
role** (passed to the API in `CreateStack`/`UpdateStack`; CloudFormation itself assumes it to
manipulate resources) — it is not caller credentials and does not interact with `atmos auth` beyond
the caller needing `iam:PassRole` on it. The docs must keep these two roles clearly separated.

## Registry & Whitelist Wiring

Adding a native component type currently means touching several places by hand — there is no single
registration point yet (`docs/prd/component-registry-pattern.md`'s own stated problem). This PRD
inherits that reality rather than trying to fix it as a side effect; the checklists below are what
`aws/cloudformation` must touch, verified against the current codebase (not the historical PR #2536
wiring, which predates several of these files). Four families: core registry/describe/list wiring,
section-whitelist plumbing, `describe affected` wiring, and the atmos.yaml base-path chain.

### Core Registry & Describe/List Wiring

1. **`pkg/config/const.go`** — `CloudFormationComponentType = "aws/cloudformation"` +
    `CloudFormationSectionName` constants (plus per-section constants for `template`, `parameters`,
    `capabilities`, `stack_policy`, following `ChartSectionName`/`ValuesSectionName`).
2. **`pkg/component/aws/cloudformation/`** — the `ComponentProvider` implementation itself, plus its
    blank-import registration in **`cmd/root.go:38-45`** alongside the existing
    `_ "github.com/cloudposse/atmos/pkg/component/helm"`-style imports.
3. **`internal/exec/describe_stacks_component_processor.go`** — add an entry to the `typeEntries`
    slice at `:317-334` (`applyMetadataInheritance: true`, matching the `helm`/`kubernetes` entries),
    **and** add the type to `hasStackExplicitComponents` (`:1238-1247`) — otherwise a stack containing
    only `aws/cloudformation` components is filtered out of `describe stacks` as "empty."
4. **`internal/exec/describe_stacks.go`** — `getComponentBasePath()` switch (`:460-484`). Note:
    `helm`/`kubernetes` are *not* currently wired here (a pre-existing gap — their `component_path`
    renders empty in `describe stacks` output). CloudFormation likely *should* get a real `case`, since
    a template path is as meaningful a "base path" for CFN as a root module path is for Terraform.
    Fixing this for CFN without also fixing it for Helm/Kubernetes is an acceptable, explicitly-scoped
    choice — it does not depend on the older gap being closed.
5. **`internal/exec/describe_component.go`** — two sites: `detectComponentType()`'s auto-detect
    ordered slice (`:507-516`; without it, `atmos describe component <name>` auto-detection silently
    fails for CFN components), **and** `FilterComputedFields`'s hard section whitelist (`:638-666`,
    applied under the default `describe.component.filter: schema` mode) — without the latter,
    `template`/`parameters`/`capabilities`/`stack_policy` are stripped from
    `atmos describe component` output even after surviving stack processing.
6. **`pkg/list/extract/components.go`** — three separate hardcoded per-type lists, in `Components()`
    (`:49-53`), `UniqueComponents()` (via `extractUniqueComponentType`, `:307-331`), and
    `ComponentsForStack()` (via `extractComponentType`, `:452-456`/`:63`). `helm`/`kubernetes`/
    `emulator` are *also* absent from all three today (`atmos list components` currently omits them
    entirely). This PRD's implementation should add `aws/cloudformation` to all three and, since the
    fix is adjacent and small, backfill the missing types in the same change — otherwise open a tracked
    follow-up issue per the Follow-up Tracking mandate rather than silently inheriting the gap.
7. **Secondary ad hoc lists** that don't even include every *current* native type yet — all still
    hardcoded to `[terraform, helmfile, packer]` only:
    - `cmd/list/sources.go:424`
    - `pkg/utils/component_reverse_path_utils.go:51`
    - `cmd/vendor/selector.go:188`
    - `internal/exec/vendor_component_utils.go:274`
    - `pkg/vendoring/resolve.go:362`, plus its hard gate `validateComponentType`
      (`pkg/vendoring/resolve.go:47-54`) which *errors* on any other type — see
      [Source Provisioning & Vendoring](#source-provisioning--vendoring)

    These predate `ansible`/`container`/`kubernetes`/`helm` too. Decision for implementation: either
    fix these for `aws/cloudformation` (and, ideally, backfill the other missing native types in the
    same pass) or open one tracked follow-up issue covering all of them — do not add
    `aws/cloudformation`-only patches that leave the lists further out of sync with reality.
8. **`internal/exec/describe_dependents.go:664-677`** — `findComponentSectionInCachedStacks` scans
    only `terraform`/`helmfile` sections; without the CFN section (or a generalized scan), CFN
    components can't be resolved as dependencies. See
    [Dependencies & DAG Ordering](#dependencies--dag-ordering).

### Section-Whitelist Plumbing

Built-in types do **not** get container's custom-type pass-through: `stack_processor_merge.go:529`
rebuilds each component map from explicitly named keys, so every first-class section
(`template`/`parameters`/`capabilities`/`tags`/`stack_policy`/`role_arn`/...) is **silently dropped**
unless plumbed through all of the following (helm's `chart`/`values` wiring is the reference for each
site):

1. `internal/exec/stack_processor_process_stacks.go` — the `builtInTypes` map (`:1250-1256`), a
    per-type processing block, and the `allComponents[...]` assignment (`:1185-1242`).
2. `internal/exec/stack_processor_process_stacks_helpers_extraction.go` — a type-specific
    section-key bag mirroring `helmComponentSectionKeys` (`:324-334`) and its extraction block
    (`:275-310`).
3. Same file — the capability predicates: **`supportsComponentHooks` (`:349`) and
    `supportsSourceProvision` (`:370`) must include the type or `hooks:`/`source:`/`provision:` are
    silently dropped**; evaluate `supportsGenerate`/`supportsPlugins` (`:348-377`) explicitly and
    record the choice.
4. `internal/exec/stack_processor_process_stacks_helpers.go` — `ComponentProcessorResult`
    `Component<X>`/`BaseComponent<X>` fields (`:72-104`).
5. `internal/exec/stack_processor_process_stacks_helpers_inheritance.go` — inheritance propagation
    (`:236-240`).
6. `internal/exec/stack_processor_utils.go` — base-component extraction in
    `processBaseComponentConfigInternal` (`:2325-2336`, `:2572`, `:2819-2823`).
7. `internal/exec/stack_processor_cache.go` — deep-copy of the new sections for the stack-processor
    cache (`:132-160`).
8. `internal/exec/stack_processor_merge.go` — merge + write-back into the component map
    (`:654-688`, the site that makes `:529` stop dropping the sections).

`describe stacks` output itself is pass-through (`addSectionsToComponentEntry` copies every key) —
the bottleneck is entirely these upstream sites plus `FilterComputedFields` (item 5 above).

### `describe affected` Wiring

`describe affected` — and therefore every deploy verb's `--affected` flag — has heavy per-type
wiring, three sites of which **hard-error on unknown component types**:

1. `internal/exec/describe_affected_utils_parallel.go` — per-type dispatch over the
    `components.<type>` sections (terraform/helmfile/packer/kubernetes/helm each have a branch).
2. `internal/exec/describe_affected_components.go` — a `process<Type>ComponentsIndexed` function per
    type (kubernetes `:527-616` / helm `:655-732` are the SDK-native references).
3. `internal/exec/describe_affected_changed_files_index.go:177-191` — `getRelevantFiles` base-path
    switch (an unknown type silently falls back to scanning *all* files).
4. `internal/exec/describe_affected_pattern_cache.go:56-68` — base-path switch that returns
    `ErrUnsupportedComponentType` for unknown types.
5. `internal/exec/describe_affected_utils_2.go:337-348` — `isComponentFolderChanged` switch, also
    erroring (and currently missing even `helm`).
6. `internal/exec/describe_affected_deleted.go:101,167,281` — deleted-component detection, hardcoded
    `[terraform, helmfile, packer]`.
7. `internal/exec/describe_affected_utils_optimized.go:65` — the terraform-only optimized path must
    at minimum not mis-handle stacks containing CFN components.

### atmos.yaml Base-Path Chain

Adding the `AwsCloudFormation` field to `schema.Components` (see [Schema](#schema)) is necessary but
not sufficient — `components.aws/cloudformation.base_path` must be plumbed through:

1. `Components.GetComponentConfig`'s per-type switch (`pkg/schema/schema.go:1262-1282`).
2. A `<Type>DirAbsolutePath` field on `AtmosConfiguration` (`pkg/schema/schema.go:133-134`) and its
    computation (`pkg/config/config.go:527,535`).
3. The central path resolver `pkg/utils/component_path_utils.go:204-239` — **errors on unknown
    types**, and derives `ATMOS_COMPONENTS_<TYPE>_BASE_PATH` env-var names from the type string (the
    `/`-mangling decision from [Naming & Registry Precedent](#naming--registry-precedent) lands here).
4. `pkg/provisioner/source/source.go:375-377` — source-provisioner base-path switch.
5. `pkg/hooks/command_engine.go:782-784` — hooks-engine base-path switch.
6. `internal/exec/stack_processor_process_stacks.go:1136,1194` — stack-processor base-path lookup.

## Schema

`pkg/schema/schema.go`, following the `Helm`/`Kubernetes` struct pattern:

```go
type AwsCloudFormation struct {
    BasePath          string
    Template          string
    StackName         string
    Parameters        map[string]any    // normalized at the API boundary — see Parameter Typing
    Capabilities      []string
    Tags              map[string]string
    StackPolicy       *StackPolicy
    RoleArn           string
    NotificationArns  []string
    DisableRollback   bool
    TerminationProtection bool
    TimeoutInMinutes  int
}
```

registered as a field on `type Components struct` alongside `Terraform`, `Helmfile`, `Packer`,
`Ansible`, `Kubernetes`, `Helm` (note: `Container`/`Emulator` have no struct field — they ride the
`Plugins` remain-map — but a built-in type needs the typed field for the
[base-path chain](#atmosyaml-base-path-chain)).

JSON Schema — **four files**, not one:

1. `pkg/datafetcher/schema/stacks/stack-config/1.0.json` gains an
    `aws_cloudformation_component_manifest` definition, modeled directly on `helm_component_manifest`
    — type-specific properties (`template`, `stack_name`, `parameters`, `capabilities`, `tags`,
    `stack_policy`, `role_arn`, `notification_arns`, `disable_rollback`, `termination_protection`,
    `timeout_in_minutes`) plus the shared cross-type sections
    (`vars`/`env`/`settings`/`hooks`/`generate`/`source`/`provision`/`auth`/`dependencies`/`metadata`),
    with `additionalProperties: false`.
2. `pkg/datafetcher/schema/atmos/manifest/1.0.json` — the parallel manifest schema carries its own
    copies of every per-type definition; a parity test
    (`pkg/datafetcher/schema_condition_validation_test.go:190`) fails when a definition is added to
    `stack-config/1.0.json` but omitted here.
3. `pkg/datafetcher/schema/atmos/config/1.0.json` — the atmos.yaml schema is **generated from the Go
    structs** (`pkg/config/schema/`); adding the `AwsCloudFormation` field to `Components` requires
    regenerating this committed artifact via `go generate`, never hand-editing it.
4. `tests/fixtures/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json` — the hand-maintained test
    fixture copy of the manifest schema.

(`pkg/datafetcher/schema/vendor/package/1.0.json` does *not* enumerate component types — vendoring's
type restriction lives in Go, `pkg/vendoring/resolve.go:47-54`.)

## Error Handling

New sentinel errors in `errors/errors.go`, following PR #2536's naming template
(`ErrInvalidComponents<Type>`, `ErrInvalidSpecific<Type>Component`), each wired through the error
builder with a self-contained hint per `docs/errors.md`:

- `ErrMissingAwsCloudFormationTemplate`
- `ErrMissingAwsCloudFormationStackName`
- `ErrInvalidAwsCloudFormationCapabilities`
- `ErrInvalidAwsCloudFormationSettings`
- `ErrInvalidComponentsAwsCloudFormation`
- `ErrInvalidSpecificAwsCloudFormationComponent`
- `ErrAwsCloudFormationChangeSetFailed`
- `ErrAwsCloudFormationDriftDetected` (used by an optional `--fail-on-drift` style check, not a hard
  failure by default)

## Auth, Hooks, Secrets, Stores & Workflow Integration

- **Workflows/hooks**: no bespoke step type. `type: atmos` with `command: aws cloudformation deploy
  <component>` covers workflow and hook invocation identically to every other component type.
- **Lifecycle hook events**: following the kubernetes precedent (`pkg/hooks/event.go` —
  `before.kubernetes.render` ... `after.kubernetes.delete`), this type registers paired
  `before.`/`after.` events for `plan`, `diff`, `apply`, `deploy`, and `delete`, using the literal
  type string in the event name (`before.aws/cloudformation.apply`, `after.aws/cloudformation.deploy`,
  ...). The `/` inside the dotted event name is one more item for the
  [`/`-tolerance spike](#naming--registry-precedent): any event-name parsing that splits on
  delimiters must tolerate it, or event names fall back with the flat type name.
- **Hooks**: hook `kind`s (`command`, `store`, `git`, security hooks, `step`/`steps`) are generic and
  component-type-agnostic — but the `hooks:` section only *reaches* the engine for types listed in
  `supportsComponentHooks` (`internal/exec/stack_processor_process_stacks_helpers_extraction.go:349`);
  it is **not** automatic for a built-in type. Adding `aws/cloudformation` there (and to
  `supportsSourceProvision`) is part of
  [Section-Whitelist Plumbing](#section-whitelist-plumbing); once wired, no further
  CloudFormation-specific hook machinery is needed.
- **Secrets**: full participation in declarative secrets — `secrets.vars` declarations, `!secret`
  resolution into `parameters:`, and NoEcho-aware masking, per
  [Secrets & NoEcho](#secrets--noecho). Nothing secret-related is CFN-specific except the NoEcho
  masking rule.
- **Stores**: both directions work through existing generic machinery. *Read*: `!store`/`!store.get`
  resolve anywhere in the component's config, including `parameters:` values. *Write*: a
  `kind: store` hook on `after.aws/cloudformation.apply` publishes stack Outputs to any configured
  store for cross-component (and cross-tool) consumption — the Phase 1 outputs-sharing path until
  `!aws.cloudformation.output` ships in Phase 2 (see
  [Cross-Component Outputs](#cross-component-outputs)).

## Docs Plan

- `website/docs/cli/commands/aws/cloudformation/*.mdx` — one page per verb group
  (`plan`/`diff`/`apply`/`deploy`/`delete`/`validate`/`render`/`output`/`backend`/`get`/`fmt`/
  `list`/`source`/`changeset`/`drift`/`stackset`/`tree`/`logs`/`watch`), matching the
  Helm/Kubernetes one-page-per-subcommand convention.
- `website/docs/cli/configuration/components/aws-cloudformation.mdx` — `atmos.yaml`-level config
  reference.
- `website/docs/stacks/components/aws-cloudformation.mdx` — stack-manifest component reference (per
  the "every component type needs pages in both sections" convention).
- Update `website/docs/components/components-overview.mdx` — move CloudFormation from the "beyond the
  native types" custom-component example list into the native-types table.
- Update `website/docs/components/custom.mdx` — remove or caveat the CloudFormation example, since it
  now has a first-class path.

## Migration Guide (Deferred, Scope Only)

This PRD does **not** author the migration guide — the user asked for the component design to be
approved first. This section exists so the follow-up work has an unambiguous target once approved:

- `.claude/skills/atmos-migration/references/from-rain.md` — new reference file, plus a new row in
  `atmos-migration/SKILL.md`'s routing table ("User is migrating off Rain / raw CloudFormation → use
  `references/from-rain.md`").
- `website/docs/migration/from-rain.mdx` — new page, sibling to `native-terraform.mdx`,
  `terraform-workspaces.mdx`, `terragrunt.mdx` (flat `website/docs/migration/` directory, registered in
  `website/sidebars.js`'s `"Migration Guides"` category).
- Structure: the same Crawl/Walk/Run arc as `native-terraform.mdx` (get to a working
  `atmos aws cloudformation plan`/`deploy` in ~20 minutes first; defer inheritance, catalogs, and
  provision targets to "Walk"/"Run"). Existing CFN templates and parameter files are `!include`d, not
  rewritten — migration is opt-in, matching the Terraform guide's philosophy.
- Explicitly framed as **"migrating off Rain / raw CloudFormation onto Atmos,"** not "how to write
  Rain syntax in Atmos" — there is no Rain compatibility layer to document.
- Must include a **`!Rain::` directive mapping table** (per the Non-Goals entry): for each directive
  (`Embed`, `Include`, `S3`, `Module`, `Constant`, `Env`), the Atmos-native replacement and a
  before/after template snippet — this is the single most likely blocker for a real Rain template
  and cannot be left to the reader. Also a Rain→Atmos verb cross-reference (`fmt`→`fmt`,
  `cat`→`get template`, `ls`→`list`, `bootstrap`→`backend create`, `log`→`logs`, `rm`→`delete`).

## Testing Strategy

- Unit tests with a mocked `cloudformationiface`-style AWS SDK client (interface + dependency
  injection, `go.uber.org/mock/mockgen`), no live-AWS integration tests, per the project's
  mocking-over-integration testing mandate. Mocks remain the primary tier.
- **E2E round-trips against the Floci AWS emulator** (`floci/aws` driver,
  `pkg/emulator/driver/floci.go`) as the secondary tier: Floci serves a LocalStack-style single
  edge endpoint, and existing fixtures already route the `cloudformation` service endpoint to it
  (`tests/fixtures/scenarios/native-ci-e2e/stacks/deploy/test.yaml:31`). Gated behind
  `ATMOS_TEST_FLOCI=true` + a container-runtime check; auto-started via the testcontainers pattern
  locally, service containers in CI — the same harness convention the native-CI E2E tests use. The
  SDK client's emulator endpoint must resolve through the active identity/emulator endpoint
  plumbing (the in-process path, never `os.Setenv`), same as the native `aws/emulator` identity
  path. E2E templates stick to resources Floci emulates faithfully (S3 buckets, SSM parameters,
  IAM); orchestration fidelity for exotic resource types is Floci's problem, not this test suite's.
- **`examples/cloudformation`**: a runnable example (per the example-creator conventions) whose
  stack manifest declares a `components.emulator` Floci service plus an `aws/cloudformation`
  component deploying against it — zero AWS credentials required. It doubles as the E2E fixture and
  the docs' copy-paste starting point, the same dual-use pattern the emulator examples already
  follow.
- Table-driven tests for changeset diff rendering, capability/parameter validation, and error-path
  coverage (missing template, invalid capabilities, failed changeset).
- Schema decode tests for the `aws/cloudformation` component kind, mirroring the `container` schema
  decode tests.
- Registry/whitelist wiring tests: `describe stacks`/`describe component`/`describe affected`/
  `list components` all surface a stack that only has an `aws/cloudformation` component, **and the
  described component retains every first-class section** (`template`/`parameters`/`capabilities`/
  `tags`/`stack_policy`) — guarding all four wiring families in
  [Registry & Whitelist Wiring](#registry--whitelist-wiring), since section-drop failures are silent.
- Masking tests: `NoEcho`-fed parameter values and resolved `!secret` values never appear unmasked in
  `plan`/`diff`/`describe` output (see [Secrets & NoEcho](#secrets--noecho)).
- Target: >85% coverage per the project-wide coverage mandate.

## Rollout Phases

**Phase 1 — Core Lifecycle**
`plan`/`diff`/`apply`/`deploy`/`delete`/`validate`/`render`/`output` with
[live progress streaming](#live-progress-streaming) (spinners + end-of-deploy Outputs summary) on
the mutating verbs, the `cfn` command alias, the
[`backend` artifact-bucket lifecycle group](#artifact-bucket-provisioning-backend) (packaging is
Phase 1, so its bucket provisioning must be too), registry provider, all four wiring families from
[Registry & Whitelist Wiring](#registry--whitelist-wiring) (core registry/describe/list,
section-whitelist plumbing, `describe affected`, base-path chain), the nested-subcommand
[experimental gate](#experimental-gating), schema (all four JSON schema files), docs for the core
verbs.
*Success criteria*: a real CloudFormation stack can be created, updated, and deleted end-to-end through
`atmos aws cloudformation` with per-resource events streaming during the operation and the Outputs
summary rendered at completion (also retrievable on demand via `output`, in every standard format
including `github`), with
`describe stacks`/`describe component`/`describe affected`/`list components` all correctly surfacing
`aws/cloudformation` components and every first-class section surviving stack processing.

**Phase 2 — Changesets, Drift, Outputs & Inspection**
Explicit `changeset create/execute/list/delete` verbs, `drift detect/describe`, the remaining
[Cross-Component Outputs](#cross-component-outputs) surface (the `!aws.cloudformation.output` YAML
function and the `atmos.Component(...).outputs` branch — the `output` verb itself moved to
Phase 1), and the inspection verbs: [`get template`/`get policy`](#deployed-stack-retrieval-get),
[`fmt`](#template-formatting-fmt), [`list`](#account-stack-inventory-list), and the
[`source` management verbs](#source-provisioning--vendoring) (JIT `source:` provisioning itself is
Phase 1).
*Success criteria*: a changeset can be created and reviewed before execution; drift against a live
stack is detectable and reportable; a Terraform component's `vars` can consume a CFN stack's Outputs
via `!aws.cloudformation.output`; the deployed template of a live stack can be fetched and compared
against the local render.

**Phase 3 — Stack Sets & Observability**
`stackset create/update/delete/instances`, `tree`, `logs` (with `--chart`), `watch`.
*Success criteria*: a stack set can be deployed across multiple accounts/regions; nested-stack
dependency graphs and combined event logs render correctly for a multi-stack deployment.

**Phase 4 — Migration & Graduation**
Author the `from-rain` migration guide (scope defined above), polish docs, graduate the type out of
"experimental" status (matching the bar Helm/Kubernetes are held to before their own graduation).
*Success criteria*: a real Rain user can follow the guide to a working Atmos-managed stack in the
guide's target time-to-first-deploy.

## Risks & Mitigation

| Risk | Mitigation |
|---|---|
| `/`-bearing type string (`aws/cloudformation`) breaks an assumption somewhere in the registry, JSON Schema tooling, or a whitelist site | Spike early (see [Naming & Registry Precedent](#naming--registry-precedent)); fallback to a flat `cloudformation` type with `GetGroup() == "aws"` and CLI nesting preserving the namespace experience |
| Template size exceeds CloudFormation's inline-body limit | Always package through the `provision` target abstraction (S3 upload) rather than inlining, matching `aws cloudformation package`/Rain's `pkg` behavior |
| Drift-detection API is async and rate-limited | Poll `DescribeStackDriftDetectionStatus` with backoff; document expected latency in the CLI help text, don't block on it synchronously by default |
| Stack set cross-account/region permission complexity | Scoped to Phase 3, after the core lifecycle has proven the auth/identity integration pattern on a single-account stack |
| Rollback/stack-policy misconfiguration causing a stuck stack | Surface `ROLLBACK_FAILED`/`UPDATE_ROLLBACK_FAILED` states with an explicit, actionable error hint rather than a generic API error passthrough |

## Success Criteria

- `aws/cloudformation` is a registered `ComponentProvider`, discoverable through the same registry as
  every other native type.
- A stack manifest can declare `components."aws/cloudformation".<name>` with inheritance, catalogs,
  `depends_on`, hooks, `env`/secrets, and `provision` targets all functioning identically to their
  Terraform/Helm/Kubernetes equivalents.
- `atmos describe stacks`, `atmos describe component`, `atmos describe affected`, and
  `atmos list components` all correctly surface `aws/cloudformation` components, with every
  first-class section (`template`/`parameters`/`capabilities`/`tags`/`stack_policy`) surviving stack
  processing and describe output.
- Other components can consume a CFN stack's Outputs (`!aws.cloudformation.output`,
  `atmos.Component(...).outputs`) by end of Phase 2.
- No shell-out to any external binary; all execution is AWS SDK for Go v2 calls.
- >85% test coverage on the new `pkg/component/aws/cloudformation/` package.

## FAQ

**Why not just wait for AWS to bless a Rain successor?** AWS's own guidance points to the AWS CLI,
CDK, `cfn-lint`, and `cfn-guard` — none of which are a drop-in replacement for a component-type
orchestration layer. Atmos going SDK-native sidesteps the question entirely.

**Why not use the AWS CLI (`aws cloudformation deploy`) via shell-out instead of the SDK?** The same
reasoning that put Helm and Kubernetes on Go SDKs applies here: no external binary/toolchain
dependency, structured error handling instead of parsing CLI output, and direct access to APIs (drift,
changesets, stack sets) that would otherwise require multiple `aws` CLI invocations stitched together.

**Does this replace AWS CDK?** No — CDK is a template *authoring* tool (TypeScript/Python/etc. that
synthesizes CloudFormation templates). `aws/cloudformation` is a *deployment* component type; a CDK-
synthesized template is a perfectly valid `template:` input.

**What about Pulumi/Bicep/other custom types `custom.mdx` mentions alongside CloudFormation?** Out of
scope for this PRD. `aws/cloudformation` graduating to native does not imply the others will — each
would need its own PRD evaluating whether an SDK-native or shell-out design fits.

## References

- PR #2536, `osterman/rain-component-type` (closed — prior shell-out design, superseded by this PRD)
- [aws-cloudformation/rain#801](https://github.com/aws-cloudformation/rain/pull/801) — Rain archival
  notice
- `docs/prd/component-registry-pattern.md` — registry architecture; prior CloudFormation mentions at
  the "Limited extensibility" problem statement, the plugin-components architecture diagram, and the
  Phase 6 plugin-discovery example list
- `docs/prd/container-components.md` — structural template for this PRD
- `pkg/component/helm/` — SDK-native component-type reference implementation
- `docs/prd/backend-provisioner.md` + `docs/prd/s3-backend-provisioner.md` — the backend-provisioner
  system (`pkg/provisioner/backend/`) the artifact-bucket `backend` verbs and
  `provision.backend.enabled` auto-provisioning reuse
- `docs/prd/native-ci-artifact-storage.md` — the existing `pkg/ci/artifact` `aws/s3` store used as
  the Phase 1 packaging transport
- Atmos Artifacts PRD (`docs/prd/artifacts.md`, in progress on a parallel branch, not yet on `main`)
  — the `pkg/artifact` repository framework and `artifact.repositories` config that packaging
  converges into, and the substrate for the future `kind: artifact` provision target and
  `cloudformation/template` artifact kind
- `website/docs/migration/native-terraform.mdx` — migration-guide structural template (for the
  deferred `from-rain.mdx`)

## Changelog

| Date | Change |
|---|---|
| 2026-08-24 | Initial PRD |
| 2026-08-24 | Completeness-review amendments: helm v3→v4 and container-precedent corrections; `emulator` added to type enumerations; wiring expanded to four families (section-whitelist plumbing, `describe affected`, base-path chain, extra ad hoc lists); new Cross-Component Outputs, Secrets & NoEcho, Validate/Delete semantics, Parameter Typing, Region Resolution, Vendoring, and Experimental Gating sections; JSON schema count corrected to four files; stack import/adoption added to Non-Goals |
| 2026-08-24 | Rain gap-analysis amendments: `backend` artifact-bucket lifecycle group (mirrors `atmos terraform backend`); `get template`/`get policy`, `fmt`, and account-wide `list` verbs; live progress streaming required for Phase 1 mutating verbs; `console`, `forecast`, and `!Rain::` directive preprocessing added to Non-Goals; migration guide scope gains the directive mapping table and verb cross-reference; experimental gating corrected to the existing nested-annotation mechanism |
| 2026-08-24 | Artifacts/provisioner reconciliation: bucket provisioning reuses the registered S3 backend provisioner (`pkg/provisioner/backend/`) including opt-in `provision.backend.enabled` auto-provisioning alongside the explicit `backend` verbs; packaging transport is `pkg/ci/artifact` `aws/s3` now, converging with the Artifacts PRD's `pkg/artifact` repository framework; future `kind: artifact` provision target and `cloudformation/template` artifact kind recorded (not phased); explicit no-dependency contract on the Artifacts PRD (narrow upload seam, no vocabulary squatting, deferred integrations deferred) |
| 2026-08-24 | User-research + UX amendments: User Research section added (daily-Rain-user feedback); `outputs` verb promoted to Phase 1 with end-of-deploy Outputs summary; animated spinners via `pkg/ui/spinner` specified for streaming; `cfn` Cobra alias for the `cloudformation` subcommand; lifecycle hook events enumerated (`before./after.` × plan/diff/apply/deploy/delete, kubernetes precedent) with the `/`-in-event-name spike item; explicit secrets and stores integration bullets |
| 2026-08-24 | Output verb alignment: renamed `outputs` → `output` (Cobra alias `outputs`) to match `atmos terraform output`; single-key retrieval; full standard format set (`json`/`yaml`/`hcl`/`env`/`dotenv`/`bash`/`csv`/`tsv`/`github` incl. `$GITHUB_OUTPUT` syntax) by lifting the generic formatter out of `pkg/terraform/output` into a shared package |
| 2026-08-24 | Dependencies, source & Floci amendments: Dependencies & DAG Ordering section (dependents index is type-generic; `findComponentSectionInCachedStacks` added as wiring item 8); Source Provisioning & Vendoring expanded with JIT `source:` YAML examples incl. single-file templates and the `source pull/list/describe/delete` verb group; Floci AWS emulator E2E tier and `examples/cloudformation` added to Testing Strategy |
| 2026-08-24 | Packaging destination restructured as a provision target: new `kind: aws/s3` target (`bucket`/`prefix`) replaces `settings.aws_cloudformation.s3_bucket`/`s3_prefix`; implicit single-target resolution with `packaging:` disambiguator; `--target <s3-target>` = publish-only delivery; kind string matches artifacts-PRD/stores `aws/*` vocabulary; `ProvisionTarget` struct gains the S3 fields; direct-deploy target kind is `aws/cloudformation` (the type string, kubernetes precedent) with an implicit default target when no `provision:` is declared |
| 2026-08-24 | Auth section upgraded: primary SDK seam is `pkg/aws/identity.LoadConfigWithAuth` (the `cmd/aws/*` pattern, in-process, emulator/FIPS-aware) superseding the helm env seam for client construction; component `auth:` confirmed generically plumbed; per-target `ProvisionTarget.Auth` overrides enable cross-account deploy-vs-bucket identities; `role_arn` clarified as the CloudFormation service role, not caller credentials |
