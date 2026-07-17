package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	comp "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

var describeComponentErrorModeParser *flags.StandardParser

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

// describeComponentFlags holds parsed flag values for the describe component command.
type describeComponentFlags struct {
	stack                string
	format               string
	file                 string
	processTemplates     bool
	processYamlFunctions bool
	query                string
	skip                 []string
	provenance           bool
	provenanceExplicit   bool
	errorMode            string
}

// parseDescribeComponentFlags extracts all flag values from the command.
func parseDescribeComponentFlags(cmd *cobra.Command) (describeComponentFlags, error) {
	flags := cmd.Flags()
	var f describeComponentFlags
	var err error

	if f.stack, err = flags.GetString("stack"); err != nil {
		return f, err
	}
	if f.format, err = flags.GetString("format"); err != nil {
		return f, err
	}
	if f.file, err = flags.GetString("file"); err != nil {
		return f, err
	}
	if f.processTemplates, err = flags.GetBool("process-templates"); err != nil {
		return f, err
	}
	if f.processYamlFunctions, err = flags.GetBool("process-functions"); err != nil {
		return f, err
	}
	if f.query, err = flags.GetString("query"); err != nil {
		return f, err
	}
	if f.skip, err = flags.GetStringSlice("skip"); err != nil {
		return f, err
	}
	if f.provenance, err = flags.GetBool("provenance"); err != nil {
		return f, err
	}
	// An explicit --provenance beats the `describe.provenance` config default.
	f.provenanceExplicit = flags.Changed("provenance")
	return f, nil
}

// buildConfigAndStacksInfoFromFlags constructs ConfigAndStacksInfo from global CLI flags.
func buildConfigAndStacksInfoFromFlags(cmd *cobra.Command, component, stack string) schema.ConfigAndStacksInfo {
	flags := cmd.Flags()
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
	return info
}

// resolveAuthManagerParams groups parameters for resolveAuthManager.
type resolveAuthManagerParams struct {
	props                *getRunnableDescribeComponentCmdProps
	atmosConfig          *schema.AtmosConfiguration
	component            string
	stack                string
	identityName         string
	identityExplicit     bool
	processYamlFunctions bool
}

// resolveAuthManager creates an AuthManager only when identity is explicitly requested.
// Describe-component is an inspection command: YAML functions can use ambient credentials
// or degrade according to --error-mode without triggering an eager login.
func resolveAuthManager(p *resolveAuthManagerParams) (auth.AuthManager, error) {
	if !p.identityExplicit {
		return nil, nil
	}

	mergedAuthConfig := auth.CopyGlobalAuthConfig(&p.atmosConfig.Auth)

	// Get component config to extract auth section (without YAML functions to avoid circular dependency).
	componentConfig, componentErr := p.props.executeDescribeComponent(&e.ExecuteDescribeComponentParams{
		Component:            p.component,
		Stack:                p.stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil,
	})
	if componentErr != nil {
		if errors.Is(componentErr, errUtils.ErrInvalidComponent) {
			return nil, componentErr
		}
		// For other errors (e.g., permission issues), continue with global auth config.
	} else {
		var err error
		mergedAuthConfig, err = auth.MergeComponentAuthFromConfig(&p.atmosConfig.Auth, componentConfig, p.atmosConfig, cfg.AuthSectionName)
		if err != nil {
			return nil, err
		}
	}

	return CreateAuthManagerFromIdentityWithAtmosConfig(p.identityName, mergedAuthConfig, p.atmosConfig, p.stack)
}

