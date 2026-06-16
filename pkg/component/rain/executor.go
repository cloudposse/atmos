package rain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	defaultTemplateName = "template.yaml"
	directoryMode       = 0o755
	tempFileMode        = 0o600
)

var stackTargetCommands = map[string]bool{
	"deploy":   true,
	"rm":       true,
	"logs":     true,
	"watch":    true,
	"cat":      true,
	"console":  true,
	"diff":     true,
	"forecast": true,
}

var templateTargetCommands = map[string]bool{
	"deploy":   true,
	"forecast": true,
	"diff":     true,
	"pkg":      true,
	"fmt":      true,
	"tree":     true,
}

// Execute executes a Rain command for a component.
func Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "rain.ExecuteCommand")()

	info := ctx.ConfigAndStacksInfo
	applyExecutionContextDefaults(&info, ctx)

	atmosConfig, err := initRainConfig(&info)
	if err != nil {
		return err
	}

	authManager, err := setupRainAuth(&atmosConfig, &info)
	if err != nil {
		return err
	}

	info, err = processRainStack(&atmosConfig, &info, authManager)
	if err != nil {
		return err
	}

	componentPath, err := prepareRainComponent(&atmosConfig, &info)
	if err != nil {
		return err
	}

	return executeRainCommand(&atmosConfig, &info, componentPath)
}

func initRainConfig(info *schema.ConfigAndStacksInfo) (schema.AtmosConfiguration, error) {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return atmosConfig, err
	}
	if err := checkConfig(&atmosConfig); err != nil {
		return atmosConfig, err
	}
	if info.Command == "" {
		info.Command = atmosConfig.Components.Rain.Command
		if info.Command == "" {
			info.Command = cfg.RainComponentType
		}
	}
	return atmosConfig, nil
}

func processRainStack(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
	return e.ProcessStacks(atmosConfig, *info, true, info.ProcessTemplates, info.ProcessFunctions, info.Skip, authManager)
}

func prepareRainComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", "component", info.ComponentFromArg)
		return "", nil
	}
	if err := validateComponentMetadata(info); err != nil {
		return "", err
	}
	initialPath, err := u.GetComponentPath(atmosConfig, cfg.RainComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return "", errors.Join(errUtils.ErrPathResolution, fmt.Errorf("component path: %w", err))
	}
	componentPath, err := resolveComponentPath(atmosConfig, info, initialPath)
	if err != nil {
		return "", err
	}
	return componentPath, maybeAutoGenerateFiles(atmosConfig, info, componentPath)
}

func executeRainCommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) error {
	if componentPath == "" {
		return nil
	}
	if err := auth.TerraformPreHook(atmosConfig, info); err != nil {
		return err
	}
	tenv, err := dependencies.ForComponent(atmosConfig, cfg.RainComponentType, info.StackSection, info.ComponentSection)
	if err != nil {
		return errors.Join(errUtils.ErrDependencyResolution, err)
	}
	command := tenv.Resolve(info.Command)
	args, cleanup, err := buildArgs(info, componentPath)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}

	envVars := info.SanitizedEnv
	if len(envVars) == 0 {
		e.ConvertComponentEnvSectionToList(info)
		envVars = append(envVars, info.ComponentEnvList...)
	}
	envVars = append(envVars, tenv.EnvVars()...)

	if info.DryRun {
		return data.Writeln(strings.Join(append([]string{command}, args...), " "))
	}

	return e.ExecuteShellCommand(
		*atmosConfig,
		command,
		args,
		componentPath,
		envVars,
		info.DryRun,
		info.RedirectStdErr,
	)
}

func applyExecutionContextDefaults(info *schema.ConfigAndStacksInfo, ctx *component.ExecutionContext) {
	if info.ComponentType == "" {
		info.ComponentType = cfg.RainComponentType
	}
	if info.SubCommand == "" {
		info.SubCommand = ctx.SubCommand
	}
	if info.ComponentFromArg == "" {
		info.ComponentFromArg = ctx.Component
	}
	if info.Stack == "" {
		info.Stack = ctx.Stack
	}
}

func checkConfig(atmosConfig *schema.AtmosConfiguration) error {
	if atmosConfig.Components.Rain.BasePath == "" {
		return errUtils.ErrMissingRainBasePath
	}
	return nil
}

func setupRainAuth(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	return e.SetupComponentAuthForCLI(atmosConfig, info)
}

func resolveComponentPath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, initialPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	componentPath, exists, err := component.ProvisionAndResolveComponentPath(ctx, atmosConfig, info, cfg.RainComponentType, initialPath)
	if err != nil {
		return "", err
	}
	if exists {
		return componentPath, nil
	}

	basePath, basePathErr := u.GetComponentBasePath(atmosConfig, cfg.RainComponentType)
	if basePathErr != nil {
		return "", fmt.Errorf("%w: '%s' points to the Rain component '%s', but failed to resolve base path: %w",
			errUtils.ErrInvalidComponent, info.ComponentFromArg, info.FinalComponent, basePathErr)
	}
	return "", fmt.Errorf("%w: '%s' points to the Rain component '%s', but it does not exist in '%s'",
		errUtils.ErrInvalidComponent, info.ComponentFromArg, info.FinalComponent, basePath)
}

func maybeAutoGenerateFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) error {
	if !atmosConfig.Components.Rain.AutoGenerateFiles || info.DryRun {
		return nil
	}
	generateSection, ok := info.ComponentSection[cfg.GenerateSectionName].(map[string]any)
	if !ok || len(generateSection) == 0 {
		return nil
	}
	if err := os.MkdirAll(componentPath, directoryMode); err != nil {
		return errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("auto-generation: %w", err))
	}
	if err := e.GenerateFilesForComponent(atmosConfig, info, componentPath); err != nil {
		return errors.Join(errUtils.ErrFileOperation, err)
	}
	return nil
}

func validateComponentMetadata(info *schema.ConfigAndStacksInfo) error {
	if info.ComponentIsAbstract {
		return fmt.Errorf("%w: the component '%s' cannot be provisioned because it's marked as abstract (metadata.type: abstract)",
			errUtils.ErrAbstractComponentCantBeProvisioned,
			filepath.Join(info.ComponentFolderPrefix, info.Component))
	}
	if info.ComponentIsLocked && isMutatingCommand(info.SubCommand) {
		return fmt.Errorf("%w: component '%s' cannot be modified (metadata.locked: true)",
			errUtils.ErrLockedComponentCantBeProvisioned,
			filepath.Join(info.ComponentFolderPrefix, info.Component))
	}
	return nil
}

func isMutatingCommand(command string) bool {
	return command == "deploy" || command == "rm" || command == "pkg" || command == "fmt" || command == "bootstrap" || command == "module" || command == "stackset"
}

func buildArgs(info *schema.ConfigAndStacksInfo, componentPath string) ([]string, func(), error) {
	subcommand := info.SubCommand
	args := []string{subcommand}

	templatePath := resolvePath(componentPath, stringFromSection(info.ComponentSection, "template", defaultTemplateName))
	stackName := stringFromSection(info.ComponentSection, cfg.NameSectionName, "")
	if stackTargetCommands[subcommand] && stackName == "" {
		return nil, nil, fmt.Errorf("%w: components.rain.%s.name is required for `rain %s`",
			errUtils.ErrRainStackNameMissing, info.ComponentFromArg, subcommand)
	}

	var cleanup func()
	configArgList, cleanup, err := configArgs(info, componentPath)
	if err != nil {
		return nil, nil, err
	}
	args = append(args, configArgList...)
	args = append(args, rainSettingsArgs(info.ComponentSettingsSection)...)
	args = appendTargetArgs(args, subcommand, templatePath, stackName)
	args = append(args, info.AdditionalArgsAndFlags...)
	return args, cleanup, nil
}

func commandUsesConfig(command string) bool {
	return command == "deploy" || command == "forecast" || command == "diff"
}

func configArgs(info *schema.ConfigAndStacksInfo, componentPath string) ([]string, func(), error) {
	if !commandUsesConfig(info.SubCommand) {
		return nil, nil, nil
	}
	configPath, cleanup, err := resolveConfigArg(info, componentPath)
	if err != nil {
		return nil, nil, err
	}
	if configPath == "" {
		return nil, cleanup, nil
	}
	return []string{"--config", configPath}, cleanup, nil
}

func appendTargetArgs(args []string, subcommand, templatePath, stackName string) []string {
	switch {
	case commandUsesConfig(subcommand):
		return append(args, templatePath, stackName)
	case templateTargetCommands[subcommand]:
		return append(args, templatePath)
	case stackTargetCommands[subcommand]:
		return append(args, stackName)
	default:
		return args
	}
}

func resolveConfigArg(info *schema.ConfigAndStacksInfo, componentPath string) (string, func(), error) {
	if config := stringFromSection(info.ComponentSection, "config", ""); config != "" {
		return resolvePath(componentPath, config), nil, nil
	}

	params, hasParams := info.ComponentSection["params"].(map[string]any)
	tags, hasTags := info.ComponentSection["tags"].(map[string]any)
	if !hasParams && !hasTags {
		return "", nil, nil
	}

	tmp, err := os.CreateTemp("", "atmos-rain-*.yaml")
	if err != nil {
		return "", nil, errors.Join(errUtils.ErrFileOperation, err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", nil, errors.Join(errUtils.ErrFileOperation, err)
	}

	config := map[string]any{}
	if hasParams {
		config["Parameters"] = params
	}
	if hasTags {
		config["Tags"] = tags
	}
	if err := u.WriteToFileAsYAML(tmpPath, config, tempFileMode); err != nil {
		_ = os.Remove(tmpPath) // #nosec G703 -- tmpPath is created by os.CreateTemp in the system temp directory.
		return "", nil, errors.Join(errUtils.ErrFileOperation, err)
	}

	return tmpPath, func() { _ = os.Remove(tmpPath) }, nil // #nosec G703 -- tmpPath is created by os.CreateTemp in the system temp directory.
}

func rainSettingsArgs(settings schema.AtmosSectionMapType) []string {
	rainSettings, ok := settings[cfg.RainSectionName].(map[string]any)
	if !ok {
		return nil
	}

	var args []string
	addStringFlag := func(key, flag string) {
		if value, ok := rainSettings[key].(string); ok && value != "" {
			args = append(args, flag, value)
		}
	}

	addStringFlag("region", "--region")
	addStringFlag("profile", "--profile")
	addStringFlag("s3_bucket", "--s3-bucket")
	addStringFlag("s3_prefix", "--s3-prefix")
	addStringFlag("role_arn", "--role-arn")
	return args
}

func stringFromSection(section map[string]any, key, fallback string) string {
	value, ok := section[key].(string)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func resolvePath(componentPath, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(componentPath, path)
}
