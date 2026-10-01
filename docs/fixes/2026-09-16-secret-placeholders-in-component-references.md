# Fix: Component references resolve secrets before execution

## Cause

`ExecuteDescribeComponent` serves both inspection commands and internal consumers.
Its component-type processing unconditionally set `SecretsMaskOnly` whenever output
masking was enabled. In that mode, `!secret` intentionally returns the configured
mask replacement without contacting the provider. That is appropriate for inspecting
a component without secret-provider credentials, but wrong for execution.

`!terraform.output`, `!terraform.state`, and `atmos.Component()` reused that loader.
Consequently, referenced components could supply literal `<MASKED>` values in their
backend credentials, environment, variables, or static outputs. Disabling masking
also disabled inspection-only secret resolution, explaining why `--mask=false`
could make these operations work.

This establishes a reproducible bug in the shared loader. The original reporter's
exact Atmos version and stack configuration were unavailable, so it does not prove
that every reported failure has this cause.

## Correction

The two component-description parameter structs now expose `ResolveSecrets`.
Internal value-producing callers set it to true; the loader sets `SecretsMaskOnly`
only when secret resolution was not requested and display masking is enabled.
`atmos.Component()` also passes its existing Atmos configuration to the loader,
preserving the configured secret stores and resolution context.

Resolved values still register with the I/O masker. Backend access and subprocess
inputs receive real values, while terminal output remains masked. Direct
`describe component` retains credential-free secret inspection, including the
provenance path. Disabling or skipping YAML functions still skips retrieval.
Missing required secrets fail resolution rather than becoming placeholders.

Nested references preserve the enclosing `SecretsMaskOnly` mode during inspection,
including template references and both Terraform YAML functions. Inspection bypasses
execution caches: it neither reuses resolved secret values nor stores placeholders
that a later execution could consume. Tests exercise inspection before and after
execution, with and without provenance, and require zero secret-store calls.

Custom command component execution also opts into real secret resolution. Its
configuration loader used the same inspection API.

## Other component types

Kubernetes, Helm, Helmfile, and container execution resolve their own configuration
through `ProcessStacks`, which does not force inspection mode. Their native
component references still traversed the affected loader, so they benefit from the
same fix. Regression tests cover direct secrets and all three native reference
forms in the environment of each of these four component types.

## Regression coverage

- Real stack loading with a mock secret store for output, state, and component
  references; masked and unmasked modes; a custom replacement; cached lookups;
  and missing-secret errors.
- Backend credentials, environment, and component variables retain the original
  synthetic secret, while an I/O writer redacts it.
- A producer/consumer plan uses a controlled Terraform subprocess that rejects
  incorrect credentials, verifies that the generated varfile excludes the secret,
  and echoes the received credential to verify stdout/stderr masking.
- Direct inspection, with and without provenance, makes no secret-provider calls.
- Restoring the original `SecretsMaskOnly = MaskingEnabled()` decision makes the
  regression tests fail; restoring the fix makes them pass.

The separate issue in which `TF_VAR_*` transport changes the interpretation of
untyped object variables is not changed here.

## Validation

- `go test ./internal/exec ./pkg/secrets ./pkg/terraform/output ./pkg/io ./pkg/terraform/tfvars -count=1`
- `go test ./cmd -run '^TestResolveCustomComponentConfig$' -count=1`
- Focused regression tests rerun after restoring the fix.
- `go tool mage lint:changed` and a patch-scoped custom-linter run including the
  new test file: no issues.
