package backend

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=backend_helpers.go -destination=mock_backend_helpers_test.go -package=backend

import (
	"context"
	"errors"
	"sort"

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
}

type defaultConfigInitializer struct{}

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

// CreateBackendParams contains parameters for the CreateBackend/UpdateBackend operations.
type CreateBackendParams struct {
	AtmosConfig     *schema.AtmosConfiguration
	Component       string
	Stack           string
	ComponentConfig map[string]any
	AuthContext     *schema.AuthContext
	Target          string
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
	AtmosConfig     *schema.AtmosConfiguration
	Component       string
	ComponentConfig map[string]any
	AuthContext     *schema.AuthContext
	Format          string
}

// Provisioner abstracts backend provisioning operations for testability.
type Provisioner interface {
	CreateBackend(ctx context.Context, params *CreateBackendParams) error
	DeleteBackend(ctx context.Context, params *DeleteBackendParams) error
	DescribeBackend(ctx context.Context, params *DescribeBackendParams) error
	ListBackends(ctx context.Context, params *ListBackendsParams) error
}

// defaultProvisioner implements Provisioner using production code: the
// Terraform-shape adapter in pkg/component/aws/cloudformation/backend.go,
// feeding pkg/provisioner's real (non-stub) ProvisionWithParams/
// DeleteBackendWithParams for create/update/delete, and
// pkgcfn.DescribeS3BackendTarget directly for describe/list (a real
// implementation, narrower than the still-stubbed
// provisioner.DescribeBackend/ListBackends Terraform itself uses today).
type defaultProvisioner struct{}

func (d *defaultProvisioner) CreateBackend(_ context.Context, params *CreateBackendParams) error {
	provisionSection, _ := params.ComponentConfig[cfg.ProvisionSectionName].(map[string]any)
	s3cfg, err := pkgcfn.ResolveS3BackendTarget(provisionSection, params.Target)
	if err != nil {
		return err
	}

	describeFunc := func(string, string) (map[string]any, error) {
		return pkgcfn.BuildSyntheticBackendConfig(s3cfg, params.ComponentConfig, params.AuthContext), nil
	}

	return provisioner.ProvisionWithParams(&provisioner.ProvisionParams{
		AtmosConfig:       params.AtmosConfig,
		ProvisionerType:   "backend",
		Component:         params.Component,
		Stack:             params.Stack,
		DescribeComponent: describeFunc,
		AuthContext:       params.AuthContext,
	})
}

func (d *defaultProvisioner) DeleteBackend(_ context.Context, params *DeleteBackendParams) error {
	provisionSection, _ := params.ComponentConfig[cfg.ProvisionSectionName].(map[string]any)
	s3cfg, err := pkgcfn.ResolveS3BackendTarget(provisionSection, params.Target)
	if err != nil {
		return err
	}

	describeFunc := func(string, string) (map[string]any, error) {
		return pkgcfn.BuildSyntheticBackendConfig(s3cfg, params.ComponentConfig, params.AuthContext), nil
	}

	return provisioner.DeleteBackendWithParams(&provisioner.DeleteBackendParams{
		AtmosConfig:       params.AtmosConfig,
		Component:         params.Component,
		Stack:             params.Stack,
		Force:             params.Force,
		DescribeComponent: describeFunc,
		AuthContext:       params.AuthContext,
	})
}

func (d *defaultProvisioner) DescribeBackend(ctx context.Context, params *DescribeBackendParams) error {
	provisionSection, _ := params.ComponentConfig[cfg.ProvisionSectionName].(map[string]any)
	s3cfg, err := pkgcfn.ResolveS3BackendTarget(provisionSection, params.Target)
	if err != nil {
		return err
	}

	status, err := pkgcfn.DescribeS3BackendTarget(ctx, params.AtmosConfig, s3cfg, params.ComponentConfig, params.AuthContext)
	if err != nil {
		return err
	}

	return renderBackendStatuses(params.Format, []*pkgcfn.S3BackendStatus{status})
}

func (d *defaultProvisioner) ListBackends(ctx context.Context, params *ListBackendsParams) error {
	provisionSection, _ := params.ComponentConfig[cfg.ProvisionSectionName].(map[string]any)
	targets := pkgcfn.FindS3BackendTargets(provisionSection)

	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)

	statuses := make([]*pkgcfn.S3BackendStatus, 0, len(names))
	for _, name := range names {
		status, err := pkgcfn.DescribeS3BackendTarget(ctx, params.AtmosConfig, targets[name], params.ComponentConfig, params.AuthContext)
		if err != nil {
			return err
		}
		statuses = append(statuses, status)
	}

	return renderBackendStatuses(params.Format, statuses)
}

// Package-level dependencies for production use. These can be overridden in tests.
var (
	configInit ConfigInitializer = &defaultConfigInitializer{}
	prov       Provisioner       = &defaultProvisioner{}
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
	default:
		return renderBackendStatusesTable(statuses)
	}
}

func renderBackendStatusesTable(statuses []*pkgcfn.S3BackendStatus) error {
	if len(statuses) == 0 {
		return data.Writeln("No `kind: aws/s3` provision targets declared.")
	}
	for _, s := range statuses {
		state := "does not exist"
		if s.Exists {
			state = "exists"
		}
		if err := data.Writef("%-20s %-30s %-14s %s\n", s.Target.Name, s.Target.Bucket, s.Region, state); err != nil {
			return err
		}
	}
	return nil
}
