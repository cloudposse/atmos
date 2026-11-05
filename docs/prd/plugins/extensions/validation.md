# Plugin policies, linting, and validation

**Status:** Proposed; a follow-on adapter for [Independently Versioned Atmos Plugins](../overview.md). Commands, fields, and SDK bindings introduced here are proposed APIs.

## Purpose and existing foundation

An organization should be able to publish its infrastructure standards once, pin them in projects, and run the same checks locally and in CI. Adding a naming rule, a required-tag policy, a schema, or a new validation engine must not require rebuilding Atmos.

Atmos already accepts [custom OPA policies](../../../../website/docs/validation/opa.mdx) and [JSON Schemas](../../../../website/docs/validation/json-schema.mdx). CI validation already has a [validator registry](../../../../pkg/ci/validate/validate.go), scoped to CI workflow files. [Shared validation diagnostics](../../../../pkg/validation/diagnostic.go) carry source, rule ID, severity, message, and location and support SARIF and CI annotations. Terraform linting and scanner hooks provide further existing integration paths.

Extend these foundations with a shared validation capability. Policy, linting, and validation are categories of checks with different inputs and enforcement choices, not three independent plugin managers.

| Extension | What the author delivers | Example |
|---|---|---|
| Rule pack | Rules/configuration and fixtures consumed by an existing engine. | Rego policies, JSON Schemas, TFLint configuration. |
| Validator plugin | Go or Starlark code implementing a check, engine, or external-tool adapter. | Cross-component naming rules or a proprietary policy engine. |

Rule packs reuse the plugin manifest, catalogs, OCI CRUD, version constraints, lockfiles, signatures, local overrides, and test runner. They use a data runtime and do not require a plugin subprocess. Existing OPA/JSON Schema configuration remains valid. Initial built-in adapters support OPA, JSON Schema, and the existing TFLint integration; supporting another engine is a validator-plugin use case.

## Consumer experience

```shell
atmos plugin search --capability validation.validator
atmos plugin install acme/standards --as standards --version '~1.0'
atmos plugin info standards
atmos validate
atmos validate --check naming --format json
atmos validate component vpc -s production --format sarif
```

`validation` search includes rule packs and validators. `info` lists exported check IDs, their category, input types, configuration schema, and rule documentation. Installation makes the exports available; configuration determines which checks run.

For a project file linter, add to `atmos.yaml`:

```yaml
validate:
  checks:
    naming:
      validator: acme/naming
      files: ["stacks/**/*.yaml"]
      options:
        pattern: "^[a-z][a-z0-9-]+$"
```

`acme/naming` is an exported check ID from an installed package, while `naming` is this project's configured check instance. `atmos validate` runs configured project checks alongside the existing built-ins. `--check naming` selects that configured instance. An unknown instance is an error. Project checks receive matched source files without launching component tools or authenticating to clouds.

For a policy on a resolved component, extend the existing stack configuration:

```yaml
components:
  terraform:
    vpc:
      settings:
        validation:
          required-tags:
            validator: acme/required-tags
            options:
              required: [Owner, CostCenter]
            timeout: 10
      vars:
        tags:
          Owner: platform
```

The new `validator` reference selects either a rule-pack export or a custom validator. It is mutually exclusive with legacy `schema_type`/`schema_path` selection for the same validation entry. Imports and inheritance continue to control component policy configuration. The example produces a finding because `CostCenter` is missing:

```text
ERROR acme/required-tags: Missing required tag CostCenter
  component: vpc
  stack: production
  field: vars.tags
```

`atmos validate component vpc -s production` runs component checks after stack resolution, alongside existing configured validation. Add `--check required-tags` to select a single component check. Plugin component types use the component registry for resolution; the current Terraform/Helmfile/Packer fallback cannot remain the only discovery path. Ambiguous component names require explicit `--component-type` selection.

File checks and component checks declare their input types. A file-only rule is not silently run on merged component data, and a component policy is not silently reduced to a raw-file check. Explicitly selected checks with missing or incompatible inputs fail with an actionable diagnostic. Aggregate runs label genuinely non-applicable checks as skipped, not passed.

### Plan and other generated-artifact policies

Configuration validation cannot detect everything that appears in an infrastructure plan. Support checks over explicitly supplied artifacts:

```shell
atmos validate artifact plan.json --type terraform-plan-json/v1 --check plan-policy --format sarif
```

Configure `plan-policy` under `validate.checks` with `validator: acme/no-public-ingress`; its manifest declares input `terraform-plan-json/v1`. The explicit file supplies the input for this invocation. Root `atmos validate` does not generate or guess a plan to run such a check. The first validation adapter release supplies a Terraform plan JSON adapter; other engines can declare additional artifact formats through their component/validator plugins.

