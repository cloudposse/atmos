package cmd

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	comp "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ComponentResolver handles resolving component paths to component names.
type ComponentResolver interface {
	// ResolveComponentPath resolves a path-based component reference to its component name.
	ResolveComponentPath(component, stack string) (string, error)
}

// DefaultComponentResolver implements ComponentResolver using the standard Atmos config.
type DefaultComponentResolver struct{}

// ResolveComponentPath resolves a path-based component reference using Atmos configuration.
func (r *DefaultComponentResolver) ResolveComponentPath(component, stack string) (string, error) {
	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
	}, false)
	if err != nil {
		return "", err
	}

	// Extract component info from path without type checking or stack validation.
	// Validate component will detect the component type, and stack validation
	// happens later in ExecuteValidateComponentCmd after stacks are loaded.
	componentInfo, err := u.ExtractComponentInfoFromPath(&atmosConfig, component)
	if err != nil {
		pathErr := errUtils.Build(errUtils.ErrPathResolutionFailed).
			WithHintf("Failed to resolve component from path: `%s`", component).
			WithHint("Ensure the path is within configured component directories\nRun `atmos describe config` to see component base paths").
			WithContext("path", component).
			WithContext("stack", stack).
			WithContext("error", err.Error()).
			WithExitCode(2).
			Err()
		return "", pathErr
	}

	return componentInfo.FullComponent, nil
}

// validateComponentFlags extracts and validates flags from the cobra command.
type validateComponentFlags struct {
	stack string
}

// parseValidateComponentFlags extracts flags from the command.
func parseValidateComponentFlags(cmd *cobra.Command) (*validateComponentFlags, error) {
	flags := cmd.Flags()
	stack, err := flags.GetString("stack")
	if err != nil {
		return nil, err
	}

	return &validateComponentFlags{
		stack: stack,
	}, nil
}

// resolvePathBasedComponent handles path-based component resolution.
// Returns the resolved component name or the original if no resolution needed.
func resolvePathBasedComponent(component string, flags *validateComponentFlags, resolver ComponentResolver) (string, error) {
	// Check if this is a path-based component reference.
	if !comp.IsExplicitComponentPath(component) {
		return component, nil
	}

	// Validate stack flag is provided for path-based resolution.
	if flags.stack == "" {
		stackErr := errUtils.Build(errUtils.ErrMissingStack).
			WithHintf("The `--stack` flag is required when using path-based component resolution\n\nPath-based resolution needs to validate the component exists in a stack").
			WithHintf("Usage: `atmos validate component %s --stack <stack-name>`", component).
			WithContext("component_path", component).
			WithExitCode(2).
			Err()
		return "", stackErr
	}

	// Resolve the component path to component name.
	resolvedComponent, err := resolver.ResolveComponentPath(component, flags.stack)
	if err != nil {
		return "", err
	}

	return resolvedComponent, nil
}

// validateComponentCmd validates atmos components
var validateComponentCmd = &cobra.Command{
	Use:                "component",
	Short:              "Validate an Atmos component in a stack using JSON Schema or OPA policies",
	Long:               "This command validates an Atmos component within a stack using JSON Schema or OPA policies.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		handleHelpRequest(cmd, args)
		// Check Atmos configuration
		checkAtmosConfig()

		// Handle path-based component resolution
		if len(args) > 0 {
			flags, err := parseValidateComponentFlags(cmd)
			if err != nil {
				return err
			}

			resolver := &DefaultComponentResolver{}
			resolvedComponent, err := resolvePathBasedComponent(args[0], flags, resolver)
			if err != nil {
				return err
			}

			// Update args with resolved component name.
			args[0] = resolvedComponent
		}

		_, _, err := e.ExecuteValidateComponentCmd(cmd, args)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	validateComponentCmd.DisableFlagParsing = false

	AddStackCompletion(validateComponentCmd)
	validateComponentCmd.PersistentFlags().String("schema-path", "", "Specify the path to the schema file used for validating the component configuration in the given stack, supporting schema types like jsonschema or opa.")
	validateComponentCmd.PersistentFlags().String("schema-type", "", "Validate the specified component configuration in the given stack using the provided schema file path and schema type (`jsonschema` or `opa`).")
	validateComponentCmd.PersistentFlags().StringSlice("module-paths", nil, "Specify the paths to OPA policy modules or catalogs used for validating the component configuration in the given stack.")
	validateComponentCmd.PersistentFlags().Int("timeout", 0, "Validation timeout in seconds")

	err := validateComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	validateCmd.AddCommand(validateComponentCmd)
}
