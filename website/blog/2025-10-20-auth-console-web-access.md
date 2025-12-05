---
slug: auth-console-web-access
title: Cloud Console Access with atmos auth console
sidebar_label: Cloud Console Access
authors:
  - osterman
tags:
  - feature
release: v1.196.0
---

Atmos now includes `atmos auth console`, a convenience command for opening cloud provider web consoles. Similar to `aws-vault login`, this command uses your authenticated Atmos identities to generate temporary console sign-in URLs and open them in your browser.

<!--truncate-->

## What Changed

The `atmos auth console` command generates temporary sign-in URLs for cloud provider web consoles and opens them in your default browser. This removes the need to manually copy credentials or log in separately.

### Features

- Single command to open cloud consoles
- Provider-agnostic design (AWS available now, Azure and GCP planned)
- Uses provider-native federation endpoints
- Configurable session duration (up to 12 hours for AWS)
- Service aliases: use `s3`, `ec2`, `lambda` instead of full URLs
- Navigate directly to specific console pages (100+ AWS services)
- Print URLs for scripting workflows

## Why This Helps

Instead of manually copying credentials or maintaining separate browser sessions, you can access cloud consoles with a single command:

```shell
atmos auth console
```

The command integrates with your existing Atmos auth workflows:

```shell
# Quick console access
atmos auth console

# Access specific AWS services
atmos auth console --destination https://console.aws.amazon.com/s3

# Longer sessions for complex tasks
atmos auth console --duration 4h

# Print URL for scripts
atmos auth console --print-only | pbcopy
```

## How to Use It

### Basic Usage

Open the cloud console with your default identity:

```shell
atmos auth console
```

### With Specific Identity

```shell
atmos auth console --identity prod-admin
```

### Navigate to Specific AWS Services

Atmos supports 100+ AWS service aliases for convenient shorthand access:

```shell
# S3 Console (using alias)
atmos auth console --destination s3

# EC2 Console (using alias)
atmos auth console --destination ec2

# CloudFormation Console (using alias)
atmos auth console --destination cloudformation

# Lambda Console (using alias)
atmos auth console --destination lambda

# DynamoDB Console (using alias)
atmos auth console --destination dynamodb
```

You can also use full URLs if preferred:

```shell
# Full URL format
atmos auth console --destination https://console.aws.amazon.com/s3
```

**Supported aliases include**: `s3`, `ec2`, `lambda`, `dynamodb`, `rds`, `vpc`, `iam`, `cloudformation`, `cloudwatch`, `eks`, `ecs`, `sagemaker`, `bedrock`, and many more. Aliases are case-insensitive.

### Scripting and Automation

```shell
# Print URL without opening browser
atmos auth console --print-only

# Copy to clipboard (macOS)
atmos auth console --print-only | pbcopy

# Copy to clipboard (Linux)
atmos auth console --print-only | xclip

# Use in scripts
CONSOLE_URL=$(atmos auth console --print-only --identity prod-oncall)
echo "Emergency console access: $CONSOLE_URL"
```

### Custom Session Duration

```shell
# 2-hour session for extended work
atmos auth console --duration 2h

# Maximum AWS session (12 hours)
atmos auth console --duration 12h
```

## Under the Hood

For AWS identities, Atmos uses the [AWS Federation Endpoint](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_enable-console-custom-url.html) to generate secure console URLs:

1. **Authenticate**: Atmos obtains temporary credentials using your configured identity (AWS SSO, SAML, etc.)
2. **Federation Token**: Temporary credentials are exchanged for a signin token via AWS's federation endpoint
3. **Console URL**: A special URL containing the signin token is constructed
4. **Browser Launch**: The URL opens in your default browser, providing instant console access.

### Provider-Agnostic Design

The implementation uses a flexible interface pattern that makes it easy to add support for other cloud providers:

```go
type ConsoleAccessProvider interface {
    GetConsoleURL(ctx context.Context, creds ICredentials, options ConsoleURLOptions) (url string, duration time.Duration, err error)
    SupportsConsoleAccess() bool
}
```

This means Azure Portal and Google Cloud Console support will be straightforward to add in future releases.

## Real-World Use Cases

### Incident Response

```shell
# Rapidly access production console during an incident
atmos auth console --identity prod-oncall --duration 2h
```

### Multi-Account Workflows

```shell
# Quickly switch between different account consoles
atmos auth console --identity dev-account
atmos auth console --identity staging-account
atmos auth console --identity prod-account
```

### CI/CD Integration

```shell
# Generate console URL in CI/CD for manual verification
CONSOLE_URL=$(atmos auth console --print-only)
slack-notify "Deployment complete. Verify at: $CONSOLE_URL"
```

### Team Collaboration

```shell
# Use custom issuer to track which team accessed the console
atmos auth console --issuer platform-team --duration 4h
```

## Current Provider Support

| Provider | Status | Notes |
|----------|--------|-------|
| AWS (IAM Identity Center) | ✅ Available Now | Full support with federation endpoint |
| AWS (SAML) | ✅ Available Now | Full support with federation endpoint |
| Azure | 🚧 Coming Soon | Planned for future release |
| GCP | 🚧 Coming Soon | Planned for future release |

## Security Best Practices

1. **Never Share Console URLs**: Signin tokens provide authenticated access and should be treated as sensitive credentials
2. **Use Appropriate Durations**: Choose session durations based on your actual needs (shorter is more secure)
3. **Enable Logging**: Use custom issuer names to track console access in your audit logs
4. **Require MFA**: Ensure your identity provider enforces MFA for console access

## Examples

### Opening AWS S3 Console

```shell
$ atmos auth console --destination s3
**Console URL generated**
Provider: aws-sso
Identity: prod-admin
Account: 123456789012
Session Duration: 1h

Console URL:
https://signin.aws.amazon.com/federation?Action=login&Issuer=atmos&Destination=https%3A%2F%2Fconsole.aws.amazon.com%2Fs3&SigninToken=VeryLongTokenString...

Opening console in browser...
```

### Printing URL for Scripting

```shell
$ atmos auth console --print-only
https://signin.aws.amazon.com/federation?Action=login&Issuer=atmos&Destination=https%3A%2F%2Fconsole.aws.amazon.com&SigninToken=VeryLongTokenString...
```

## Get Involved

Try it out and let us know what you think:

- Update to the latest Atmos version and run `atmos auth console`
- Share feedback on what works well and what could be improved
- Tell us which cloud providers you'd like to see supported next
- Contribute Azure or GCP support at [github.com/cloudposse/atmos](https://github.com/cloudposse/atmos)

## Related Documentation

- [atmos auth console command reference](/cli/commands/auth/console)
- [AWS Console Federation](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_enable-console-custom-url.html)

---

This feature is available in Atmos v2.x and later. Update your installation to start using `atmos auth console` today!
