# Auth Context and Multi-Identity Support PRD

## Executive Summary

This document defines the architecture for passing authentication credentials to in-process SDK calls and supporting multiple concurrent cloud provider identities. It addresses a critical bug where `!terraform.state` YAML functions fail to use Atmos-managed authentication credentials, and establishes a pattern for supporting multiple cloud provider identities simultaneously (e.g., AWS + GitHub + Azure).

**Key Design Decision:** AuthContext is a **runtime execution context** (like React/Next.js context providers) that is created by commands and passed TO the auth system to be populated, then passed throughout the execution chain. It is NOT owned by ConfigAndStacksInfo or any single component.

## Problem Statement

### Background

Atmos introduced an authentication system (`atmos auth`) that manages credentials for various cloud providers and identity types. The system writes credentials to managed files and sets environment variables to make those credentials available to infrastructure tools.

### Current Challenges

1. **`!terraform.state` ignores auth credentials**: The `!terraform.state` YAML function makes in-process AWS SDK calls to read Terraform state from S3, but these calls do not use Atmos-managed credentials.

2. **Environment variables only work for spawned processes**: Auth credentials are stored in `ComponentEnvSection` and `ComponentEnvList`, which are only passed to spawned processes (terraform, helmfile, packer) via `exec.Command()`.

3. **In-process SDK calls use ambient credentials**: When Go code calls AWS SDK directly (not spawning terraform), the SDK reads from the process environment, which contains the user's original `AWS_PROFILE` and `~/.aws/` files, not Atmos-managed credentials.

4. **No support for multiple concurrent identities**: Current architecture only supports one active identity at a time, but users need to use credentials from multiple providers simultaneously (e.g., AWS for infrastructure + GitHub for vendoring).

5. **Workaround doesn't work**: Setting `AWS_PROFILE=` (empty) doesn't work because `AWS_SHARED_CREDENTIALS_FILE` and `AWS_CONFIG_FILE` still point to the system's `~/.aws/` files, not Atmos-managed files.

### User Impact

**Reported Issue:**
```bash
# User has authenticated with Atmos auth
$ atmos auth login my-sso-identity

# User tries to use component with !terraform.state function
$ atmos terraform plan my-component -s dev

# Error: Unable to read state file - permission denied
# Cause: AWS SDK is using ambient AWS_PROFILE from shell, not Atmos auth
```

**Root Cause:**
```
User runs: atmos terraform plan
  ↓
Auth system sets ComponentEnvSection:
  - AWS_SHARED_CREDENTIALS_FILE=/home/user/.atmos/auth/aws-sso/credentials
  - AWS_CONFIG_FILE=/home/user/.atmos/auth/aws-sso/config
  - AWS_PROFILE=my-sso-identity
  ↓
YAML function !terraform.state is evaluated
  ↓
GetTerraformState() → ReadTerraformBackendS3() → LoadAWSConfig()
  ↓
AWS SDK calls config.LoadDefaultConfig(ctx)
  ↓
SDK reads from PROCESS environment (not ComponentEnvSection):
  - AWS_PROFILE=my-default-profile (from user's shell)
  - AWS_SHARED_CREDENTIALS_FILE=~/.aws/credentials (default)
  - AWS_CONFIG_FILE=~/.aws/config (default)
  ↓
SDK uses wrong credentials → Access Denied
```

## Design Goals

1. **Fix `!terraform.state` auth bug**: Make in-process SDK calls use Atmos-managed credentials
2. **Support multiple concurrent identities**: Enable AWS + GitHub + Azure credentials simultaneously via multiple `--identity` flags
3. **AuthContext as runtime provider context**:
   - **Lifetime**: Single `atmos` command execution (not persisted)
   - **Ownership**: Created by commands, passed to auth system to populate, then passed throughout execution
   - **Scope**: Contains ALL active identities for the current command (AWS + GitHub + Azure simultaneously)
4. **Single source of truth**: `AuthContext` is the authoritative source for all auth credentials
   - `ComponentEnvSection`/`ComponentEnvList` are **derived from** `AuthContext`
   - No duplication or synchronization issues
5. **Separation of concerns**:
   - **AuthManager**: Handles persistence (keychain), authentication (SSO/SAML), credential caching
   - **AuthContext**: Runtime container for active credentials during command execution
   - **Commands**: Create AuthContext, pass to AuthManager to populate, pass to functions that need auth
6. **Minimal code changes**: Thread auth context through existing call chains with minimal disruption
7. **Backward compatibility**: Auth context is optional; existing code continues to work
8. **Clear flow of auth data**:
   - Commands create `AuthContext{}`
   - Auth system populates `authContext.AWS`, `authContext.GitHub`, etc.
   - Commands pass `authContext` to all functions needing credentials
   - Functions use credentials directly from `authContext`
9. **Extensible architecture**: Easy to add new cloud providers (Azure, GCP, etc.)

## Technical Specification

### Key Design Principle: AuthContext as Runtime Provider

**AuthContext is a runtime execution context (similar to React/Next.js context providers) that flows through the entire command execution.**

