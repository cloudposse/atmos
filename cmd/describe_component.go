package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	comp "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// describeComponentCmd describes configuration for components.
var describeComponentCmd = &cobra.Command{
	Use:                "component",
	Short:              "Show configuration details for an Atmos component in a stack",
	Long:               `Display the configuration details for a specific Atmos component within a designated Atmos stack, including its dependencies, settings, and overrides.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.ExactArgs(1),
	RunE: getRunnableDescribeComponentCmd(getRunnableDescribeComponentCmdProps{
		checkAtmosConfigE:        checkAtmosConfigE,
		initCliConfig:            cfg.InitCliConfig,
		isExplicitComponentPath:  comp.IsExplicitComponentPath,
		resolveComponentFromPath: e.ResolveComponentFromPathWithoutTypeCheck,
		executeDescribeComponent: e.ExecuteDescribeComponent,
		newDescribeComponentExec: e.NewDescribeComponentExec(),
	}),
	ValidArgsFunction: ComponentsArgCompletion,
}

type getRunnableDescribeComponentCmdProps struct {
	checkAtmosConfigE        func(opts ...AtmosValidateOption) error
	initCliConfig            func(info schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
	isExplicitComponentPath  func(component string) bool
	resolveComponentFromPath func(atmosConfig *schema.AtmosConfiguration, component string, stack string) (string, error)
	executeDescribeComponent func(params *e.ExecuteDescribeComponentParams) (map[string]any, error)
	newDescribeComponentExec e.DescribeComponentCmdExec
}

func getRunnableDescribeComponentCmd(
	g getRunnableDescribeComponentCmdProps,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		if err := g.checkAtmosConfigE(); err != nil {
			return err
		}

		if len(args) != 1 {
			return errors.New("invalid arguments. The command requires one argument `component`")
		}

		flags := cmd.Flags()

		stack, err := flags.GetString("stack")
		if err != nil {
			return err
		}

		format, err := flags.GetString("format")
		if err != nil {
			return err
		}

		file, err := flags.GetString("file")
		if err != nil {
			return err
		}

		processTemplates, err := flags.GetBool("process-templates")
		if err != nil {
			return err
		}

		processYamlFunctions, err := flags.GetBool("process-functions")
		if err != nil {
			return err
		}

		query, err := flags.GetString("query")
		if err != nil {
			return err
		}

		skip, err := flags.GetStringSlice("skip")
		if err != nil {
			return err
		}

		provenance, err := flags.GetBool("provenance")
		if err != nil {
			return err
		}

		component := args[0]

		// Determine if we need path resolution.
		// Only resolve as a filesystem path if the argument explicitly indicates a path.
		// Otherwise, treat it as a component name (even if it contains slashes).
		needsPathResolution := g.isExplicitComponentPath(component)

		// Build ConfigAndStacksInfo from global flags so --base-path, --config,
		// --config-path, and --profile are preserved (matching ProcessCommandLineArgs).
		info := schema.ConfigAndStacksInfo{
			ComponentFromArg: component,
			Stack:            stack,
		}
		if bp, _ := flags.GetString("base-path"); bp != "" {
			info.BasePath = bp
		}
		if cfgFiles, _ := flags.GetStringSlice("config"); len(cfgFiles) > 0 {
			info.AtmosConfigFilesFromArg = cfgFiles
		}
		if cfgDirs, _ := flags.GetStringSlice("config-path"); len(cfgDirs) > 0 {
			info.AtmosConfigDirsFromArg = cfgDirs
		}
		if profiles, _ := flags.GetStringSlice("profile"); len(profiles) > 0 {
			info.ProfilesFromArg = profiles
		}

		// Load atmos configuration. Use processStacks=true when path resolution is needed
		// because the resolver needs StackConfigFilesAbsolutePaths to find stacks and
		// detect ambiguity.
		atmosConfig, err := g.initCliConfig(info, needsPathResolution)
		if err != nil {
			// If config loading failed and we're trying to resolve a path,
			// try to give a more specific error about the path.
			if needsPathResolution {
				// Try to determine if the path is outside component directories.
				// Since we don't have config, we can't determine base paths,
				// so we just indicate that path resolution requires valid config.
				pathErr := errUtils.Build(errUtils.ErrPathResolutionFailed).
					WithHintf("Failed to initialize config for path: `%s`\n\nPath resolution requires valid Atmos configuration", component).
					WithHint("Verify `atmos.yaml` exists in your repository root or `.atmos/` directory\nRun `atmos describe config` to validate your configuration").
					WithContext("component_arg", component).
					WithContext("stack", stack).
					WithCause(err).
					WithExitCode(2).
					Err()
				return pathErr
			}
			return errors.Join(errUtils.ErrFailedToInitConfig, err)
		}

		// Resolve path-based component arguments to component names.
		if needsPathResolution {
			// We don't know the component type yet - describe component detects it.
			// Use the full resolver with stack validation to:
			// 1. Extract the component name from the path.
			// 2. Look up which Atmos components reference this terraform folder in the stack.
			// 3. If multiple components reference the same folder, return an ambiguous path error.
			resolvedComponent, resolveErr := g.resolveComponentFromPath(&atmosConfig, component, stack)
			if resolveErr != nil {
				// Return the error directly to preserve detailed hints and exit codes.
				return resolveErr
			}
			component = resolvedComponent
		}

		// Get identity flag value.
		identityName := GetIdentityFromFlags(cmd, os.Args)
		identityExplicit := cmd.Flags().Changed(IdentityFlagName)

		// Only create auth manager when YAML functions are enabled or identity is explicitly requested via CLI flag.
		// When functions are disabled (--process-functions=false), there are no YAML functions
		// (like !terraform.state) that need auth credentials, so identity resolution is unnecessary.
		// Environment variables (ATMOS_IDENTITY) do not count as explicit — only the --identity CLI flag does.
		var authManager auth.AuthManager
		if processYamlFunctions || identityExplicit {
			// Get component-specific auth config and merge with global auth config.
			// This follows the same pattern as terraform.go to handle stack-level default identities.
			// Start with global config.
			mergedAuthConfig := auth.CopyGlobalAuthConfig(&atmosConfig.Auth)

			// Get component config to extract auth section (without processing YAML functions to avoid circular dependency).
			componentConfig, componentErr := g.executeDescribeComponent(&e.ExecuteDescribeComponentParams{
				Component:            component,
				Stack:                stack,
				ProcessTemplates:     false,
				ProcessYamlFunctions: false, // Avoid circular dependency with YAML functions that need auth.
				Skip:                 nil,
				AuthManager:          nil, // No auth manager yet - we're determining which identity to use.
			})
			if componentErr != nil {
				// If component doesn't exist, exit immediately before attempting authentication.
				// This prevents prompting for identity when the component is invalid.
				if errors.Is(componentErr, errUtils.ErrInvalidComponent) {
					return componentErr
				}
				// For other errors (e.g., permission issues), continue with global auth config.
			} else {
				// Merge component-specific auth with global auth.
				mergedAuthConfig, err = auth.MergeComponentAuthFromConfig(&atmosConfig.Auth, componentConfig, &atmosConfig, cfg.AuthSectionName)
				if err != nil {
					return err
				}
			}

			// Create and authenticate AuthManager using merged auth config.
			// Use the WithAtmosConfig variant to enable stack-level default identity loading.
			authManager, err = CreateAuthManagerFromIdentityWithAtmosConfig(identityName, mergedAuthConfig, &atmosConfig)
			if err != nil {
				return err
			}
		}

		err = g.newDescribeComponentExec.ExecuteDescribeComponentCmd(e.DescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     processTemplates,
			ProcessYamlFunctions: processYamlFunctions,
			Skip:                 skip,
			Query:                query,
			Format:               format,
			File:                 file,
			Provenance:           provenance,
			AuthManager:          authManager,
		})
		return err
	}
}

func init() {
	describeComponentCmd.DisableFlagParsing = false
	AddStackCompletion(describeComponentCmd)
	describeComponentCmd.PersistentFlags().StringP("format", "f", "yaml", "The output format")
	describeComponentCmd.PersistentFlags().String("file", "", "Write the result to the file")
	describeComponentCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	describeComponentCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
	describeComponentCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")
	describeComponentCmd.PersistentFlags().Bool("provenance", false, "Enable provenance tracking to show where configuration values originated")

	err := describeComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	describeCmd.AddCommand(describeComponentCmd)
}
