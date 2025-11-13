# Authentication Flow: !terraform.state YAML Function

This document explains how authentication works with the `!terraform.state` YAML function, including how it differs from `!terraform.output` and how it uses the AWS SDK directly to read Terraform state.

## How !terraform.state Works

The `!terraform.state` YAML function reads Terraform state directly from S3 using the AWS SDK Go v2. This approach:

1. Uses AWS SDK directly (no terraform binary execution)
2. Reads backend configuration from component metadata
3. Creates S3 client with AuthContext credentials
4. Fetches state file from S3
5. Parses JSON and extracts requested attribute

This is faster than `!terraform.output` because it avoids subprocess overhead.

## Authentication Flow

### Overview

When you use `!terraform.state` in a component configuration:

```yaml
components:
  terraform:
    my-component:
      vars:
        vpc_id: !terraform.state vpc vpc_id
        subnet_ids: !terraform.state vpc public_subnet_ids
```

Atmos resolves authentication the same way as `!terraform.output`:

1. **Identity specification** - Uses `--identity` flag if provided
2. **Auto-detection** - Finds default identity from configuration
3. **Interactive selection** - Prompts user if no defaults (TTY mode only)
4. **External auth fallback** - Uses environment variables, Leapp, or IMDS
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
YAML Function: !terraform.state vpc vpc_id
  ↓
processTagTerraformStateWithContext()
  ├─ Extracts authContext from stackInfo
  └─ Calls GetState(authContext)
      ↓
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

## Nested Function Authentication

### What Are Nested Functions?

Nested functions occur when a component's configuration contains `!terraform.state` (or `!terraform.output`) functions that reference other components, which themselves also contain `!terraform.state` or `!terraform.output` functions in their configurations.

**Example:**

```yaml
# Component 1: tgw/routes (being deployed)
components:
  terraform:
    tgw/routes:
      vars:
        transit_gateway_route_tables:
          - routes:
              # Level 1: This references tgw/attachment
              - attachment_id: !terraform.state tgw/attachment transit_gateway_vpc_attachment_id

# Component 2: tgw/attachment (referenced by tgw/routes)
components:
  terraform:
    tgw/attachment:
      vars:
        # Level 2: This also has !terraform.state functions
        transit_gateway_id: !terraform.state tgw/hub core-use2-network transit_gateway_id
```

When Atmos evaluates the Level 1 function, it needs to read the `tgw/attachment` component configuration. When that configuration is processed, it encounters the Level 2 function, which must also be evaluated with proper authentication.

### How Authentication Propagates

Atmos propagates authentication through nested function evaluations using the `AuthManager`:

```text
Level 1: atmos terraform apply tgw/routes -s core-use2-network
  ├─ Creates AuthManager with core-network/terraform identity
  ├─ Stores AuthManager in configAndStacksInfo.AuthManager
  ├─ Populates configAndStacksInfo.AuthContext from AuthManager
  └─ Evaluates component configuration
      ↓
Level 2: !terraform.state tgw/attachment transit_gateway_vpc_attachment_id
  ├─ Extracts authContext and authManager from stackInfo
  ├─ Calls GetTerraformState(authContext, authManager)
  └─ ExecuteDescribeComponent(AuthManager: authManager) ✅ Passes AuthManager!
      ↓
Level 3: Processing tgw/attachment component config
  ├─ AuthManager propagated from Level 2
  ├─ Populates stackInfo.AuthContext from AuthManager
  └─ Evaluates nested !terraform.state functions
      ↓
Level 4: !terraform.state tgw/hub core-use2-network transit_gateway_id
  ├─ Extracts authContext from stackInfo ✅ AuthContext available!
  ├─ Creates AWS SDK config with authenticated credentials
  └─ Successfully reads state from S3
```

**Key Points:**

1. **AuthManager is stored** in `configAndStacksInfo.AuthManager` at the top level
2. **AuthManager is passed** through `GetTerraformState()` to `ExecuteDescribeComponent()`
3. **AuthContext is populated** at each level from the AuthManager
4. **All nested levels** use the same authenticated session

