# PRD: Web Console Access for Atmos Auth

## Status

**Implementation Status**: ✅ Completed
**Version**: Initial release
**Last Updated**: 2025-10-20

## Overview

The `atmos auth console` command provides web console/browser access to cloud providers using authenticated Atmos identities. Similar to AWS Vault's `aws-vault login` command, this feature enables instant browser-based access to cloud provider consoles without manually copying credentials.

## Motivation

AWS Vault's `aws-vault login` command is beloved by developers because it:
- Provides instant browser-based access to the AWS Console using temporary credentials
- Eliminates the need to manually copy credentials into the console
- Supports federated access with temporary session credentials
- Works seamlessly with MFA and SSO workflows

Atmos Auth should provide similar functionality but in a **provider-agnostic** way that works with AWS, Azure, GCP, and potentially other providers.

## Design

### 1. Provider Interface Extension

Add a new optional interface that providers can implement to support web console access:

```go
// pkg/auth/types/interfaces.go

// ConsoleAccessProvider is an optional interface that providers can implement
// to support web console/browser-based login.
type ConsoleAccessProvider interface {
    // GetConsoleURL generates a web console sign-in URL using the provided credentials.
    // Returns:
    //   - url: The sign-in URL to open in a browser
    //   - duration: How long the URL remains valid
    //   - error: Any error encountered
    GetConsoleURL(ctx context.Context, creds ICredentials, options ConsoleURLOptions) (url string, duration time.Duration, error error)

    // SupportsConsoleAccess returns true if this provider supports web console access.
    SupportsConsoleAccess() bool
}

// ConsoleURLOptions provides configuration for console URL generation.
type ConsoleURLOptions struct {
    // Destination is the specific console page to navigate to (optional).
    // For AWS: "https://console.aws.amazon.com/s3" or similar
    // For Azure: "https://portal.azure.com/#blade/..."
    // For GCP: "https://console.cloud.google.com/..."
    Destination string

    // SessionDuration is the requested duration for the console session.
    // Providers may have maximum limits (e.g., AWS: 12 hours).
    SessionDuration time.Duration

    // Issuer is an optional identifier shown in the console URL (used by AWS).
    Issuer string

    // OpenInBrowser if true, automatically opens the URL in the default browser.
    OpenInBrowser bool
}
```

### 2. AWS Implementation

Implement console access for AWS using the federation endpoint:

