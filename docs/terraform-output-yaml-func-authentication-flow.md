# Authentication Flow: !terraform.output YAML Function

## Overview

The `!terraform.output` YAML function has a more complex authentication flow than `!terraform.state` because it:

1. Executes the terraform binary (`terraform init` and `terraform output`)
2. Reads the generated backend configuration from `backend.tf.json`
3. Uses AWS credentials to assume roles and access remote state in S3

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
YAML function encountered: !terraform.output vpc subnet_ids
  ↓
internal/exec/yaml_func_terraform_output.go:processTagTerraformOutputWithContext()
  ↓ Line 102-106: Extracts authContext from stackInfo.AuthContext
  ↓ Line 108: Passes authContext to outputGetter.GetOutput()
  ↓
internal/exec/terraform_output_getter.go:GetOutput()
  ↓ Line 39: Calls GetTerraformOutput()
  ↓
internal/exec/terraform_output_utils.go:GetTerraformOutput()
  ↓ Line 552-557: Creates authContextWrapper from authContext
  ↓ Line 559-566: Calls ExecuteDescribeComponent(AuthManager: authMgr)
  ↓ Line 586: Calls execTerraformOutput(authContext)
  ↓
internal/exec/terraform_output_utils.go:execTerraformOutput()
  ↓ Line 258: Auto-generates backend.tf.json with backend config
  ↓ Line 310: Gets environment variables from parent process
  ↓ Lines 312-330: CRITICAL AUTHENTICATION STEP
  ↓   if authContext != nil && authContext.AWS != nil:
  ↓     - Calls awsCloud.PrepareEnvironment()
  ↓     - Sets AWS_SHARED_CREDENTIALS_FILE = authContext.AWS.CredentialsFile
  ↓     - Sets AWS_CONFIG_FILE = authContext.AWS.ConfigFile
  ↓     - Sets AWS_PROFILE = authContext.AWS.Profile
  ↓     - Sets AWS_REGION = authContext.AWS.Region
  ↓     - Clears conflicting credential env vars (AWS_ACCESS_KEY_ID, etc.)
  ↓     - Disables IMDS fallback (AWS_EC2_METADATA_DISABLED=true)
  ↓
  ↓ Line 349: tf.SetEnv(environMap) - Sets env vars on terraform executor
  ↓
  ↓ Line 377: tf.Init() - Executes "terraform init" with AWS credentials
  ↓   - Reads backend.tf.json
  ↓   - Uses AWS profile to assume role
  ↓   - Initializes backend connection to S3
  ↓
  ↓ Line 438: tf.Output() - Executes "terraform output" with AWS credentials
  ↓   - Uses AWS profile to assume role
  ↓   - Reads state from S3 backend
  ↓   - Extracts output values
  ↓
  ↓ Returns output values
```

## Key Differences from !terraform.state

### !terraform.state

- **Direct AWS SDK usage**: Uses AWS SDK Go v2 to read state from S3
- **AuthContext usage**: Passed to AWS SDK config loader
- **Credential source**: AuthContext provides credentials directly to SDK

### !terraform.output

- **Terraform binary execution**: Spawns terraform process with environment variables
- **AuthContext usage**: Converted to environment variables (AWS_PROFILE, AWS_SHARED_CREDENTIALS_FILE, etc.)
- **Credential source**: Terraform binary reads credentials from files using AWS_PROFILE
- **Backend configuration**: Auto-generates backend.tf.json with role ARN and workspace
- **Role assumption**: Terraform binary handles role assumption using AWS SDK within the binary.

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

**File**: `internal/exec/yaml_func_terraform_output.go`
**Lines**: 102-108

```go
// Extract authContext from stackInfo if available.
var authContext *schema.AuthContext
if stackInfo != nil {
	authContext = stackInfo.AuthContext
}