This ensures that deeply nested component configurations can access remote state without requiring separate authentication at each level.

### Common Nested Function Scenarios

#### Scenario 1: Transit Gateway Hub-Spoke Architecture

```yaml
# Hub account manages routes, needs attachment IDs from spokes
tgw/routes:
  vars:
    routes:
      - attachment_id: !terraform.state tgw/attachment spoke-stack transit_gateway_vpc_attachment_id
```

The hub account's `tgw/routes` component reads state from spoke accounts' `tgw/attachment` components.

#### Scenario 2: Service Mesh Configuration

```yaml
# API gateway reads endpoints from multiple services
api-gateway:
  vars:
    backends:
      - url: !terraform.state service-a alb_dns_name
      - url: !terraform.state service-b alb_dns_name
      - url: !terraform.state service-c alb_dns_name
```

Each service component may itself have `!terraform.state` functions for VPC configuration.

#### Scenario 3: Networking with Dependencies

```yaml
# Application component needs VPC details
app:
  vars:
    vpc_id: !terraform.state vpc vpc_id
    subnet_ids: !terraform.state vpc private_subnet_ids

# VPC component references shared services
vpc:
  vars:
    dns_servers: !terraform.state shared-services dns_server_ips
    nat_gateway_id: !terraform.state shared-services nat_gateway_id
```

The `app` component triggers evaluation of `vpc` component config, which in turn evaluates `shared-services` component config.

### Authentication Behavior for Nested Functions

All nested function evaluations inherit the same AuthManager and credentials from the top-level command execution. This means:

- **Single authentication session** - All nested components use the same authenticated identity
- **Consistent credentials** - No need to re-authenticate at each nesting level
- **Transitive permissions** - The identity must have access to all resources across nested components

If different components require different credentials, use component-level auth configuration to specify different default identities. Note that during a single command execution, all nested evaluations will use the top-level credentials.

### Best Practices for Nested Functions

1. **Use Same Account When Possible**
   - Nested functions work best when all components are in the same AWS account
   - Cross-account scenarios may require additional IAM configuration

2. **Configure Adequate Permissions**
   - Ensure the identity has access to all state buckets it needs to read
   - Include transitive dependencies (if A reads B, and B reads C, identity needs access to both B and C state buckets)

3. **Test Nested Configurations**
   - Use `atmos describe component` to test nested function resolution
   - Enable debug logging to verify authentication at each level

4. **Document Dependencies**
   - Document which components reference others via `!terraform.state`
   - Use `settings.depends_on` to declare explicit dependencies

5. **Cache Optimization**
   - Nested function results are cached to avoid redundant state reads
   - Cache is per-component per-stack combination

## Identity Resolution

Identity resolution works identically to `!terraform.output`. See the [terraform-output-yaml-func-authentication-flow.md](./terraform-output-yaml-func-authentication-flow.md#identity-resolution) document for complete details.

**Summary:**

**Explicit identity:**
```bash
atmos terraform plan component -s stack --identity core-auto/terraform
```

**Auto-detected identity (single default):**
```bash
# No --identity flag needed when default configured
atmos terraform plan component -s stack
```

**Interactive selection (no defaults, TTY mode):**
```bash
# Prompts once for identity selection
atmos terraform plan component -s stack
```

**External auth (disabled or CI mode):**
```bash
# Use environment variables or external auth
atmos terraform plan component -s stack --identity=off
```

## Component-Level Auth Configuration

Component-level auth configuration works identically to `!terraform.output`. Components can define their own auth configuration that overrides global settings.

See [terraform-output-yaml-func-authentication-flow.md](./terraform-output-yaml-func-authentication-flow.md#component-level-auth-configuration) for detailed examples and use cases.

## How Credentials Flow

### 1. AuthManager Creation

**File:** `pkg/auth/manager_helpers.go`

The `CreateAndAuthenticateManager()` function handles identity resolution and authentication, returning an AuthManager with populated AuthContext containing AWS credentials.

### 2. AuthContext Population

**File:** `internal/exec/utils.go`

The `ProcessComponentConfig()` function populates `stackInfo.AuthContext` from the AuthManager:

```go
// Populate AuthContext from AuthManager if provided.
if authManager != nil {
	managerStackInfo := authManager.GetStackInfo()
	if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
		configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
	}
}
```

### 3. AWS SDK Configuration

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
```

This configuration:
- Reads credentials from Atmos-managed credential files
- Uses the specified profile for role assumption
- Sets the AWS region for API calls
- Disables IMDS fallback to prevent timeout errors

### 4. S3 State Retrieval

The function creates an S3 client and retrieves the state file:

```go
// Create S3 client with authenticated config.
s3Client := s3.NewFromConfig(cfg)

// Read state file from S3.
result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
	Bucket: &bucket,
	Key:    &key,
})

