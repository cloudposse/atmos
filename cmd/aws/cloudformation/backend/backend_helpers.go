package backend

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=backend_helpers.go -destination=mock_backend_helpers_test.go -package=backend

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	pkgcfn "github.com/cloudposse/atmos/pkg/component/aws/cloudformation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ConfigInitializer abstracts CFN configuration/auth setup and component
// description for testability. It uses the SDK-native aws/cloudformation
// component's own auth idiom — e.SetupComponentAuthForCLI + e.PropagateAuth,
// the same helpers cmd/aws/cloudformation/list.go already calls — rather than
// cmd/terraform/backend's InitConfigAndAuth (auth.MergeComponentAuthFromConfig
// + CreateAndAuthenticateManagerWithAtmosConfigForStack), since CFN already
// has its own established idiom for this in the same command group.
type ConfigInitializer interface {
	// InitConfigAndAuth loads Atmos config and authenticates the active
	// identity for component/stack, returning a ConfigAndStacksInfo carrying
	// the resulting AuthManager/AuthContext (via e.PropagateAuth).
	InitConfigAndAuth(component, stack, identity string) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, error)
	// DescribeComponent returns the real stack-configured component section
	// (including provision.targets) for component/stack, authenticated via
	// info's AuthManager.
	DescribeComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, component, stack string) (map[string]any, error)
	// DescribeComponentStatic returns the component section for component/stack
	// without authenticating and without evaluating templates or YAML functions,
	// for dry runs that must make no AWS calls. It fails for a component or stack
	// that does not exist.
	DescribeComponentStatic(component, stack string) (map[string]any, error)
}

type defaultConfigInitializer struct{}

// InitConfigAndAuth loads CLI configuration and authenticates the selected component scope.
func (d *defaultConfigInitializer) InitConfigAndAuth(component, stack, identity string) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, error) {
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
		Identity:         cfg.NormalizeIdentityValue(identity),
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return nil, nil, errors.Join(errUtils.ErrFailedToInitConfig, err)
	}

	authManager, err := e.SetupComponentAuthForCLI(&atmosConfig, &info)
	if err != nil {
		return nil, nil, err
	}
	e.PropagateAuth(&info, authManager)

	return &atmosConfig, &info, nil
}

// DescribeComponent resolves component templates using the authenticated caller without evaluating YAML functions.
func (d *defaultConfigInitializer) DescribeComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, component, stack string) (map[string]any, error) {
	var authManager auth.AuthManager
	if info != nil {
		authManager, _ = info.AuthManager.(auth.AuthManager)
	}
	return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		AtmosConfig:          atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: false,
		AuthManager:          authManager,
	})
}

// DescribeComponentStatic reads the manifest without authentication or dynamic evaluation for dry runs.
func (d *defaultConfigInitializer) DescribeComponentStatic(component, stack string) (map[string]any, error) {
	info := schema.ConfigAndStacksInfo{ComponentFromArg: component, Stack: stack}
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return nil, errors.Join(errUtils.ErrFailedToInitConfig, err)
	}
	return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		AtmosConfig:          &atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
	})
}

// CreateBackendParams contains parameters for the CreateBackend/UpdateBackend operations.
type CreateBackendParams struct {
	RequestedIdentity string
	AtmosConfig       *schema.AtmosConfiguration
	Component         string
	Stack             string
	ComponentConfig   map[string]any
	AuthContext       *schema.AuthContext
	Target            string

	// targetAuth caches each target's resolved credentials for the lifetime of
	// these params, so the existence check and the provisioning step of one
	// create/update run authenticate the target once instead of twice.
	targetAuth map[string]*schema.AuthContext
}

// DeleteBackendParams contains parameters for the DeleteBackend operation.
type DeleteBackendParams struct {
	CreateBackendParams
	Force bool
}

// DescribeBackendParams contains parameters for the DescribeBackend operation.
type DescribeBackendParams struct {
	CreateBackendParams
	Format string
}

