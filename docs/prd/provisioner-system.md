# PRD: Provisioner System

**Status:** Draft for Review
**Version:** 1.0
**Last Updated:** 2025-11-19
**Author:** Erik Osterman

---

## Executive Summary

The Provisioner System provides a generic, self-registering infrastructure for automatically provisioning resources needed by Atmos components. Provisioners declare when they need to run by registering themselves with specific hook events, creating a decentralized and extensible architecture.

**Key Principle:** Each provisioner knows its own requirements and timing - the system provides discovery and execution infrastructure.

---

## Overview

### What is a Provisioner?

A provisioner is a self-contained module that:
1. **Detects** if provisioning is needed (checks configuration)
2. **Provisions** required infrastructure (creates resources)
3. **Self-registers** with the hook system (declares timing)

### Core Capabilities

- **Self-Registration**: Provisioners declare when they need to run
- **Hook Integration**: Leverages existing hook system infrastructure
- **AuthContext Support**: Receives authentication from component config
- **Extensibility**: New provisioners added via simple registration
- **Discoverability**: Hook system queries "what runs at this event?"

### First Implementation

**Backend Provisioner** - Automatically provisions Terraform state backends (S3, GCS, Azure) before `terraform init`. See `backend-provisioner.md` for details.

---

## Architecture

### Provisioner Registration Pattern

```go
// pkg/provisioner/provisioner.go

package provisioner

import (
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Provisioner defines a self-registering provisioner.
type Provisioner struct {
	Type      string           // Provisioner type ("backend", "component", etc.)
	HookEvent hooks.HookEvent  // When to run (self-declared)
	Func      ProvisionerFunc  // What to run
}

// ProvisionerFunc is the function signature for all provisioners.
type ProvisionerFunc func(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,
) error

// Global registry: hook event → list of provisioners
var provisionersByEvent = make(map[hooks.HookEvent][]Provisioner)

// RegisterProvisioner allows provisioners to self-register.
func RegisterProvisioner(p Provisioner) {
	provisionersByEvent[p.HookEvent] = append(
		provisionersByEvent[p.HookEvent],
		p,
	)
}

// GetProvisionersForEvent returns all provisioners registered for a hook event.
func GetProvisionersForEvent(event hooks.HookEvent) []Provisioner {
	if provisioners, ok := provisionersByEvent[event]; ok {
		return provisioners
	}
	return []Provisioner{}
}
```

### Self-Registration Example

```go
// pkg/provisioner/backend/backend.go

package backend

import (
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/provisioner"
)

func init() {
	// Backend provisioner declares: "I need to run before terraform.init"
	provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "backend",
		HookEvent: hooks.BeforeTerraformInit,  // Self-declared timing
		Func:      ProvisionBackend,
	})
}

func ProvisionBackend(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,
) error {
	// Check if provision.backend.enabled
	if !isBackendProvisioningEnabled(componentSections) {
		return nil
	}

	// Provision backend (see backend-provisioner.md)
	return provisionBackendInfrastructure(atmosConfig, componentSections, authContext)
}
```

---

## Hook System Integration

### System Hook Execution

```go
// pkg/hooks/system_hooks.go

// ExecuteProvisionerHooks triggers all provisioners registered for a hook event.
func ExecuteProvisionerHooks(
	event HookEvent,
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
) error {
	provisioners := provisioner.GetProvisionersForEvent(event)

	for _, p := range provisioners {
		// Check if this provisioner is enabled in component config
		if shouldRunProvisioner(p.Type, stackInfo.ComponentSections) {
			ui.Info(fmt.Sprintf("Running %s provisioner...", p.Type))

			if err := p.Func(
				atmosConfig,
				&stackInfo.ComponentSections,
				stackInfo.AuthContext,
			); err != nil {
				return fmt.Errorf("provisioner '%s' failed: %w", p.Type, err)
			}
		}
	}

	return nil
}

// shouldRunProvisioner checks if provisioner is enabled in configuration.
func shouldRunProvisioner(provisionerType string, componentSections map[string]any) bool {
	provisionConfig, ok := componentSections["provision"].(map[string]any)
	if !ok {
		return false
	}

	typeConfig, ok := provisionConfig[provisionerType].(map[string]any)
	if !ok {
		return false
	}

	enabled, ok := typeConfig["enabled"].(bool)
	return ok && enabled
}
```

