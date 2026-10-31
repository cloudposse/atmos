// https://github.com/roboll/helmfile#cli-reference

package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	helmplugin "github.com/cloudposse/atmos/pkg/helm/plugin"
	"github.com/cloudposse/atmos/pkg/helmfile"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	// ComponentTypeHelmfile is the component type identifier for helmfile.
	componentTypeHelmfile = "helmfile"

	// Log key constants.
	logKeyCluster = "cluster"

	// HelmDiffColorEnvVar is the env var the helm-diff plugin reads to decide
	// whether to colorize its output, overriding its own (unreliable, once piped
	// through helmfile) isatty check.
	helmDiffColorEnvVar = "HELM_DIFF_COLOR"
)

// helmDiffColorEnv returns "HELM_DIFF_COLOR=true" when colorEnabled is true and
// none of globalEnv, envVars, or the parent process environment already set
// HELM_DIFF_COLOR, or "" when no override is needed (color disabled, or the
// user already decided -- including explicitly setting HELM_DIFF_COLOR to an
// empty string to opt out).
//
// The globalEnv parameter is atmos.yaml's top-level `env` section
// (atmosConfig.Env). It must be checked here, not left to ExecuteShellCommand's
// own env merge: that merge is process env < globalEnv < envVars
// (env.MergeGlobalEnv appends globalEnv after the process env, then
// ExecuteShellCommand appends envVars after that, and exec.Cmd resolves
// duplicate "KEY=value" entries by last-one-wins). Since envVars is the same
// slice this override gets appended to, an override added here would
// unconditionally outrank an explicit `env: HELM_DIFF_COLOR: false` in
// atmos.yaml regardless of this function's own envVars/os.LookupEnv checks.
//
// Presence in the parent process environment is checked with os.LookupEnv rather
// than viper: viper.BindEnv is forbidden outside pkg/flags/ (see .golangci.yml's
// forbidigo config), and even there it couldn't tell "unset" apart from
// "explicitly set to empty" by default (viper's getEnv treats an empty env var
// as unset unless AllowEmptyEnv is toggled globally). The ok result from
// os.LookupEnv is the repo's existing idiom for exactly this presence check --
// see pkg/flags/standard_parser.go's IsBoolFlagExplicitlySet and
// pkg/flags/interactive.go's ATMOS_FORCE_TTY check.
func helmDiffColorEnv(envVars []string, globalEnv map[string]string, colorEnabled bool) string {
	if !colorEnabled {
		return ""
	}
	if _, explicitlySet := globalEnv[helmDiffColorEnvVar]; explicitlySet {
		return ""
	}
	if envKeyIsSet(envVars, helmDiffColorEnvVar) {
		return ""
	}
	if _, explicitlySet := os.LookupEnv(helmDiffColorEnvVar); explicitlySet {
		return ""
	}
	return helmDiffColorEnvVar + "=true"
}

// helmDiffColorEnabled reports whether atmos's color decision for the stream
// helm-diff's output actually reaches (helmfile's stdout, piped through to
// atmos's own stdout) says color should be used. Kept as its own small,
// Terminal-injectable function -- rather than inlined at the call site -- so
// a test can swap in a fake Terminal whose ColorProfileFor varies by stream,
// proving this asks about Stdout specifically rather than regressing to the
// general, stderr-biased ColorProfile().
func helmDiffColorEnabled(term terminal.Terminal) bool {
	return term.ColorProfileFor(terminal.Stdout) != terminal.ColorNone
}