// ListBackendsParams contains parameters for the ListBackends operation.
type ListBackendsParams struct {
	RequestedIdentity string
	Stack             string
	AtmosConfig       *schema.AtmosConfiguration
	Component         string
	ComponentConfig   map[string]any
	AuthContext       *schema.AuthContext
	Format            string
}

// Provisioner abstracts backend provisioning operations for testability.
type Provisioner interface {
	CreateBackend(ctx context.Context, params *CreateBackendParams) error
	DeleteBackend(ctx context.Context, params *DeleteBackendParams) error
	DescribeBackend(ctx context.Context, params *DescribeBackendParams) error
	ListBackends(ctx context.Context, params *ListBackendsParams) error
	// BackendExists reports whether the target bucket already exists — used by
	// create/update to decide whether a confirmation prompt is needed before
	// reconciling (and overwriting) an existing bucket's defaults.
	BackendExists(ctx context.Context, params *CreateBackendParams) (bool, error)
}

// defaultProvisioner implements Provisioner using production code: the
// Terraform-shape adapter in pkg/component/aws/cloudformation/backend.go,
// feeding pkg/provisioner's real (non-stub) ProvisionWithParams/
// DeleteBackendWithParams for create/update/delete, and
// pkgcfn.DescribeS3BackendTarget directly for describe/list (a real
// implementation, narrower than the still-stubbed
// provisioner.DescribeBackend/ListBackends Terraform itself uses today).
type defaultProvisioner struct{}

// CreateBackend resolves the S3 target and provisions it with that target's independent credentials.
func (d *defaultProvisioner) CreateBackend(ctx context.Context, params *CreateBackendParams) error {
	provisionSection, _ := params.ComponentConfig[cfg.ProvisionSectionName].(map[string]any)
	s3cfg, err := pkgcfn.ResolveS3BackendTarget(provisionSection, params.Target)
	if err != nil {
		return err
	}

	authContext, err := resolveBackendTargetAuth(params, s3cfg.Name)
	if err != nil {
		return err
	}

	return pkgcfn.ProvisionS3BackendTarget(ctx, &pkgcfn.ProvisionS3BackendParams{
		AtmosConfig:     params.AtmosConfig,
		Target:          s3cfg,
		ComponentConfig: params.ComponentConfig,
		AuthContext:     authContext,
		Component:       params.Component,
		Stack:           params.Stack,
	})
}

// BackendExists checks the selected bucket using the same target credentials as backend creation.
func (d *defaultProvisioner) BackendExists(ctx context.Context, params *CreateBackendParams) (bool, error) {
	provisionSection, _ := params.ComponentConfig[cfg.ProvisionSectionName].(map[string]any)
	s3cfg, err := pkgcfn.ResolveS3BackendTarget(provisionSection, params.Target)
	if err != nil {
		return false, err
	}

	authContext, err := resolveBackendTargetAuth(params, s3cfg.Name)
	if err != nil {
		return false, err
	}

	status, err := pkgcfn.DescribeS3BackendTarget(ctx, params.AtmosConfig, s3cfg, params.ComponentConfig, authContext)
	if err != nil {
		return false, err
	}
	return status.Exists, nil
}

// DeleteBackend adapts the S3 target and its credentials to the shared backend deletion service.
func (d *defaultProvisioner) DeleteBackend(ctx context.Context, params *DeleteBackendParams) error {
	provisionSection, _ := params.ComponentConfig[cfg.ProvisionSectionName].(map[string]any)
	s3cfg, err := pkgcfn.ResolveS3BackendTarget(provisionSection, params.Target)
	if err != nil {
		return err
	}

	authContext, err := resolveBackendTargetAuth(&params.CreateBackendParams, s3cfg.Name)
	if err != nil {
		return err
	}

	describeFunc := func(string, string) (map[string]any, error) {
		return pkgcfn.BuildSyntheticBackendConfig(s3cfg, params.ComponentConfig, authContext), nil
	}

	return provisioner.DeleteBackendWithParams(&provisioner.DeleteBackendParams{
		AtmosConfig:       params.AtmosConfig,
		Component:         params.Component,
		Stack:             params.Stack,
		Force:             params.Force,
		DescribeComponent: describeFunc,
		AuthContext:       authContext,
		Context:           ctx,
	})
}

