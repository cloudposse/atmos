## Notes

This example runs **Terraform tests** (`*.tftest.hcl`) against a **local AWS sandbox** — no AWS account or
credentials required. The `apply` run blocks in a Terraform test normally create real infrastructure, so
they usually need a cloud account and spend. Here they run against an
[Atmos emulator component](https://atmos.tools/cli/commands/emulator/usage): a stack-scoped container that
Atmos starts and stops for you. By default it runs [Floci](https://github.com/floci-io/floci), a free,
MIT-licensed AWS emulator and a drop-in replacement for LocalStack Community Edition (EOL'd March 2026).

The `app` component provisions an S3 bucket (with versioning) and a DynamoDB table. There is **no
`providers.tf`**: a single `aws/emulator` identity in `atmos.yaml` binds every component to the emulator,
and the provider-config contributor injects the AWS provider settings (dummy credentials, path-style S3,
skip-flags, endpoint) automatically.

The emulator lifecycle is **declarative**. The `app` component declares two `kind: step` lifecycle hooks
(see `stacks/catalog/app.yaml`) that use the `emulator` step type:

```yaml
hooks:
  start-emulator:
    kind: step
    type: emulator
    events: [before.terraform.test]
    with: { component: aws, action: up }
  stop-emulator:
    kind: step
    type: emulator
    events: [after.terraform.test]
    when: always                       # tear down even if a test fails
    with: { component: aws, action: down }
```

## Usage

A container runtime — Docker or Podman — is the only prerequisite. Then a single command runs the tests
(the hooks bring the emulator up and down around them):

```shell
atmos terraform test app -s local
```

Or use the bundled custom command, which also validates the stacks first:

```shell
atmos test
```

In CI (where `$GITHUB_STEP_SUMMARY` is set and CI is auto-detected), `atmos terraform test` writes a
pass/fail **step summary** to the GitHub Actions job summary — the same native-CI path as `plan`/`apply`.