```go
// pkg/auth/cloud/aws/console.go

package aws

import (
    "context"
    "encoding/json"
    "fmt"
    "net/url"
    "time"

    "github.com/cloudposse/atmos/pkg/auth/types"
    "github.com/cloudposse/atmos/pkg/utils"
)

const (
    // AWSFederationEndpoint is the AWS console federation endpoint.
    AWSFederationEndpoint = "https://signin.aws.amazon.com/federation"

    // AWSConsoleDestination is the default AWS console destination.
    AWSConsoleDestination = "https://console.aws.amazon.com/"

    // AWSMaxSessionDuration is the maximum session duration for AWS console (12 hours).
    AWSMaxSessionDuration = 12 * time.Hour

    // AWSDefaultSessionDuration is the default session duration (1 hour).
    AWSDefaultSessionDuration = 1 * time.Hour
)

// AWSConsoleURLGenerator generates AWS console federation URLs.
type AWSConsoleURLGenerator struct{}

// GetConsoleURL generates an AWS console sign-in URL using temporary credentials.
func (g *AWSConsoleURLGenerator) GetConsoleURL(ctx context.Context, creds types.ICredentials, options types.ConsoleURLOptions) (string, time.Duration, error) {
    // Step 1: Extract AWS credentials from the interface.
    awsCreds, ok := creds.(*types.AWSCredentials)
    if !ok {
        return "", 0, fmt.Errorf("expected AWS credentials, got %T", creds)
    }

    // Step 2: Validate credentials have required fields.
    if awsCreds.AccessKeyID == "" || awsCreds.SecretAccessKey == "" || awsCreds.SessionToken == "" {
        return "", 0, fmt.Errorf("temporary credentials required (access key, secret key, and session token)")
    }

    // Step 3: Determine session duration.
    duration := options.SessionDuration
    if duration == 0 {
        duration = AWSDefaultSessionDuration
    }
    if duration > AWSMaxSessionDuration {
        duration = AWSMaxSessionDuration
    }

    // Step 4: Create session JSON for federation endpoint.
    sessionJSON := map[string]string{
        "sessionId":    awsCreds.AccessKeyID,
        "sessionKey":   awsCreds.SecretAccessKey,
        "sessionToken": awsCreds.SessionToken,
    }

    sessionData, err := json.Marshal(sessionJSON)
    if err != nil {
        return "", 0, fmt.Errorf("failed to marshal session data: %w", err)
    }

    // Step 5: Request sign-in token from federation endpoint.
    signinToken, err := g.getSigninToken(ctx, sessionData, duration)
    if err != nil {
        return "", 0, fmt.Errorf("failed to get signin token: %w", err)
    }

    // Step 6: Construct console login URL.
    destination := options.Destination
    if destination == "" {
        destination = AWSConsoleDestination
    }

    issuer := options.Issuer
    if issuer == "" {
        issuer = "atmos"
    }

    loginURL := fmt.Sprintf("%s?Action=login&Issuer=%s&Destination=%s&SigninToken=%s",
        AWSFederationEndpoint,
        url.QueryEscape(issuer),
        url.QueryEscape(destination),
        url.QueryEscape(signinToken),
    )

    return loginURL, duration, nil
}

// getSigninToken requests a signin token from the AWS federation endpoint.
func (g *AWSConsoleURLGenerator) getSigninToken(ctx context.Context, sessionData []byte, duration time.Duration) (string, error) {
    // Build federation endpoint URL for getSigninToken action.
    federationURL := fmt.Sprintf("%s?Action=getSigninToken&SessionDuration=%d&Session=%s",
        AWSFederationEndpoint,
        int(duration.Seconds()),
        url.QueryEscape(string(sessionData)),
    )

    // Make HTTP request to federation endpoint.
    response, err := utils.HTTPGet(ctx, federationURL)
    if err != nil {
        return "", fmt.Errorf("failed to call federation endpoint: %w", err)
    }

    // Parse response to extract SigninToken.
    var result struct {
        SigninToken string `json:"SigninToken"`
    }
    if err := json.Unmarshal(response, &result); err != nil {
        return "", fmt.Errorf("failed to parse federation response: %w", err)
    }

    if result.SigninToken == "" {
        return "", fmt.Errorf("empty signin token received from federation endpoint")
    }

    return result.SigninToken, nil
}

// SupportsConsoleAccess returns true for AWS.
func (g *AWSConsoleURLGenerator) SupportsConsoleAccess() bool {
    return true
}
```

### 3. Azure Implementation (Sketch)

```go
// pkg/auth/cloud/azure/console.go

package azure

import (
    "context"
    "fmt"
    "net/url"
    "time"

    "github.com/cloudposse/atmos/pkg/auth/types"
)

const (
    // AzurePortalURL is the Azure portal base URL.
    AzurePortalURL = "https://portal.azure.com"
)

// AzureConsoleURLGenerator generates Azure portal URLs.
type AzureConsoleURLGenerator struct{}

// GetConsoleURL generates an Azure portal sign-in URL.
func (g *AzureConsoleURLGenerator) GetConsoleURL(ctx context.Context, creds types.ICredentials, options types.ConsoleURLOptions) (string, time.Duration, error) {
    // Azure uses OAuth tokens - portal login is handled via browser OAuth flow.
    // For Azure, we can:
    // 1. Use the access token to generate a portal URL with tenant context
    // 2. Or redirect to Azure CLI-style device code flow

    destination := options.Destination
    if destination == "" {
        destination = AzurePortalURL
    }

    // Azure credentials would include tenant ID, subscription ID, etc.
    // The implementation would construct a portal URL with appropriate context.

    return destination, 0, nil
}

// SupportsConsoleAccess returns true for Azure.
func (g *AzureConsoleURLGenerator) SupportsConsoleAccess() bool {
    return true
}
```

