# Authentication Flow: !terraform.output YAML Function

## Overview

The `!terraform.output` YAML function has a more complex authentication flow than `!terraform.state` because it:

1. Executes the terraform binary (`terraform init` and `terraform output`)
2. Reads the generated backend configuration from `backend.tf.json`
3. Uses AWS credentials to assume roles and access remote state in S3

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
YAML function encountered: !terraform.output vpc subnet_ids
  ↓
internal/exec/yaml_func_terraform_output.go:processTagTerraformOutputWithContext()
  ↓ Extracts authContext from stackInfo.AuthContext
  ↓ Passes authContext to outputGetter.GetOutput()
  ↓
internal/exec/terraform_output_getter.go:GetOutput()
  ↓ Calls GetTerraformOutput()
  ↓
internal/exec/terraform_output_utils.go:GetTerraformOutput()
  ↓ Creates authContextWrapper from authContext
  ↓ Calls ExecuteDescribeComponent(AuthManager: authMgr)
  ↓ Calls execTerraformOutput(authContext)
  ↓
internal/exec/terraform_output_utils.go:execTerraformOutput()
  ↓ Auto-generates backend.tf.json with backend config
  ↓ Gets environment variables from parent process
  ↓ CRITICAL AUTHENTICATION STEP:
  ↓   if authContext != nil && authContext.AWS != nil:
  ↓     - Calls awsCloud.PrepareEnvironment()
  ↓     - Sets AWS_SHARED_CREDENTIALS_FILE = authContext.AWS.CredentialsFile
  ↓     - Sets AWS_CONFIG_FILE = authContext.AWS.ConfigFile
  ↓     - Sets AWS_PROFILE = authContext.AWS.Profile
  ↓     - Sets AWS_REGION = authContext.AWS.Region
  ↓     - Clears conflicting credential env vars (AWS_ACCESS_KEY_ID, etc.)
  ↓     - Disables IMDS fallback (AWS_EC2_METADATA_DISABLED=true)
  ↓
  ↓ tf.SetEnv(environMap) - Sets env vars on terraform executor
  ↓
  ↓ tf.Init() - Executes "terraform init" with AWS credentials
  ↓   - Reads backend.tf.json
  ↓   - Uses AWS profile to assume role
  ↓   - Initializes backend connection to S3
  ↓
  ↓ tf.Output() - Executes "terraform output" with AWS credentials
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

## Identity Resolution

### How Identity is Determined

Atmos resolves which identity to use in the following order:

1. **`--identity` CLI flag** (highest priority)
   ```bash
   atmos terraform plan component -s stack --identity core-auto/terraform
   ```
   - Explicitly specified identity is used
   - No auto-detection or prompting

2. **Auto-detection from default identity**
   ```bash
   atmos terraform plan component -s stack
   # No --identity flag provided
   ```
   - If exactly ONE default identity exists in config, uses it automatically
   - Checks both global `atmos.yaml` and stack-level config
   - No user interaction needed

3. **Interactive selection**
   ```bash
   atmos terraform plan component -s stack
   # No --identity flag, no defaults configured, TTY available
   ```
   - Prompts user ONCE to select from available identities
   - Selected identity cached for entire command execution
   - Only in interactive mode (TTY available, not CI)

4. **No authentication** (lowest priority)
   ```bash
   atmos terraform plan component -s stack
   # No --identity flag, no defaults, non-interactive (CI)
   ```
   - Returns nil AuthManager
   - Allows external auth mechanisms (env vars, Leapp, IMDS)
   - Backward compatible behavior

5. **Explicitly disabled**
   ```bash
   atmos terraform plan component -s stack --identity=off
   ```
   - Uses `--identity=off/false/no/0` to disable Atmos Auth
   - Allows external identity mechanisms

### Auto-Detection Details

**File**: `pkg/auth/manager_helpers.go`
**Function**: `autoDetectDefaultIdentity()`

```go
// autoDetectDefaultIdentity attempts to find and return a default identity from configuration.
// Returns empty string if no default identity is found (not an error condition).
// If multiple defaults exist and allowInteractive is true, prompts user to select.
func autoDetectDefaultIdentity(authConfig *schema.AuthConfig, allowInteractive bool) (string, error) {
    // Create temporary manager to call GetDefaultIdentity
    tempManager, err := NewAuthManager(authConfig, credStore, validator, tempStackInfo)

    // Try to get default identity (forceSelect=false, doesn't prompt)
    defaultIdentity, err := tempManager.GetDefaultIdentity(false)
    if err != nil {
        // If interactive mode is allowed, prompt user to select
        if allowInteractive {
            defaultIdentity, err = tempManager.GetDefaultIdentity(true)
            return defaultIdentity, err
        }
        return "", nil // Non-interactive - no auth
    }

    return defaultIdentity, nil
}
```

