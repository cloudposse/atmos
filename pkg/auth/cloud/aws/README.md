# AWS Cloud Package

This package provides AWS-specific functionality for the Atmos authentication system.

## Session Duration Configuration

Session duration can be configured for both providers (SSO, SAML) and identities (IAM users):

```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      session:
        duration: "8h"

  identities:
    emergency-admin:
      kind: aws/user
      session:
        duration: "12h"  # Formats: integers (seconds), Go durations ("1h"), or days ("1d")
```

**AWS IAM user limits**: 15m-12h (no MFA) or 15m-36h (with MFA). Default: 12h

## Custom Endpoint URL

The AWS cloud package supports a custom endpoint URL, which is useful for testing with Floci, LocalStack, or other AWS-compatible services.

### Configuration

You can configure a custom endpoint URL at either the identity or provider level:

#### Identity-Level Configuration

For identities, add `endpoint_url` to `spec`:

```yaml
auth:
  identities:
    localstack-superuser:
      kind: aws/user
      credentials:
        access_key_id: test
        secret_access_key: test
        region: us-east-1
      spec:
        endpoint_url: "http://localhost:4566"
```

#### Provider-Level Configuration

For providers, add `endpoint_url` to `spec`:

```yaml
auth:
  providers:
    localstack-sso:
      kind: aws/iam-identity-center
      start_url: https://localstack.awsapps.com/start/
      region: us-east-1
      spec:
        endpoint_url: "http://localhost:4566"
```

Legacy configurations using `credentials.aws.resolver.url` on identities or
`spec.aws.resolver.url` on providers are still accepted, but new configuration
should use `spec.endpoint_url`.

### Precedence

When both identity and provider have endpoint configurations, the **identity endpoint takes precedence**.
New `spec.endpoint_url` values take precedence over legacy `aws.resolver.url`
values at the same level.

### Usage

The custom endpoint URL is automatically applied when:
- AWS identities authenticate (user, assume-role, permission-set)
- AWS providers authenticate (SSO, SAML)

All AWS SDK calls will be directed to the configured endpoint URL.

### Floci Example

For a complete AWS emulator example, see:
- `/examples/demo-floci/atmos.yaml`

### Implementation Details

The endpoint URL is implemented using AWS SDK v2's `config.WithBaseEndpoint`:

```go
return config.WithBaseEndpoint(url)
```

This ensures all AWS services (STS, SSO, etc.) use the custom endpoint. The base endpoint approach is the recommended method in AWS SDK v2 for setting custom endpoints.

## Process Credentials (`credential_process`)

`credential_process.go` implements the AWS [process-credential format](https://docs.aws.amazon.com/sdkref/latest/guide/feature-process-credentials.html)
for both directions of Atmos Auth's `credential_process` support.

### Producing credentials

`NewProcessCredentials` converts Atmos AWS credentials into the process-credential document and
`MarshalProcessCredentials` renders it as compact JSON:

```json
{"Version":1,"AccessKeyId":"ASIA...","SecretAccessKey":"...","SessionToken":"...","Expiration":"2026-10-02T18:30:00Z"}
```

`SessionToken` and `Expiration` are omitted when empty (long-lived keys). `Expiration` is RFC3339 in UTC.
This backs `atmos aws credential-process` and `atmos auth env --format=credential-process`.

### Consuming a helper

`RetrieveProcessCredentials` runs a configured `credential_process` command on behalf of the
`aws/credential-process` identity and returns the credentials it prints.

- The command runs through the platform shell, like the AWS SDK for Go: `sh -c` on Linux and macOS,
  `%COMSPEC% /S /C "<command>"` on Windows (via `pkg/process.NewShellCommand`). The AWS CLI does not
  use a shell, so pipes and environment variable expansion work in Atmos only. Stdin and stderr are
  inherited so a helper can prompt for MFA when Atmos runs interactively.
- The default timeout is 1 minute. `WithCredentialProcessTimeout` overrides it for Go callers, but the
  `aws/credential-process` identity does not expose it, so users cannot change it. `exec.Cmd.WaitDelay`
  (2 seconds) bounds how long Wait blocks on the output pipe after a timeout kill.
- `ATMOS_AUTH_CREDENTIAL_PROCESS_CHAIN` is set for every helper to detect recursion when a helper
  calls back into Atmos for an identity that is already resolving.
- Errors never include the helper's stdout, because the AWS SDK embeds raw output in parse errors.
  Invalid output is reported as the list of missing or invalid fields without their values.
- `WithCredentialProcessCommandBuilder` and `WithCredentialProcessEnviron` let tests substitute the
  command builder and base environment.

See `docs/prd/aws-credential-process.md` for the full design.
