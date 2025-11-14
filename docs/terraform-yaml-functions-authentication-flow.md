# Authentication Flow: Atmos YAML Functions

This document explains how authentication works with `!terraform.state` and `!terraform.output` YAML functions,
including identity resolution, credential management, and the execution flow through the Atmos codebase.

For complete documentation on Atmos YAML functions, see:
- [Atmos YAML Functions](https://atmos.tools/functions/yaml/)
- [`!terraform.state` function](https://atmos.tools/functions/yaml/terraform.state)
- [`!terraform.output` function](https://atmos.tools/functions/yaml/terraform.output)

## Overview of Atmos YAML Functions

Atmos provides two YAML functions for reading Terraform state and outputs:

### !terraform.state

Reads Terraform state **directly from S3** using the AWS SDK Go v2:

1. Uses AWS SDK directly (no terraform binary execution)
2. Reads backend configuration from component metadata
3. Creates S3 client with AuthContext credentials
4. Fetches state file from S3
5. Parses JSON and extracts requested attribute

**Usage:**

```yaml
components:
  terraform:
    my-component:
      vars:
        vpc_id: !terraform.state vpc vpc_id
        subnet_ids: !terraform.state vpc public_subnet_ids
```

### !terraform.output

Executes the **terraform binary** as a subprocess to retrieve outputs:

1. Executes `terraform init` to initialize the backend
2. Executes `terraform output` to read output values
3. Provides AWS credentials via environment variables
4. Auto-generates `backend.tf.json` with backend configuration

**Usage:**

```yaml
components:
  terraform:
    my-component:
      vars:
        subnet_ids: !terraform.output vpc public_subnet_ids
```

### Key Differences

| Aspect              | !terraform.state             | !terraform.output              |
|---------------------|------------------------------|--------------------------------|
| **Execution**       | AWS SDK Go v2 directly       | Terraform binary subprocess    |
| **Credentials**     | SDK config from AuthContext  | Environment variables          |
| **Speed**           | Faster (no subprocess)       | Slower (process spawn)         |
| **Backend config**  | Read from component metadata | Auto-generated backend.tf.json |
| **Role assumption** | SDK handles it               | Terraform binary handles it    |
| **State parsing**   | Direct JSON parsing          | Terraform output formatting    |
| **Use case**        | Accessing state attributes   | Getting formatted outputs      |

**Authentication is identical** for both functions - they share the same AuthManager and AuthContext flow.

## Authentication Flow

### Overview

When you use either `!terraform.state` or `!terraform.output` in a component configuration, Atmos resolves
authentication in this order:

1. **Identity specification** - Uses `--identity` flag if provided
2. **Auto-detection** - Finds default identity from configuration
3. **Interactive selection** - Prompts user if no defaults (TTY mode only)
4. **External auth fallback** - Uses environment variables, Leapp, or IMDS
5. **Explicit disable** - Respects `--identity=off` to skip Atmos Auth

### Call Flow

Both functions follow the same authentication flow through the codebase:

```text
Terraform Command Execution
  ↓
ExecuteTerraform()
  ├─ Creates AuthManager from --identity flag or auto-detection
  ├─ Stores authenticated identity in info.Identity for hooks
  └─ Calls ProcessStacks(authManager)
      ↓
ProcessStacks()
  └─ Calls ProcessComponentConfig(authManager)
      ├─ Populates stackInfo.AuthContext from AuthManager
      └─ Component YAML processed with AuthContext available
          ↓
YAML Function: !terraform.state vpc vpc_id  OR  !terraform.output vpc subnet_ids
  ↓
processTagTerraformStateWithContext()  OR  processTagTerraformOutputWithContext()
  ├─ Extracts authContext from stackInfo
  └─ Calls GetState(authContext) or GetOutput(authContext)
```

After this point, the paths diverge based on the function type:

#### !terraform.state Path

```text
GetTerraformState()
  ├─ Creates AWS SDK config from authContext:
  │   • Uses AWS_SHARED_CREDENTIALS_FILE
  │   • Uses AWS_CONFIG_FILE
  │   • Uses AWS_PROFILE
  │   • Uses AWS_REGION
  │   • Disables IMDS fallback
  ├─ Creates S3 client with authenticated config
  ├─ Calls GetObject() to read state from S3
  ├─ Parses state JSON
  └─ Extracts and returns attribute value
```

#### !terraform.output Path

```text
execTerraformOutput()
  ├─ Generates backend.tf.json
  ├─ Converts AuthContext to environment variables:
  │   • AWS_PROFILE
  │   • AWS_SHARED_CREDENTIALS_FILE
  │   • AWS_CONFIG_FILE
  │   • AWS_REGION
  │   • AWS_EC2_METADATA_DISABLED=true
  ├─ tf.Init() - Terraform reads credentials from files
  └─ tf.Output() - Returns output values
```

## Nested Function Authentication

### What Are Nested Functions?

Nested functions occur when a component's configuration contains `!terraform.state` or `!terraform.output` functions
that reference other components, which themselves also contain these functions in their configurations.

**Example:**

```yaml
# Component 1: api-gateway (being deployed)
components:
  terraform:
    api-gateway:
      vars:
        backends:
          # Level 1: This references backend-service
          - endpoint: !terraform.output backend-service alb_dns_name

# Component 2: backend-service (referenced by api-gateway)
components:
  terraform:
    backend-service:
      vars:
        # Level 2: This also has !terraform.state functions
        vpc_id: !terraform.state vpc vpc_id
        subnet_ids: !terraform.state vpc private_subnet_ids
```

When Atmos evaluates the Level 1 function, it needs to read the `backend-service` component configuration. When that
configuration is processed, it encounters the Level 2 functions, which must also be evaluated with proper
authentication.

### How Authentication Propagates

Atmos propagates authentication through nested function evaluations using the `AuthManager`:

```text
Level 1: atmos terraform apply api-gateway -s production
  ├─ Creates AuthManager with prod-deploy identity
  ├─ Stores AuthManager in configAndStacksInfo.AuthManager
  ├─ Populates configAndStacksInfo.AuthContext from AuthManager
  └─ Evaluates component configuration
      ↓
Level 2: !terraform.output backend-service alb_dns_name
  ├─ Extracts authContext and authManager from stackInfo
  ├─ Calls GetTerraformOutput(authContext, authManager)
  └─ ExecuteDescribeComponent(AuthManager: authManager) ✅ Passes AuthManager!
      ↓
Level 3: Processing backend-service component config
  ├─ AuthManager propagated from Level 2
  ├─ Populates stackInfo.AuthContext from AuthManager
  └─ Evaluates nested !terraform.state functions
      ↓
Level 4: !terraform.state vpc vpc_id
  ├─ Extracts authContext from stackInfo ✅ AuthContext available!
  ├─ Creates AWS SDK config OR environment variables (depending on function type)
  └─ Successfully retrieves state/output value
```

**Key Points:**

1. **AuthManager is stored** in `configAndStacksInfo.AuthManager` at the top level
2. **AuthManager is passed** through `GetTerraformState()`/`GetTerraformOutput()` to `ExecuteDescribeComponent()`
3. **AuthContext is populated** at each level from the AuthManager
4. **All nested levels** use the same authenticated session

This ensures that deeply nested component configurations can execute terraform operations with proper credentials
without requiring separate authentication at each level.

### Common Nested Function Scenarios

#### Scenario 1: Microservices Architecture

```yaml
# API Gateway aggregates endpoints from multiple services
api-gateway:
  vars:
    service_endpoints:
      auth: !terraform.output auth-service endpoint_url
      users: !terraform.output users-service endpoint_url
      orders: !terraform.output orders-service endpoint_url
```

Each service component may have `!terraform.state` functions reading VPC, database, or cache configurations.

#### Scenario 2: Infrastructure Layering

```yaml
# Application tier references platform tier
app-component:
  vars:
    database_url: !terraform.output database connection_string
    cache_endpoint: !terraform.state redis endpoint

# Platform tier references network tier
database:
  vars:
    subnet_ids: !terraform.state vpc database_subnet_ids
    security_group_id: !terraform.state vpc database_sg_id
```

The `app-component` evaluation triggers `database` evaluation, which triggers `vpc` evaluation.

#### Scenario 3: Transit Gateway Hub-Spoke Architecture

```yaml
# Hub account manages routes, needs attachment IDs from spokes
tgw/routes:
  vars:
    routes:
      - attachment_id: !terraform.state tgw/attachment spoke-stack transit_gateway_vpc_attachment_id
```

The hub account's `tgw/routes` component reads state from spoke accounts' `tgw/attachment` components.

### Authentication Behavior for Nested Functions

All nested function evaluations inherit the same AuthManager and credentials from the top-level command execution. This
means:

- **Single authentication session** - All nested components use the same authenticated identity
- **Consistent credentials** - No need to re-authenticate at each nesting level
- **Transitive permissions** - The identity must have access to all resources across nested components

If different components require different credentials, use component-level auth configuration to specify different
default identities.

### Performance Considerations

When using nested YAML functions:

- **!terraform.output** spawns a terraform process (init + output) for each call
- **!terraform.state** is faster - no subprocess, direct S3 read via AWS SDK
- **Prefer !terraform.state** when possible for better performance
- Cache results are shared across nested evaluations
- Deep nesting (Level 4+) can impact performance
- Consider flattening dependencies for complex scenarios

### Mixing !terraform.state and !terraform.output

You can mix both function types in nested scenarios:

```yaml
# Component using both function types
my-component:
  vars:
    # Fast state read (no subprocess)
    vpc_id: !terraform.state vpc vpc_id

    # Formatted output (uses terraform binary)
    formatted_config: !terraform.output config-service json_config
```

**Authentication works the same way** for both function types when nested:

- Both receive `AuthManager` from parent context
- Both populate `AuthContext` for their own nested calls
- Mixed nesting works seamlessly (state can call output, output can call state)

### Best Practices for Nested Functions

1. **Prefer !terraform.state for Speed**

- Use `!terraform.state` instead of `!terraform.output` when possible
- Faster execution (no subprocess, no terraform init)
- Same authentication propagation behavior

2. **Configure Adequate Permissions**

- Ensure the identity has access to all resources needed across nested components
- Include transitive dependencies (if A calls B, and B calls C, identity needs permissions for all)

3. **Test Nested Configurations**

- Use `atmos describe component` to test nested function resolution
- Enable debug logging to verify authentication at each level:
  ```bash
  atmos terraform plan component -s stack --logs-level=Debug
  ```

4. **Document Dependencies**

- Document which components reference others via YAML functions
- Use `settings.depends_on` to declare explicit dependencies
- This helps with deployment ordering and troubleshooting

5. **Cache Optimization**

- Nested function results are cached to avoid redundant operations
- Cache is per-component per-stack combination
- Use `skipCache: false` (default) for better performance

## Identity Resolution

### Explicit Identity

Specify an identity explicitly with the `--identity` flag:

```bash
atmos terraform plan component -s stack --identity core-auto/terraform
```

This bypasses auto-detection and uses the specified identity directly.

### Auto-Detection

When no `--identity` flag is provided, Atmos searches for default identities in configuration:

**Global default** (in `atmos.yaml`):

```yaml
auth:
  identities:
    core-auto/terraform:
      kind: aws/permission-set
      default: true
      # ... configuration
```

**Stack-level default** (in stack manifest):

```yaml
auth:
  identities:
    prod-admin:
      default: true
```

**Behavior:**

- **Single default** - Uses it automatically
- **Multiple defaults** - Prompts user in TTY mode, fails in CI
- **No defaults** - Prompts user in TTY mode, uses external auth in CI

### Interactive Selection

In TTY mode with no default identities, Atmos prompts once:

```bash
$ atmos terraform plan component -s stack
? Select identity:
  > core-auto/terraform
    core-identity/managers-team-access
    prod-deploy
```

The selected identity is stored in `info.Identity` to prevent double-prompting from hooks.

### External Authentication

Use `--identity=off` to disable Atmos Auth and rely on external mechanisms:

```bash
atmos terraform plan component -s stack --identity=off
```

This allows:

- Environment variables (`AWS_PROFILE`, `AWS_ACCESS_KEY_ID`, etc.)
- Leapp credential management
- EC2 instance roles (IMDS)
- AWS credential files in standard locations

## Component-Level Auth Configuration

Atmos supports defining auth configuration at three levels:

1. **Global** (in `atmos.yaml`) - Applies to all components
2. **Stack-level** (in stack YAML) - Applies to stack
3. **Component-level** (in component section) - Highest precedence

### Example: Component-Specific Identity

```yaml
components:
  terraform:
    security-component:
      auth:
        identities:
          security-team:
            kind: aws/permission-set
            default: true
            via:
              provider: aws-sso
            principal:
              name: SecurityTeamAccess
      vars:
      # component variables
```

This component always uses the `security-team` identity, overriding global defaults.

### How Merging Works

The `GetComponentAuthConfig()` function:

1. Starts with global auth config from `atmos.yaml`
2. Searches for the component in stack files
3. Extracts component-specific `auth:` section if present
4. Deep merges component config with global config
5. Returns merged config for authentication

**Key behaviors:**

- Component identities override global identities with the same name
- Component defaults take precedence over global defaults

### Component-Level Auth Override in Nested Functions

When evaluating nested YAML functions, Atmos checks each component's configuration for an `auth:` section at every
nesting level:

1. **Component has `auth:` section with default identity** → Merges with global auth and creates component-specific
   AuthManager
2. **Component has no `auth:` section OR no default identity** → Inherits parent's AuthManager

This enables each nesting level to optionally override authentication while defaulting to the parent's credentials.

#### Example: Multi-Account Nested Functions

```yaml
# Global auth configuration
auth:
  identities:
    dev-account:
      default: true
      kind: aws/permission-set
      via:
        provider: aws-sso
        account: "111111111111"
        permission_set: "DevAccess"

# Component 1: Uses global auth (dev account)
components:
  terraform:
    api-gateway:
      vars:
        # Reads from backend-service which is in a different account
        backend_url: !terraform.output backend-service endpoint

# Component 2: Overrides auth for prod account access
components:
  terraform:
    backend-service:
      auth:
        identities:
          prod-account:
            default: true
            kind: aws/permission-set
            via:
              provider: aws-sso
              account: "222222222222"
              permission_set: "ProdReadOnly"
      vars:
        # This component's outputs are in prod account
        database_url: !terraform.state database connection_string
```

**Authentication flow:**

1. `api-gateway` evaluated with dev-account credentials (global default)
2. Encounters `!terraform.output backend-service` → checks for `auth:` section with default identity
3. Finds `auth:` section with default identity in `backend-service` → creates new AuthManager with prod-account
   credentials
4. `backend-service` config evaluated with prod-account credentials
5. Nested `!terraform.state database` inherits prod-account credentials from parent

#### Component-Level Auth Resolution Algorithm

**File:** `internal/exec/terraform_nested_auth_helper.go`

The `resolveAuthManagerForNestedComponent()` function implements this logic:

```go
func resolveAuthManagerForNestedComponent(
    atmosConfig *schema.AtmosConfiguration,
    component string,
    stack string,
    parentAuthManager auth.AuthManager,
) (auth.AuthManager, error) {
    // 1. Get component config WITHOUT processing templates/functions
    //    (avoids circular dependency)
    componentConfig, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
        Component:            component,
        Stack:                stack,
        ProcessTemplates:     false,
        ProcessYamlFunctions: false,
        AuthManager:          nil,
    })

    // 2. Check for auth section in component config
    authSection, hasAuthSection := componentConfig[cfg.AuthSectionName].(map[string]any)
    if !hasAuthSection || authSection == nil {
        // No auth config → inherit parent
        return parentAuthManager, nil
    }

    // 3. Check if component's auth config has a default identity
    hasDefault := hasDefaultIdentity(authSection)
    if !hasDefault {
        // No default identity → inherit parent (prevents interactive selector)
        return parentAuthManager, nil
    }

    // 4. Merge component auth with global auth
    mergedAuthConfig, err := auth.MergeComponentAuthFromConfig(
        &atmosConfig.Auth,
        componentConfig,
        atmosConfig,
        cfg.AuthSectionName,
    )

    // 5. Create and authenticate new AuthManager with merged config
    componentAuthManager, err := auth.CreateAndAuthenticateManager(
        "",
        mergedAuthConfig,
        "__NO_SELECT__",
    )

    return componentAuthManager, nil
}
```

#### Best Practices for Component-Level Auth in Nested Functions

1. **Minimize Auth Overrides**

- Use component-level auth only when necessary
- Prefer global defaults for simplicity
- Document why specific components need different credentials

2. **Test Cross-Account Access**

- Verify IAM permissions for cross-account output/state reads
- Test nested function resolution with component-level auth
- Use `atmos describe component` to verify auth resolution

3. **Cache Considerations**

- Each component-specific AuthManager creates a new authentication session
- Output/state reads are still cached per-component to avoid redundant operations
- Authentication is performed once per unique component auth configuration

4. **Debug Component-Level Auth**
   ```bash
   # Enable debug logging to see auth resolution
   ATMOS_LOGS_LEVEL=Debug atmos describe component my-component -s stack
   ```

   Look for log lines like:
   ```
   Component has auth config with default identity, creating component-specific AuthManager
   Created component-specific AuthManager identityChain=[prod-account]
   ```

#### Limitations and Considerations

- **Single top-level identity** - When using `--identity` flag, it applies at the top level only
- **Nested overrides respected** - Component-level auth in nested components is still respected
- **No re-prompting** - Interactive identity selection happens once at the top level
- **Default identity required** - Component auth override only works if the component's auth section defines a default
  identity (prevents interactive selector from showing for nested components)
- **Transitive permissions** - Top-level identity must have permissions for initial component, nested components use
  their own auth

## How Credentials Flow

### 1. AuthManager Creation

**File:** `pkg/auth/manager_helpers.go`

The `CreateAndAuthenticateManager()` function:

- Checks if auth is explicitly disabled (`--identity=off`)
- Auto-detects default identity if no identity provided
- Creates AuthManager with resolved identity
- Authenticates to populate AuthContext
- Returns AuthManager with AWS credentials

### 2. AuthContext Population

**File:** `internal/exec/utils.go`

The `ProcessComponentConfig()` function receives the AuthManager and populates `stackInfo.AuthContext`:

```go
// Populate AuthContext from AuthManager if provided.
if authManager != nil {
    managerStackInfo := authManager.GetStackInfo()
    if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
        configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
    }
}
```

This makes AuthContext available to all YAML functions during component processing.

### 3a. AWS SDK Configuration (!terraform.state)

**File:** `internal/exec/terraform_state_utils.go`

The `GetTerraformState()` function creates an AWS SDK config from AuthContext:

```go
// Create AWS config with credentials from AuthContext.
if authContext != nil && authContext.AWS != nil {
    // Use profile-based credentials from Atmos-managed files.
    cfg, err := awsConfig.LoadDefaultConfig(ctx,
        awsConfig.WithSharedCredentialsFiles(
            []string{authContext.AWS.CredentialsFile},
        ),
        awsConfig.WithSharedConfigFiles(
            []string{authContext.AWS.ConfigFile},
        ),
        awsConfig.WithSharedConfigProfile(authContext.AWS.Profile),
        awsConfig.WithRegion(authContext.AWS.Region),
        awsConfig.WithEC2IMDSClientEnableState(awsImds.ClientDisabled),
    )
}

// Create S3 client and retrieve state file.
s3Client := s3.NewFromConfig(cfg)
result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
    Bucket: &bucket,
    Key:    &key,
})
```

### 3b. Environment Variable Preparation (!terraform.output)

**File:** `internal/exec/terraform_output_utils.go`

The `execTerraformOutput()` function converts AuthContext to environment variables:

```go
// Add auth-based environment variables if authContext is provided.
if authContext != nil && authContext.AWS != nil {
    environMap = awsCloud.PrepareEnvironment(
        environMap,
        authContext.AWS.Profile,
        authContext.AWS.CredentialsFile,
        authContext.AWS.ConfigFile,
        authContext.AWS.Region,
    )
}
```

This sets:

- `AWS_PROFILE` - Profile name for credential lookup
- `AWS_SHARED_CREDENTIALS_FILE` - Path to Atmos-managed credentials
- `AWS_CONFIG_FILE` - Path to Atmos-managed config
- `AWS_REGION` - AWS region for API calls
- `AWS_EC2_METADATA_DISABLED=true` - Disables IMDS fallback

### 4. Terraform Execution

The terraform binary reads credentials from the files specified in environment variables:

```go
// Set environment variables on terraform executor.
err = tf.SetEnv(environMap)

// Execute terraform init with credentials.
err = tf.Init(ctx, initOptions...)

// Execute terraform output with credentials.
outputMeta, outputErr = tf.Output(ctx)
```

## Error Handling

Common authentication errors:

### General Errors

1. **No credentials available** - Occurs when AuthContext is not provided and external auth fails
   ```
   failed to execute terraform: no credentials available
   failed to read Terraform state: no credentials available
   ```

2. **Invalid credentials** - Cannot authenticate to AWS
   ```
   Error: error configuring S3 Backend: InvalidAccessKeyId
   operation error S3: GetObject, https response error InvalidAccessKeyId
   ```

3. **Missing S3 permissions** - IAM role lacks required permissions
   ```
   Error: AccessDenied: Access Denied
   operation error S3: GetObject, https response error AccessDenied
   ```

### !terraform.output Specific Errors

4. **Backend initialization fails** - Cannot connect to S3 backend
   ```
   Error: Backend initialization required
   ```

5. **Output not found** - Requested output doesn't exist in state
   ```
   The output variable requested could not be found
   ```

### !terraform.state Specific Errors

6. **State file not found** - State doesn't exist at expected location
   ```
   operation error S3: GetObject, https response error NoSuchKey
   ```

7. **Attribute not found** - Requested attribute doesn't exist in state
   ```
   attribute 'vpc_id' not found in terraform state
   ```

## Best Practices

### Configure Default Identities

Set default identities to avoid specifying `--identity` on every command:

```yaml
# atmos.yaml
auth:
  identities:
    dev:
      default: true
      kind: aws/permission-set
      via:
        provider: aws-sso
```

### Use Component-Level Defaults

Override defaults for specific components:

```yaml
# stacks/catalog/security.yaml
components:
  terraform:
    security-scanner:
      auth:
        identities:
          security-team:
            default: true
```

### Disable Auth for Local Development

Use `--identity=off` when using external credential management:

```bash
# Use Leapp or environment variables
atmos terraform plan component -s stack --identity=off
```

### CI/CD Configuration

In CI, either:

- Set explicit identity: `--identity ci-deploy`
- Configure default identity in `atmos.yaml`
- Use `--identity=off` with environment variables

### Prefer !terraform.state for Performance

When both functions would work:

- Use `!terraform.state` for better performance (no subprocess)
- Use `!terraform.output` only when you need terraform-formatted output

## Troubleshooting

### IMDS Timeout Errors

If you see:

```
dial tcp 169.254.169.254:80: i/o timeout
```

This means:

- No AuthContext was provided (no `--identity` flag)
- No default identity configured
- External auth failed (no env vars, no credential files)
- System falling back to IMDS on non-EC2 instance

**Solution:** Configure a default identity or use `--identity` flag.

### S3 Access Denied

If you see:

```
operation error S3: GetObject, https response error AccessDenied
```

Check that your identity has:

1. `s3:GetObject` permission on the state bucket
2. `s3:ListBucket` permission on the state bucket
3. Correct role assumption chain configured

### Debug Logging

Enable debug logging to see credential resolution:

```bash
atmos terraform plan component -s stack --logs-level=Debug
ATMOS_LOGS_LEVEL=Debug atmos terraform plan component -s stack
```

Look for log lines showing:

- Identity resolution
- AuthManager creation
- AuthContext population
- AWS credential file paths
- Component-level auth overrides