// ExecuteHelmfile executes helmfile commands.
//
//nolint:gocognit,revive,cyclop,funlen,gocritic // Existing execution pipeline; reporting adds only a deferred snapshot.
func ExecuteHelmfile(info schema.ConfigAndStacksInfo) (resultErr error) {
	defer perf.Track(nil, "exec.ExecuteHelmfile")()
	defer attachComponentReporting(&resultErr, &info, "helmfile", info.SubCommand)

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	// Add the `command` from `components.helmfile.command` from `atmos.yaml`.
	if info.ComponentCommand != "" {
		info.Command = info.ComponentCommand
	}
	if info.Command == "" {
		if atmosConfig.Components.Helmfile.Command != "" {
			info.Command = atmosConfig.Components.Helmfile.Command
		} else {
			info.Command = cfg.HelmfileComponentType
		}
	}

	if info.DryRun {
		return previewHelmfile(&atmosConfig, &info)
	}

	if info.SubCommand == "version" {
		tenv, err := dependencies.ForComponent(&atmosConfig, componentTypeHelmfile, nil, nil)
		if err != nil {
			return err
		}
		return ExecuteShellCommand(
			atmosConfig,
			tenv.Resolve(info.Command),
			[]string{info.SubCommand},
			"",
			tenv.EnvVars(),
			info.DryRun,
			info.RedirectStdErr,
		)
	}

	authManager, err := SetupComponentAuthForCLI(&atmosConfig, &info)
	if err != nil {
		return err
	}

	info, err = ProcessStacks(&atmosConfig, info, true, true, true, nil, authManager)
	if err != nil {
		return err
	}

	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", "component", info.ComponentFromArg)
		return nil
	}

	err = checkHelmfileConfig(&atmosConfig)
	if err != nil {
		return err
	}

	// Check if the component exists as a helmfile component.
	componentPath, err := u.GetComponentPath(&atmosConfig, componentTypeHelmfile, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return fmt.Errorf("failed to resolve component path: %w", err)
	}

	// Auto-generate files BEFORE path validation when the following conditions hold.
	// 1. auto_generate_files is enabled.
	// 2. Component has a generate section.
	// 3. Not in dry-run mode (to avoid filesystem modifications).
	// This allows generating entire components from stack configuration.
	if atmosConfig.Components.Helmfile.AutoGenerateFiles && !info.DryRun { //nolint:nestif
		generateSection := tfgenerate.GetGenerateSectionFromComponent(info.ComponentSection)
		if generateSection != nil {
			// Ensure component directory exists for file generation.
			if mkdirErr := os.MkdirAll(componentPath, 0o755); mkdirErr != nil { //nolint:revive
				return errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("auto-generation: %w", mkdirErr))
			}

			// Generate files before path validation.
			if genErr := GenerateFilesForComponent(&atmosConfig, &info, componentPath); genErr != nil {
				return errors.Join(errUtils.ErrFileOperation, genErr)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	componentPath, componentPathExists, err := component.ProvisionAndResolveComponentPath(
		ctx, provisioner.OutputWriters{}, &atmosConfig, &info, cfg.HelmfileComponentType, componentPath,
	)
	if err != nil {
		return err
	}
	if !componentPathExists {
		basePath, _ := u.GetComponentBasePath(&atmosConfig, componentTypeHelmfile)
		return fmt.Errorf(
			"%w: '%s' points to the Helmfile component '%s', but it does not exist in '%s'",
			errUtils.ErrInvalidComponent,
			info.ComponentFromArg,
			info.FinalComponent,
			basePath,
		)
	}

	if err := checkHelmfileRestrictions(&info); err != nil {
		return err
	}

	// Resolve and install component dependencies.
	tenv, err := dependencies.ForComponent(&atmosConfig, componentTypeHelmfile, info.StackSection, info.ComponentSection)
	if err != nil {
		return err
	}
	info.ComponentEnvList = append(info.ComponentEnvList, tenv.EnvVars()...)

	// Ensure declared Helm plugins (e.g. helm-diff) are installed into the managed
	// HELM_PLUGINS directory and expose it to the helmfile subprocess. Helmfile
	// shells out to helm, which requires plugins like helm-diff for diff/apply.
	if pluginSpecs := helmplugin.ExtractSpecs(info.ComponentSection); len(pluginSpecs) > 0 {
		pluginsDir, perr := helmplugin.EnsureForComponent(context.Background(), tenv.Resolve("helm"), pluginSpecs)
		if perr != nil {
			return perr
		}
		if pluginsDir != "" {
			info.ComponentEnvList = append(info.ComponentEnvList, "HELM_PLUGINS="+pluginsDir)
		}
	}

	// Print component variables.
	log.Debug("Variables for component in stack", "component", info.ComponentFromArg, "stack", info.Stack, "variables", info.ComponentVarsSection)

	// Check if the component 'settings.validation' section is specified and validate the component.
	valid, err := ValidateComponent(
		&atmosConfig,
		info.ComponentFromArg,
		info.ComponentSection,
		"",
		"",
		nil,
		0,
	)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf(
			"%w: the component '%s' did not pass the validation policies",
			errUtils.ErrInvalidComponent,
			info.ComponentFromArg,
		)
	}

	// Write variables to a file.
	varFile := constructHelmfileComponentVarfileName(&info)
	varFilePath := constructHelmfileComponentVarfilePath(&atmosConfig, &info)

	log.Debug("Writing the variables to file:", "file", varFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsYAML(varFilePath, info.ComponentVarsSection, 0o644)
		if err != nil {
			return err
		}
	}

	// Handle `helmfile deploy` custom command.
	if info.SubCommand == "deploy" {
		info.SubCommand = "sync"
	}

	// Extract the Atmos-specific `--target` flag (provision delivery) so it is not
	// forwarded to the helmfile binary, and resolve the selected target.
	flagTarget, strippedArgs := helmfile.ExtractTargetFlag(info.AdditionalArgsAndFlags)
	info.AdditionalArgsAndFlags = strippedArgs
	var selectedTarget *target.SelectedTarget
	if flagTarget != "" {
		provisionSection, _ := info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
		selectedTarget, err = target.SelectTarget(provisionSection, flagTarget)
		if err != nil {
			return err
		}
	}

	stackContext := cfg.GetContextFromVars(info.ComponentVarsSection)

	envVarsEKS := []string{}

	if atmosConfig.Components.Helmfile.UseEKS {
		// Resolve cluster name using the helmfile package.
		clusterInput := helmfile.ClusterNameInput{
			FlagValue:   info.ClusterName,
			ConfigValue: atmosConfig.Components.Helmfile.ClusterName,
			Template:    atmosConfig.Components.Helmfile.ClusterNameTemplate,
			Pattern:     atmosConfig.Components.Helmfile.ClusterNamePattern,
		}

		clusterResult, err := helmfile.ResolveClusterName(
			clusterInput,
			&stackContext,
			&atmosConfig,
			info.ComponentSection,
			ProcessTmpl,
		)
		if err != nil {
			return err
		}

		clusterName := clusterResult.ClusterName
		if clusterResult.IsDeprecated {
			log.Warn("cluster_name_pattern is deprecated, use cluster_name_template with Go template syntax instead")
		}
		log.Debug("Using cluster name", logKeyCluster, clusterName, "source", clusterResult.Source)

		// Resolve AWS auth using the helmfile package.
		authInput := helmfile.AuthInput{
			Identity:       info.Identity,
			ProfilePattern: atmosConfig.Components.Helmfile.HelmAwsProfilePattern,
		}

		authResult, err := helmfile.ResolveAWSAuth(authInput, &stackContext)
		if err != nil {
			return err
		}

		useIdentityAuth := authResult.UseIdentityAuth
		helmAwsProfile := authResult.Profile
		if authResult.IsDeprecated {
			log.Warn("helm_aws_profile_pattern is deprecated, use --identity flag instead")
		}
		log.Debug("Using AWS auth", "source", authResult.Source, "useIdentity", useIdentityAuth)

		// Download kubeconfig by running `aws eks update-kubeconfig`.
		kubeconfigPath := filepath.Join(atmosConfig.Components.Helmfile.KubeconfigPath, info.ContextPrefix+"-kubecfg")
		log.Debug("Downloading and saving kubeconfig", logKeyCluster, clusterName, "path", kubeconfigPath)

		// Build aws eks update-kubeconfig command args.
		awsArgs := buildHelmfileEKSArgs(clusterName, stackContext.Region, kubeconfigPath, useIdentityAuth, helmAwsProfile)

		err = ExecuteShellCommand(
			atmosConfig,
			tenv.Resolve("aws"),
			awsArgs,
			componentPath,
			tenv.EnvVars(),
			info.DryRun,
			info.RedirectStdErr,
		)
		if err != nil {
			return err
		}

		// Set environment variables for helmfile execution.
		if !useIdentityAuth && helmAwsProfile != "" {
			envVarsEKS = append(envVarsEKS, fmt.Sprintf("AWS_PROFILE=%s", helmAwsProfile))
		}
		envVarsEKS = append(envVarsEKS, fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	}

	// Print command info.
	log.Debug("Command info:")
	log.Debug("Helmfile binary: " + info.Command)
	log.Debug("Helmfile command: " + info.SubCommand)

	// https://github.com/roboll/helmfile#cli-reference
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options "--no-color --namespace=test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options "--no-color --namespace test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options="--no-color --namespace=test"
	// atmos helmfile diff echo-server -s tenant1-ue2-dev --global-options="--no-color --namespace test"
	log.Debug("Global", "options", info.GlobalOptions)

	log.Debug("Arguments and flags", "additional", info.AdditionalArgsAndFlags)
	log.Debug("Component: " + info.ComponentFromArg)

	if len(info.BaseComponent) > 0 {
		log.Debug("Helmfile component: " + info.BaseComponent)
	}

	if info.Stack == info.StackFromArg {
		log.Debug("Stack: " + info.StackFromArg)
	} else {
		log.Debug("Stack: " + info.StackFromArg)
		log.Debug("Stack path: " + filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath, info.Stack))
	}

	workingDir := constructHelmfileComponentWorkingDir(&atmosConfig, &info)
	log.Debug("Using", "working dir", workingDir)

	// Prepare arguments and flags.
	allArgsAndFlags := buildHelmfileArgs(&info, varFile, info.SubCommand)

	// Convert ComponentEnvSection to ComponentEnvList.
	// ComponentEnvSection is populated by auth hooks and stack config env sections.
	for k, v := range info.ComponentEnvSection {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("%s=%v", k, v))
	}

	// Prepare ENV vars.
	envVars := append(info.ComponentEnvList, []string{
		fmt.Sprintf("STACK=%s", info.Stack),
	}...)

	// Append the context ENV vars (first check if they are not set by the caller).
	env := os.Getenv("NAMESPACE")
	if env == "" {
		envVars = append(envVars, fmt.Sprintf("NAMESPACE=%s", stackContext.Namespace))
	}
	env = os.Getenv("TENANT")
	if env == "" {
		envVars = append(envVars, fmt.Sprintf("TENANT=%s", stackContext.Tenant))
	}
	env = os.Getenv("ENVIRONMENT")
	if env == "" {
		envVars = append(envVars, fmt.Sprintf("ENVIRONMENT=%s", stackContext.Environment))
	}
	env = os.Getenv("STAGE")
	if env == "" {
		envVars = append(envVars, fmt.Sprintf("STAGE=%s", stackContext.Stage))
	}
	env = os.Getenv("REGION")
	if env == "" {
		envVars = append(envVars, fmt.Sprintf("REGION=%s", stackContext.Region))
	}

	// Set KUBECONFIG: When UseEKS is true, the EKS-specific kubeconfig (from envVarsEKS)
	// takes precedence over the general KubeconfigPath setting.
	if atmosConfig.Components.Helmfile.KubeconfigPath != "" && !atmosConfig.Components.Helmfile.UseEKS {
		envVars = append(envVars, fmt.Sprintf("KUBECONFIG=%s", atmosConfig.Components.Helmfile.KubeconfigPath))
	}

	if atmosConfig.Components.Helmfile.UseEKS {
		envVars = append(envVars, envVarsEKS...)
	}
	envVars = append(envVars, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	envVars = append(envVars, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))

	// Helmfile pipes the helm-diff plugin's output so it can label it per release,
	// which makes helm-diff's own isatty check on its stdout see a pipe instead of
	// the real terminal, even when the user's terminal supports color. HELM_DIFF_COLOR
	// is the documented override for exactly this case (databus23/helm-diff); set it
	// when atmos's own color decision says color should be on and the user hasn't
	// already set it themselves.
	//
	// ColorProfileFor(Stdout), not ColorProfile(): helmfile's labeled diff output
	// (and thus helm-diff's ANSI codes) reaches atmos's own stdout, not stderr.
	// ColorProfile() is cached from stderr's TTY status (atmos's own UI output),
	// so it would say "color enabled" even when stdout is redirected to a file
	// but stderr is still an interactive terminal -- leaking ANSI codes into
	// non-interactive output. ColorProfileFor(Stdout) still honors NO_COLOR/
	// --force-color/CLICOLOR_FORCE the same way; only the TTY-dependent
	// auto-detect fallback changes to check the stream the output actually goes to.
	if v := helmDiffColorEnv(envVars, atmosConfig.Env, helmDiffColorEnabled(terminal.New())); v != "" {
		envVars = append(envVars, v)
	}

	envVars, err = prepareComponentAuthEnvironment(authManager, info.Identity, envVars)
	if err != nil {
		return err
	}

	log.Debug("Using ENV vars:")
	for _, v := range envVars {
		log.Debug(v)
	}

	// Provision-target delivery: when a non-cluster target is selected (e.g. a git
	// deployment repository), render the helmfile to manifests with `helmfile
	// template` and deliver them to the target instead of applying to a cluster.
	if selectedTarget != nil && selectedTarget.Kind != target.KindKubernetes {
		defer func() {
			if rmErr := os.Remove(varFilePath); rmErr != nil {
				log.Warn(rmErr.Error())
			}
		}()

		if info.NodeHooks != nil {
			if beforeErr := info.NodeHooks.Before(context.Background(), &info); beforeErr != nil {
				return fmt.Errorf("%w: %w", errUtils.ErrPerComponentHookFailed, beforeErr)
			}
		}

		rendered, deliverErr := deliverHelmfileToTarget(&atmosConfig, &info, helmfileTargetDelivery{
			varFile:       varFile,
			componentPath: componentPath,
			envVars:       envVars,
			flagTarget:    flagTarget,
		})
		if info.NodeHooks != nil {
			if afterErr := info.NodeHooks.After(context.Background(), &info, rendered, deliverErr); afterErr != nil && deliverErr == nil {
				deliverErr = afterErr
			}
		}
		return deliverErr
	}

	if info.NodeHooks != nil {
		if beforeErr := info.NodeHooks.Before(context.Background(), &info); beforeErr != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrPerComponentHookFailed, beforeErr)
		}
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	shellOpts := []ShellCommandOption{WithEnvironment(info.SanitizedEnv)}
	if info.NodeHooks != nil {
		shellOpts = append(shellOpts, WithStdoutCapture(&stdoutBuf), WithStderrCapture(&stderrBuf))
	}

	// Resolve the helmfile binary through the toolchain environment so a
	// toolchain-installed helmfile (under the install path, not the system PATH)
	// is found — mirroring the `version` subcommand above. Falls back to the bare
	// command name when no toolchain dependency provides it.
	err = executeHelmfileCommandWithRetry(&atmosConfig, &info, tenv, retryExecParams{
		allArgsAndFlags: allArgsAndFlags,
		componentPath:   componentPath,
		envVars:         envVars,
	}, shellOpts...)
	if info.NodeHooks != nil {
		if afterErr := info.NodeHooks.After(context.Background(), &info, stdoutBuf.String()+stderrBuf.String(), err); afterErr != nil && err == nil {
			err = afterErr
		}
	}
	if err != nil {
		return err
	}

	// Cleanup.
	err = os.Remove(varFilePath)
	if err != nil {
		log.Warn(err.Error())
	}

	return nil
}