**Behavior:**
- **Single default**: Auto-detects and uses it
- **Multiple defaults** (interactive): Prompts user to choose
- **Multiple defaults** (CI): Returns nil (no auth)
- **No defaults** (interactive): Prompts user to select from all identities
- **No defaults** (CI): Returns nil (no auth)

### Identity Storage for Hooks

**File**: `internal/exec/terraform.go`
**Function**: `ExecuteTerraform()`

```go
// If AuthManager was created and identity was auto-detected (info.Identity was empty),
// store the authenticated identity back into info.Identity so that hooks can access it.
// This prevents TerraformPreHook from prompting for identity selection again.
if authManager != nil && info.Identity == "" {
    chain := authManager.GetChain()
    if len(chain) > 0 {
        // The last element in the chain is the authenticated identity.
        authenticatedIdentity := chain[len(chain)-1]
        info.Identity = authenticatedIdentity
        log.Debug("Stored authenticated identity for hooks", "identity", authenticatedIdentity)
    }
}
```

**Why this is critical:**
- Prevents double-prompting when hooks run (e.g., TerraformPreHook)
- Ensures hooks use the same identity selected/auto-detected at the start
- Maintains single source of truth for identity throughout command execution

## Component-Level Auth Configuration

Atmos supports defining auth configuration at **three levels** with component-level having the highest precedence:

1. **Global** (in `atmos.yaml`) - Lowest precedence
2. **Stack-level** (in stack YAML files) - Medium precedence
3. **Component-level** (in component section of stack YAML) - Highest precedence

### Global Auth Configuration

Defined in `atmos.yaml`:

```yaml
auth:
  identities:
    global-identity:
      default: true
      provider: aws-sso
      # ... provider config
```

### Component-Level Auth Configuration

Defined in component section of stack YAML files:

```yaml
components:
  terraform:
    my-component:
      auth:
        identities:
          component-specific-identity:
            default: true
            provider: aws-sso
            # ... provider config
      vars:
        # component vars
```

### How Component-Level Auth Works

**File**: `internal/exec/utils.go`
**Function**: `GetComponentAuthConfig()`

This function:
1. Starts with global auth config from `atmos.yaml`
2. Searches for the component in stack files
3. Extracts component-specific `auth:` section if present
4. Merges component auth config with global config
5. Returns merged config for authentication

**Key behaviors:**
- Component identities **override** global identities with the same name
- Component defaults take precedence over global defaults
- If component defines `identity-a` with `default: true` and global defines `identity-b` with `default: true`, both are in the merged config (multiple defaults scenario)
- The merged config is used by `CreateAndAuthenticateManager()` for identity resolution

### Example: Component-Specific Default

**Global config** (`atmos.yaml`):
```yaml
auth:
  identities:
    dev-identity:
      default: true
```

**Component config** (`stacks/catalog/my-component.yaml`):
```yaml
components:
  terraform:
    my-component:
      auth:
        identities:
          prod-identity:
            default: true
```

**Result when running** `atmos terraform plan my-component -s prod-stack`:
- Merged config has BOTH `dev-identity` and `prod-identity`
- Both have `default: true`
- In interactive mode: User prompted to choose between the two defaults
- In CI mode: Returns nil (no authentication) due to ambiguous defaults

### Example: Override Global Identity

**Global config**:
```yaml
auth:
  identities:
    shared-identity:
      default: false
      provider: aws-sso
```

**Component config**:
```yaml
components:
  terraform:
    my-component:
      auth:
        identities:
          shared-identity:
            default: true  # Override: make it default for this component
```

**Result**: Component configuration overrides global - `shared-identity` becomes the default for `my-component` only.

### Use Cases

1. **Different identities per environment**:
   ```yaml
   # stacks/prod.yaml
   components:
     terraform:
       app:
         auth:
           identities:
             prod-admin:
               default: true

   # stacks/dev.yaml
   components:
     terraform:
       app:
         auth:
           identities:
             dev-user:
               default: true
   ```