```
┌─────────────────────────────────────────────────────────────────┐
│ Command Entry Point (terraform, describe, workflow, etc.)      │
│  1. Creates: authContext := &schema.AuthContext{}              │
│  2. Authenticates: authManager.Authenticate(ctx, id, authContext)│
│  3. Passes: ProcessYAML(..., authContext), RunTerraform(..., authContext)│
└─────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────┐
│ AuthContext (Runtime Container - NOT Persisted)            │
│  ├─ AWS: {CredentialsFile, ConfigFile, Profile, Region}    │
│  ├─ GitHub: {Token, APIEndpoint} (future)                  │
│  └─ Azure: {TenantID, SubscriptionID, Token} (future)      │
└─────────────────────────────────────────────────────────────┘
                           │
                           │ Passed explicitly to:
                           │
           ┌───────────────┴────────────────┐
           │                                │
           ▼                                ▼
┌──────────────────────┐       ┌─────────────────────────┐
│ ComponentEnvSection  │       │ In-Process SDK Calls    │
│ (Derived for spawned │       │ (Direct access)         │
│  processes)          │       │                         │
│                      │       │ LoadAWSConfigWithAuth(  │
│ - AWS_PROFILE=...    │       │   authContext.AWS       │
│ - AWS_CONFIG_FILE=...│       │ )                       │
└──────────────────────┘       │ VendorPull(             │
                               │   authContext.GitHub    │
                               │ )                       │
                               └─────────────────────────┘
```

**Benefits:**
- No duplication of credential paths
- Impossible to have mismatched credentials between env vars and SDK calls
- Clear ownership: Commands create, AuthManager populates, functions consume
- Testable: Pass mock AuthContext in tests without globals
- Explicit: Every function signature shows if it needs auth

### Architecture Overview

**Current Flow (Broken):**
```
Auth Login
  ↓
PostAuthenticate() sets ComponentEnvSection
  ↓
ComponentEnvList passed to exec.Command() for spawned processes
  ↓
In-process SDK calls → Use ambient process environment ❌
```

**New Flow (Fixed with Runtime Provider Pattern):**
```
Command Entry (atmos terraform plan --identity aws:dev --identity github:api)
  ↓
Create runtime context: authContext := &schema.AuthContext{}
  ↓
For each --identity flag:
  authManager.Authenticate(ctx, "aws:dev", authContext)
    → Populates authContext.AWS
  authManager.Authenticate(ctx, "github:api", authContext)
    → Populates authContext.GitHub
  ↓
AuthContext now contains: {AWS: {...}, GitHub: {...}}
  ↓
Pass authContext to execution chain:
  - ConfigAndStacksInfo.AuthContext = authContext (reference for later use)
  - ProcessCustomYamlTags(..., authContext) → YAML functions have auth
  - GetTerraformState(..., authContext) → !terraform.state has auth
  - VendorPull(..., authContext) → GitHub vendoring has auth
  ↓
In-process SDK calls use authContext directly:
  - LoadAWSConfigWithAuth(authContext.AWS) → S3 state access ✅
  - GitHubClient.New(authContext.GitHub) → API calls ✅
  ↓
Spawned processes get derived env vars:
  - SetEnvironmentVariables(authContext) → ComponentEnvSection
  - exec.Command() receives ComponentEnvList
```

### Schema Changes

#### Add AuthContext Types

**File:** `pkg/schema/schema.go`

```go
// AuthContext is a runtime execution context that holds active authentication credentials
// for multiple cloud providers simultaneously during a single Atmos command execution.
//
// Design Pattern: Similar to React/Next.js context providers, AuthContext is:
// - Created by command entry points (terraform, describe, workflow, auth, etc.)
// - Passed TO AuthManager.Authenticate() to be populated
// - Passed explicitly through the execution chain to all functions needing auth
// - NOT persisted (lifetime = single command execution)
// - NOT owned by ConfigAndStacksInfo (just referenced)
//
// Multi-Identity Support: A single AuthContext can hold credentials for multiple providers:
//   authContext.AWS     → AWS credentials (if --identity aws:... was used)
//   authContext.GitHub  → GitHub credentials (if --identity github:... was used)
//   authContext.Azure   → Azure credentials (if --identity azure:... was used)
//
// Usage Example:
//   authContext := &schema.AuthContext{}
//   authManager.Authenticate(ctx, "aws:dev-admin", authContext)
//   authManager.Authenticate(ctx, "github:api-token", authContext)
//   // Now authContext contains both AWS and GitHub credentials
//   ProcessCustomYamlTags(..., authContext)  // YAML functions can use both
//   VendorPull(..., authContext.GitHub)      // GitHub vendoring uses GitHub creds
//   GetTerraformState(..., authContext.AWS)  // Terraform state uses AWS creds
type AuthContext struct {
	// AWS holds AWS credentials if an AWS identity is active.
	AWS *AWSAuthContext `json:"aws,omitempty" yaml:"aws,omitempty"`

	// GitHub holds GitHub credentials if a GitHub identity is active (future).
	// GitHub *GitHubAuthContext `json:"github,omitempty" yaml:"github,omitempty"`

	// Azure holds Azure credentials if an Azure identity is active (future).
	// Azure *AzureAuthContext `json:"azure,omitempty" yaml:"azure,omitempty"`

	// GCP holds GCP credentials if a GCP identity is active (future).
	// GCP *GCPAuthContext `json:"gcp,omitempty" yaml:"gcp,omitempty"`
}

// AWSAuthContext holds AWS-specific authentication context.
// This is populated by the AWS auth system and consumed by AWS SDK calls.
type AWSAuthContext struct {
	// CredentialsFile is the absolute path to the AWS credentials file managed by Atmos.
	// Example: /home/user/.atmos/auth/aws-sso/credentials
	CredentialsFile string `json:"credentials_file" yaml:"credentials_file"`

	// ConfigFile is the absolute path to the AWS config file managed by Atmos.
	// Example: /home/user/.atmos/auth/aws-sso/config
	ConfigFile string `json:"config_file" yaml:"config_file"`

	// Profile is the AWS profile name to use from the credentials file.
	// This corresponds to the identity name in Atmos auth config.
	Profile string `json:"profile" yaml:"profile"`

	// Region is the AWS region (optional, may be empty if not specified in identity).
	Region string `json:"region,omitempty" yaml:"region,omitempty"`
}

// Future: Add Azure, GCP, GitHub auth contexts following same pattern
// type AzureAuthContext struct { ... }
// type GCPAuthContext struct { ... }
// type GitHubAuthContext struct { ... }
```

