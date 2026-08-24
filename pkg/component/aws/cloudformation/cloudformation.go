package cloudformation

import (
	"context"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	// Blank import registers the "git" provision target kind so it is available
	// for delivery whenever aws/cloudformation components are executed.
	_ "github.com/cloudposse/atmos/pkg/provisioner/target/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentProvider implements the component.ComponentProvider interface for
// native aws/cloudformation components.
type ComponentProvider struct{}

// executeOperation is a seam so tests can stub execution.
var executeOperation = Execute

func init() {
	defer perf.Track(nil, "cloudformation.init")()

	if err := component.Register(&ComponentProvider{}); err != nil {
		panic(fmt.Sprintf("failed to register aws/cloudformation component provider: %v", err))
	}
}

// GetType returns the component type identifier.
func (p *ComponentProvider) GetType() string {
	defer perf.Track(nil, "cloudformation.GetType")()
	return cfg.CloudFormationComponentType
}

// GetGroup returns the component group for categorization.
func (p *ComponentProvider) GetGroup() string {
	defer perf.Track(nil, "cloudformation.GetGroup")()
	return "AWS"
}

// GetBasePath returns the base directory for aws/cloudformation components.
func (p *ComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "cloudformation.GetBasePath")()

	if atmosConfig == nil || atmosConfig.Components.CloudFormation.BasePath == "" {
		return DefaultConfig().BasePath
	}
	return atmosConfig.Components.CloudFormation.BasePath
}

// ListComponents discovers all aws/cloudformation components in a stack.
func (p *ComponentProvider) ListComponents(_ context.Context, _ string, stackConfig map[string]any) ([]string, error) {
	defer perf.Track(nil, "cloudformation.ListComponents")()

	componentsSection, ok := stackConfig["components"].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	cfnComponents, ok := componentsSection[cfg.CloudFormationComponentType].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	componentNames := make([]string, 0, len(cfnComponents))
	for name := range cfnComponents {
		componentNames = append(componentNames, name)
	}
	sort.Strings(componentNames)
	return componentNames, nil
}

// ValidateComponent validates aws/cloudformation component configuration.
func (p *ComponentProvider) ValidateComponent(config map[string]any) error {
	defer perf.Track(nil, "cloudformation.ValidateComponent")()

	return validateComponentConfig(config)
}

// Execute runs the requested subcommand for an aws/cloudformation component.
func (p *ComponentProvider) Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "cloudformation.Execute")()

	switch ctx.SubCommand {
	case "render":
		return executeOperation(ctx, OperationRender)
	case "diff", "plan":
		return executeOperation(ctx, OperationDiff)
	case "apply", "deploy":
		return executeOperation(ctx, OperationApply)
	case "delete", "destroy":
		return executeOperation(ctx, OperationDelete)
	case "validate":
		return executeOperation(ctx, OperationValidate)
	case "output", "outputs":
		return executeOperation(ctx, OperationOutput)
	default:
		return fmt.Errorf("%w: %q", errUtils.ErrInvalidSpecificAwsCloudFormationComponent, ctx.SubCommand)
	}
}

// GenerateArtifacts is a no-op for aws/cloudformation components (the template
// is self-contained; no codegen-artifact output like Terraform's `generate`).
func (p *ComponentProvider) GenerateArtifacts(_ *component.ExecutionContext) error {
	defer perf.Track(nil, "cloudformation.GenerateArtifacts")()
	return nil
}

// GetAvailableCommands returns the subcommands aws/cloudformation components support.
func (p *ComponentProvider) GetAvailableCommands() []string {
	defer perf.Track(nil, "cloudformation.GetAvailableCommands")()
	return []string{"render", "diff", "plan", "apply", "deploy", "delete", "validate", "output"}
}
