# PRD: AWS `credential_process` Support (Consume and Produce)

## Overview

AWS SDKs and the AWS CLI support a standard extension point called
[`credential_process`](https://docs.aws.amazon.com/sdkref/latest/guide/feature-process-credentials.html):
a profile in `~/.aws/config` names a command, the SDK runs it, and the command prints a small JSON
document with temporary (or long-lived) credentials.

Atmos Auth should speak this protocol in both directions:

1. **Consume** — a new `aws/credential-process` identity kind runs an external helper (Okta CLI,
    aws-sso-cli, aws-vault, Granted, a corporate SAML tool, …) and uses the credentials it returns.
    ([#1734](https://github.com/cloudposse/atmos/issues/1734), supersedes the PRD in
    [#1795](https://github.com/cloudposse/atmos/pull/1795))
2. **Produce** — `atmos aws credential-process --identity=<name>` prints credentials for any Atmos
    identity in the same format, so a `~/.aws/config` profile can delegate to Atmos.
    ([#3248](https://github.com/cloudposse/atmos/issues/3248))

## Problem

### Producing: running `aws` from anywhere

Teams that adopt `atmos auth` in place of tools like `aws-sso-util` or Granted lose a workflow they
relied on: running the AWS CLI (or any SDK-based tool) from any directory with a named profile.
Today they must run `atmos auth exec -- aws ...` from inside the Atmos project. Users have worked
around this with shell scripts that `eval $(atmos auth env)` and re-encode the result as JSON,
without a correct `Expiration`.

### Consuming: existing credential helpers

Many organizations already source temporary AWS credentials from a helper process. Atmos Auth
cannot use those credentials today, so those users cannot adopt `atmos auth shell`, identity
chaining, or the automatic authentication of `atmos terraform`.

## User Stories

- **As a platform engineer** who uses Atmos Auth for SSO, **I want** an `~/.aws/config` profile that
  calls Atmos, **so that** `aws --profile app-sandbox-1 s3 ls` works from any directory.
- **As an engineer** whose company vends credentials through a custom helper, **I want** Atmos to run
  that helper, **so that** I can use `atmos auth shell`, `atmos terraform`, and identity chaining
  without copying credentials around.
- **As an engineer** with a helper that returns a base session, **I want** to chain
  `aws/assume-role` identities from it, **so that** I can reach every account I am allowed to.

## Goals

1. Follow the AWS process-credential specification exactly, in both directions.
2. Work with every existing Atmos Auth workflow: `auth shell`, `auth exec`, `auth env`, `auth whoami`,
    identity chaining, and commands that authenticate automatically.
3. Respect credential lifetimes: reuse unexpired credentials, refresh expired ones.
4. Never leak secrets in logs or error messages.
5. Work from any directory when producing credentials.

## Non-Goals

- Non-AWS credential helpers (for example, GCP executable-sourced credentials).
- Writing or managing `~/.aws/config` for the user.
- Implementing credential helpers. Atmos only consumes and produces the protocol.

## Consume: `aws/credential-process` Identity

### Configuration

```yaml
auth:
  identities:
    corp-base:
      kind: aws/credential-process
      credentials:
        credential_process: corp-credential-helper --account=prod   # Required.
        region: us-east-1                                      # Optional.
      spec:
        endpoint_url: http://localhost:4566                    # Optional, for emulators.

    prod-admin:
      kind: aws/assume-role
      via:
        identity: corp-base
      principal:
        assume_role: arn:aws:iam::111111111111:role/Admin
```

<dl>
  <dt><code>credentials.credential_process</code> (required)</dt>
  <dd>The command to run. It is executed through the platform shell, like the AWS SDK for Go
  (<code>sh -c</code> on Linux and macOS, <code>%COMSPEC% /S /C "&lt;command&gt;"</code> on Windows with the
  command line passed verbatim). A command copied from <code>~/.aws/config</code> therefore runs as
  written, and pipes and environment variable expansion work. The AWS CLI itself does not use a shell
  (it splits the command into arguments), so shell-only syntax is not portable back to an AWS CLI
  profile. Stdin and stderr are inherited so a helper can prompt for MFA or print browser-login
  instructions when Atmos runs interactively.</dd>
  <dt><code>credentials.region</code> (optional)</dt>
  <dd>Default region written to the Atmos-managed AWS config.</dd>
  <dt><code>spec.endpoint_url</code> (optional)</dt>
  <dd>Custom AWS endpoint, used with emulators.</dd>
</dl>

The identity is **standalone**: it has no `via`, because its credentials come from the helper.

### Why a Dedicated Kind (and Not `aws/user`)

The `aws/user` kind represents an **IAM user**: it takes long-lived access keys, calls STS `GetSessionToken`,
prompts for the IAM user's MFA device, and enforces IAM-user session limits.

Most credential helpers return **temporary** credentials for an SSO session, a SAML federation, or an
assumed role. Those credentials are not tied to an IAM user, `GetSessionToken` cannot be called with
them, and MFA is the helper's responsibility. Adding `credential_process` to `aws/user` would overload
that kind with a meaning it does not have.

The `aws/credential-process` kind is a sibling of `aws/ambient`: both are standalone identities whose
credentials originate outside Atmos. The difference is that `aws/ambient` passes the ambient
environment through untouched, while `aws/credential-process` runs a specific command and writes the
result to Atmos-managed credential files, like every other Atmos AWS identity.

### Behavior

- **No transformation.** Credentials are used exactly as returned. Atmos makes no STS call and
  never prompts for MFA.
- **Caching.** Credentials are written to the Atmos-managed AWS files
  (`~/.config/atmos/<realm>/aws/aws-credential-process/`) with their expiration. While those
  credentials have at least the required validity left (15 minutes by default, or the
  `--min-validity` of `atmos aws credential-process`), Atmos reuses them instead of running the helper
  again. Credentials with less time left are refreshed before they expire, so a helper that returns
  15-minute credentials runs on every invocation.
- **No `Expiration`.** Credentials without an expiration are never reused across Atmos invocations;
  the helper owns their lifecycle and runs every time.
- **Keyring.** Helper output is never stored in the keyring, on any command. The identity opts out
  through the optional `types.CredentialPersistence` interface, and the manager skips every keyring read
  and write for it. `atmos auth logout` removes any stale entry written by an older version, without
  `--keychain`. The helper is the source of truth.
- **Timeout.** The helper must finish within 1 minute (the AWS SDK default). The limit is not
  configurable, so a helper that waits for an MFA code or a browser login can time out. After the
  timeout, Atmos stops waiting within a couple of seconds even if grandchildren keep the output pipe open.
- **Prompts.** Atmos prompts only when stdin and stderr are both terminals. Under a parent that
  captures stderr, prompts fail fast with `ErrAuthPromptUnavailable` and a hint to run
  `atmos auth login`.
- **Console.** `atmos auth console` needs the helper to return temporary credentials with a session
  token. Long-lived keys fail.
- **Validation.** `credential_process` is required. `via`, `session`, `principal`, `access_key_id`,
  `secret_access_key`, and `mfa_arn` are rejected, with hints (for example pointing to `aws/user` for
  IAM-user credentials). The manager validates every identity in a chain before it authenticates any
  step, so an invalid later identity never triggers an upstream helper.

### Identity Chaining Fix

Chaining from any standalone identity builds the chain `[root-identity, child, …]`. When the root had
no valid cached credentials, the auth manager tried to authenticate the root as if it were a
*provider*, failing with “provider not registered.” This already affected `aws/user` roots configured
with YAML keys once their session expired, and would always affect `aws/credential-process`.

The manager now authenticates a standalone identity at the root of a chain through the standalone
path. Chains rooted at registered providers are unchanged.

## Produce: `atmos aws credential-process`

### Usage

```shell
atmos aws credential-process --identity=<name> [--min-validity=15m]
```

Alias (same output):

```shell
atmos auth env --format=credential-process --identity=<name>
```

The canonical command is `atmos aws credential-process`. It lives next to `atmos aws eks token`,
which plays the same role for `kubectl`.

### `~/.aws/config` Recipes

```ini
[profile app-sandbox-1]
credential_process = atmos --chdir=/path/to/infrastructure aws credential-process --identity=app-sandbox-1
region = us-east-1
```

Atmos locates `atmos.yaml` the usual way, so any of these work from any directory:

- `--chdir=/path/to/project` (or `-C`) — the most robust; behaves exactly as if run from the project.
- `--config-path=/path/to/project` or `ATMOS_CLI_CONFIG_PATH`.
- A global `~/.atmos/atmos.yaml` that defines the identities.

### Behavior

1. Resolve the identity from `--identity`, then `ATMOS_IDENTITY`, then the single identity marked
    `default: true`. The command never prompts: the AWS CLI captures stderr, so an interactive
    selector would be invisible and would hang the `aws` command. `--identity=false`, `--identity`
    without a value, no default, and multiple defaults all fail immediately with a hint
    (`ErrCredentialProcessIdentityRequired`, `ErrNoDefaultIdentity`, `ErrMultipleDefaultIdentities`).
2. If cached credentials for the identity have more than `--min-validity` (default 15 minutes) of
    lifetime left, print them without any network call. The value needs a unit (for example `30m`) and
    can also come from `ATMOS_AWS_CREDENTIAL_PROCESS_MIN_VALIDITY`. The AWS CLI runs `credential_process`
    once per CLI invocation, so this keeps every `aws` command fast.
3. Otherwise authenticate the identity. This is the authentication `atmos auth login` performs, except
    that auto-triggered integrations (ECR login, EKS kubeconfig, and similar) are skipped because the
    command runs as a non-interactive helper. Identities that cache their own credentials (`aws/user`
    sessions, `aws/credential-process` helper output) also refresh a cache that does not satisfy
    `--min-validity`. If the source still issues shorter-lived credentials, they are printed as is.
4. Print exactly one JSON document to stdout. Everything else (logs, notices) goes to stderr.

The alias `atmos auth env --format=credential-process` follows the same rules, and adds `--output-file`
(written atomically with mode `0600`, never falling back to `$GITHUB_ENV`). `--login=false` with this
format is an error, and `--min-validity` with any other format is an error.

When the identity needs an interactive login that cannot happen (for example, an expired SSO session
while the AWS CLI captures stderr), the command fails fast with an explanation and a hint to run
`atmos auth login --identity=<name>` (SSO errors hint `atmos auth login --provider=<name>`) first.

### Output Contract

```json
{"Version":1,"AccessKeyId":"ASIA…","SecretAccessKey":"…","SessionToken":"…","Expiration":"2026-10-02T18:30:00Z"}
```

- `Version` is always `1`.
- `AccessKeyId` and `SecretAccessKey` are always present.
- `SessionToken` and `Expiration` are omitted when empty (long-lived keys).
- `Expiration` is RFC3339 in UTC.

The output is written unmasked: it is the purpose of the command. Secret masking still applies to
every other Atmos output.

## Security

- **Command execution** uses the platform shell, like the AWS SDK for Go. `credential_process`
  comes from `atmos.yaml`, which is trusted configuration — the same trust level as `!exec` and
  custom commands.
- **No secrets in errors.** The AWS SDK embeds raw helper output in its parse errors. Atmos never
  wraps those errors; it reports which fields are missing or invalid without their values.
- **Recursion guard.** Atmos sets `ATMOS_AUTH_CREDENTIAL_PROCESS_CHAIN` for every helper it runs.
  If a helper calls back into Atmos for an identity that is already resolving, Atmos fails instead
  of recursing forever (for example, `credential_process: atmos aws credential-process --identity=<itself>`).
- **No keyring persistence** of helper output.

## Errors

| Sentinel | When | Hint |
|---|---|---|
| `ErrCredentialProcessFailed` | Helper could not start, exited non-zero, or timed out | Run the credential_process command for the identity manually in a terminal to see its error output |
| `ErrCredentialProcessInvalidOutput` | Helper printed something other than a valid version-1 document | The cause lists the missing or invalid fields, never their values. Hints describe the required format and suggest running the command manually and comparing its stdout |
| `ErrCredentialProcessRecursion` | Helper re-entered Atmos for the same identity | Point the helper at a different identity, or remove the call back into Atmos from its command |
| `ErrCredentialProcessIdentityRequired` | `--identity=false` or a bare `--identity` on the producer | Pass `--identity=<name>` or set `ATMOS_IDENTITY`; run `atmos auth list` |
| `ErrNoDefaultIdentity`, `ErrMultipleDefaultIdentities` | The producer was given no identity and zero or several defaults exist | Pass `--identity=<name>` or set `ATMOS_IDENTITY`; run `atmos auth list` |
| `ErrAuthPromptUnavailable` | A prompt (MFA, credentials, selection) is needed but stdin or stderr is not a terminal | Run `atmos auth login --identity=<name>` in a terminal first |
| `ErrIdentityNotAWS` | `credential-process` requested for a non-AWS identity | None; the message names the identity and the credential type it produces |

## Testing

- **Unit tests** use the Go test binary as a fake, cross-platform helper (no shell scripts): valid
  output with and without session token and expiration, invalid JSON, wrong version, missing keys,
  non-zero exit, timeout, recursion guard, and a regression test that secrets never appear in errors.
- **Identity tests** cover validation, file caching and reuse, re-execution on expiry, and
  non-persistence in the keyring. The identity has no STS client at all, so no STS call is possible.
- **Manager tests** reproduce the standalone-root chaining failure and verify provider-rooted chains
  are unchanged.
- **Command tests** verify stdout contains exactly the JSON document, unmasked, for both the
  canonical command and the `auth env` alias.
- **End-to-end tests against the Floci AWS emulator** (opt-in: they skip when no Floci
  endpoint is available, and run in the Floci CI job):
  - An AWS SDK client configured with `credential_process = atmos aws credential-process …` calls
    STS and S3.
  - An `aws/credential-process` identity whose helper is Atmos itself (round trip) authenticates and
    its credentials work against STS and S3.
  - An `aws/assume-role` chained from it.

## Documentation

- Identity reference: new “Credential Process” section in
  `website/docs/cli/configuration/auth/identities.mdx`.
- Command reference: `website/docs/cli/commands/aws/aws-credential-process.mdx`, and the
  `credential-process` format in `auth env`.
- Migration guides (`from-aws-config`, `from-granted`): `credential_process` now has an equivalent.
- Changelog blog post and roadmap entry.

## References

- [AWS SDKs and Tools: Process credential provider](https://docs.aws.amazon.com/sdkref/latest/guide/feature-process-credentials.html)
- [AWS CLI: Sourcing credentials with an external process](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sourcing-external.html)
- `docs/prd/ambient-identity.md` — the standalone identity pattern.
- `docs/prd/credential-retrieval-consolidation.md` — why `LoadCredentials` must be side-effect free.
- `docs/prd/auth-realm-architecture.md` — credential file layout.