### 4. GCP Implementation (Sketch)

```go
// pkg/auth/cloud/gcp/console.go

package gcp

import (
    "context"
    "fmt"
    "time"

    "github.com/cloudposse/atmos/pkg/auth/types"
)

const (
    // GCPConsoleURL is the GCP console base URL.
    GCPConsoleURL = "https://console.cloud.google.com"
)

// GCPConsoleURLGenerator generates GCP console URLs.
type GCPConsoleURLGenerator struct{}

// GetConsoleURL generates a GCP console sign-in URL.
func (g *GCPConsoleURLGenerator) GetConsoleURL(ctx context.Context, creds types.ICredentials, options types.ConsoleURLOptions) (string, time.Duration, error) {
    // GCP uses OAuth tokens - console login is handled via browser OAuth flow.
    // Similar to Azure, GCP would use OAuth-based authentication.

    destination := options.Destination
    if destination == "" {
        destination = GCPConsoleURL
    }

    return destination, 0, nil
}

// SupportsConsoleAccess returns true for GCP.
func (g *GCPConsoleURLGenerator) SupportsConsoleAccess() bool {
    return true
}
```

### 5. CLI Command

Add a new `atmos auth console` command:

```go
// cmd/auth_console.go

package cmd

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "runtime"
    "time"

    "github.com/spf13/cobra"

    "github.com/cloudposse/atmos/pkg/auth"
    "github.com/cloudposse/atmos/pkg/auth/credentials"
    "github.com/cloudposse/atmos/pkg/auth/types"
    "github.com/cloudposse/atmos/pkg/auth/validation"
    cfg "github.com/cloudposse/atmos/pkg/config"
    "github.com/cloudposse/atmos/pkg/schema"
    u "github.com/cloudposse/atmos/pkg/utils"
)

var (
    consoleDestination   string
    consoleDuration      time.Duration
    consoleIssuer        string
    consolePrintOnly     bool
    consoleSkipOpen      bool
)

// authConsoleCmd opens the cloud provider web console using authenticated credentials.
var authConsoleCmd = &cobra.Command{
    Use:   "console",
    Short: "Open cloud provider web console in browser",
    Long: `Open the cloud provider web console in your default browser using authenticated credentials.

This command generates a temporary console sign-in URL using your authenticated identity's
credentials and opens it in your default browser. Supports AWS, Azure, GCP, and other providers
that implement console access.`,
    Example: `
# Open AWS console with default identity
atmos auth console

# Open console with specific identity
atmos auth console --identity prod-admin

# Open console to specific AWS service
atmos auth console --destination https://console.aws.amazon.com/s3

# Print URL without opening browser
atmos auth console --print-only