### Integration with Terraform Execution

```go
// internal/exec/terraform.go

func ExecuteTerraform(atmosConfig, stackInfo, ...) error {
	// 1. Auth setup (existing system hook)
	if err := auth.TerraformPreHook(atmosConfig, stackInfo); err != nil {
		return err
	}

	// 2. Provisioner system hooks (NEW)
	if err := hooks.ExecuteProvisionerHooks(
		hooks.BeforeTerraformInit,
		atmosConfig,
		stackInfo,
	); err != nil {
		return err
	}

	// 3. User-defined hooks (existing)
	if err := hooks.ExecuteUserHooks(hooks.BeforeTerraformInit, ...); err != nil {
		return err
	}

	// 4. Terraform execution
	return terraform.Init(...)
}
```

---

## Configuration Schema

### Stack Manifest Configuration

```yaml
# stacks/dev/us-east-1.yaml
components:
  terraform:
    vpc:
      # Component authentication
      auth:
        providers:
          aws:
            type: aws-sso
            identity: dev-admin

      # Provisioning configuration
      provision:
        backend:           # Backend provisioner
          enabled: true

        # Future provisioners:
        # component:       # Component provisioner
        #   vendor: true
        # network:         # Network provisioner
        #   vpc: true
```

### Schema Structure

```yaml
provision:
  <provisioner-type>:
    enabled: boolean
    # Provisioner-specific configuration
```

**Key Points:**
- `provision` block contains all provisioner configurations
- Each provisioner type has its own sub-block
- Provisioners check their own `enabled` flag
- Provisioner-specific options defined by implementation

---

## AuthContext Integration

### Authentication Flow

```
Component Definition (stack manifest)
  ↓
auth.providers.aws.identity: "dev-admin"
  ↓
TerraformPreHook (auth system hook)
  ↓
AuthContext populated with credentials
  ↓
ProvisionerHooks (receives AuthContext)
  ↓
Provisioners use AuthContext for cloud operations
```

### AuthContext Usage in Provisioners

```go
func ProvisionBackend(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,  // Populated by auth system
) error {
	// Extract backend configuration
	backendConfig := (*componentSections)["backend"].(map[string]any)
	region := backendConfig["region"].(string)

	// Load AWS config using authContext (from component's identity)
	cfg, err := awsUtils.LoadAWSConfigWithAuth(
		ctx,
		region,
		"",  // roleArn from backend config (if needed)
		15*time.Minute,
		authContext.AWS,  // Credentials from component's auth.identity
	)

	// Use config for provisioning
	client := s3.NewFromConfig(cfg)
	return provisionBucket(client, bucket)
}
```

### Identity Inheritance

**Provisioners inherit the component's identity:**
- Component defines `auth.providers.aws.identity: "dev-admin"`
- Auth system populates `AuthContext`
- Provisioners receive `AuthContext` automatically
- No separate provisioning identity needed

**Role assumption** (if needed) extracted from provisioner-specific config:
- Backend: `backend.assume_role.role_arn`
- Component: `component.source.assume_role` (hypothetical)
- Each provisioner defines its own role assumption pattern

---

## Package Structure

