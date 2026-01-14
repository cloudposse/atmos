package cmd

import (
	"context"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen@latest -source=interfaces.go -destination=mock_interfaces_test.go -package=cmd

// ConfigLoader loads Atmos configuration.
type ConfigLoader interface {
	// InitCliConfig initializes the CLI configuration.
	InitCliConfig(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error)
}

// ComponentDescriber describes components from stacks.
type ComponentDescriber interface {
	// DescribeComponent returns the component configuration from stack.
	DescribeComponent(component, stack string) (map[string]any, error)
}

// AuthMerger merges authentication configuration.
type AuthMerger interface {
	// MergeComponentAuth merges component auth config with global auth config.
	MergeComponentAuth(globalAuth *schema.AuthConfig, componentConfig map[string]any, atmosConfig *schema.AtmosConfiguration, sectionName string) (*schema.AuthConfig, error)
}

// AuthCreator creates authentication managers.
type AuthCreator interface {
	// CreateAuthManager creates and authenticates an auth manager.
	CreateAuthManager(identity string, authConfig *schema.AuthConfig, flagValue string) (auth.AuthManager, error)
}

// SourceProvisioner provisions component sources.
type SourceProvisioner interface {
	// Provision vendors a component source.
	Provision(ctx context.Context, params *source.ProvisionParams) error
}

// Dependencies holds all external dependencies for source commands.
type Dependencies struct {
	ConfigLoader ConfigLoader
	Describer    ComponentDescriber
	AuthMerger   AuthMerger
	AuthCreator  AuthCreator
	Provisioner  SourceProvisioner
}