# Specify session duration (AWS: max 12 hours)
atmos auth console --duration 2h
`,
    RunE:               executeAuthConsoleCommand,
    FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func executeAuthConsoleCommand(cmd *cobra.Command, args []string) error {
    handleHelpRequest(cmd, args)

    // Load atmos config.
    atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
    if err != nil {
        return fmt.Errorf("failed to load atmos config: %w", err)
    }

    // Create auth manager.
    authManager, err := createAuthManager(&atmosConfig.Auth)
    if err != nil {
        return fmt.Errorf("failed to create auth manager: %w", err)
    }

    // Get identity from flag or use default.
    identityName, _ := cmd.Flags().GetString("identity")

    // Authenticate to get credentials.
    ctx := context.Background()
    whoami, err := authManager.Authenticate(ctx, identityName)
    if err != nil {
        return fmt.Errorf("authentication failed: %w", err)
    }

    // Retrieve credentials from store.
    credStore := credentials.NewCredentialStore()
    creds, err := credStore.Retrieve(whoami.Identity)
    if err != nil {
        return fmt.Errorf("failed to retrieve credentials: %w", err)
    }

    // Check if provider supports console access.
    consoleProvider, supportsConsole := checkConsoleSupport(authManager, whoami.Identity)
    if !supportsConsole {
        return fmt.Errorf("provider %q does not support web console access", whoami.Provider)
    }

    // Generate console URL.
    options := types.ConsoleURLOptions{
        Destination:     consoleDestination,
        SessionDuration: consoleDuration,
        Issuer:          consoleIssuer,
        OpenInBrowser:   !consoleSkipOpen && !consolePrintOnly,
    }

    consoleURL, duration, err := consoleProvider.GetConsoleURL(ctx, creds, options)
    if err != nil {
        return fmt.Errorf("failed to generate console URL: %w", err)
    }

    // Print URL and session info.
    u.PrintfMessageToTUI("**Console URL generated**\n")
    u.PrintfMessageToTUI("Provider: %s\n", whoami.Provider)
    u.PrintfMessageToTUI("Identity: %s\n", whoami.Identity)
    if duration > 0 {
        u.PrintfMessageToTUI("Session Duration: %s\n", duration)
    }
    u.PrintfMessageToTUI("\n")

    if consolePrintOnly {
        fmt.Println(consoleURL)
        return nil
    }

    u.PrintfMessageToTUI("Console URL:\n%s\n\n", consoleURL)

    // Open in browser unless skipped.
    if !consoleSkipOpen {
        u.PrintfMessageToTUI("Opening console in browser...\n")
        if err := openBrowser(consoleURL); err != nil {
            u.PrintfMessageToTUI("**Warning:** Failed to open browser automatically: %v\n", err)
            u.PrintfMessageToTUI("Please open the URL above manually.\n")
        }
    }

    return nil
}

// checkConsoleSupport checks if the provider supports console access.
func checkConsoleSupport(authManager auth.AuthManager, identityName string) (types.ConsoleAccessProvider, bool) {
    // Get provider kind for the identity.
    providerKind, err := authManager.GetProviderKindForIdentity(identityName)
    if err != nil {
        return nil, false
    }

    // Check if provider supports console access based on kind.
    // This could be done via a registry or factory pattern.
    switch providerKind {
    case "aws/iam-identity-center", "aws/saml":
        // Return AWS console URL generator.
        generator := &aws.AWSConsoleURLGenerator{}
        return generator, true
    case "azure/oidc":
        // Return Azure console URL generator.
        generator := &azure.AzureConsoleURLGenerator{}
        return generator, true
    case "gcp/oidc":
        // Return GCP console URL generator.
        generator := &gcp.GCPConsoleURLGenerator{}
        return generator, true
    default:
        return nil, false
    }
}

// openBrowser opens the specified URL in the system's default browser.
func openBrowser(url string) error {
    var cmd *exec.Cmd

    switch runtime.GOOS {
    case "darwin":
        cmd = exec.Command("open", url)
    case "windows":
        cmd = exec.Command("cmd", "/c", "start", url)
    default: // linux, freebsd, openbsd, etc.
        cmd = exec.Command("xdg-open", url)
    }

    return cmd.Start()
}

func init() {
    authConsoleCmd.Flags().StringVar(&consoleDestination, "destination", "",
        "Specific console page to navigate to (e.g., https://console.aws.amazon.com/s3)")
    authConsoleCmd.Flags().DurationVar(&consoleDuration, "duration", 1*time.Hour,
        "Console session duration (provider may have max limits)")
    authConsoleCmd.Flags().StringVar(&consoleIssuer, "issuer", "atmos",
        "Issuer identifier for the console session (AWS only)")
    authConsoleCmd.Flags().BoolVar(&consolePrintOnly, "print-only", false,
        "Print the console URL without opening browser")
    authConsoleCmd.Flags().BoolVar(&consoleSkipOpen, "no-open", false,
        "Generate URL but don't open browser automatically")

    authCmd.AddCommand(authConsoleCmd)
}
```

### 6. Utility Functions

