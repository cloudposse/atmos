# TFLint Reference

## Core config

Put `.tflint.hcl` at the desired discovery level. The Terraform ruleset works
without a plugin configuration; configure plugins only for provider-specific
checks.

```hcl
plugin "terraform" {
  enabled = true
  preset  = "recommended"
}

# Keep the rule installed but do not report it for this project.
rule "terraform_typed_variables" {
  enabled = false
}
```

Use a rule block with `enabled = false` for durable, reviewable project policy.
Use the CLI flags below for a one-off hook, workflow, or diagnostic run. Do not
disable a rule merely to hide a finding; document the accepted exception in the
configuration or code review.

## Rule selection and severity

```yaml
hooks:
  tflint:
    events: [before.terraform.plan]
    kind: tflint
    args:
      - --enable-rule=terraform_unused_declarations
      - --disable-rule=terraform_typed_variables
      - --minimum-failure-severity=warning
```

- `--enable-rule=<name>` enables a named rule for that invocation.
- `--disable-rule=<name>` disables a named rule for that invocation.
- `--minimum-failure-severity=<severity>` controls which finding severities
  produce a non-zero exit status. Pair it with `on_failure: fail` when lint is a gate.
- `--filter=<path-or-pattern>` narrows linting while debugging a specific file.
- `--format=sarif` is the required format for Atmos's structured scanner
  handling; keep it when changing hook args.

Use `tflint --help` from the component toolchain version to verify supported
flags and exact severity values. TFLint capabilities vary by installed version
and enabled plugins; do not apply flags copied from an unrelated version blindly.

## Provider plugins

Provider plugins add cloud-specific checks. They require plugin declarations in
the config and a `tflint --init` before linting. Run initialization through a
toolchain-aware hook or workflow step against the same component directory.

```hcl
plugin "aws" {
  enabled = true
  version = "0.39.0"
  source  = "github.com/terraform-linters/tflint-ruleset-aws"
}
```

```yaml
hooks:
  tflint-init:
    events: [before.terraform.plan]
    kind: command
    command: tflint --chdir=$ATMOS_COMPONENT_PATH --init
  tflint:
    events: [before.terraform.plan]
    kind: tflint
    on_failure: fail
```

Treat plugin versions as part of the repository's lint policy. Pin them, review
their rule changes, and avoid performing `--init` in an offline CI job unless
the plugin cache is intentionally prepared.

## Troubleshooting sequence

1. Run `atmos terraform lint <component> --stack <stack>` to reproduce with the
   component's resolved TFLint version and environment.
2. Check which `.tflint.hcl` wins: component, components base path, repository
   root, then `components.terraform.lint.config`.
3. Confirm `dependencies.tools.tflint` in the resolved stack/component config
   when versions differ across components.
4. Run `tflint --init` only when the active config declares provider plugins.
5. Keep `--format=sarif` for hook and CI output; inspect the command output for
   the rule name before enabling, disabling, or suppressing it.
