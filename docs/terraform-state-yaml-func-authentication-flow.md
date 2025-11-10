# Authentication Flow: !terraform.state YAML Function

## Overview

The `!terraform.state` YAML function has a simpler authentication flow than `!terraform.output` because it:

1. Uses AWS SDK Go v2 directly to read state from S3
2. Reads the backend configuration from component metadata
3. Uses AWS credentials from AuthContext to access remote state in S3

## Complete Call Flow

```text
Terraform Command Execution (with or without --identity flag)
  ↓
cmd/terraform_utils.go:terraformRun()
  ↓ Parses --identity flag → info.Identity (or empty if not provided)
  ↓
internal/exec/terraform.go:ExecuteTerraform()
  ↓ Creates AuthManager from info.Identity
  ↓ If info.Identity is empty, auto-detects default identity
  ↓ If no defaults exist and interactive mode, prompts user ONCE
  ↓ Stores authenticated identity in info.Identity for hooks
  ↓
internal/exec/utils.go:ProcessStacks(..., authManager)
  ↓ Passes authManager parameter (OUR FIX)
  ↓
internal/exec/utils.go:ProcessComponentConfig(..., authManager)
  ↓ Populates configAndStacksInfo.AuthContext from authManager (OUR FIX)
  ↓
Component YAML processed with stackInfo.AuthContext available
  ↓
YAML function encountered: !terraform.state vpc vpc_id
  ↓
internal/exec/yaml_func_terraform_state.go:processTagTerraformStateWithContext()
  ↓ Extracts authContext from stackInfo.AuthContext
  ↓ Passes authContext to stateGetter.GetState()
  ↓
internal/exec/terraform_state_getter.go:GetState()
  ↓ Calls GetTerraformState()
  ↓
internal/exec/terraform_state_utils.go:GetTerraformState()
  ↓ Creates authContextWrapper from authContext
  ↓ Calls ExecuteDescribeComponent(AuthManager: authMgr)
  ↓ CRITICAL AUTHENTICATION STEP:
  ↓   if authContext != nil && authContext.AWS != nil:
  ↓     - Creates AWS SDK config with authContext credentials
  ↓     - Uses AWS_SHARED_CREDENTIALS_FILE from authContext
  ↓     - Uses AWS_CONFIG_FILE from authContext
  ↓     - Uses AWS_PROFILE from authContext
  ↓     - Uses AWS_REGION from authContext
  ↓     - Disables IMDS fallback (AWS_EC2_METADATA_DISABLED=true)
  ↓
  ↓ Creates S3 client with authenticated config
  ↓ Calls GetObject() to read state from S3
  ↓   - Uses AWS profile to assume role
  ↓   - Reads state file from S3 bucket
  ↓   - Returns state data
  ↓
  ↓ Parses state JSON
  ↓ Extracts attribute value from state
  ↓
  ↓ Returns attribute value
```

## Key Differences from !terraform.output

### !terraform.state

- **Direct AWS SDK usage**: Uses AWS SDK Go v2 to read state from S3
- **AuthContext usage**: Passed to AWS SDK config loader
- **Credential source**: AuthContext provides credentials directly to SDK
- **No binary execution**: Pure Go code using AWS SDK
- **Faster execution**: No subprocess overhead

### !terraform.output

- **Terraform binary execution**: Spawns terraform process with environment variables
- **AuthContext usage**: Converted to environment variables (AWS_PROFILE, AWS_SHARED_CREDENTIALS_FILE, etc.)
- **Credential source**: Terraform binary reads credentials from files using AWS_PROFILE
- **Backend configuration**: Auto-generates backend.tf.json with role ARN and workspace
- **Subprocess overhead**: Must execute terraform binary

## Identity Resolution