Add HTTP utilities if not already present:

```go
// pkg/utils/http_utils.go

package utils

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "time"
)

// HTTPGet performs an HTTP GET request with context.
func HTTPGet(ctx context.Context, url string) ([]byte, error) {
    client := &http.Client{
        Timeout: 10 * time.Second,
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
    }

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read response: %w", err)
    }

    return body, nil
}
```

## Usage Examples

### Basic Usage

```bash
# Open console with default identity
atmos auth console

# Open console with specific identity
atmos auth console --identity prod-admin

# Open console to specific AWS service
atmos auth console --destination https://console.aws.amazon.com/s3

# Open Azure portal to specific blade
atmos auth console --identity azure-prod --destination "https://portal.azure.com/#blade/HubsExtension/BrowseResourceGroups"
```

### Advanced Usage

```bash
# Print URL without opening browser (useful for scripts)
atmos auth console --print-only

# Generate URL but don't auto-open
atmos auth console --no-open

# Specify custom session duration (AWS max: 12h)
atmos auth console --duration 2h

# Custom issuer name (shows in AWS console URL)
atmos auth console --issuer my-org
```

### Integration with Shell

```bash
# Copy URL to clipboard
atmos auth console --print-only | pbcopy  # macOS
atmos auth console --print-only | xclip   # Linux

# Open in specific browser
BROWSER=firefox atmos auth console

# Use in scripts
CONSOLE_URL=$(atmos auth console --print-only)
echo "Console: $CONSOLE_URL"
```

## Implementation Plan

### Phase 1: Core Infrastructure
1. Add `ConsoleAccessProvider` interface to `pkg/auth/types/interfaces.go`
2. Add `ConsoleURLOptions` struct to `pkg/auth/types/interfaces.go`
3. Add HTTP utilities to `pkg/utils/http_utils.go` (if needed)
4. Add browser opening utilities (cross-platform)

### Phase 2: AWS Implementation
1. Implement `AWSConsoleURLGenerator` in `pkg/auth/cloud/aws/console.go`
2. Add AWS federation endpoint integration
3. Add comprehensive tests for AWS console URL generation
4. Document AWS-specific options and limitations

### Phase 3: CLI Command
1. Create `cmd/auth_console.go` with cobra command
2. Add flags for destination, duration, issuer, etc.
3. Implement provider detection and console URL generation
4. Add browser opening functionality
5. Create comprehensive tests

### Phase 4: Azure & GCP (Future)
1. Implement Azure portal URL generation (when Azure auth is added)
2. Implement GCP console URL generation (when GCP auth is added)
3. Add provider-specific tests

### Phase 5: Documentation
1. Create Docusaurus documentation at `website/docs/cli/commands/auth/console.mdx`
2. Add examples to blog post or changelog
3. Update main auth documentation to mention console access

## Testing Strategy

### Unit Tests

```go
// pkg/auth/cloud/aws/console_test.go

func TestAWSConsoleURLGenerator_GetConsoleURL(t *testing.T) {
    tests := []struct {
        name        string
        creds       *types.AWSCredentials
        options     types.ConsoleURLOptions
        expectError bool
    }{
        {
            name: "basic URL generation",
            creds: &types.AWSCredentials{
                AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
                SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
                SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
            },
            options: types.ConsoleURLOptions{
                SessionDuration: 1 * time.Hour,
            },
            expectError: false,
        },
        {
            name: "missing session token",
            creds: &types.AWSCredentials{
                AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
                SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
            },
            options:     types.ConsoleURLOptions{},
            expectError: true,
        },
        {
            name: "custom destination",
            creds: &types.AWSCredentials{
                AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
                SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
                SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
            },
            options: types.ConsoleURLOptions{
                Destination:     "https://console.aws.amazon.com/s3",
                SessionDuration: 2 * time.Hour,
            },
            expectError: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            generator := &AWSConsoleURLGenerator{}
            ctx := context.Background()

            url, duration, err := generator.GetConsoleURL(ctx, tt.creds, tt.options)

            if tt.expectError {
                require.Error(t, err)
                return
            }

            require.NoError(t, err)
            require.NotEmpty(t, url)
            require.Contains(t, url, "signin.aws.amazon.com/federation")
            require.Contains(t, url, "Action=login")

            if tt.options.Destination != "" {
                require.Contains(t, url, tt.options.Destination)
            }

            if tt.options.SessionDuration > 0 {
                require.Equal(t, tt.options.SessionDuration, duration)
            }
        })
    }
}
```