#### Update ConfigAndStacksInfo

**File:** `pkg/schema/schema.go`

```go
type ConfigAndStacksInfo struct {
	// ... existing fields ...
	ComponentEnvSection           AtmosSectionMapType
	ComponentAuthSection          AtmosSectionMapType
	ComponentEnvList              []string

	// AuthContext is a REFERENCE to the runtime authentication context.
	// The actual AuthContext is created by commands and passed throughout execution.
	// ConfigAndStacksInfo holds a reference for convenience (e.g., in YAML processing).
	//
	// Ownership: Commands create AuthContext, ConfigAndStacksInfo just references it.
	// Lifetime: Single command execution (not persisted).
	//
	// It enables multiple cloud provider identities to be active simultaneously
	// (e.g., AWS + GitHub credentials in the same component).
	AuthContext *AuthContext

	// ... remaining fields ...
}
```

### Auth System Changes

#### Update AuthManager.Authenticate() Signature

**File:** `pkg/auth/manager.go`

The core refactoring changes `AuthManager.Authenticate()` to accept AuthContext as a parameter (to be populated) rather than creating it internally.

**OLD (Current Implementation):**
```go
// AuthManager creates and manages stackInfo internally
func NewAuthManager(config, credStore, validator, stackInfo) AuthManager
func (m *manager) Authenticate(ctx context.Context, identityName string) (*types.WhoAmI, error)
  // Uses m.stackInfo internally
  // PostAuthenticate(m.stackInfo, ...) populates m.stackInfo.AuthContext
```

**NEW (Refactored to Runtime Provider Pattern):**
```go
// AuthManager doesn't own stackInfo or authContext - they're passed IN
func NewAuthManager(config, credStore, validator) AuthManager  // NO stackInfo parameter
func (m *manager) Authenticate(ctx context.Context, identityName string, authContext *schema.AuthContext) (*types.WhoAmI, error)
  // Receives authContext as parameter
  // PostAuthenticate(authContext, ...) populates the passed-in authContext
```

**Rationale:**
- AuthManager focuses on authentication logic (SSO, SAML, keychain, caching)
- AuthContext is owned by commands, not by AuthManager
- Commands can call `Authenticate()` multiple times with same authContext to populate multiple identities
- Example: `authManager.Authenticate(ctx, "aws:dev", authContext)` then `authManager.Authenticate(ctx, "github:api", authContext)`

#### Update Identity.PostAuthenticate() Signature

**Files:**
- `pkg/auth/identities/aws/assume_role.go`
- `pkg/auth/identities/aws/permission_set.go`
- `pkg/auth/identities/aws/user.go`
- `pkg/auth/types/interfaces.go`

**OLD:**
```go
PostAuthenticate(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds types.ICredentials) error
```

**NEW:**
```go
PostAuthenticate(ctx context.Context, authContext *schema.AuthContext, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds types.ICredentials) error
```

**Rationale:**
PostAuthenticate needs BOTH parameters because they serve different purposes:

1. **`authContext` (WRITE)**: Populate runtime authentication credentials
   - Sets `authContext.AWS`, `authContext.GitHub`, etc.
   - Used by in-process SDK calls (e.g., `!terraform.state` reading S3)
   - Runtime only - not persisted

2. **`stackInfo` (READ + WRITE)**: Access merged stack configuration
   - **READ from `stackInfo.ComponentAuthSection`**: Stack-level auth overrides merged via inheritance
     - Example: Component may override identity's AWS region, session duration, env vars
     - Example: Component may override provider configuration (SSO endpoint, role session name)
   - **WRITE to `stackInfo.ComponentEnvSection`**: Populate env vars for spawned processes
     - Derives env vars from `authContext` (single source of truth)
     - Adds stack-specific env vars from merged auth config
   - **READ from `stackInfo.ComponentSection`**: Component-specific settings that affect auth

**Why Both Parameters Are Required:**

Stacks can override identity configuration via deep merge inheritance:

```yaml
# atmos.yaml (global)
auth:
  identities:
    dev-admin:
      kind: aws/permission-set
      principal: { account: { name: dev } }
      env:
        - key: ENV_TYPE
          value: dev

# stacks/prod/vpc.yaml (component override)
components:
  terraform:
    vpc:
      auth:
        identities:
          dev-admin:  # Same name - gets merged
            principal: { account: { name: prod } }  # Override to prod account
            env:
              - key: ENV_TYPE
                value: prod  # Override env var
              - key: AUDIT_REQUIRED
                value: "true"  # Add new env var
```