// DescribeBackend reads and formats one S3 target using its own authentication scope.
func (d *defaultProvisioner) DescribeBackend(ctx context.Context, params *DescribeBackendParams) error {
	provisionSection, _ := params.ComponentConfig[cfg.ProvisionSectionName].(map[string]any)
	s3cfg, err := pkgcfn.ResolveS3BackendTarget(provisionSection, params.Target)
	if err != nil {
		return err
	}

	authContext, err := resolveBackendTargetAuth(&params.CreateBackendParams, s3cfg.Name)
	if err != nil {
		return err
	}

	status, err := pkgcfn.DescribeS3BackendTarget(ctx, params.AtmosConfig, s3cfg, params.ComponentConfig, authContext)
	if err != nil {
		return err
	}

	return renderBackendStatuses(params.Format, []*pkgcfn.S3BackendStatus{status})
}

// ListBackends reads S3 targets in name order, resolving credentials independently for each target.
// A target whose authentication or bucket check fails is rendered as an error row naming the target
// and the cause instead of hiding the healthy targets; after rendering, a non-nil error is returned
// so the exit code is non-zero if any target failed.
func (d *defaultProvisioner) ListBackends(ctx context.Context, params *ListBackendsParams) error {
	provisionSection, _ := params.ComponentConfig[cfg.ProvisionSectionName].(map[string]any)
	targets := pkgcfn.FindS3BackendTargets(provisionSection)

	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)

	statuses := make([]*pkgcfn.S3BackendStatus, 0, len(names))
	var failures []targetFailure
	for _, name := range names {
		status, err := describeListedTarget(ctx, params, name, targets[name])
		if err != nil {
			status = pkgcfn.NewS3BackendStatusError(targets[name], err)
			failures = append(failures, targetFailure{name: name, err: err})
		}
		statuses = append(statuses, status)
	}

	if err := renderBackendStatuses(params.Format, statuses); err != nil {
		return err
	}
	return backendTargetsFailedError(len(statuses), failures)
}

// targetFailure pairs a target name with the error that kept it from being inspected.
type targetFailure struct {
	name string
	err  error
}

// describeListedTarget authenticates one listed target with its own credentials and checks its bucket.
func describeListedTarget(ctx context.Context, params *ListBackendsParams, name string, s3cfg *pkgcfn.S3BackendTarget) (*pkgcfn.S3BackendStatus, error) {
	authContext, err := resolveBackendTargetAuth(&CreateBackendParams{
		AtmosConfig: params.AtmosConfig, Component: params.Component, Stack: params.Stack,
		ComponentConfig: params.ComponentConfig, AuthContext: params.AuthContext,
		RequestedIdentity: params.RequestedIdentity,
	}, name)
	if err != nil {
		return nil, err
	}
	return pkgcfn.DescribeS3BackendTarget(ctx, params.AtmosConfig, s3cfg, params.ComponentConfig, authContext)
}

// backendTargetsFailedError reports the failed targets after every row has been rendered, or nil when all
// succeeded. Each target's own error stays in the chain so callers can still match it with errors.Is.
func backendTargetsFailedError(total int, failures []targetFailure) error {
	if len(failures) == 0 {
		return nil
	}
	names := make([]string, 0, len(failures))
	causes := make([]error, 0, len(failures))
	for _, f := range failures {
		names = append(names, f.name)
		causes = append(causes, fmt.Errorf("target %q: %w", f.name, f.err))
	}
	return errUtils.Build(errUtils.ErrAwsCloudFormationBackendTargetsFailed).
		WithCause(errors.Join(causes...)).
		WithExplanationf("%d of %d targets failed: %s.", len(failures), total, strings.Join(names, ", ")).
		WithHintf("Run `atmos aws cloudformation backend describe --target %s` for details on a failed target.", names[0]).
		WithContext("failed_targets", strings.Join(names, ",")).
		WithContext("failed_count", strconv.Itoa(len(failures))).
		Err()
}

