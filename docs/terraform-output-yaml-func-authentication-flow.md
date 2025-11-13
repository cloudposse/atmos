# Authentication Flow: !terraform.output YAML Function

This document explains how authentication works with the `!terraform.output` YAML function, including identity resolution, credential management, and the execution flow through the Atmos codebase.

## How !terraform.output Works

The `!terraform.output` YAML function executes the terraform binary to retrieve outputs from Terraform state. This requires:

1. Executing `terraform init` to initialize the backend
2. Executing `terraform output` to read output values
3. Providing AWS credentials via environment variables
4. Auto-generating `backend.tf.json` with backend configuration

Unlike `!terraform.state` which uses the AWS SDK directly, `!terraform.output` spawns a terraform process and passes credentials through environment variables.

## Authentication Flow

### Overview

When you use `!terraform.output` in a component configuration:

```yaml
components:
  terraform:
    my-component:
      vars:
        subnet_ids: !terraform.output vpc public_subnet_ids
```

Atmos resolves authentication in this order:

1. **Identity specification** - Uses `--identity` flag if provided
2. **Auto-detection** - Finds default identity from configuration
3. **Interactive selection** - Prompts user if no defaults (TTY mode only)
5. **Explicit disable** - Respects `--identity=off` to skip Atmos Auth

### Call Flow

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
YAML Function: !terraform.output vpc subnet_ids
  ↓
processTagTerraformOutputWithContext()
  ├─ Extracts authContext from stackInfo
  └─ Calls GetOutput(authContext)
      ↓
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

Nested functions occur when a component's configuration contains `!terraform.output` (or `!terraform.state`) functions that reference other components, which themselves also contain `!terraform.output` or `!terraform.state` functions in their configurations.

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
        # Level 2: This also has !terraform.output functions
        vpc_id: !terraform.output vpc vpc_id
        subnet_ids: !terraform.output vpc private_subnet_ids
```

When Atmos evaluates the Level 1 function, it needs to read the `backend-service` component configuration to execute `terraform output`. When that configuration is processed, it encounters the Level 2 functions, which must also be evaluated with proper authentication.

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
  └─ Evaluates nested !terraform.output functions
      ↓
Level 4: !terraform.output vpc vpc_id
  ├─ Extracts authContext from stackInfo ✅ AuthContext available!
  ├─ Converts AuthContext to environment variables
  ├─ Executes terraform output with authenticated credentials
  └─ Successfully retrieves output value
```

**Key Points:**

1. **AuthManager is stored** in `configAndStacksInfo.AuthManager` at the top level
2. **AuthManager is passed** through `GetTerraformOutput()` to `ExecuteDescribeComponent()`
3. **AuthContext is populated** at each level from the AuthManager
4. **All nested levels** use the same authenticated session

This ensures that deeply nested component configurations can execute `terraform output` with proper credentials without requiring separate authentication at each level.

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

Each service component may have `!terraform.output` functions reading VPC, database, or cache configurations.

#### Scenario 2: Infrastructure Layering

```yaml
# Application tier references platform tier
app-component:
  vars:
    database_url: !terraform.output database connection_string
    cache_endpoint: !terraform.output redis endpoint

# Platform tier references network tier
database:
  vars:
    subnet_ids: !terraform.output vpc database_subnet_ids
    security_group_id: !terraform.output vpc database_sg_id
```

The `app-component` evaluation triggers `database` evaluation, which triggers `vpc` evaluation.

#### Scenario 3: Multi-Region Deployments

```yaml
# Global load balancer references regional backends
global-lb:
  vars:
    backends:
      - endpoint: !terraform.output app us-east-1 lb_dns_name
      - endpoint: !terraform.output app us-west-2 lb_dns_name
      - endpoint: !terraform.output app eu-west-1 lb_dns_name
```

Each regional `app` component may have nested `!terraform.output` functions for regional resources.

### Authentication Behavior for Nested Functions

All nested function evaluations inherit the same AuthManager and credentials from the top-level command execution. This means:

- **Single authentication session** - All nested components use the same authenticated identity
- **Consistent credentials** - Environment variables set once and used by all nested terraform executions
- **Transitive permissions** - The identity must have access to all resources across nested components

If different components require different credentials, use component-level auth configuration to specify different default identities. Note that during a single command execution, all nested evaluations will use the top-level credentials.

### Performance Considerations

When using nested `!terraform.output` functions:

- Each `!terraform.output` spawns a terraform process (init + output)
- Consider using `!terraform.state` when possible (faster, no subprocess)
- Cache results are shared across nested evaluations
- Deep nesting (Level 4+) can impact performance
- Consider flattening dependencies for complex scenarios

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
   - Document which components reference others via `!terraform.output`
   - Use `settings.depends_on` to declare explicit dependencies
   - This helps with deployment ordering and troubleshooting

5. **Cache Optimization**
   - Nested function results are cached to avoid redundant terraform executions
   - Cache is per-component per-stack combination
   - Use `skipCache: false` (default) for better performance

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

### Use Cases

**Different identities per environment:**
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

**Component-specific permissions:**
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

### 3. Environment Variable Preparation

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

The terraform binary:
1. Reads `backend.tf.json` for backend configuration
2. Uses `AWS_PROFILE` to find credentials in `AWS_SHARED_CREDENTIALS_FILE`
3. Assumes the role specified in backend config
4. Connects to S3 backend
5. Reads state and extracts output values

## Differences from !terraform.state

| Aspect | !terraform.state | !terraform.output |
|--------|------------------|-------------------|
| **Execution** | AWS SDK Go v2 directly | Terraform binary subprocess |
| **Credentials** | SDK config from AuthContext | Environment variables |
| **Speed** | Faster (no subprocess) | Slower (process spawn) |
| **Backend config** | Read from component metadata | Auto-generated backend.tf.json |
| **Role assumption** | SDK handles it | Terraform binary handles it |

Both functions receive AuthContext through `stackInfo` and use it to access AWS credentials.

## Error Handling

Common authentication errors:

1. **No credentials available** - Occurs when AuthContext is not provided and external auth fails
   ```
   failed to execute terraform: no credentials available
   ```

2. **Invalid credentials** - Terraform cannot authenticate to S3 backend
   ```
   Error: error configuring S3 Backend: InvalidAccessKeyId
   ```

3. **Missing S3 permissions** - IAM role lacks required permissions
   ```
   Error: AccessDenied: Access Denied
   ```

4. **Backend initialization fails** - Cannot connect to S3 backend
   ```
   Error: Backend initialization required
   ```

5. **Output not found** - Requested output doesn't exist in state
   ```
   The output variable requested could not be found
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
      # ... configuration
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

Enable debug logging to see credential resolution:

```bash
atmos terraform plan component -s stack --logs-level=Debug
ATMOS_LOGS_LEVEL=Debug atmos terraform plan component -s stack
```