```
pkg/provisioner/
  ├── provisioner.go           # Core registry and types
  ├── provisioner_test.go      # Registry tests
  ├── backend/                 # Backend provisioner
  │   ├── backend.go           # Backend provisioner implementation
  │   ├── backend_test.go
  │   ├── registry.go          # Backend-specific registry (S3, GCS, Azure)
  │   ├── s3.go                # S3 backend provisioner
  │   ├── gcs.go               # GCS backend provisioner (future)
  │   └── azurerm.go           # Azure backend provisioner (future)
  └── component/               # Future: Component provisioner
      └── component.go

pkg/hooks/
  ├── event.go                 # Hook event constants
  ├── system_hooks.go          # ExecuteProvisionerHooks()
  └── hooks_test.go
```

---

## Adding a New Provisioner

### Step 1: Define Provisioner Logic

```go
// pkg/provisioner/myprovisioner/myprovisioner.go

package myprovisioner

import (
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

func init() {
	// Self-register with hook system
	provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "myprovisioner",
		HookEvent: hooks.BeforeComponentLoad,  // Declare when to run
		Func:      ProvisionMyResource,
	})
}

func ProvisionMyResource(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,
) error {
	// 1. Check if enabled
	if !isEnabled(componentSections, "myprovisioner") {
		return nil
	}

	// 2. Extract configuration
	config := extractConfig(componentSections)

	// 3. Provision resource
	return provisionResource(config, authContext)
}

func isEnabled(componentSections *map[string]any, provisionerType string) bool {
	provisionConfig, ok := (*componentSections)["provision"].(map[string]any)
	if !ok {
		return false
	}

	typeConfig, ok := provisionConfig[provisionerType].(map[string]any)
	if !ok {
		return false
	}

	enabled, ok := typeConfig["enabled"].(bool)
	return ok && enabled
}
```

### Step 2: Import Provisioner Package

```go
// cmd/root.go or appropriate location

import (
	_ "github.com/cloudposse/atmos/pkg/provisioner/backend"      // Backend provisioner
	_ "github.com/cloudposse/atmos/pkg/provisioner/myprovisioner" // Your provisioner
)
```

### Step 3: Configure in Stack Manifest

```yaml
provision:
  myprovisioner:
    enabled: true
    # Provisioner-specific configuration
```

**That's it!** The provisioner is now active.

---

## Hook Events

### Existing Hook Events (for reference)

```go
// pkg/hooks/event.go

const (
	BeforeTerraformPlan  HookEvent = "before.terraform.plan"
	AfterTerraformPlan   HookEvent = "after.terraform.plan"
	BeforeTerraformApply HookEvent = "before.terraform.apply"
	AfterTerraformApply  HookEvent = "after.terraform.apply"
)
```

### New Hook Events for Provisioners

```go
const (
	// Provisioner hook events
	BeforeTerraformInit  HookEvent = "before.terraform.init"
	AfterTerraformInit   HookEvent = "after.terraform.init"
	BeforeComponentLoad  HookEvent = "before.component.load"
	AfterComponentLoad   HookEvent = "after.component.load"
)
```

**Provisioners can register for any hook event** - the system is fully extensible.

---

## Testing Strategy

### Unit Tests

**Registry Tests:**
```go
func TestRegisterProvisioner(t *testing.T)
func TestGetProvisionersForEvent(t *testing.T)
func TestGetProvisionersForEvent_NoProvisioners(t *testing.T)
func TestMultipleProvisionersForSameEvent(t *testing.T)
```

**Hook Integration Tests:**
```go
func TestExecuteProvisionerHooks(t *testing.T)
func TestExecuteProvisionerHooks_ProvisionerDisabled(t *testing.T)
func TestExecuteProvisionerHooks_ProvisionerFails(t *testing.T)
func TestExecuteProvisionerHooks_MultipleProvisioners(t *testing.T)
```

### Integration Tests

```go
func TestProvisionerSystemIntegration(t *testing.T) {
	// Register test provisioner
	provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "test",
		HookEvent: hooks.BeforeTerraformInit,
		Func: func(atmosConfig, componentSections, authContext) error {
			// Test provisioning logic
			return nil
		},
	})

	// Execute hook system
	err := hooks.ExecuteProvisionerHooks(
		hooks.BeforeTerraformInit,
		atmosConfig,
		stackInfo,
	)

	assert.NoError(t, err)
}
```

