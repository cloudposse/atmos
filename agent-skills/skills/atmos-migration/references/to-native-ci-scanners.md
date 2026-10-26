# Migrating Scanner Actions to Native CI

Use this reference when replacing scanner setup and execution actions in GitHub Actions.
Load [atmos-toolchain](../../atmos-toolchain/SKILL.md) to declare pinned tools,
[atmos-lint](../../atmos-lint/SKILL.md) for TFLint, and
[atmos-hooks](../../atmos-hooks/SKILL.md) for lifecycle hooks and failure handling.

## Map the Existing Operation

Inspect the action version, inputs, config files, working directory, environment/secrets, trigger
conditions, and downstream report consumers before replacing it. An action version is not
necessarily the scanner binary version: preserve the resolved tool version, plugin versions,
arguments, exclusions, scan targets, and severity or cost thresholds.

| Existing action or operation | Native replacement |
|---|---|
| `terraform-linters/setup-tflint` and TFLint execution | Pin `tflint` in `dependencies.tools`; use `atmos terraform lint` for standalone checks or `kind: tflint` for lifecycle checks. Load `atmos-lint`. |
| `bridgecrewio/checkov-action` | Pin Checkov and use `kind: checkov` for supported scans. Load `atmos-hooks`. |
| `aquasecurity/trivy-action` | Pin Trivy and use `kind: trivy` for IaC configuration scans. Load `atmos-hooks`; the default hook runs `trivy config`, not an image or filesystem vulnerability scan. |
| `Checkmarx/kics-github-action` | Pin KICS and use `kind: kics` for supported IaC scans. Load `atmos-hooks`. |
| `infracost/actions` setup and cost estimation | Pin Infracost and use `kind: infracost` for cost breakdowns. Preserve API credentials, usage inputs, baseline comparisons, and any separate budget gate; a breakdown alone does not replace them. |
| tfsec actions | Evaluate `kind: trivy` for equivalent IaC coverage. If rules, suppressions, or failure semantics differ, retain tfsec through a pinned command hook or workflow step until that difference is resolved. |

Use `kind: command` hooks or toolchain-backed workflow steps for unsupported operations, such as
scanner modes or custom policy scripts that the named hook cannot preserve. Keep the original
scope: do not replace a repository-wide, image, or plan-JSON scan with a component-directory scan.
Do not require a Terraform plan solely to run a previously standalone check.

## Declare Tools and Attach Checks

Put scanner pins in the owning component's `dependencies.tools`, or in the owning workflow/custom
command for standalone automation. Resolve tool identifiers and aliases using `atmos-toolchain`.
Atmos installs and injects declared tools automatically; remove redundant setup actions without
adding routine `atmos toolchain install` steps. Keep scanner-specific setup such as TFLint plugin
initialization when required.

For example, migrating a blocking, CI-only TFLint check into the plan lifecycle:

```yaml
# Stack manifest; use the project's existing component and scanner version.
components:
  terraform:
    vpc:
      dependencies:
        tools:
          tflint: "0.59.1" # Example pin; preserve the existing version.
      hooks:
        lint:
          events: [before.terraform.plan]
          kind: tflint
          when: ci
          on_failure: fail
```

For a standalone check, invoke `atmos terraform lint vpc -s prod` instead. Preserve existing
`.tflint.hcl`, plugin initialization, and rule settings following `atmos-lint`.
Choose hook events based on required inputs: source scans can run before plan, while a scan
requiring a generated plan must run after it exists. Preserve CI-only and per-environment scope
with `when:` rather than enabling a previously CI-only check for every local command.

## Preserve Enforcement and Reports

Scanner kinds default to `on_failure: warn`. For an existing blocking check, explicitly set
`on_failure: fail` **and** configure the scanner to exit nonzero at the original failure threshold.
Keep intentionally advisory checks advisory.

- Checkov's default hook arguments include `--soft-fail`; override that behavior for blocking
  policies while preserving the original selective soft/hard-fail thresholds and exclusions.
- Trivy's default hook arguments do not set a findings exit code; preserve the original exit-code
  and severity policy rather than assuming a rendered finding fails the job.
- Preserve KICS failure thresholds and TFLint exit behavior. Retain custom Infracost budget checks
  as command hooks or workflow steps if a cost breakdown cannot enforce the original policy.

Nonempty hook `args` **replace**, rather than append to, the kind's default arguments. When
translating action inputs, retain the required subcommand, target, and structured-report flags.
Checkov writes `results_sarif.sarif` under `$ATMOS_OUTPUT_DIR`; KICS writes `results.sarif` there;
Trivy writes SARIF to `$ATMOS_OUTPUT_FILE`; TFLint emits SARIF on stdout; Infracost writes JSON to
`$ATMOS_OUTPUT_FILE`. Preserve these contracts for native report parsing. Load
[atmos-ci](../../atmos-ci/SKILL.md) when mapping annotations, summaries, or code-scanning uploads;
native reporting does not imply that every legacy artifact upload or required check is preserved.
Follow the [Native CI permission mapping](../../atmos-ci/references/native-ci.md#minimal-permissions)
for API tokens, commit statuses versus Check Runs, SARIF uploads, and fork PR limitations.

Before removing the old action, verify a clean case and a known policy violation against the same
target. Confirm that the violation blocks the same operation, exclusions still apply, and reports
reach the expected consumers. Verify any unsupported mode through its retained command/workflow
path. If credentials or runner access prevent execution, report that validation gap explicitly.