A required plan check bound to an apply/deploy operation must inspect JSON derived from the exact saved plan that will be applied. A changed plan requires reevaluation. An unavailable plan fails the gate with instructions to produce a saved plan; it cannot be treated as approval. Standalone artifact validation reports findings but is not a reusable authorization token for later applies.

### Enforcement and output

- `atmos validate` remains an explicit check. Installing a package does not attach hooks or change deployment behavior.
- Existing component preflight validation evaluates configured component checks. Additional command/lifecycle gates use explicitly configured hooks with `on_failure: fail` for required checks; existing hook defaults remain unchanged.
- Findings retain `error`, `warning`, and `notice` severities. Error findings fail a validation command; warnings/notices remain visible without failing it. A required hook propagates the command failure to the parent operation.
- Missing validators, crashes, timeouts, malformed results, and unavailable required inputs are execution errors, never successful empty reports. These errors fail validation even when no rule finding was produced.
- Existing `disabled` configuration continues to work for component checks. Disabled checks are reported as disabled, not passed. Required organizational enforcement belongs in the trusted CI configuration; a repository-local setting alone does not establish centrally controlled policy.
- Checks are read-only by contract. Automatic fixes and remediation are outside this initial capability. Native code retains the platform's trusted-code execution model; the contract is not an OS sandbox.
- All validation entrypoints gain consistent `text`, `rich`, `json`, and `sarif` output for plugin findings while preserving existing supported formats. CI output uses the current provider's annotations, checks, and summaries, subject to existing reporting and fork-trust gates.
- Diagnostics identify plugin version, configured check, stable namespaced rule ID, severity, message, component/stack or artifact context, and source location where available. Field paths map through Atmos provenance; derived values without a source location retain their logical path without inventing a line number.
- Check implementations return findings separately from execution errors. Atmos owns sorting, masking, rendering, exit status, and CI reporting. Validators never implement their own GitHub comment client merely to report findings.

## Developer experience

### Package existing rules

```shell
atmos plugin create acme-standards --capability validation.rulepack --runtime data
cd acme-standards
atmos plugin validate .
atmos plugin test .
atmos plugin pack . --output dist/acme-standards
atmos plugin publish dist/acme-standards --to oci://ghcr.io/acme/atmos-plugins/standards
```

The scaffold generates a manifest, sample rule, engine configuration, passing/failing input fixtures, contract scenarios, example consumer configuration, README, and publication workflow. Authors edit the rules and expected findings. They do not implement RPC or credential lifecycle handlers.

```yaml
apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: acme/standards
  version: 1.0.0
spec:
  runtime:
    type: data
  capabilities:
    validation:
      rules:
        acme/required-tags:
          category: policy
          input: component
          engine: opa
          path: policies/required-tags.rego
          options_schema: schemas/required-tags.json
```

Exports include the engine entrypoint/query and additional modules where needed. The OPA adapter preserves existing Atmos policy evaluation conventions; JSON Schema uses its existing evaluator. A TFLint rule pack supplies configuration to the current scanner adapter, preserving component-local precedence unless the user explicitly selects the pack for that invocation. Tool binaries and upstream linter plugins are installed and pinned through explicit dependency setup, not downloaded during validation.

Package-relative rules, imports, and schemas resolve inside the verified installed package. Manifest validation checks paths, schemas, engine availability, and declared input types. Rules do not need a native executable; engines may still invoke their explicitly configured external tools. A combined package may include rules and a Go/Starlark validator, with each export selecting its implementation.

### Write a custom validator

```shell
atmos plugin create acme-checks --capability validation.validator --runtime starlark
```

The scaffold supplies `validator.star`, schemas, fixtures, contract scenarios, and example configuration. A proposed Starlark callback for a resolved-component check is:

```python
def validate(request):
    tags = request.component.get("vars", {}).get("tags", {})
    if tags.get("Owner"):
        return []
    return [validation.finding(
        rule_id = "acme/owner-required",
        severity = "error",
        message = "Set vars.tags.Owner to the responsible team",
        field = "vars.tags.Owner",
    )]

plugin.register_validator("acme/owner-required", validate = validate)
```

The manifest exports that ID under `spec.capabilities.validation.validators`, declaring `category: validation` and `input: component`; the runtime is Starlark API 1. `request` carries immutable selected inputs, options, provenance, and invocation metadata. The Go equivalent implements `Validate(context.Context, validation.Request, api.Services) (validation.Report, error)` using the separately versioned SDK. A file linter receives selected files; an artifact validator receives the declared artifact format and metadata. Credentials are not injected by default.