// Package-level dependencies for production use. These can be overridden in tests.
var (
	resolveTargetAuth                   = pkgcfn.ResolveTargetAuth
	configInit        ConfigInitializer = &defaultConfigInitializer{}
	prov              Provisioner       = &defaultProvisioner{}
)

// SetConfigInitializer sets the config initializer (for testing).
// If nil is passed, resets to default implementation.
func SetConfigInitializer(ci ConfigInitializer) {
	if ci == nil {
		configInit = &defaultConfigInitializer{}
		return
	}
	configInit = ci
}

// SetProvisioner sets the provisioner (for testing).
// If nil is passed, resets to default implementation.
func SetProvisioner(p Provisioner) {
	if p == nil {
		prov = &defaultProvisioner{}
		return
	}
	prov = p
}

// ResetDependencies resets dependencies to production defaults (for test cleanup).
func ResetDependencies() {
	configInit = &defaultConfigInitializer{}
	prov = &defaultProvisioner{}
}

// renderBackendStatuses writes S3 backend status entries in the requested
// format (table/yaml/json), used by both `backend describe` (one entry) and
// `backend list` (many).
func renderBackendStatuses(format string, statuses []*pkgcfn.S3BackendStatus) error {
	switch format {
	case "json":
		return data.WriteJSON(statuses)
	case "yaml", "":
		return data.WriteYAML(statuses)
	case "table":
		return renderBackendStatusesTable(statuses)
	default:
		return fmt.Errorf("%w: %q (supported: json, yaml, table)", errUtils.ErrInvalidFlagValue, format)
	}
}

// backendTableRowFormat lays out the target/bucket/region/status table.
const backendTableRowFormat = "%-20s %-30s %-14s %s\n"

// renderBackendStatusesTable writes bucket existence information or an explicit empty-target message.
func renderBackendStatusesTable(statuses []*pkgcfn.S3BackendStatus) error {
	if len(statuses) == 0 {
		return data.Writeln("No `kind: aws/s3` provision targets declared.")
	}
	if err := data.Writef(backendTableRowFormat, "TARGET", "BUCKET", "REGION", "STATUS"); err != nil {
		return err
	}
	for _, s := range statuses {
		state := "does not exist"
		switch {
		case s.Error != "":
			state = "error: " + s.Error
		case s.Exists:
			state = "exists"
		}
		if err := data.Writef(backendTableRowFormat, s.Target.Name, s.Target.Bucket, s.Region, state); err != nil {
			return err
		}
	}
	return nil
}

// resolveBackendTargetAuth isolates one bucket's credentials from component auth
// and from sibling buckets listed in the same command.
func resolveBackendTargetAuth(params *CreateBackendParams, name string) (*schema.AuthContext, error) {
	if cached, ok := params.targetAuth[name]; ok {
		return cached, nil
	}
	provision, _ := params.ComponentConfig[cfg.ProvisionSectionName].(map[string]any)
	targets, _ := provision["targets"].(map[string]any)
	block, _ := targets[name].(map[string]any)
	info := &schema.ConfigAndStacksInfo{
		Stack: params.Stack, ComponentFromArg: params.Component,
		ComponentSection: params.ComponentConfig, AuthContext: params.AuthContext,
		Identity: cfg.NormalizeIdentityValue(params.RequestedIdentity),
	}
	resolved, err := resolveTargetAuth(params.AtmosConfig, info, name, block, params.RequestedIdentity)
	if err != nil {
		return nil, err
	}
	if params.targetAuth == nil {
		params.targetAuth = make(map[string]*schema.AuthContext)
	}
	params.targetAuth[name] = resolved.AuthContext
	return resolved.AuthContext, nil
}
