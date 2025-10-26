package exec

import "github.com/cloudposse/atmos/pkg/schema"

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
	)
}
