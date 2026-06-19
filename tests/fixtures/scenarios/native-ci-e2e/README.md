# Native CI E2E

This fixture exists to exercise Atmos native CI behavior in GitHub Actions.
It is intentionally test-focused rather than an example for users to copy.

The corresponding workflow exposes visible jobs for inspecting native CI
summaries, outputs, status checks, PR comments, and hook annotations:

- `[native ci] terraform plan`
- `[native ci] terraform apply`

The fixture intentionally enables the overloaded set of native CI features we
want to validate together: Atmos output variables, summaries, annotations,
status checks, PR comments, SARIF hook handling, Atmos native CI cache, the
Terraform registry cache, eager provider mirroring, and toolchain-managed
Terraform installation.