**Merged configuration for `vpc` component:**
- `authContext`: Gets runtime credentials for prod account (from merged principal)
- `stackInfo.ComponentEnvSection`: Gets `ENV_TYPE=prod` and `AUDIT_REQUIRED=true` (from merged env)

This separation allows:
- **Runtime credentials** (authContext) to be used for in-process SDK calls
- **Stack-specific overrides** (stackInfo) to customize auth behavior per component
- **Environment variables** to include both auth context and stack overrides

#### Add SetAuthContext Function

**File:** `pkg/auth/cloud/aws/setup.go`

```go
// SetAuthContext populates AWS auth context with Atmos-managed credential paths.
// This enables in-process AWS SDK calls to use Atmos-managed credentials.
//
// Parameters:
//   - authContext: Runtime auth context to populate (passed by caller)
//   - stackInfo: Stack configuration (may contain component-level auth overrides)
//   - providerName: Auth provider name (e.g., "aws-sso")
//   - identityName: Identity name (e.g., "dev-admin")
//   - creds: Authenticated credentials (may contain region info)
func SetAuthContext(authContext *schema.AuthContext, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds types.ICredentials) error {
	if authContext == nil {
		return nil // No auth context to populate
	}

	m, err := NewAWSFileManager()
	if err != nil {
		return errors.Join(errUtils.ErrAuthAwsFileManagerFailed, err)
	}

	credentialsPath := m.GetCredentialsPath(providerName)
	configPath := m.GetConfigPath(providerName)

	// Extract region from credentials if available.
	var region string
	if awsCreds, ok := creds.(*AWSCredentials); ok && awsCreds != nil {
		region = awsCreds.Region
	}

	// Check for component-level region override from merged auth config.
	// Stack inheritance allows components to override identity configuration.
	if stackInfo != nil && stackInfo.ComponentAuthSection != nil {
		if identities, ok := stackInfo.ComponentAuthSection["identities"].(map[string]any); ok {
			if identityCfg, ok := identities[identityName].(map[string]any); ok {
				if regionOverride, ok := identityCfg["region"].(string); ok && regionOverride != "" {
					region = regionOverride
					log.Debug("Using component-level region override", "region", region)
				}
			}
		}
	}

	// Populate AWS auth context.
	authContext.AWS = &schema.AWSAuthContext{
		CredentialsFile: credentialsPath,
		ConfigFile:      configPath,
		Profile:         identityName,
		Region:          region,
	}

	log.Debug("Set AWS auth context",
		"profile", identityName,
		"credentials", credentialsPath,
		"config", configPath,
		"region", region,
	)

	return nil
}
```

#### Update SetEnvironmentVariables to Use AuthContext

**File:** `pkg/auth/cloud/aws/setup.go`

```go
// SetEnvironmentVariables sets AWS environment variables from AuthContext.
// This derives environment variables from the auth context (single source of truth).
// The env vars are used by spawned processes (terraform, helmfile, packer).
func SetEnvironmentVariables(stackInfo *schema.ConfigAndStacksInfo) error {
	if stackInfo == nil || stackInfo.AuthContext == nil || stackInfo.AuthContext.AWS == nil {
		return nil // No auth context to derive from
	}

	awsAuth := stackInfo.AuthContext.AWS

	// Derive environment variables from auth context.
	utils.SetEnvironmentVariable(stackInfo, "AWS_SHARED_CREDENTIALS_FILE", awsAuth.CredentialsFile)
	utils.SetEnvironmentVariable(stackInfo, "AWS_CONFIG_FILE", awsAuth.ConfigFile)
	utils.SetEnvironmentVariable(stackInfo, "AWS_PROFILE", awsAuth.Profile)

	if awsAuth.Region != "" {
		utils.SetEnvironmentVariable(stackInfo, "AWS_REGION", awsAuth.Region)
	}

	return nil
}
```

#### Update Identity PostAuthenticate Methods

**Files:**
- `pkg/auth/identities/aws/assume_role.go`
- `pkg/auth/identities/aws/permission_set.go`
- `pkg/auth/identities/aws/user.go`

```go
func (i *AssumeRoleIdentity) PostAuthenticate(
	ctx context.Context,
	stackInfo *schema.ConfigAndStacksInfo,
	providerName, identityName string,
	creds types.ICredentials,
) error {
	// ... existing credential setup ...

	if err := awsCloud.SetupFiles(providerName, identityName, creds); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	// NEW: Set auth context (single source of truth).
	if err := awsCloud.SetAuthContext(stackInfo, providerName, identityName); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	// NEW: Derive environment variables from auth context.
	// This populates ComponentEnvSection from the auth context.
	if err := awsCloud.SetEnvironmentVariables(stackInfo); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	return nil
}

// Apply same pattern to PermissionSetIdentity and UserIdentity
```

### AWS SDK Integration Changes

#### Add LoadAWSConfigWithAuth Function

**File:** `internal/aws_utils/aws_utils.go`