---

## Future Provisioner Types

### Component Provisioner

**Purpose:** Auto-vendor components from remote sources

**Hook Event:** `before.component.load`

**Configuration:**
```yaml
provision:
  component:
    vendor: true
    source: "github.com/cloudposse/terraform-aws-components//modules/vpc"
```

**Implementation:**
```go
func init() {
	provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "component",
		HookEvent: hooks.BeforeComponentLoad,
		Func:      ProvisionComponent,
	})
}
```

### Network Provisioner

**Purpose:** Auto-create VPCs/networks for testing

**Hook Event:** `before.component.init`

**Configuration:**
```yaml
provision:
  network:
    vpc: true
    cidr: "10.0.0.0/16"
```

### Workflow Provisioner

**Purpose:** Auto-generate workflows from templates

**Hook Event:** `before.workflow.execute`

**Configuration:**
```yaml
provision:
  workflow:
    template: "deploy-stack"
```

---

## Performance Considerations

### Caching

Provisioners should implement client caching:
```go
var clientCache sync.Map

func getCachedClient(cacheKey string, authContext) (Client, error) {
	if cached, ok := clientCache.Load(cacheKey); ok {
		return cached.(Client), nil
	}

	client := createClient(authContext)
	clientCache.Store(cacheKey, client)
	return client, nil
}
```

### Idempotency

**All provisioners must be idempotent:**
- Check if resource exists before creating
- Return nil (no error) if already provisioned
- Safe to run multiple times

**Example:**
```go
func ProvisionResource(config, authContext) error {
	exists, err := checkResourceExists(config.Name)
	if err != nil {
		return err
	}

	if exists {
		ui.Info("Resource already exists (idempotent)")
		return nil
	}

	return createResource(config)
}
```

---

## Error Handling

### Error Patterns

```go
// Provisioner-specific errors
var (
	ErrProvisionerDisabled = errors.New("provisioner disabled")
	ErrProvisionerFailed   = errors.New("provisioner failed")
	ErrResourceExists      = errors.New("resource already exists")
)

// Use error builder for detailed errors
func ProvisionResource() error {
	return errUtils.Build(errUtils.ErrProvisionerFailed).
		WithHint("Verify credentials have required permissions").
		WithContext("provisioner", "backend").
		WithContext("resource", "s3-bucket").
		WithExitCode(2).
		Err()
}
```

---

## Security Considerations

### AuthContext Requirements

1. **Provisioners MUST use AuthContext** for cloud operations
2. **Never use ambient credentials** (environment variables, instance metadata)
3. **Respect component's identity** - don't override auth
4. **Role assumption** extracted from provisioner-specific config

### Least Privilege

Provisioners should:
- Document required IAM permissions
- Request minimal permissions
- Fail gracefully on permission denied
- Provide clear error messages with required permissions

---

## Documentation Requirements

### Each Provisioner Must Provide

1. **Purpose** - What does it provision?
2. **Hook Event** - When does it run?
3. **Configuration** - What options are available?
4. **Requirements** - What permissions/dependencies needed?
5. **Examples** - Usage examples
6. **Migration** - How to migrate from manual provisioning

---

## Success Metrics

### Adoption Metrics

- Number of provisioner types implemented
- Number of components using provisioners
- Provisioner invocation frequency

### Performance Metrics

- Provisioner execution time (p50, p95, p99)
- Cache hit rate
- Error rate per provisioner type

### Quality Metrics

- Test coverage for provisioner system (target: >90%)
- Test coverage per provisioner (target: >85%)
- Number of provisioner-related issues

---

## CLI Commands for Provisioner Management

### Overview

Atmos provides dedicated CLI commands for managing provisioned resources throughout their lifecycle (SDLC):