func getRunnableDescribeComponentCmd(
	g getRunnableDescribeComponentCmdProps,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := g.checkAtmosConfigE(); err != nil {
			return err
		}

		if len(args) != 1 {
			return fmt.Errorf("%w: the command requires one argument `component`", errUtils.ErrInvalidArguments)
		}

		f, err := parseDescribeComponentFlags(cmd)
		if err != nil {
			return err
		}

		component := args[0]
		needsPathResolution := g.isExplicitComponentPath(component)

		info := buildConfigAndStacksInfoFromFlags(cmd, component, f.stack)

		atmosConfig, err := g.initCliConfig(info, needsPathResolution)
		if err != nil {
			return handleConfigError(err, needsPathResolution, component, f.stack)
		}
		if cmd.Flags().Lookup(describeErrorModeFlagName) != nil {
			if err = resolveDescribeErrorModeFlag(cmd, viper.GetViper(), describeComponentErrorModeParser); err != nil {
				return err
			}
			if f.errorMode, err = cmd.Flags().GetString(describeErrorModeFlagName); err != nil {
				return err
			}
		}
		f.errorMode = e.ResolveErrorMode(f.errorMode, atmosConfig.Describe.ErrorMode)
		if f.errorMode != "strict" && f.errorMode != "warn" && f.errorMode != "silent" {
			return fmt.Errorf("%w: %q", e.ErrInvalidErrorMode, f.errorMode)
		}

		component, err = resolveComponentFromPathIfNeeded(&g, &atmosConfig, component, f.stack, needsPathResolution)
		if err != nil {
			return err
		}

		identityName := GetIdentityFromFlags(cmd, os.Args)
		identityExplicit := cmd.Flags().Changed(cfg.IdentityFlagName)
		authManager, err := resolveAuthManager(&resolveAuthManagerParams{
			props:                &g,
			atmosConfig:          &atmosConfig,
			component:            component,
			stack:                f.stack,
			identityName:         identityName,
			identityExplicit:     identityExplicit,
			processYamlFunctions: f.processYamlFunctions,
		})
		if err != nil {
			return err
		}

		return g.newDescribeComponentExec.ExecuteDescribeComponentCmd(e.DescribeComponentParams{
			Component:            component,
			Stack:                f.stack,
			ProcessTemplates:     f.processTemplates,
			ProcessYamlFunctions: f.processYamlFunctions,
			Skip:                 f.skip,
			Query:                f.query,
			Format:               f.format,
			File:                 f.file,
			Provenance:           f.provenance,
			ProvenanceExplicit:   f.provenanceExplicit,
			ErrorMode:            f.errorMode,
			AuthManager:          authManager,
		})
	}
}

// resolveComponentFromPathIfNeeded resolves a filesystem path to a component name when needed.
func resolveComponentFromPathIfNeeded(g *getRunnableDescribeComponentCmdProps, atmosConfig *schema.AtmosConfiguration, component, stack string, needsPathResolution bool) (string, error) {
	if !needsPathResolution {
		return component, nil
	}
	return g.resolveComponentFromPath(atmosConfig, component, stack)
}

// handleConfigError wraps config loading errors with path-resolution context when applicable.
func handleConfigError(err error, needsPathResolution bool, component, stack string) error {
	if !needsPathResolution {
		return errors.Join(errUtils.ErrFailedToInitConfig, err)
	}
	return errUtils.Build(errUtils.ErrPathResolutionFailed).
		WithHintf("Failed to initialize config for path: `%s`\n\nPath resolution requires valid Atmos configuration", component).
		WithHint("Verify `atmos.yaml` exists in your repository root or `.atmos/` directory\nRun `atmos describe config` to validate your configuration").
		WithContext("component_arg", component).
		WithContext("stack", stack).
		WithCause(err).
		WithExitCode(2).
		Err()
}

func init() {
	describeComponentCmd.DisableFlagParsing = false
	AddStackCompletion(describeComponentCmd)
	describeComponentCmd.PersistentFlags().StringP("format", "f", "yaml", "The output format")
	describeComponentCmd.PersistentFlags().String("file", "", "Write the result to the file")
	describeComponentCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	describeComponentCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
	describeComponentCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")
	describeComponentCmd.PersistentFlags().Bool("provenance", false, "Show where configuration values originated (enabled by default; disable with --provenance=false or describe.provenance in atmos.yaml)")
	describeComponentErrorModeParser = newDescribeErrorModeParser()
	describeComponentErrorModeParser.RegisterPersistentFlags(describeComponentCmd)
	if err := describeComponentErrorModeParser.BindToViper(viper.GetViper()); err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	err := describeComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	describeCmd.AddCommand(describeComponentCmd)
}