2. **Component-specific permissions**:
   ```yaml
   components:
     terraform:
       security-component:
         auth:
           identities:
             security-team:
               default: true

       app-component:
         auth:
           identities:
             app-team:
               default: true
   ```

3. **No auth for specific components**:
   ```yaml
   components:
     terraform:
       local-only-component:
         # No auth section - uses global config
         # If global has no defaults, no authentication
   ```

### Implementation Flow

```text
ExecuteTerraform()
  ↓
GetComponentAuthConfig(stack, component, componentType)
  ↓ Loads stacksMap via FindStacksMap()
  ↓ Searches for component in stack files
  ↓ Extracts component auth section
  ↓ Merges with global auth config
  ↓ Returns merged config
  ↓
CreateAndAuthenticateManager(identity, mergedAuthConfig)
  ↓ Uses merged config for auto-detection
  ↓ Finds default identities from both global + component
  ↓ Authenticates with resolved identity
```

## Critical Code Sections

### 1. Identity Resolution and AuthManager Creation

**File**: `pkg/auth/manager_helpers.go`
**Function**: `CreateAndAuthenticateManager()`

```go
func CreateAndAuthenticateManager(
    identityName string,
    authConfig *schema.AuthConfig,
    selectValue string,
) (AuthManager, error) {
    // Check if authentication is explicitly disabled
    if identityName == cfg.IdentityFlagDisabledValue {
        return nil, nil
    }

    // Auto-detect default identity if no identity name provided
    if identityName == "" {
        if authConfig == nil || len(authConfig.Identities) == 0 {
            return nil, nil // No auth configured
        }

        interactive := isInteractive()
        defaultIdentity, err := autoDetectDefaultIdentity(authConfig, interactive)
        if err != nil {
            return nil, err
        }

        if defaultIdentity == "" {
            return nil, nil // No authentication
        }

        identityName = defaultIdentity
    }

    // Create and authenticate with resolved identity
    authManager, err := NewAuthManager(authConfig, credStore, validator, authStackInfo)
    _, err = authManager.Authenticate(context.Background(), identityName)
    return authManager, err
}
```

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

**File**: `internal/exec/yaml_func_terraform_output.go`
**Function**: `processTagTerraformOutputWithContext()`

```go
// Extract authContext from stackInfo if available.
var authContext *schema.AuthContext
if stackInfo != nil {
	authContext = stackInfo.AuthContext
}

value, exists, err := outputGetter.GetOutput(atmosConfig, stack, component, output, false, authContext)
```

### 4. Environment Variable Preparation

**File**: `internal/exec/terraform_output_utils.go`
**Function**: `execTerraformOutput()`

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

### 5. Terraform Execution with Credentials

**File**: `internal/exec/terraform_output_utils.go`
**Function**: `execTerraformOutput()`

```go
// Set the environment variables in the process that executes the `tfexec` functions.
if len(environMap) > 0 {
	err = tf.SetEnv(environMap)
	if err != nil {
		return nil, err
	}
}

// Execute terraform init with credentials.
err = tf.Init(ctx, initOptions...)

// Execute terraform output with credentials.
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

### Test Scenario 1: Explicit `--identity` Flag

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
# 2. Authenticated identity stored in info.Identity
# 3. AuthContext populated in stackInfo
# 4. !terraform.output receives AuthContext
# 5. execTerraformOutput sets AWS_PROFILE, AWS_SHARED_CREDENTIALS_FILE, etc.
# 6. terraform init and terraform output execute with proper credentials
# 7. No IMDS timeout errors
# 8. No prompts (identity explicitly specified)
```

### Test Scenario 2: Auto-Detection of Default Identity

```yaml
# atmos.yaml or stack config
auth:
  identities:
    core-auto/terraform:
      kind: aws/permission-set
      default: true  # Mark as default
      via:
        provider: aws-sso
      principal:
        name: TerraformApplyAccess
        account:
          name: core-auto
```

```bash
# Execute WITHOUT --identity flag
atmos terraform plan runs-on/cloudposse -s core-ue2-auto

# Expected behavior:
# 1. No --identity flag provided (info.Identity is empty)
# 2. CreateAndAuthenticateManager auto-detects default identity
# 3. Authenticates with "core-auto/terraform" automatically
# 4. Stores "core-auto/terraform" in info.Identity
# 5. Rest of flow continues as scenario 1
# 6. No prompts (default auto-detected)
```