```go
// LoadAWSConfigWithAuth loads AWS config, preferring auth context if available.
// If authContext is provided, it uses the Atmos-managed credentials files and profile.
// Otherwise, it falls back to standard AWS SDK credential resolution.
func LoadAWSConfigWithAuth(
	ctx context.Context,
	region string,
	roleArn string,
	assumeRoleDuration time.Duration,
	authContext *schema.AWSAuthContext,
) (aws.Config, error) {
	defer perf.Track(nil, "aws_utils.LoadAWSConfigWithAuth")()

	var cfgOpts []func(*config.LoadOptions) error

	// If auth context is provided, use Atmos-managed credentials.
	if authContext != nil {
		log.Debug("Using Atmos auth context for AWS SDK",
			"profile", authContext.Profile,
			"credentials", authContext.CredentialsFile,
			"config", authContext.ConfigFile,
		)

		// Set custom credential and config file paths.
		// This overrides the default ~/.aws/credentials and ~/.aws/config.
		cfgOpts = append(cfgOpts,
			config.WithSharedCredentialsFiles([]string{authContext.CredentialsFile}),
			config.WithSharedConfigFiles([]string{authContext.ConfigFile}),
			config.WithSharedConfigProfile(authContext.Profile),
		)

		// Use region from auth context if not explicitly provided.
		if region == "" && authContext.Region != "" {
			region = authContext.Region
		}
	}

	// Set region if provided.
	if region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(region))
	}

	// Load base config.
	baseCfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("%w: %v", errUtils.ErrLoadAwsConfig, err)
	}

	// Conditionally assume role if specified.
	if roleArn != "" {
		log.Debug("Assuming role", "ARN", roleArn)
		stsClient := sts.NewFromConfig(baseCfg)

		creds := stscreds.NewAssumeRoleProvider(stsClient, roleArn, func(o *stscreds.AssumeRoleOptions) {
			o.Duration = assumeRoleDuration
		})

		cfgOpts = append(cfgOpts, config.WithCredentialsProvider(aws.NewCredentialsCache(creds)))

		// Reload full config with assumed role credentials.
		return config.LoadDefaultConfig(ctx, cfgOpts...)
	}

	return baseCfg, nil
}

// LoadAWSConfig is kept for backward compatibility.
// It wraps LoadAWSConfigWithAuth with nil authContext.
func LoadAWSConfig(ctx context.Context, region string, roleArn string, assumeRoleDuration time.Duration) (aws.Config, error) {
	defer perf.Track(nil, "aws_utils.LoadAWSConfig")()

	return LoadAWSConfigWithAuth(ctx, region, roleArn, assumeRoleDuration, nil)
}
```

### Terraform Backend Changes

#### Update Function Signatures

**File:** `internal/exec/terraform_state_utils.go`

```go
func GetTerraformState(
	atmosConfig *schema.AtmosConfiguration,
	yamlFunc string,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext, // NEW: Optional auth context
) (any, error) {
	defer perf.Track(atmosConfig, "exec.GetTerraformState")()

	// ... existing cache logic ...

	componentSections, err := ExecuteDescribeComponent(component, stack, true, true, nil)
	if err != nil {
		er := fmt.Errorf("%w `%s` in stack `%s`\nin YAML function: `%s`\n%v", errUtils.ErrDescribeComponent, component, stack, yamlFunc, err)
		return nil, er
	}

	// ... existing static remote state logic ...

	// Read Terraform backend with auth context.
	backend, err := tb.GetTerraformBackend(atmosConfig, &componentSections, authContext)
	if err != nil {
		er := fmt.Errorf("%w for component `%s` in stack `%s`\nin YAML function: `%s`\n%v", errUtils.ErrReadTerraformState, component, stack, yamlFunc, err)
		return nil, er
	}

	// ... existing output retrieval logic ...
}
```

**File:** `internal/terraform_backend/terraform_backend_utils.go`

```go
func GetTerraformBackend(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext, // NEW: Optional auth context
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "terraform_backend.GetTerraformBackend")()

	RegisterTerraformBackends()

	backendType := GetComponentBackendType(componentSections)
	if backendType == "" {
		backendType = cfg.BackendTypeLocal
	}

	readBackendStateFunc := GetTerraformBackendReadFunc(backendType)
	if readBackendStateFunc == nil {
		return nil, fmt.Errorf("%w: `%s`\nsupported backends: `local`, `s3`", errUtils.ErrUnsupportedBackendType, backendType)
	}

	// Pass auth context to backend reader.
	content, err := readBackendStateFunc(atmosConfig, componentSections, authContext)
	if err != nil {
		return nil, err
	}

	// ... existing state file processing ...
}
```

**File:** `internal/terraform_backend/terraform_backend_s3.go`

