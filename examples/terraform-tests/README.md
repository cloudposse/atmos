---
title: Terraform Tests
tags: [Components]
cast:
  file: /casts/examples/terraform-tests/stack-discovery.cast
  title: atmos terraform tests
---

## Notes

This example runs **Terraform tests** (`*.tftest.hcl`) against a **local AWS sandbox** — no AWS account or
credentials required. The `apply` run blocks in a Terraform test normally create real infrastructure, so
they usually need a cloud account and spend. Here they run against an
[Atmos emulator component](https://atmos.tools/cli/commands/emulator/usage): a stack-scoped container that
Atmos starts and stops for you. By default it runs [Floci](https://github.com/floci-io/floci), a free,
MIT-licensed AWS emulator and a drop-in replacement for LocalStack Community Edition (EOL'd March 2026).

The `fixtures` stack defines two Terraform components:

- `vpc` provisions a fixture VPC in the emulator.
- `app` provisions an S3 bucket (with versioning) and a DynamoDB table, and requires the fixture VPC to
  already exist. The app component looks the VPC up by tag; it does not create the VPC itself.

There is **no `providers.tf`**: a single `aws/emulator` identity in `atmos.yaml` binds every component to
the emulator, and the provider-config contributor injects the AWS provider settings (dummy credentials,
path-style S3, skip-flags, endpoint) automatically.

The fixture lifecycle is **declarative** and owned by the component being tested. The `app` component
declares two ordered `kind: steps` lifecycle hooks (see `stacks/catalog/app.yaml`):

```yaml
hooks:
  test-fixtures-up:
    kind: steps
    on_failure: fail
    events: [before.terraform.test]
    with:
      - type: emulator
        component: aws
        action: up
      - type: atmos
        command: terraform apply vpc -s fixtures -auto-approve

  test-fixtures-down:
    kind: steps
    events: [after.terraform.test]
    when: always
    with:
      - type: atmos
        command: terraform destroy vpc -s fixtures -auto-approve
      - type: emulator
        component: aws
        action: down
```

Atmos passes two generated varfiles to native `terraform test`: the normal component varfile for module
inputs, and a test varfile from `test.vars`. The `fixtures` stack uses `test.vars.fixture_vpc_id` with
`!terraform.state` so the test can assert against the VPC ID after the hook applies the fixture. Test
files declare only the test-scope variables they reference, so other test files can ignore those values
without undeclared-variable warnings.

## Usage

A container runtime — Docker or Podman — is the only prerequisite. Then a single command runs the tests
(the hooks bring the emulator and fixture VPC up and down around them):

```shell
atmos terraform test app -s fixtures
```

Or use the bundled custom command, which also validates the stacks first:

```shell
atmos test
```

In CI (where `$GITHUB_STEP_SUMMARY` is set and CI is auto-detected), `atmos terraform test` writes a
pass/fail **step summary** to the GitHub Actions job summary — the same native-CI path as `plan`/`apply`.