// Parse state JSON and extract attribute.
var state map[string]any
json.NewDecoder(result.Body).Decode(&state)

// Navigate to requested attribute in state.
value := extractAttribute(state, attributePath)
```

## Differences from !terraform.output

| Aspect | !terraform.state | !terraform.output |
|--------|------------------|-------------------|
| **Execution** | AWS SDK Go v2 directly | Terraform binary subprocess |
| **Credentials** | SDK config from AuthContext | Environment variables |
| **Speed** | Faster (no subprocess) | Slower (process spawn) |
| **Backend config** | Read from component metadata | Auto-generated backend.tf.json |
| **Role assumption** | SDK handles it | Terraform binary handles it |
| **State parsing** | Direct JSON parsing | Terraform output formatting |
| **Use case** | Accessing state attributes | Getting formatted outputs |

## Error Handling

Common authentication errors:

1. **No credentials available** - Occurs when AuthContext is not provided and external auth fails
   ```
   failed to read Terraform state: no credentials available
   ```

2. **Invalid credentials** - AWS SDK cannot authenticate
   ```
   operation error S3: GetObject, https response error InvalidAccessKeyId
   ```

3. **Missing S3 permissions** - IAM role lacks required permissions
   ```
   operation error S3: GetObject, https response error AccessDenied
   ```

4. **State file not found** - State doesn't exist at expected location
   ```
   operation error S3: GetObject, https response error NoSuchKey
   ```

5. **Attribute not found** - Requested attribute doesn't exist in state
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

Override defaults for components that need different credentials:

```yaml
# stacks/catalog/security.yaml
components:
  terraform:
    security-scanner:
      auth:
        identities:
          security-team:
            default: true
      vars:
        findings: !terraform.state other-security-component findings
```

### Disable Auth for Local Development

Use `--identity=off` when using external credential management:

```bash
atmos terraform plan component -s stack --identity=off
```

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
- AWS SDK falling back to IMDS on non-EC2 instance

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

### Attribute Not Found

If the attribute exists in state but isn't found:

1. Check the attribute path is correct
2. Verify the state file has the expected structure
3. Use `terraform show -json` to inspect state structure

Example state structure:
```json
{
  "resources": [{
    "type": "aws_vpc",
    "name": "main",
    "values": {
      "vpc_id": "vpc-12345",
      "cidr_block": "10.0.0.0/16"
    }
  }]
}
```

Access with:
```yaml
vpc_id: !terraform.state vpc vpc_id
```

### Authentication Works for !terraform.output but Not !terraform.state

Both functions use the same authentication system. If one works and the other doesn't:

1. Check IAM permissions - might have different requirements
2. Verify backend configuration in component metadata
3. Check state file exists in S3
4. Enable debug logging to see credential resolution

Enable debug logging:
```bash
atmos terraform plan component -s stack --logs-level=Debug
ATMOS_LOGS_LEVEL=Debug atmos terraform plan component -s stack
```