```go
// Update function signature type.
type TerraformBackendReadFunc func(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext, // NEW: Optional auth context
) ([]byte, error)

func ReadTerraformBackendS3(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext, // NEW: Optional auth context
) ([]byte, error) {
	defer perf.Track(nil, "terraform_backend.ReadTerraformBackendS3")()

	backend := GetComponentBackend(componentSections)

	// Use auth context if available.
	s3Client, err := getCachedS3ClientWithAuth(&backend, authContext)
	if err != nil {
		return nil, err
	}

	return ReadTerraformBackendS3Internal(s3Client, componentSections, &backend)
}

// getCachedS3ClientWithAuth creates or retrieves a cached S3 client with auth context support.
func getCachedS3ClientWithAuth(backend *map[string]any, authContext *schema.AuthContext) (S3API, error) {
	region := GetBackendAttribute(backend, "region")
	roleArn := GetS3BackendAssumeRoleArn(backend)

	// Build cache key based on region, role, and auth profile.
	cacheKey := fmt.Sprintf("region=%s;role_arn=%s", region, roleArn)
	if authContext != nil && authContext.AWS != nil {
		cacheKey += fmt.Sprintf(";profile=%s", authContext.AWS.Profile)
	}

	// Check cache.
	if cached, ok := s3ClientCache.Load(cacheKey); ok {
		return cached.(S3API), nil
	}

	// Build S3 client with auth context.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Extract AWS auth context.
	var awsAuthContext *schema.AWSAuthContext
	if authContext != nil {
		awsAuthContext = authContext.AWS
	}

	// Load AWS config with auth context.
	cfg, err := awsUtils.LoadAWSConfigWithAuth(ctx, region, roleArn, 15*time.Minute, awsAuthContext)
	if err != nil {
		return nil, err
	}

	s3Client := s3.NewFromConfig(cfg)
	s3ClientCache.Store(cacheKey, s3Client)
	return s3Client, nil
}
```

**File:** `internal/terraform_backend/terraform_backend_local.go`

```go
// Update to match new signature (auth context not used for local backend).
func ReadTerraformBackendLocal(
	_ *schema.AtmosConfiguration,
	componentSections *map[string]any,
	_ *schema.AuthContext, // Unused for local backend
) ([]byte, error) {
	defer perf.Track(nil, "terraform_backend.ReadTerraformBackendLocal")()

	// ... existing implementation unchanged ...
}
```

### YAML Function Processing Changes

#### Update ProcessCustomYamlTags

**File:** `internal/exec/yaml_func_utils.go`

```go
func ProcessCustomYamlTags(
	atmosConfig *schema.AtmosConfiguration,
	input schema.AtmosSectionMapType,
	currentStack string,
	skip []string,
	stackInfo *schema.ConfigAndStacksInfo, // NEW: Stack info for auth context
) (schema.AtmosSectionMapType, error) {
	defer perf.Track(atmosConfig, "exec.ProcessCustomYamlTags")()

	return processNodes(atmosConfig, input, currentStack, skip, stackInfo), nil
}

func processNodes(
	atmosConfig *schema.AtmosConfiguration,
	data map[string]any,
	currentStack string,
	skip []string,
	stackInfo *schema.ConfigAndStacksInfo, // NEW
) map[string]any {
	newMap := make(map[string]any)
	var recurse func(any) any

	recurse = func(node any) any {
		switch v := node.(type) {
		case string:
			return processCustomTags(atmosConfig, v, currentStack, skip, stackInfo)

		case map[string]any:
			newNestedMap := make(map[string]any)
			for k, val := range v {
				newNestedMap[k] = recurse(val)
			}
			return newNestedMap

		case []any:
			newSlice := make([]any, len(v))
			for i, val := range v {
				newSlice[i] = recurse(val)
			}
			return newSlice

		default:
			return v
		}
	}

	for k, v := range data {
		newMap[k] = recurse(v)
	}

	return newMap
}

func processCustomTags(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	skip []string,
	stackInfo *schema.ConfigAndStacksInfo, // NEW
) any {
	switch {
	case strings.HasPrefix(input, u.AtmosYamlFuncTemplate) && !skipFunc(skip, u.AtmosYamlFuncTemplate):
		return processTagTemplate(input)
	case strings.HasPrefix(input, u.AtmosYamlFuncExec) && !skipFunc(skip, u.AtmosYamlFuncExec):
		res, err := u.ProcessTagExec(input)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return res
	case strings.HasPrefix(input, u.AtmosYamlFuncStoreGet) && !skipFunc(skip, u.AtmosYamlFuncStoreGet):
		return processTagStoreGet(atmosConfig, input, currentStack)
	case strings.HasPrefix(input, u.AtmosYamlFuncStore) && !skipFunc(skip, u.AtmosYamlFuncStore):
		return processTagStore(atmosConfig, input, currentStack)
	case strings.HasPrefix(input, u.AtmosYamlFuncTerraformOutput) && !skipFunc(skip, u.AtmosYamlFuncTerraformOutput):
		return processTagTerraformOutput(atmosConfig, input, currentStack)
	case strings.HasPrefix(input, u.AtmosYamlFuncTerraformState) && !skipFunc(skip, u.AtmosYamlFuncTerraformState):
		return processTagTerraformState(atmosConfig, input, currentStack, stackInfo) // Pass stackInfo
	case strings.HasPrefix(input, u.AtmosYamlFuncEnv) && !skipFunc(skip, u.AtmosYamlFuncEnv):
		res, err := u.ProcessTagEnv(input)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return res
	default:
		return input
	}
}
```

**File:** `internal/exec/yaml_func_terraform_state.go`

```go
func processTagTerraformState(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	stackInfo *schema.ConfigAndStacksInfo, // NEW: Stack info for auth context
) any {
	defer perf.Track(atmosConfig, "exec.processTagTerraformState")()

	log.Debug("Executing Atmos YAML function", "function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncTerraformState)
	errUtils.CheckErrorPrintAndExit(err, "", "")

	var component string
	var stack string
	var output string

	// ... existing argument parsing ...

	// Extract auth context from stack info.
	var authContext *schema.AuthContext
	if stackInfo != nil {
		authContext = stackInfo.AuthContext
	}

	// Pass auth context to GetTerraformState.
	value, err := GetTerraformState(atmosConfig, input, stack, component, output, false, authContext)
	errUtils.CheckErrorPrintAndExit(err, "", "")
	return value
}
```