```bash
# Provision resources explicitly
atmos provision <provisioner-type> <component> --stack <stack>

# Examples
atmos provision backend vpc --stack dev
atmos provision component app --stack prod
atmos provision network vpc --stack test
```

### Command Structure

**File:** `cmd/provision/provision.go`

```go
// cmd/provision/provision.go

package provision

import (
	"github.com/spf13/cobra"
	"github.com/cloudposse/atmos/cmd/internal/registry"
)

type ProvisionCommandProvider struct{}

func (p *ProvisionCommandProvider) ProvideCommands() []*cobra.Command {
	return []*cobra.Command{ProvisionCmd}
}

func (p *ProvisionCommandProvider) GetGroup() string {
	return "Provisioning Commands"
}

var ProvisionCmd = &cobra.Command{
	Use:   "provision <type> <component> --stack <stack>",
	Short: "Provision infrastructure resources",
	Long: `Provision infrastructure resources required by components.

Provisioners create infrastructure that components depend on (backends, networks, etc.).
This command allows explicit provisioning outside of the automatic hook system.

Examples:
  # Provision S3 backend for vpc component
  atmos provision backend vpc --stack dev

  # Provision network infrastructure
  atmos provision network vpc --stack test

Supported provisioner types:
  backend    - Provision Terraform state backends (S3, GCS, Azure)
  component  - Provision component dependencies (future)
  network    - Provision network infrastructure (future)
`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		provisionerType := args[0]
		component := args[1]
		stack, _ := cmd.Flags().GetString("stack")

		return exec.ExecuteProvision(provisionerType, component, stack)
	},
}

func init() {
	ProvisionCmd.Flags().StringP("stack", "s", "", "Atmos stack name (required)")
	ProvisionCmd.MarkFlagRequired("stack")

	// Register with command registry
	registry.Register(&ProvisionCommandProvider{})
}
```

### Implementation: ExecuteProvision

**File:** `internal/exec/provision.go`

```go
// internal/exec/provision.go

package exec

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ExecuteProvision provisions infrastructure for a component.
func ExecuteProvision(provisionerType, component, stack string) error {
	// 1. Load configuration and stacks
	info, err := ProcessStacks(atmosConfig, stack, component, ...)
	if err != nil {
		return fmt.Errorf("failed to process stacks: %w", err)
	}

	// 2. Setup authentication (TerraformPreHook)
	if err := auth.TerraformPreHook(info.AtmosConfig, info); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// 3. Get provisioner for type
	provisioners := provisioner.GetProvisionersForEvent(hooks.ManualProvision)
	var targetProvisioner *provisioner.Provisioner

	for _, p := range provisioners {
		if p.Type == provisionerType {
			targetProvisioner = &p
			break
		}
	}

	if targetProvisioner == nil {
		return fmt.Errorf("provisioner type '%s' not found", provisionerType)
	}

	// 4. Execute provisioner
	ui.Info(fmt.Sprintf("Provisioning %s for component '%s' in stack '%s'...",
		provisionerType, component, stack))

	if err := targetProvisioner.Func(
		info.AtmosConfig,
		&info.ComponentSections,
		info.AuthContext,
	); err != nil {
		// CRITICAL: Propagate error to caller (exit with non-zero)
		return fmt.Errorf("provisioning failed: %w", err)
	}

	ui.Success(fmt.Sprintf("Successfully provisioned %s", provisionerType))
	return nil
}
```

### Usage Examples

#### Explicit Backend Provisioning

```bash
# Provision backend before applying
atmos provision backend vpc --stack dev

# Then apply infrastructure
atmos terraform apply vpc --stack dev
```

#### Dry-Run Mode (Future Enhancement)

```bash
# Preview what would be provisioned
atmos provision backend vpc --stack dev --dry-run

# Output:
# Would create:
#   - S3 bucket: acme-terraform-state-dev
#   - Settings: versioning, encryption, public access block
#   - Tags: ManagedBy=Atmos, Purpose=TerraformState
```

