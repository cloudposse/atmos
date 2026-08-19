package exec

import (
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StacksProcessor defines operations for processing stack manifests.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type StacksProcessor interface {
	// ExecuteDescribeStacks processes stack manifests and returns the final map of stacks and components.
	ExecuteDescribeStacks(
		atmosConfig *schema.AtmosConfiguration,
		filterByStack string,
		components []string,
		componentTypes []string,
		sections []string,
		ignoreMissingFiles bool,
		processTemplates bool,
		processYamlFunctions bool,
		includeEmptyStacks bool,
		skip []string,
		authManager auth.AuthManager,
	) (map[string]any, error)
}

// DefaultStacksProcessor provides the default implementation of StacksProcessor.
type DefaultStacksProcessor struct{}

// ExecuteDescribeStacks delegates to the package-level ExecuteDescribeStacks function.
//
//nolint:revive // Signature matches existing ExecuteDescribeStacks function
func (d *DefaultStacksProcessor) ExecuteDescribeStacks(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	processTemplates bool,
	processYamlFunctions bool,
	includeEmptyStacks bool,
	skip []string,
	authManager auth.AuthManager,
) (map[string]any, error) {
	return ExecuteDescribeStacks(
		atmosConfig,
		filterByStack,
		components,
		componentTypes,
		sections,
		ignoreMissingFiles,
		processTemplates,
		processYamlFunctions,
		includeEmptyStacks,
		skip,
		authManager,
	)
}

// ExecuteDescribeStacksScoped delegates to the package-level scoped variant,
// which lets components excluded by the tags/labels filters skip evaluation
// entirely (the early-skip gate) instead of being row-filtered afterwards.
//
//nolint:revive // Signature matches ExecuteDescribeStacksScoped.
func (d *DefaultStacksProcessor) ExecuteDescribeStacksScoped(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	processTemplates bool,
	processYamlFunctions bool,
	includeEmptyStacks bool,
	skip []string,
	authManager auth.AuthManager,
	authDisabled bool,
	tagsFilter []string,
	labelsFilter map[string]string,
) (map[string]any, error) {
	return ExecuteDescribeStacksScoped(
		atmosConfig,
		filterByStack,
		components,
		componentTypes,
		sections,
		ignoreMissingFiles,
		processTemplates,
		processYamlFunctions,
		includeEmptyStacks,
		skip,
		authManager,
		authDisabled,
		tagsFilter,
		labelsFilter,
		DescribeStacksErrorOptions{},
	)
}

// ExecuteDescribeStacksWithAuthDisabled delegates to the package-level auth-disabled variant.
//
//nolint:revive // Signature matches ExecuteDescribeStacksWithAuthDisabled.
func (d *DefaultStacksProcessor) ExecuteDescribeStacksWithAuthDisabled(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	processTemplates bool,
	processYamlFunctions bool,
	includeEmptyStacks bool,
	skip []string,
	authManager auth.AuthManager,
	authDisabled bool,
) (map[string]any, error) {
	return ExecuteDescribeStacksWithAuthDisabled(
		atmosConfig,
		filterByStack,
		components,
		componentTypes,
		sections,
		ignoreMissingFiles,
		processTemplates,
		processYamlFunctions,
		includeEmptyStacks,
		skip,
		authManager,
		authDisabled,
	)
}