#### Update Call Sites

**Files to update:**
- `internal/exec/describe_stacks.go` (3 call sites)
- `internal/exec/utils.go` (1 call site)
- `internal/exec/terraform_generate_backends.go` (1 call site)
- `internal/exec/terraform_generate_varfiles.go` (1 call site)

**Example from `internal/exec/utils.go`:**

```go
// Process YAML functions in Atmos manifest sections.
if processYamlFunctions {
	// Pass configAndStacksInfo to provide auth context.
	componentSectionConverted, err := ProcessCustomYamlTags(
		atmosConfig,
		configAndStacksInfo.ComponentSection,
		configAndStacksInfo.Stack,
		skip,
		&configAndStacksInfo, // NEW: Pass stack info
	)
	if err != nil {
		return configAndStacksInfo, err
	}
	configAndStacksInfo.ComponentSection = componentSectionConverted
}
```

## Implementation Plan

### Phase 1: Schema and Auth System (Week 1)

**Tasks:**
1. Add `AuthContext` and `AWSAuthContext` types to `pkg/schema/schema.go`
2. Add `AuthContext *AuthContext` field to `ConfigAndStacksInfo`
3. Implement `SetAuthContext()` in `pkg/auth/cloud/aws/setup.go`
4. Update `PostAuthenticate()` in all 3 AWS identity types
5. Add unit tests for auth context population

**Deliverables:**
- Auth system populates `AuthContext` during login
- Tests verify auth context is set correctly

### Phase 2: AWS SDK Integration (Week 1)

**Tasks:**
1. Implement `LoadAWSConfigWithAuth()` in `internal/aws_utils/aws_utils.go`
2. Update `LoadAWSConfig()` to be a wrapper
3. Add unit tests with mock AWS config
4. Update S3 backend to use new function

**Deliverables:**
- AWS SDK can load config from auth context
- Tests verify credentials file paths are used

### Phase 3: Terraform Backend Changes (Week 2)

**Tasks:**
1. Update `GetTerraformBackend()` signature
2. Update `ReadTerraformBackendS3()` signature
3. Update `ReadTerraformBackendLocal()` signature
4. Implement `getCachedS3ClientWithAuth()`
5. Update backend function registry types
6. Add integration tests

**Deliverables:**
- Backend reading functions accept auth context
- S3 client uses auth context when available

### Phase 4: YAML Processing (Week 2)

**Tasks:**
1. Update `ProcessCustomYamlTags()` signature
2. Update `processNodes()` signature
3. Update `processCustomTags()` signature
4. Update `processTagTerraformState()` signature
5. Update all call sites in:
   - `describe_stacks.go`
   - `utils.go`
   - `terraform_generate_backends.go`
   - `terraform_generate_varfiles.go`
6. Add end-to-end tests

**Deliverables:**
- `!terraform.state` receives auth context
- Full integration test passes

### Phase 5: Testing and Documentation (Week 3)

**Tasks:**
1. Create comprehensive integration test
2. Test with real AWS SSO credentials
3. Test with multiple identity types
4. Update documentation
5. Add migration guide
6. Performance testing

**Deliverables:**
- Full test coverage
- Documentation complete
- Performance validated

## Testing Strategy

### Unit Tests

**Auth Context Population:**
```go
func TestSetAuthContext(t *testing.T) {
	stackInfo := &schema.ConfigAndStacksInfo{}
	err := awsCloud.SetAuthContext(stackInfo, "aws-sso", "my-identity")

	require.NoError(t, err)
	require.NotNil(t, stackInfo.AuthContext)
	require.NotNil(t, stackInfo.AuthContext.AWS)
	assert.Equal(t, "my-identity", stackInfo.AuthContext.AWS.Profile)
	assert.Contains(t, stackInfo.AuthContext.AWS.CredentialsFile, ".atmos/auth")
}
```

**AWS Config Loading:**
```go
func TestLoadAWSConfigWithAuth(t *testing.T) {
	authContext := &schema.AWSAuthContext{
		CredentialsFile: "/test/credentials",
		ConfigFile:      "/test/config",
		Profile:         "test-profile",
		Region:          "us-east-1",
	}

	ctx := context.Background()
	cfg, err := LoadAWSConfigWithAuth(ctx, "", "", 0, authContext)

	require.NoError(t, err)
	assert.Equal(t, "us-east-1", cfg.Region)
	// Verify config uses custom files (requires AWS SDK testing utilities)
}
```

### Integration Tests

**End-to-End Test:**
```go
func TestTerraformStateWithAuth(t *testing.T) {
	// Setup: Create test stack with !terraform.state
	// Setup: Mock S3 backend with test state file
	// Setup: Create Atmos auth identity

	// Authenticate
	authManager := /* create auth manager */
	_, err := authManager.Authenticate(ctx, "test-identity")
	require.NoError(t, err)

	// Process component with !terraform.state
	stackInfo, err := ExecuteDescribeComponent("test-component", "test-stack", true, true, nil)
	require.NoError(t, err)

	// Verify auth context was used (check logs or mock calls)
	assert.NotNil(t, stackInfo.AuthContext)
	assert.NotNil(t, stackInfo.AuthContext.AWS)
}
```