#### Multiple Components

```bash
# Provision backend for multiple components
for comp in vpc eks rds; do
  atmos provision backend $comp --stack dev
done
```

### Automatic vs Manual Provisioning

**Automatic (via hooks):**
```bash
# Provisioning happens automatically before terraform init
atmos terraform apply vpc --stack dev
# → TerraformPreHook (auth)
# → ProvisionerHook (backend provision if enabled)
# → terraform init
# → terraform apply
```

**Manual (explicit command):**
```bash
# User explicitly provisions resources
atmos provision backend vpc --stack dev

# Then runs terraform separately
atmos terraform apply vpc --stack dev
```

### When to Use Manual Provisioning

1. **Separate provisioning step** - CI/CD pipelines with distinct stages
2. **Troubleshooting** - Isolate provisioning from application
3. **Batch operations** - Provision multiple backends at once
4. **Validation** - Verify provisioning without running terraform

---

## Error Handling and Propagation

### Error Handling Contract

**All provisioners MUST:**
1. Return `error` on failure (never panic)
2. Return `nil` on success or idempotent skip
3. Use wrapped errors with context
4. Provide actionable error messages

### Error Propagation Flow

```
Provisioner fails
  ↓
Returns error with context
  ↓
Hook system catches error
  ↓
Propagates to main execution
  ↓
Atmos exits with non-zero code
  ↓
CI/CD pipeline fails
```

### Implementation: Hook System Error Handling

```go
// pkg/hooks/system_hooks.go

// ExecuteProvisionerHooks triggers all provisioners and PROPAGATES ERRORS.
func ExecuteProvisionerHooks(
	event HookEvent,
	atmosConfig *schema.AtmosConfiguration,
	stackInfo *schema.ConfigAndStacksInfo,
) error {
	provisioners := provisioner.GetProvisionersForEvent(event)

	for _, p := range provisioners {
		if shouldRunProvisioner(p.Type, stackInfo.ComponentSections) {
			ui.Info(fmt.Sprintf("Running %s provisioner...", p.Type))

			// Execute provisioner
			if err := p.Func(
				atmosConfig,
				&stackInfo.ComponentSections,
				stackInfo.AuthContext,
			); err != nil {
				// CRITICAL: Return error immediately (fail fast)
				// Do NOT continue to next provisioner
				ui.Error(fmt.Sprintf("Provisioner '%s' failed", p.Type))
				return fmt.Errorf("provisioner '%s' failed: %w", p.Type, err)
			}

			ui.Success(fmt.Sprintf("Provisioner '%s' completed", p.Type))
		}
	}

	return nil
}
```

### Error Examples

**Configuration Error:**
```go
if bucket == "" {
	return fmt.Errorf("%w: bucket name is required in backend configuration",
		errUtils.ErrInvalidConfig)
}
```

**Provisioning Error:**
```go
if err := createBucket(bucket); err != nil {
	return errUtils.Build(errUtils.ErrBackendProvision).
		WithHint("Verify AWS credentials have s3:CreateBucket permission").
		WithContext("bucket", bucket).
		WithContext("region", region).
		WithExitCode(2).
		Err()
}
```

**Permission Error:**
```go
if isPermissionDenied(err) {
	return errUtils.Build(errUtils.ErrBackendProvision).
		WithHint("Required permissions: s3:CreateBucket, s3:PutBucketVersioning").
		WithHintf("Check IAM policy for identity: %s", authContext.AWS.Profile).
		WithContext("action", "CreateBucket").
		WithContext("bucket", bucket).
		WithExitCode(2).
		Err()
}
```

### Exit Codes

| Exit Code | Meaning | Example |
|-----------|---------|---------|
| 0 | Success or idempotent | Bucket already exists |
| 1 | General error | Unexpected failure |
| 2 | Configuration error | Missing required parameter |
| 3 | Permission error | IAM permission denied |
| 4 | Resource conflict | Bucket name already taken |