An adapter for an external tool uses Atmos process execution and maps structured output, such as SARIF, into the shared report. It distinguishes the tool's findings exit code from a crash or invocation failure. A native validator requiring remote services must declare those inputs and provide mocks for its default contract tests.

## Tests and implementation boundaries

`atmos plugin test .` runs both data-rule and executable-validator scenarios through Atmos adapters. Each rule scaffold includes a passing input and a failing input with assertions for rule IDs, severity, messages, and locations/field paths. An empty result cannot satisfy a fixture expecting a violation. Mocked tests cover deterministic output, invalid options, missing input, cancellation, tool errors, and secret redaction; live integrations remain explicit opt-in.

Reuse the existing validation report and renderers as Atmos model. Expose a versioned wire/SDK representation rather than exporting internal Go structs as the compatibility contract. Add adapters from rule engines and plugin runtimes, and extend the existing project/component validation dispatch to use registered check descriptors. CI validator registration and scanner hooks participate through adapters; keep their current standalone commands working.

The platform suite verifies the same findings through terminal, JSON, SARIF, and CI output; unaffected-file filtering; source provenance; project/global isolation; package upgrades and rollback; legacy OPA/JSON Schema behavior; and required gates that fail on both findings and execution errors. Plan-policy tests verify that a decision about one saved plan cannot authorize a changed plan. Do not add a separate policy server or require a hosted catalog.

## Integration

| Aspect | Today |
|---|---|
| Registry | None for engines. [`validate_component.go`](../../../../internal/exec/validate_component.go) hardcodes `switch schemaType` over `"jsonschema"` and `"opa"`. The only registry is CI-only: [`validate.Register`](../../../../pkg/ci/validate/validate.go) with `Validator{Name, Validate(ctx, Request) (validation.Report, error)}`, whose single registrant is actionlint. |
| Duplicate policy | Not applicable for engines. The CI registry panics on a nil, unnamed, or duplicate validator. |
| Context on the interface | Not applicable for engines. Yes for the CI validator. |
| Reachable from outside the module | Yes for the CI registry and [`validation.Diagnostic`](../../../../pkg/validation/diagnostic.go). |
| Closed lists to open | The `switch schemaType` dispatch. `ValidateWithCue` in [`validate_utils.go`](../../../../internal/exec/validate_utils.go) has no caller, so the CUE engine is unreachable. |
| Boundary leaks | None for the CI validator. Engine adapters must not expose `validation.Report` internals; only `Diagnostic` crosses. |

Reuse [`validation.Diagnostic`](../../../../pkg/validation/diagnostic.go), `Report`, [SARIF output](../../../../pkg/validation/sarif.go), and `ToAnnotations` as Atmos model. `validate.checks`, `settings.validation.<name>.validator`, and `atmos validate artifact` do not exist today; all three are new.

**Adapter:** `pkg/plugin/adapter/validation` implements the new engine and check interfaces (N4) and generalizes the CI `validate.Validator`. It calls the `Validator` wire service and consumes the `api.Diagnostics`, `api.FS`, `api.Process`, and `api.Toolchain` API services.

**Injection points:**
- Component validation dispatch in `validate_component.go`: read check descriptors from the engine and check registries instead of the `switch`.
- Root `atmos validate`: run configured `validate.checks`.
- A new `atmos validate artifact` subcommand.
- CI validators: register through the same check registry; actionlint keeps working through an adapter.

**Prerequisites:** P0.8 Validation engine and check registries (N4); Phase 3 work package WP3.5 (validation). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:**
- `validation.Report` internals: only `Diagnostic` records (source, rule ID, severity, message, location) cross.
- `*schema.AtmosConfiguration`: replaced by the selected inputs, options, and provenance in the validation request.

## Acceptance criteria

- A customer publishes a rule pack to OCI, installs and pins it, and runs its configured checks through normal validation commands without compiling Atmos.
- A customer scaffolds a Go or Starlark validator and passes generated positive/negative contract fixtures before publishing.
- Package installation alone does not activate checks or attach deployment gates.
- Existing configured policies and lint behavior remain compatible; built-in schema/structural validation cannot be replaced by an external check.
- Custom component types participate in component validation through the registry.
- Findings preserve identity, severity, context, and truthful source locations across supported output formats.
- Required checks block on missing plugins, unavailable inputs, errors, crashes, and timeouts as well as violations.
- Artifact checks distinguish source configuration from generated plans and evaluate the actual saved plan when used as an apply/deploy gate.
- The release documentation includes rule-pack, Starlark-validator, and external-linter adapter examples with the complete scaffold-to-publication workflow.