### Manual Testing

**Test Cases:**
1. Authenticate with AWS SSO → Run component with `!terraform.state` → Verify success
2. Set external `AWS_PROFILE` → Authenticate with Atmos → Verify Atmos auth takes precedence
3. Use component with multiple `!terraform.state` calls → Verify caching works
4. Use component without auth → Verify falls back to ambient credentials (backward compat)

## Security Considerations

1. **Credential file paths**: Auth context contains absolute paths to credential files. These paths are internal to Atmos and should not be logged at INFO level or exposed in error messages.

2. **Cache key security**: S3 client cache keys include profile names. Ensure cache is process-local and not shared across security boundaries.

3. **Temporary credentials**: Auth context supports temporary credentials (SSO). Ensure SDK properly handles credential expiration and refresh.

4. **Isolation**: Auth context is scoped to `ConfigAndStacksInfo`, which is component/stack specific. Multiple concurrent components can have different auth contexts.

## Performance Considerations

1. **Caching**: S3 client cache key now includes auth profile. This may increase cache misses but ensures correct credentials are used.

2. **Memory**: `AuthContext` adds minimal memory overhead (~200 bytes per component). No performance impact expected.

3. **Config loading**: `LoadAWSConfigWithAuth()` uses same AWS SDK code paths. No performance regression expected.

## Backward Compatibility

1. **Auth context is optional**: All new parameters are `*AuthContext` (nullable). Existing code without auth context continues to work.

2. **LoadAWSConfig preserved**: Old function is wrapper around new function with `nil` auth context.

3. **Process environment fallback**: When auth context is `nil`, AWS SDK falls back to standard credential resolution (process env, ~/.aws/ files, etc.).

4. **ComponentEnvList unchanged**: Environment variables for spawned processes continue to work exactly as before.

## Future Extensions

### Multi-Provider Support

**Azure:**
```go
type AzureAuthContext struct {
	SubscriptionID string
	TenantID       string
	ClientID       string
	TokenFile      string
}

authContext.Azure = &AzureAuthContext{...}
```

**GCP:**
```go
type GCPAuthContext struct {
	ProjectID           string
	ServiceAccountFile  string
	ImpersonateAccount  string
}

authContext.GCP = &GCPAuthContext{...}
```

**GitHub:**
```go
type GitHubAuthContext struct {
	Token     string
	TokenFile string
	AppID     string
	InstallID string
}

authContext.GitHub = &GitHubAuthContext{...}
```

### Store Integration

The `!store` YAML function could also benefit from auth context for accessing secrets:

```go
func processTagStore(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	stackInfo *schema.ConfigAndStacksInfo, // Add stack info
) any {
	// Use authContext for AWS SSM Parameter Store, Azure Key Vault, etc.
}
```

## Success Metrics

1. **Bug fix**: `!terraform.state` works with Atmos auth credentials (100% success rate)
2. **Test coverage**: >90% coverage for new code
3. **Performance**: No regression in YAML processing time
4. **Adoption**: Existing code continues to work without changes (0 breaking changes)
5. **Extensibility**: Can add new provider auth context in <1 day

## References

- [Original Issue Report](#problem-statement)
- [Atmos Auth PRD](../pkg/auth/docs/PRD/PRD-Atmos-Auth.md)
- [AWS SDK Go v2 Configuration](https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/)
- [Error Handling Strategy](error-handling-strategy.md)

## Appendix: Call Chain Diagram

```
User: atmos terraform plan component -s stack
  ↓
ExecuteDescribeComponent()
  ├─ ProcessStackConfig()
  │   ├─ Auth: PostAuthenticate()
  │   │   ├─ SetupFiles() → Write ~/.atmos/auth/aws-sso/credentials
  │   │   ├─ SetAuthContext() → stackInfo.AuthContext.AWS = {...} ✅ NEW
  │   │   └─ SetEnvironmentVariables() → stackInfo.ComponentEnvList += [...]
  │   │
  │   └─ ProcessCustomYamlTags(stackInfo) ✅ Pass stackInfo
  │       └─ processTagTerraformState(stackInfo) ✅ Extract authContext
  │           └─ GetTerraformState(..., authContext) ✅ Thread through
  │               └─ GetTerraformBackend(..., authContext) ✅ Thread through
  │                   └─ ReadTerraformBackendS3(..., authContext) ✅ Thread through
  │                       └─ getCachedS3ClientWithAuth(authContext) ✅ Use context
  │                           └─ LoadAWSConfigWithAuth(authContext.AWS) ✅ Load config
  │                               └─ config.WithSharedCredentialsFiles([authContext.CredentialsFile])
  │                               └─ config.WithSharedConfigFiles([authContext.ConfigFile])
  │                               └─ config.WithSharedConfigProfile(authContext.Profile)
  │                               └─ AWS SDK reads from Atmos-managed files ✅ SUCCESS
  │
  └─ ExecuteTerraform()
      └─ exec.Command("terraform", args..., stackInfo.ComponentEnvList)
          └─ Terraform process uses env vars from ComponentEnvList ✅ EXISTING
```

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-10-21 | AI Assistant | Initial PRD |
