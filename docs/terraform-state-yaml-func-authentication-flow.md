# Authentication Flow: !terraform.state YAML Function

## Overview

The `!terraform.state` YAML function has a simpler authentication flow than `!terraform.output` because it:

1. Uses AWS SDK Go v2 directly to read state from S3
2. Reads the backend configuration from component metadata
3. Uses AWS credentials from AuthContext to access remote state in S3

## Complete Call Flow

```text
Terraform Command Execution (with --identity flag)
  ↓
cmd/terraform_utils.go:terraformRun()
  ↓ Parses --identity flag → info.Identity
  ↓
internal/exec/terraform.go:ExecuteTerraform()
  ↓ Creates AuthManager from info.Identity (OUR FIX)
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
  ↓ Line 89-93: Extracts authContext from stackInfo.AuthContext
  ↓ Line 95: Passes authContext to stateGetter.GetState()
  ↓
internal/exec/terraform_state_getter.go:GetState()
  ↓ Line 40: Calls GetTerraformState()
  ↓
internal/exec/terraform_state_utils.go:GetTerraformState()
  ↓ Line 232-239: Creates authContextWrapper from authContext
  ↓ Line 241-248: Calls ExecuteDescribeComponent(AuthManager: authMgr)
  ↓ Line 280-296: CRITICAL AUTHENTICATION STEP
  ↓   if authContext != nil && authContext.AWS != nil:
  ↓     - Creates AWS SDK config with authContext credentials
  ↓     - Uses AWS_SHARED_CREDENTIALS_FILE from authContext
  ↓     - Uses AWS_CONFIG_FILE from authContext
  ↓     - Uses AWS_PROFILE from authContext
  ↓     - Uses AWS_REGION from authContext
  ↓     - Disables IMDS fallback (AWS_EC2_METADATA_DISABLED=true)
  ↓
  ↓ Line 298: Creates S3 client with authenticated config
  ↓ Line 310: Calls GetObject() to read state from S3
  ↓   - Uses AWS profile to assume role
  ↓   - Reads state file from S3 bucket
  ↓   - Returns state data
  ↓
  ↓ Line 330: Parses state JSON
  ↓ Line 350: Extracts attribute value from state
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

## Critical Code Sections

### 1. AuthContext Population (OUR FIX)

**File**: `internal/exec/utils.go`
**Line**: ~400 (in ProcessComponentConfig)

```go
// Populate AuthContext from AuthManager if provided (from --identity flag).
if authManager != nil {
	managerStackInfo := authManager.GetStackInfo()
	if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
		configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
	}
}
```

### 2. AuthContext Extraction in YAML Function

**File**: `internal/exec/yaml_func_terraform_state.go`
**Lines**: 89-95

```go
// Extract authContext from stackInfo if available.
var authContext *schema.AuthContext
if stackInfo != nil {
	authContext = stackInfo.AuthContext
}

value, err := stateGetter.GetState(atmosConfig, stack, component, attribute, authContext)
```

### 3. AWS SDK Config Creation with AuthContext

**File**: `internal/exec/terraform_state_utils.go`
**Lines**: 280-296

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

### 4. S3 State Retrieval with Credentials

**File**: `internal/exec/terraform_state_utils.go`
**Lines**: 298-330

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

To verify the fix works for `!terraform.state`:

```bash
# Component config with !terraform.state function
# stacks/catalog/runs-on/cloudposse.yaml
components:
  terraform:
    runs-on/cloudposse:
      vars:
        vpc_id: !terraform.state vpc vpc_id
        subnet_ids: !terraform.state vpc public_subnet_ids

# Execute with --identity flag
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --identity core-identity/managers-team-access

# Expected behavior:
# 1. AuthManager created from --identity flag
# 2. AuthContext populated in stackInfo
# 3. !terraform.state receives AuthContext
# 4. GetTerraformState creates AWS SDK config with auth credentials
# 5. S3 client reads state with proper role assumption
# 6. No IMDS timeout errors
```

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                     User Executes Command                        │
│  atmos terraform plan component -s stack --identity identity    │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                    ExecuteTerraform() [OUR FIX]                  │
│  - Parses --identity flag                                        │
│  - Creates AuthManager with credentials                          │
│  - Stores identity info in AuthContext                           │
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

Our fix **correctly handles both** `!terraform.state` and `!terraform.output` YAML functions:

1. **Common path**: Both functions receive AuthContext through stackInfo from ProcessComponentConfig
2. **Divergent execution**:
   - `!terraform.state`: Uses AuthContext directly with AWS SDK to read S3
   - `!terraform.output`: Converts AuthContext to environment variables for terraform binary

The key insight is that **both authentication methods rely on stackInfo.AuthContext being populated**, which our fix ensures by threading AuthManager through ProcessComponentConfig.

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

Common errors with `!terraform.state`:

1. **Missing AuthContext**: "failed to load AWS config" with IMDS timeout
2. **Invalid credentials**: "operation error S3: GetObject, failed to sign request"
3. **Missing permissions**: "AccessDenied: Access Denied"
4. **Invalid state path**: "NoSuchKey: The specified key does not exist"
5. **Invalid attribute**: "attribute 'xyz' not found in state"

All errors are properly wrapped with context for debugging.