### Test Scenario 3: Interactive Selection (No Defaults)

```bash
# No --identity flag, no default identities configured, interactive terminal
atmos terraform plan runs-on/cloudposse -s core-ue2-auto

# Expected behavior:
# 1. No --identity flag provided (info.Identity is empty)
# 2. No default identities found
# 3. Interactive mode detected (TTY available)
# 4. User prompted ONCE to select from available identities
# 5. Selected identity stored in info.Identity
# 6. Rest of flow continues with selected identity
# 7. TerraformPreHook uses stored identity (no second prompt)
```

### Test Scenario 4: CI Mode (No Defaults)

```bash
# No --identity flag, no defaults, CI environment
CI=true atmos terraform plan runs-on/cloudposse -s core-ue2-auto

# Expected behavior:
# 1. No --identity flag provided
# 2. No default identities found
# 3. Non-interactive mode detected (CI=true)
# 4. CreateAndAuthenticateManager returns nil (no auth)
# 5. Command falls back to external auth (env vars, IMDS, etc.)
# 6. Backward compatible behavior
```

### Test Scenario 5: Explicitly Disabled Auth

```bash
# Disable Atmos Auth to use external identity mechanism (Leapp, env vars, etc.)
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --identity=off

# Expected behavior:
# 1. --identity=off parsed as cfg.IdentityFlagDisabledValue
# 2. CreateAndAuthenticateManager returns nil immediately
# 3. No AuthContext populated
# 4. !terraform.output functions use external auth
# 5. Allows users to manage credentials externally
```

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                     User Executes Command                        │
│  atmos terraform plan component -s stack [--identity identity]  │
│  (--identity flag is optional)                                   │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                    ExecuteTerraform()                            │
│  - Parses --identity flag (or empty if not provided)            │
│  - Calls CreateAndAuthenticateManager(info.Identity, ...)       │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│              CreateAndAuthenticateManager()                      │
│  Identity Resolution (in order):                                 │
│  1. If --identity=off → return nil (auth disabled)               │
│  2. If --identity provided → use it                              │
│  3. If no flag, check for default identity:                      │
│     - Single default → use it (auto-detect)                      │
│     - Multiple defaults (interactive) → prompt user              │
│     - Multiple defaults (CI) → return nil                        │
│     - No defaults (interactive) → prompt user                    │
│     - No defaults (CI) → return nil                              │
│  4. Authenticate with resolved identity                          │
│  5. Return AuthManager with AuthContext                          │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│              ExecuteTerraform() - Identity Storage               │
│  - If authManager != nil && info.Identity was empty:             │
│    • Extract authenticated identity from GetChain()              │
│    • Store in info.Identity for hooks                            │
│    • Log: "Stored authenticated identity for hooks"              │
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

## Authentication Requirements

Both `!terraform.state` and `!terraform.output` functions require:

1. **AuthContext populated** from AuthManager
2. **Valid AWS credentials** in Atmos-managed credential files
3. **IAM permissions** to read S3 state (s3:GetObject, s3:ListBucket)
4. **Role assumption** configured in identity provider
5. **Backend configuration** accessible in component metadata

## Error Handling

Common errors:

1. **No credentials available**: "failed to execute terraform" - Occurs when AuthContext is not provided (no `--identity` flag) and the default AWS credential chain has no valid credentials (no environment variables, no shared credentials file, IMDS timeout on non-EC2 instances)
2. **Invalid credentials**: "Error: error configuring S3 Backend" - Terraform cannot authenticate to S3 backend with provided credentials
3. **Missing S3 permissions**: "Error: AccessDenied: Access Denied" - IAM role lacks s3:GetObject permission on the state bucket
4. **Backend initialization fails**: "Error: Backend initialization required" - Terraform cannot connect to S3 backend or workspace doesn't exist
5. **Missing backend configuration**: "backend configuration not found" - Component metadata doesn't contain required backend settings
6. **Terraform binary not found**: "terraform not found in PATH" - Terraform binary is not installed or not in system PATH
7. **Output not found**: "The output variable requested could not be found" - Requested output doesn't exist in terraform state

**Note:** Missing AuthContext (no `--identity` flag) is not an error - `!terraform.output` gracefully falls back to the default AWS credential chain for backward compatibility.

All errors are properly wrapped with context for debugging.

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