// executeHelmfileCommandWithRetry runs the resolved helmfile subcommand through
// ExecuteShellCommandWithRetry. Extracted from ExecuteHelmfile so the retry wiring can
// be unit-tested directly with a fake invoke, without standing up ExecuteHelmfile's full
// stack-processing/auth/toolchain preamble or requiring a real helmfile binary.
func executeHelmfileCommandWithRetry(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	tenv *dependencies.ToolchainEnvironment,
	params retryExecParams,
	shellOpts ...ShellCommandOption,
) error {
	return ExecuteShellCommandWithRetry(
		atmosConfig,
		info,
		info.SubCommand,
		func(o ...ShellCommandOption) error {
			return ExecuteShellCommand(
				*atmosConfig,
				tenv.Resolve(info.Command),
				params.allArgsAndFlags,
				params.componentPath,
				params.envVars,
				info.DryRun,
				info.RedirectStdErr,
				o...,
			)
		},
		shellOpts...,
	)
}

// renderAndDeliver is a seam over helmfile.RenderAndDeliver so the inline
// call-site can be unit-tested without invoking the helmfile binary.
var renderAndDeliver = helmfile.RenderAndDeliver

// helmfileTargetDelivery bundles the inputs for rendering a helmfile and
// delivering the result to a provision target.
type helmfileTargetDelivery struct {
	varFile       string
	componentPath string
	envVars       []string
	flagTarget    string
}

// deliverHelmfileToTarget renders the helmfile to manifests and delivers them to
// the selected provision target. The feature logic lives in pkg/helmfile; this is
// the thin inline call-site.
func deliverHelmfileToTarget(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	d helmfileTargetDelivery,
) (string, error) {
	templateArgs := buildHelmfileArgs(info, d.varFile, "template")

	var envProvider target.IdentityEnvironmentProvider
	if mgr, ok := info.AuthManager.(auth.AuthManager); ok {
		envProvider = mgr
	}
	provisionSection, _ := info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)

	return renderAndDeliver(context.Background(), &helmfile.RenderDeliverInput{
		AtmosConfig:      atmosConfig,
		Info:             info,
		Command:          info.Command,
		Args:             templateArgs,
		WorkingDir:       d.componentPath,
		EnvVars:          d.envVars,
		ProvisionSection: provisionSection,
		FlagTarget:       d.flagTarget,
		EnvProvider:      envProvider,
	})
}

// resolveDefaultIdentity and prepareComponentAuthEnvironment now live in utils_auth.go
// so the helmfile and packer subprocess executors share one credential-injection path.
