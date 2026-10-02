# AWS credential_process: Both Directions

AWS SDKs and the AWS CLI share one extension point, `credential_process`: a profile in
`~/.aws/config` names a command, the SDK runs it, and the command prints a small JSON document with
credentials. Atmos Auth supports this protocol in both directions.

| Direction | Use | Feature |
|-----------|-----|---------|
| Atmos consumes a helper | An existing command (Okta CLI, aws-sso-cli, aws-vault, Granted, a corporate SAML tool) already prints AWS credentials | Identity kind `aws/credential-process` |
| Atmos produces credentials | The AWS CLI or any SDK-based tool must use an Atmos identity from any directory | Command `atmos aws credential-process` |

## Consume a Helper: aws/credential-process

```yaml
auth:
  identities:
    corp-base:
      kind: aws/credential-process
      credentials:
        credential_process: corp-credential-helper --account=prod   # Required
        region: us-east-1                                                   # Optional
      spec:
        endpoint_url: http://localhost:4566                                 # Optional: emulators
```

The identity is standalone. It takes no `via`. Credentials come from the helper.

### Choosing Between aws/user, aws/ambient, and aws/credential-process

| Situation | Kind |
|-----------|------|
| An IAM user with long-lived access keys; Atmos calls STS `GetSessionToken` and handles MFA | `aws/user` |
| The environment already provides credentials (IRSA, instance profile, ECS task role, `AWS_PROFILE`) | `aws/ambient` |
| A command already prints temporary credentials | `aws/credential-process` |

### Behavior

- **Used as returned.** Atmos makes no STS call and never prompts for MFA. The helper owns MFA.
- **Shell execution.** The command runs through the platform shell (`sh -c` on Linux and macOS,
  `cmd.exe /C` on Windows), like the AWS CLI, so a command copied from `~/.aws/config` behaves the same.
- **Terminal passthrough.** The helper keeps stdin and stderr, so it can prompt for MFA or print
  browser-login instructions.
- **Caching.** Credentials are written to Atmos-managed AWS files with their `Expiration` and reused
  until they expire.
- **No `Expiration`.** The helper runs on every Atmos invocation, because Atmos cannot know how long
  the credentials last.
- **Timeout.** The helper must finish within 1 minute.
- **No keyring storage.** Helper output is never stored in the keyring.
- **Validation.** `credential_process` is required. Atmos rejects `via`, `access_key_id`,
  `secret_access_key`, and `mfa_arn` on this kind. Use `aws/user` for IAM user keys.

### Chain a Role from a Helper

```yaml
auth:
  identities:
    corp-base:
      kind: aws/credential-process
      credentials:
        credential_process: >-
          okta-aws-cli web --format=process-credentials --org-domain=example.okta.com --oidc-client-id=0oa1example --aws-iam-idp=arn:aws:iam::111111111111:saml-provider/okta --aws-iam-role=arn:aws:iam::111111111111:role/Admin
    prod-admin:
      kind: aws/assume-role
      via:
        identity: corp-base
      principal:
        assume_role: arn:aws:iam::111111111111:role/Admin
```

### Common Helper Commands

Each helper must print AWS process-credential JSON to stdout.

| Helper | Example `credential_process` |
|--------|------------------------------|
| Okta AWS CLI | `okta-aws-cli web --format=process-credentials --org-domain=<org> --oidc-client-id=<id> --aws-iam-idp=<idp-arn> --aws-iam-role=<role-arn>` |
| aws-sso-cli | `aws-sso process --sso=<instance> --arn=arn:aws:iam::111111111111:role/Admin` |
| aws-vault | `aws-vault export --format=json my-profile` |
| Granted | `granted credential-process --profile=my-profile` |
| Custom script | Any script that prints the JSON document, for example one that reads a secrets manager |

Check each helper's own documentation for the exact flags in your installed version.

### Validation and Runtime Errors

| Error | Meaning | Fix |
|-------|---------|-----|
| `credential_process command failed` | The helper could not start, exited non-zero, or timed out | Run the command in a terminal to see its error output |
| `credential_process returned invalid output` | The helper printed something other than a valid version-1 document; the message lists missing or invalid fields, never their values | Fix the helper output |
| `credential_process recursion detected` | The helper called back into Atmos for an identity that was already resolving | Point the helper at a different identity |

## Produce Credentials: atmos aws credential-process

```shell
atmos aws credential-process --identity=<name> [--min-validity=15m]
```

Prints credentials for any Atmos AWS identity in the process-credential format. The alias
The alias `atmos auth env --format=credential-process --identity=<name>` prints the same output.

Identity resolution order: `--identity`, then `ATMOS_IDENTITY`, then the default identity. Interactive
selection is not available because the AWS CLI captures stdout.

Cached credentials that expire later than `--min-validity` (default `15m`) print immediately with no
network call. Otherwise Atmos authenticates the identity as `atmos auth login` would.

### Output Contract

```json
{"Version":1,"AccessKeyId":"ASIA...","SecretAccessKey":"...","SessionToken":"...","Expiration":"2026-10-02T18:30:00Z"}
```

The `Version` field is always `1`. Long-lived keys omit `SessionToken` and `Expiration`. The expiration
is RFC3339 in UTC. The output is unmasked. Logs go to stderr.

### Profile Recipes

Use `--chdir` for the most robust setup:

```ini
[profile app-sandbox-1]
credential_process = atmos --chdir=/path/to/infrastructure aws credential-process --identity=app-sandbox-1
region = us-east-1
```

Other ways to locate `atmos.yaml` from any directory:

```ini
# Explicit config path
credential_process = atmos --config-path=/path/to/infrastructure aws credential-process --identity=app-sandbox-1

# Short form, with ATMOS_CLI_CONFIG_PATH exported in the shell that runs aws
credential_process = atmos aws credential-process --identity=app-sandbox-1
```

A global `~/.atmos/atmos.yaml` that defines the identities also works with no path flags. On Windows,
quote paths that contain spaces.

### Verify

```shell
aws sts get-caller-identity --profile=app-sandbox-1
```

## Troubleshooting

- **The command asks you to log in.** The AWS CLI owns the terminal, so an expired SSO session cannot
  open a browser. Run `atmos auth login --identity=<name>` first, then retry.
- **Atmos cannot find the configuration.** Add `--chdir=/path/to/infrastructure` to the profile.
- **Recursion error.** An `aws/credential-process` identity whose helper is
  `atmos aws credential-process --identity=<itself>` loops. Point the helper at a different identity.
- **Output looks wrong.** Run the command in a terminal. Stdout must contain exactly one JSON document.

## Migrating from Other Tools

- **aws-sso-util or a shell script around `atmos auth env`.** Replace the script with a
  `credential_process` profile that calls `atmos aws credential-process`. The output includes a
  correct `Expiration`.
- **Granted or aws-vault.** Keep them as the credential source with `aws/credential-process`, or move
  to native Atmos identities (`aws/permission-set`) and use the producer command so
  `aws --profile=<name>` keeps working. See the `atmos-migration` skill.
- **Older Atmos versions without `aws/credential-process`.** Use `aws/ambient` with `AWS_PROFILE`
  set in the environment so the AWS SDK default credential chain resolves the helper-backed profile.
