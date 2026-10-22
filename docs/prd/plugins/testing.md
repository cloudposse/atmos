# Testing Atmos plugins

**Status:** Native OIDC and lifecycle tests implemented; this document retains broader release acceptance targets. See the [first-release checkpoint](first-release.md#native-pilot-checkpoint). Starlark parity and live customer validation are not claimed as complete. [All plugin PRDs](README.md).

Plugin authors own their repository's tests and release pipeline. Atmos supplies SDK testing helpers and a runtime contract runner. Contract tests exercise the plugin through the actual Atmos adapter, not just direct Go method calls.

Reuse Atmos's existing automation test-step execution and reporting for authored assertion steps, cancellation, and grouped results. The plugin runner adds capability fixtures and mocked API services, rather than a second scheduling or results engine. Authors can also test shared Starlark logic with the existing [automation testing workflow](../../../website/docs/automation/testing.mdx); the proposed plugin contract runner does not depend on the still-proposed general Starlark test-file discovery feature.

```shell
# Go-specific unit tests, independent of an Atmos source checkout.
go test ./... -race
atmos plugin build .
# Static package/schema checks; no plugin execution.
atmos plugin validate .
# Default mocked behavioral suite for either runtime.
atmos plugin test .
# Run one scenario while developing; emit machine-readable results in CI.
atmos plugin test . --run expired-session --format json
# Explicit live-service suite, with separately supplied test credentials.
atmos plugin test . --integration
```

`test` loads `tests/contract.yaml`, starts a temporary Atmos process with isolated credentials and configuration, and invokes native RPC or Starlark callbacks using the same capability semantics. Test time, environment, prompts, HTTP responses, and persistence are supplied by the harness. Fixtures contain synthetic credentials. There is no automatic login with the developer's real identity.

An auth scenario looks like this; the scaffold generates a complete version of this file:

```yaml
apiVersion: atmos/v1
kind: AtmosPluginTestConfig
metadata:
  name: acme-auth-contracts
spec:
  scenarios:
    - name: expired-session
      suite: contract
      capability: auth.provider
      kind: acme/broker
      fixture: fixtures/expired-session.yaml
      expect:
        credential_type: aws/v1
        broker_requests: 1
        reused_expired_credentials: false
```

The `aws/v1` credential type is registered; see [credential types](platform/configuration.md#credential-types).

The fixture supplies provider configuration, a frozen clock, an expired stored session, a synthetic input token, and the expected mock broker response. The runner requests authentication through Atmos and asserts that the expired session is replaced. Unexpected calls through `api.HTTP`/process services fail the mocked suite. Native plugins can bypass API services, so this is not an OS network sandbox; authors using their own HTTP clients inject a local mock endpoint, and CI can enforce network isolation externally.

| Test layer | What it proves | Default execution |
|---|---|---|
| Author unit tests | Exchange logic, response mapping, validation, error handling. | Go tooling or Starlark scenarios using mocked API services. |
| Shared contract tests | Manifest/handler agreement, schemas, lifecycle, credential envelopes, diagnostics, cancellation and declared optional features. | `atmos plugin test .`; identical applicable cases for Go and Starlark. |
| Local consumer test | A packaged plugin participates in real Atmos commands, configuration and chains. | Generated example project with a profile that sets `path`; mock broker for automated tests. |
| Live integration tests | The real broker accepts the flow and issues usable credentials. | Explicit `--integration`; dedicated test account, credentials and cleanup. |
| Atmos platform tests | Loader/registry adapters, OCI lifecycle, compatibility and built-in interoperability. | Atmos repository CI using fixture plugins and a local OCI registry. |

The generated auth suite covers successful login, rejected upstream proof, missing inputs, noninteractive prompt rejection, reuse of a valid session, renewal of an expired session, unsupported refresh, logout, realm separation, malformed credentials, and log redaction. Live scenarios declare `suite: integration` and the names of their required test inputs. `--integration` selects those scenarios and fails clearly if required inputs are absent. Tests that need live credentials are reported as not selected by default, not represented as passing.

Example output makes the behavior under test visible:

```text
acme/auth 1.2.0 · auth.provider acme/broker · native protocol 1
PASS schema-validation
PASS login-with-mock-broker
PASS reuse-valid-session
PASS expired-session
PASS noninteractive-prompt-rejected
PASS secrets-redacted
Live integration suite: not selected (use --integration)
```

Platform tests additionally exercise plugin credentials feeding a built-in identity against a mock cloud endpoint; schema validation before execution; timeouts/crashes; project/global precedence; `path` packages; frozen installs; and unchanged built-in behavior. Release tests publish, discover, install, update, unlist, and delete against an ephemeral OCI registry, including interrupted operations and digest failures.

Each plugin release runs its contract fixtures against the oldest and newest Atmos versions it declares supported. Generated release workflows require validation, author tests, and contract tests before publishing; supported platform binaries are smoke-tested on corresponding runners. The SDK and Atmos CI retain old fixture plugins to detect protocol regressions. `test` returns nonzero on failure and includes plugin version, scenario, operation, and expected/actual non-secret values in diagnostics.


## Go OIDC first-release gate

Run the [OIDC pilot conformance suite](oidc-pilot.md#conformance-and-pilot-sign-off) through the native plugin runtime. It adds a real loopback callback, a local mock issuer with signed tokens/JWKS, downstream broker and cloud fixtures, persistence across Atmos processes, and concurrent refresh/logout coverage. The supplied-token broker example above is a smaller unit of this flow.

The first-release runner supports Go auth and executable-command fixtures. Starlark execution and parity are added with that runtime; their absence does not block `plugin test` or the pilot. Live issuer/broker validation is explicit and reported independently of mock conformance. Tests also exercise the [complete command inventory](first-release.md#complete-command-inventory), existing scaffold integration, and [OCI reuse](platform/oci-reuse.md#verification-and-delivery).

## Policy, UI, schema, and coverage acceptance

- Exercise the [policy matrix](policy.md#acceptance) against both CEL and Starlark: company registry allow/deny, direct references, catalog destinations, cached activation, explicit development/layout exceptions, inspected names, and normalized capability IDs. Assert denial happens before network, credentials, executable startup, or config/lock mutation as applicable.
- Test signature enforcement independently: wrong identity/issuer/digest, invalid or missing transparency evidence, expired trust metadata, layered constraints, and offline re-verification. General policy success must not bypass signature requirements.
- Validate the generated Atmos configuration schema and shared manifest schemas, including publisher settings and `on: plugin` input/schema registration. Reject unsupported required contracts and reserved settings rather than silently accepting them.
- Test common output formats, terminal/color degradation, installed green dots, narrow widths, and CSV/TSV quoting. Machine output must remain parseable and free of UI decorations.
- Target at least 90% aggregate statement coverage for authored plugin implementation, auth registry, shared OCI/API changes, plugin CLI, and SDK. Report the exact scope and exclusions (generated protobufs, generated mocks, and test support); do not present scoped coverage as whole-repository or Codecov patch coverage. Prioritize behavioral negative paths over tests that mirror implementation.
- Record and validate the emulator-backed lifecycle using `atmos --chdir=demo/casts casts generate examples native-auth-plugins lifecycle` and the corresponding `casts validate` command. Embed the recording in the [changelog post](../../../website/blog/2026-10-19-native-auth-plugins.mdx). The generated and downloaded binaries both run the OIDC suite; the recording is not evidence of a live enterprise IdP deployment.
- Generate protocol bindings with `atmos dev generate proto`, which pins tools through the existing custom-command/toolchain mechanism; no `generate.sh` entry point. Verify parity of `pkg/plugin/proto` and `sdk/proto` from one source.

## Additional release gates

The [extension PRDs](extensions/README.md) define applicable scenario suites. First-release combined-package tests use auth and an executable command; MCP/skills tests follow their adapters. Also test [combined package atomicity](package-composition.md#acceptance), [compiler-free platform restoration](distribution.md#acceptance-criteria), and [toolchain dependency locks](toolchain-dependencies.md). The same package release must drive all its extensions; MCP sessions retain a stable selection until restart. Data-only skills/rules validate without subprocess launch.