### Integration Tests

```go
// cmd/auth_console_test.go

func TestAuthConsoleCommand(t *testing.T) {
    // Skip if no AWS credentials available.
    if os.Getenv("ATMOS_TEST_SKIP_AWS") == "true" {
        t.Skip("Skipping AWS integration test")
    }

    // Test with mock credentials.
    // Test print-only flag.
    // Test error handling.
}
```

## Security Considerations

1. **URL Security**: Console URLs contain signin tokens that are valid for 15 minutes
   - Never log complete URLs
   - Clear URLs from memory after use
   - Warn users not to share URLs

2. **Credential Validation**: Only generate console URLs for valid, non-expired credentials

3. **HTTPS Only**: All federation endpoints must use HTTPS

4. **Token Expiration**: Enforce maximum session durations per provider limits

5. **Browser Security**: Opening URLs in browser may expose tokens in browser history
   - Consider adding warning for sensitive environments
   - Document best practices for shared systems

## Documentation Requirements

### CLI Help Text

```
USAGE
  atmos auth console [flags]

DESCRIPTION
  Open the cloud provider web console in your default browser using
  authenticated credentials. This command generates a temporary console
  sign-in URL using your authenticated identity's credentials.

FLAGS
  -i, --identity string       Identity to use for console access
      --destination string    Specific console page to navigate to
      --duration duration     Console session duration (default 1h)
      --issuer string         Issuer identifier (AWS only, default "atmos")
      --print-only            Print URL without opening browser
      --no-open               Generate URL but don't open browser

EXAMPLES
  # Open console with default identity
  atmos auth console

  # Open AWS S3 console
  atmos auth console --destination https://console.aws.amazon.com/s3

  # Print URL only
  atmos auth console --print-only

  # Longer session (AWS max: 12h)
  atmos auth console --duration 2h
```

### Website Documentation

Create `website/docs/cli/commands/auth/console.mdx` with:
- Purpose and use cases
- Provider support matrix
- Examples for each supported provider
- Security best practices
- Troubleshooting guide

## Future Enhancements

1. **Custom Browser Selection**: Support `BROWSER` environment variable
2. **Console Profiles**: Save favorite console destinations
3. **Session Management**: Track open console sessions
4. **MFA Support**: Handle MFA prompts during console URL generation
5. **Multi-Account**: Quick switching between different accounts/identities
6. **URL Shortening**: Optional URL shortening for easier sharing (with warnings)
7. **QR Codes**: Generate QR codes for mobile console access

## Alternatives Considered

### Alternative 1: Embedding Web Server
Instead of generating URLs, embed a local web server that handles OAuth flows.

**Pros**: More secure, no URL exposure
**Cons**: More complex, requires managing local server lifecycle

**Decision**: Start with URL generation (simpler), consider web server for future

### Alternative 2: Provider-Specific Commands
Create separate commands like `atmos auth console-aws`, `atmos auth console-azure`

**Pros**: More explicit, provider-specific options
**Cons**: Not scalable, violates DRY principle

**Decision**: Use unified command with provider detection

### Alternative 3: Extend Existing Commands
Add `--console` flag to `atmos auth login`

**Pros**: Fewer commands
**Cons**: Mixes concerns, less discoverable

**Decision**: Separate command is clearer and more flexible

## Success Criteria

1. ✅ Users can open AWS console with a single command
2. ✅ Provider-agnostic interface supports multiple clouds
3. ✅ URLs are generated securely and expire appropriately
4. ✅ Works across macOS, Linux, and Windows
5. ✅ Comprehensive documentation and examples
6. ✅ 80%+ test coverage for new code (achieved 85.9%)
7. ✅ Zero breaking changes to existing auth functionality

