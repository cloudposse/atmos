# Native CI E2E

This fixture exists to exercise Atmos native CI behavior in GitHub Actions.
It is intentionally test-focused rather than an example for users to copy.

The corresponding workflow exposes visible jobs for inspecting native CI
summaries, outputs, status checks, PR comments, and Code Scanning annotations:

- `[native ci] terraform plan`
- `[native ci] terraform apply`

The fixture intentionally enables the overloaded set of native CI features we
want to validate together: Atmos output variables, summaries, status checks, PR
comments, SARIF hook handling, Atmos native CI cache, the Terraform registry
cache, eager provider mirroring, and toolchain-managed Terraform installation.

Scanner findings are produced by real scanner hook kinds (`checkov`, `trivy`,
and `kics`) against separate intentionally-insecure Terraform files. Keep those
targets separate so the pull request's Code Scanning annotations show the
scanner names and distinct file/line locations. Do not replace them with a
synthetic SARIF fixture; generic `format: sarif` coverage belongs in Go tests,
not in this visual E2E workflow.