### Terraform Execution Flow with Error Handling

```go
// internal/exec/terraform.go

func ExecuteTerraform(atmosConfig, stackInfo, ...) error {
	// 1. Auth setup
	if err := auth.TerraformPreHook(atmosConfig, stackInfo); err != nil {
		return err  // Fail fast - auth required
	}

	// 2. Provisioner hooks
	if err := hooks.ExecuteProvisionerHooks(
		hooks.BeforeTerraformInit,
		atmosConfig,
		stackInfo,
	); err != nil {
		// CRITICAL: If provisioning fails, DO NOT continue to terraform
		ui.Error("Provisioning failed - cannot proceed with terraform")
		return fmt.Errorf("provisioning failed: %w", err)
	}

	// 3. User hooks
	if err := hooks.ExecuteUserHooks(hooks.BeforeTerraformInit, ...); err != nil {
		return err
	}

	// 4. Terraform execution (only if provisioning succeeded)
	return terraform.Init(...)
}
```

### CI/CD Integration

**GitHub Actions Example:**
```yaml
- name: Provision Backend
  run: atmos provision backend vpc --stack dev
  # If provisioning fails, pipeline stops here (exit code != 0)

- name: Apply Infrastructure
  run: atmos terraform apply vpc --stack dev
  # Only runs if previous step succeeded
```

**Error Output:**
```
Error: provisioner 'backend' failed: backend provisioning failed:
failed to create bucket: operation error S3: CreateBucket,
https response error StatusCode: 403, AccessDenied

Hint: Verify AWS credentials have s3:CreateBucket permission
Context: bucket=acme-terraform-state-dev, region=us-east-1

Exit code: 3
```

---

## Related Documents

- **[Backend Provisioner](./backend-provisioner.md)** - Backend provisioner interface and registry
- **[S3 Backend Provisioner](./s3-backend-provisioner.md)** - S3 backend implementation (reference implementation)

---

## Appendix: Complete Example

### Provisioner Implementation

```go
// pkg/provisioner/example/example.go

package example

import (
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

func init() {
	provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "example",
		HookEvent: hooks.BeforeTerraformInit,
		Func:      ProvisionExample,
	})
}

func ProvisionExample(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,
) error {
	// 1. Check if enabled
	provisionConfig, ok := (*componentSections)["provision"].(map[string]any)
	if !ok {
		return nil
	}

	exampleConfig, ok := provisionConfig["example"].(map[string]any)
	if !ok {
		return nil
	}

	enabled, ok := exampleConfig["enabled"].(bool)
	if !ok || !enabled {
		return nil
	}

	// 2. Extract configuration
	resourceName := exampleConfig["name"].(string)

	// 3. Check if already exists (idempotent)
	exists, err := checkResourceExists(resourceName, authContext)
	if err != nil {
		return fmt.Errorf("failed to check resource: %w", err)
	}

	if exists {
		ui.Info(fmt.Sprintf("Resource '%s' already exists", resourceName))
		return nil
	}

	// 4. Provision resource
	ui.Info(fmt.Sprintf("Provisioning resource '%s'...", resourceName))
	if err := createResource(resourceName, authContext); err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	ui.Success(fmt.Sprintf("Resource '%s' provisioned successfully", resourceName))
	return nil
}
```

### Stack Configuration

```yaml
components:
  terraform:
    myapp:
      auth:
        providers:
          aws:
            type: aws-sso
            identity: dev-admin

      provision:
        example:
          enabled: true
          name: "my-example-resource"
```

---

**End of PRD**

**Status:** Ready for Review
**Next Steps:**
1. Review provisioner system architecture
2. Implement core registry (`pkg/provisioner/provisioner.go`)
3. Integrate with hook system (`pkg/hooks/system_hooks.go`)
4. See `backend-provisioner.md` for first provisioner implementation