## Implementation Details

### Files Created

**Core Implementation:**
- `cmd/auth_console.go` - CLI command implementation
- `cmd/markdown/atmos_auth_console_usage.md` - Usage examples
- `pkg/auth/types/constants.go` - Provider kind constants
- `pkg/auth/cloud/aws/console.go` - AWS console URL generator
- `pkg/auth/cloud/aws/destinations.go` - AWS service alias mappings (100+ services)
- `pkg/http/client.go` - HTTP client interface and implementation
- `pkg/http/mock_client.go` - Generated mock for testing

**Tests:**
- `pkg/auth/cloud/aws/console_test.go` - Console URL generation tests (15 test cases)
- `pkg/auth/cloud/aws/destinations_test.go` - Destination alias tests

**Documentation:**
- `website/docs/cli/commands/auth/console.mdx` - CLI reference
- `website/blog/2025-10-20-auth-console-web-access.md` - Feature announcement
- `docs/prd/auth-console-command.md` - This PRD

### Actual Features Implemented

**Command Flags:**
- `--destination` - Console page to navigate to (supports aliases and full URLs)
- `--duration` - Session duration (default: 1h, max: 12h for AWS)
- `--issuer` - Custom issuer identifier (default: "atmos")
- `--print-only` - Print URL to stdout without opening browser
- `--no-open` - Generate URL but don't open browser
- `--identity` / `-i` - Identity to use (inherited from auth command group)

**Shell Autocomplete:**
- Destination flag: Autocompletes 117 AWS service aliases
- Identity flag: Autocompletes configured identities from atmos.yaml

**Output Formatting:**
- Uses charmbracelet/lipgloss for styled terminal output
- Atmos theme colors (cyan headers, gray labels, white values)
- Session expiration timestamp in addition to duration
- URL hidden by default (only shown on error or with `--no-open`)
- Success indicator: `✓ Opened console in browser`
- Warning indicator for browser open failures

**AWS Service Aliases:**
- 100+ supported services including: s3, ec2, lambda, dynamodb, rds, vpc, iam, etc.
- Case-insensitive alias matching
- Categories: Storage, Compute, Database, Networking, Security, Analytics, ML, etc.

**Provider Support:**
- ✅ AWS (IAM Identity Center and SAML providers)
- 🚧 Azure (planned, returns clear error message)
- 🚧 GCP (planned, returns clear error message)

### Architecture Decisions

1. **Provider Constants**: Created `pkg/auth/types/constants.go` to avoid magic strings
2. **HTTP Package**: Moved HTTP utilities from `pkg/utils` to dedicated `pkg/http` package
3. **Browser Opening**: Consolidated to existing `OpenUrl()` function in `pkg/utils/url_utils.go`
4. **Interface Pattern**: Optional `ConsoleAccessProvider` interface - providers implement if supported
5. **Destination Resolution**: Fail-fast validation before HTTP calls to avoid unnecessary requests
6. **Test Isolation**: Mocked HTTP client using mockgen for testing without network calls

### Test Coverage

- **Overall**: 85.9% statement coverage
- **Console Tests**: 15 test cases covering success, error, and edge cases
- **Destination Tests**: All 100+ aliases tested for correctness
- **Mocking**: Complete HTTP request/response mocking for isolated testing

## References

- [AWS Vault Login Documentation](https://github.com/99designs/aws-vault/blob/master/USAGE.md)
- [AWS Federation Console Access](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_enable-console-custom-url.html)
- [AWS GetFederationToken API](https://docs.aws.amazon.com/STS/latest/APIReference/API_GetFederationToken.html)
- [Azure Portal URL Structure](https://portal.azure.com)
- [GCP Console Authentication](https://console.cloud.google.com)
- [Charmbracelet Lipgloss](https://github.com/charmbracelet/lipgloss)