Identity resolution for `!terraform.state` works the same as `!terraform.output`. See the [terraform-output-yaml-func-authentication-flow.md](./terraform-output-yaml-func-authentication-flow.md#identity-resolution) document for complete details.

**Summary of identity resolution order:**

1. **`--identity` CLI flag** (highest priority) - Explicit identity specification
2. **Auto-detection from default identity** - Single default from config
3. **Interactive selection** - User prompted once (TTY only)
4. **No authentication** - Backward compatible (CI mode or no auth configured)
5. **Explicitly disabled** - `--identity=off` for external auth

**Key behavior:**
- Selected/auto-detected identity is stored in `info.Identity`
- Prevents double-prompting from hooks
- Single authentication per command execution

## Critical Code Sections

### 1. Identity Resolution and AuthManager Creation

See the [terraform-output-yaml-func-authentication-flow.md](./terraform-output-yaml-func-authentication-flow.md#critical-code-sections) document for complete implementation details of `CreateAndAuthenticateManager()` and `autoDetectDefaultIdentity()`.

### 2. AuthContext Population

**File**: `internal/exec/utils.go`
**Function**: `ProcessComponentConfig()`

```go
// Populate AuthContext from AuthManager if provided (from --identity flag).
if authManager != nil {
	managerStackInfo := authManager.GetStackInfo()
	if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
		configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
	}
}
```

### 3. AuthContext Extraction in YAML Function

**File**: `internal/exec/yaml_func_terraform_state.go`
**Function**: `processTagTerraformStateWithContext()`

```go
// Extract authContext from stackInfo if available.
var authContext *schema.AuthContext
if stackInfo != nil {
	authContext = stackInfo.AuthContext
}

value, err := stateGetter.GetState(atmosConfig, stack, component, attribute, authContext)
```

### 4. AWS SDK Config Creation with AuthContext

**File**: `internal/exec/terraform_state_utils.go`
**Function**: `GetTerraformState()`

```go
// Add auth-based configuration if authContext is provided.
var cfg aws.Config
var err error

if authContext != nil && authContext.AWS != nil {
	log.Debug("Loading AWS config with auth context",
		"profile", authContext.AWS.Profile,
		"credentials_file", authContext.AWS.CredentialsFile,
		"config_file", authContext.AWS.ConfigFile,
		"region", authContext.AWS.Region,
	)

	// Load AWS config with Atmos-managed credentials.
	cfg, err = awsCloud.LoadAWSConfigWithAuth(
		ctx,
		authContext.AWS.Region,
		authContext.AWS.Profile,
		authContext.AWS.CredentialsFile,
		authContext.AWS.ConfigFile,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config with auth context: %w", err)
	}

	log.Debug("Successfully loaded AWS config with auth context",
		"profile", authContext.AWS.Profile,
		"region", cfg.Region,
	)
} else {
	// Fall back to default AWS credential chain.
	cfg, err = awsCloud.LoadDefaultAWSConfig(ctx, backendRegion)
	if err != nil {
		return nil, fmt.Errorf("failed to load default AWS config: %w", err)
	}
}
```

### 5. S3 State Retrieval with Credentials

**File**: `internal/exec/terraform_state_utils.go`
**Function**: `GetTerraformState()`

```go
// Create S3 client with authenticated config.
s3Client := s3.NewFromConfig(cfg)

// Build S3 GetObject request.
getObjectInput := &s3.GetObjectInput{
	Bucket: aws.String(backendBucket),
	Key:    aws.String(stateKey),
}

// Retrieve state from S3.
log.Debug("Retrieving Terraform state from S3",
	"bucket", backendBucket,
	"key", stateKey,
	"region", cfg.Region,
)

getObjectOutput, err := s3Client.GetObject(ctx, getObjectInput)
if err != nil {
	return nil, fmt.Errorf("failed to get object from S3: %w", err)
}
defer getObjectOutput.Body.Close()

// Read state data.
stateData, err := io.ReadAll(getObjectOutput.Body)
if err != nil {
	return nil, fmt.Errorf("failed to read state data: %w", err)
}

// Parse state JSON.
var state map[string]interface{}
if err := json.Unmarshal(stateData, &state); err != nil {
	return nil, fmt.Errorf("failed to parse state JSON: %w", err)
}
```

## Why Our Fix Works for !terraform.state

✅ **AuthManager Created**: ExecuteTerraform creates AuthManager from info.Identity

✅ **AuthContext Threaded**: ProcessComponentConfig populates stackInfo.AuthContext from AuthManager.

✅ **YAML Function Access**: processTagTerraformStateWithContext extracts authContext from stackInfo

✅ **AWS SDK Config**: GetTerraformState uses authContext to create authenticated AWS SDK config

✅ **Direct S3 Access**: The AWS SDK reads state directly from S3 with proper credentials

✅ **No IMDS Fallback**: IMDS is disabled when using Atmos-managed credentials

## Testing Verification

Testing scenarios for `!terraform.state` are identical to `!terraform.output`. Both YAML functions use the same identity resolution and authentication flow. See the [terraform-output-yaml-func-authentication-flow.md](./terraform-output-yaml-func-authentication-flow.md#testing-verification) document for complete test scenarios.

### Quick Test Examples

#### Test 1: Explicit `--identity` Flag

```bash
# Component config with !terraform.state function
components:
  terraform:
    runs-on/cloudposse:
      vars:
        vpc_id: !terraform.state vpc vpc_id
        subnet_ids: !terraform.state vpc public_subnet_ids

# Execute with --identity flag
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --identity core-identity/managers-team-access

# Expected: Authenticates with specified identity, no prompts
```

#### Test 2: Auto-Detection of Default Identity

```yaml
# atmos.yaml or stack config
auth:
  identities:
    core-auto/terraform:
      default: true
```

```bash
# Execute WITHOUT --identity flag
atmos terraform plan runs-on/cloudposse -s core-ue2-auto

# Expected: Auto-detects default identity, authenticates automatically, no prompts
```

#### Test 3: Interactive Selection (No Defaults)

```bash
# No --identity flag, no defaults, interactive terminal
atmos terraform plan runs-on/cloudposse -s core-ue2-auto

# Expected: Prompts user ONCE to select identity, no second prompt from hooks
```

For complete test scenarios including CI mode and explicitly disabled auth, see [Testing Verification](./terraform-output-yaml-func-authentication-flow.md#testing-verification) in the terraform-output documentation.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                     User Executes Command                        │
│  atmos terraform plan component -s stack [--identity identity]  │
│  (--identity flag is optional)                                   │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│              CreateAndAuthenticateManager()                      │
│  Identity Resolution (see terraform-output doc for details):     │
│  1. --identity=off → nil (auth disabled)                         │
│  2. --identity provided → use it                                 │
│  3. No flag → auto-detect default or prompt (interactive)        │
│  4. Authenticate with resolved identity                          │
│  5. Return AuthManager with AuthContext                          │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│              ExecuteTerraform() - Identity Storage               │
│  - Store authenticated identity in info.Identity for hooks       │
│  - Prevents TerraformPreHook from prompting again                │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│              ProcessStacks() → ProcessComponentConfig()          │
│  - Receives AuthManager parameter [OUR FIX]                      │
│  - Populates stackInfo.AuthContext [OUR FIX]                     │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                   YAML Function Evaluation                       │
│  !terraform.state vpc vpc_id                                     │
│  - Has access to stackInfo.AuthContext                           │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                  GetTerraformState()                             │
│  - Receives authContext parameter                                │
│  - Extracts backend config from component metadata               │
│  - Creates AWS SDK config with authContext credentials:          │
│    • Uses AWS_PROFILE from authContext.AWS.Profile               │
│    • Uses AWS_SHARED_CREDENTIALS_FILE                            │
│    • Uses AWS_CONFIG_FILE                                        │
│    • Uses AWS_REGION from authContext.AWS.Region                 │
│    • Disables IMDS (AWS_EC2_METADATA_DISABLED=true)              │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                  AWS SDK Direct S3 Access                        │
│  - Creates S3 client with authenticated config                   │
│  - Calls s3Client.GetObject() with credentials                   │
│  - AWS SDK handles role assumption transparently                 │
│  - Reads state file from S3 bucket                               │
│  - Parses JSON state                                             │
│  - Extracts attribute value                                      │
│  - Returns value to YAML function                                │
└───────────────────────────────────────────────────────────────────┘
```

## Conclusion

The authentication system **correctly handles both** `!terraform.state` and `!terraform.output` YAML functions with flexible identity resolution:

1. **Flexible Identity Resolution**:
   - CLI flag authentication (`--identity`)
   - Auto-detection of default identities
   - Interactive selection when no defaults
   - Backward compatible (no auth when not configured)
   - Support for external auth (`--identity=off`)

2. **Common Authentication Path**:
   - Both functions receive AuthContext through stackInfo from ProcessComponentConfig
   - Identity resolution happens once at command start
   - Selected/auto-detected identity stored for hooks
   - No double-prompting

3. **Divergent Execution**:
   - `!terraform.state`: Uses AuthContext directly with AWS SDK to read S3
   - `!terraform.output`: Converts AuthContext to environment variables for terraform binary

4. **Key Features**:
   - ✅ No `--identity` flag required when default identity configured
   - ✅ Single prompt for identity selection (no double-prompting)
   - ✅ CI/CD friendly (auto-detects non-interactive mode)
   - ✅ Backward compatible (existing workflows unchanged)
   - ✅ Flexible (supports external auth mechanisms)

The key insight is that **both authentication methods rely on stackInfo.AuthContext being populated**, which the system ensures by:
1. Creating AuthManager with auto-detected or explicitly specified identity
2. Storing authenticated identity in info.Identity for hooks
3. Threading AuthManager through ProcessComponentConfig to populate AuthContext

## Performance Comparison

### !terraform.state Advantages

- ✅ **Faster**: No subprocess overhead
- ✅ **Simpler**: Direct AWS SDK usage
- ✅ **Lower latency**: Single process execution
- ✅ **Better debugging**: Errors in same process

### !terraform.output Advantages

- ✅ **Official terraform binary**: Uses terraform's own state parser
- ✅ **Complex outputs**: Handles computed outputs, data sources
- ✅ **Output transformations**: Supports terraform output expressions
- ✅ **Workspace awareness**: Respects terraform workspace configuration

## When to Use Each Function

### Use !terraform.state when:

- Reading simple state attributes (vpc_id, subnet_ids, etc.)
- Performance is critical
- State is in S3 backend
- You need fast lookups during stack processing

### Use !terraform.output when:

- Reading computed outputs from data sources
- Need terraform's output parsing logic
- Working with complex output expressions
- Need guaranteed compatibility with terraform's state format

## Authentication Requirements

Both functions require:

1. **AuthContext populated** from AuthManager
2. **Valid AWS credentials** in Atmos-managed credential files
3. **IAM permissions** to read S3 state (s3:GetObject, s3:ListBucket)
4. **Role assumption** configured in identity provider
5. **Backend configuration** accessible in component metadata

## Error Handling

Common errors:

1. **No credentials available**: "failed to load AWS config" - Occurs when AuthContext is not provided (no `--identity` flag) and the default AWS credential chain has no valid credentials (no environment variables, no shared credentials file, IMDS timeout on non-EC2 instances)
2. **Invalid credentials**: "operation error S3: GetObject, failed to sign request" - AWS SDK cannot sign the S3 request with provided credentials
3. **Missing S3 permissions**: "AccessDenied: Access Denied" - IAM role lacks s3:GetObject permission on the state bucket
4. **Invalid state path**: "NoSuchKey: The specified key does not exist" - State file doesn't exist at the calculated S3 path
5. **Missing backend configuration**: "backend configuration not found" - Component metadata doesn't contain required backend settings
6. **Invalid attribute path**: "attribute 'xyz' not found in state" - Requested attribute doesn't exist in the terraform state outputs

**Note:** Missing AuthContext (no `--identity` flag) is not an error - `!terraform.state` gracefully falls back to the default AWS credential chain for backward compatibility.

All errors are properly wrapped with context for debugging.