value, exists, err := outputGetter.GetOutput(atmosConfig, stack, component, output, false, authContext)
```

### 3. Environment Variable Preparation

**File**: `internal/exec/terraform_output_utils.go`
**Lines**: 312-330

```go
// Add auth-based environment variables if authContext is provided.
if authContext != nil && authContext.AWS != nil {
	log.Debug("Adding auth-based environment variables",
		"profile", authContext.AWS.Profile,
		"credentials_file", authContext.AWS.CredentialsFile,
		"config_file", authContext.AWS.ConfigFile,
	)

	// Use shared AWS environment preparation helper.
	// This clears conflicting credential env vars, sets AWS_SHARED_CREDENTIALS_FILE,
	// AWS_CONFIG_FILE, AWS_PROFILE, region, and disables IMDS fallback.
	environMap = awsCloud.PrepareEnvironment(
		environMap,
		authContext.AWS.Profile,
		authContext.AWS.CredentialsFile,
		authContext.AWS.ConfigFile,
		authContext.AWS.Region,
	)
}
```

### 4. Terraform Execution with Credentials

**File**: `internal/exec/terraform_output_utils.go`
**Lines**: 348-356, 377, 438

```go
// Set the environment variables in the process that executes the `tfexec` functions.
if len(environMap) > 0 {
	err = tf.SetEnv(environMap)
	if err != nil {
		return nil, err
	}
}

// Line 377: terraform init with credentials.
err = tf.Init(ctx, initOptions...)

// Line 438: terraform output with credentials.
outputMeta, outputErr = tf.Output(ctx)
```

## Why Our Fix Works for !terraform.output

✅ **AuthManager Created**: ExecuteTerraform creates AuthManager from info.Identity

✅ **AuthContext Threaded**: ProcessComponentConfig populates stackInfo.AuthContext from AuthManager.

✅ **YAML Function Access**: processTagTerraformOutputWithContext extracts authContext from stackInfo

✅ **Environment Variables Set**: execTerraformOutput uses authContext to set AWS env vars

✅ **Terraform Binary Uses Credentials**: The terraform binary reads AWS credentials from the environment variables and
files

## Testing Verification

To verify the fix works for `!terraform.output`:

```bash
# Component config with !terraform.output function
# stacks/catalog/runs-on/cloudposse.yaml
components:
  terraform:
    runs-on/cloudposse:
      vars:
        subnet_ids: !terraform.output vpc public_subnet_ids

# Execute with --identity flag
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --identity core-identity/managers-team-access

# Expected behavior:
# 1. AuthManager created from --identity flag
# 2. AuthContext populated in stackInfo
# 3. !terraform.output receives AuthContext
# 4. execTerraformOutput sets AWS_PROFILE, AWS_SHARED_CREDENTIALS_FILE, etc.
# 5. terraform init and terraform output execute with proper credentials
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
│  !terraform.output vpc subnet_ids                                │
│  - Has access to stackInfo.AuthContext                           │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                  execTerraformOutput()                           │
│  - Receives authContext parameter                                │
│  - Generates backend.tf.json with role ARN                       │
│  - Converts authContext to environment variables:                │
│    • AWS_PROFILE = authContext.AWS.Profile                       │
│    • AWS_SHARED_CREDENTIALS_FILE = authContext.AWS.CredentialsFile │
│    • AWS_CONFIG_FILE = authContext.AWS.ConfigFile                │
│    • AWS_REGION = authContext.AWS.Region                         │
│    • AWS_EC2_METADATA_DISABLED = true (disable IMDS)             │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                  Terraform Binary Execution                      │
│  terraform init:                                                 │
│  - Reads backend.tf.json                                         │
│  - Uses AWS_PROFILE to read credentials from AWS_SHARED_CREDENTIALS_FILE │
│  - Assumes role specified in backend config                      │
│  - Connects to S3 backend                                        │
│                                                                   │
│  terraform output:                                               │
│  - Uses same AWS credentials from environment                    │
│  - Reads state from S3                                           │
│  - Returns output values                                         │
└───────────────────────────────────────────────────────────────────┘
```

## Conclusion

Our fix **correctly handles both** `!terraform.state` and `!terraform.output` YAML functions:

1. **Common path**: Both functions receive AuthContext through stackInfo from ProcessComponentConfig
2. **Divergent execution**:
   - `!terraform.state`: Uses AuthContext directly with AWS SDK to read S3
   - `!terraform.output`: Converts AuthContext to environment variables for terraform binary

The key insight is that **both authentication methods rely on stackInfo.AuthContext being populated**, which our fix
ensures by threading AuthManager through ProcessComponentConfig.
