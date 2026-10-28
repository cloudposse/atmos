package terraform

import (
	context "context"
	errors "errors"
	fmt "fmt"
	os "os"
	filepath "path/filepath"
	time "time"

	tfconfig "github.com/hashicorp/terraform-config-inspect/tfconfig"

	errUtils "github.com/cloudposse/atmos/errors"
	component "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	degradation "github.com/cloudposse/atmos/pkg/degradation"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	perf "github.com/cloudposse/atmos/pkg/perf"
	provisioner "github.com/cloudposse/atmos/pkg/provisioner"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	schema "github.com/cloudposse/atmos/pkg/schema"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
	tfvars "github.com/cloudposse/atmos/pkg/terraform/tfvars"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// resolveAndProvisionComponentPath resolves the filesystem path for a terraform component,
// optionally auto-generates files, performs JIT source provisioning, and validates
// that the resulting directory actually exists.
// The provision-and-resolve component function is a seam for testing JIT provisioning context propagation.
var provisionAndResolveTerraformComponentPath = component.ProvisionAndResolveComponentPath

// The before-init provisioner function is a seam for testing context propagation.
var executeBeforeInitProvisioners = provisioner.ExecuteProvisioners

func (e *Executor) resolveAndProvisionComponentPath(ctx context.Context, writers provisioner.OutputWriters, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	componentPath, err := u.GetComponentPath(atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return "", fmt.Errorf("failed to resolve component path: %w", err)
	}

	// Provision source before generating files: when provision.workdir.enabled
	// is true the resolved path is the workdir, and generated files must land
	// there rather than in the base component directory.
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	componentPath, componentPathExists, err := provisionAndResolveTerraformComponentPath(
		ctx, writers, atmosConfig, info, cfg.TerraformComponentType, componentPath,
	)
	if err != nil {
		return "", err
	}

	if err = e.autoGenerateComponentFiles(atmosConfig, info, componentPath); err != nil {
		return "", err
	}

	if !componentPathExists {
		basePath, _ := u.GetComponentBasePath(atmosConfig, cfg.TerraformComponentType)
		return "", fmt.Errorf(
			"%w: '%s' points to the Terraform component '%s', but it does not exist in '%s'",
			errUtils.ErrInvalidTerraformComponent,
			info.ComponentFromArg,
			info.FinalComponent,
			basePath,
		)
	}

	return componentPath, nil
}

// autoGenerateComponentFiles creates the component directory and generates source files
// when AutoGenerateFiles is enabled and a generate section is present.
func (e *Executor) autoGenerateComponentFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) error {
	if !atmosConfig.Components.Terraform.AutoGenerateFiles || info.DryRun {
		return nil
	}
	generateSection := tfgenerate.GetGenerateSectionFromComponent(info.ComponentSection)
	if generateSection == nil {
		return nil
	}
	if mkdirErr := os.MkdirAll(componentPath, dirPermissions); mkdirErr != nil {
		return errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("auto-generation: %w", mkdirErr))
	}
	return e.services.GenerateFilesForComponent(atmosConfig, info, componentPath)
}

// checkComponentRestrictions returns an error when the requested subcommand is not
// permitted for the component due to its metadata (abstract, locked) or the configured
// backend type (HTTP backend does not support workspaces).
func checkComponentRestrictions(info *schema.ConfigAndStacksInfo) error {
	// Abstract components cannot be provisioned.
	if info.ComponentIsAbstract {
		switch info.SubCommand {
		case "plan", subcommandApply, subcommandDeploy, subcommandWorkspace:
			return fmt.Errorf(
				"%w: the component '%s' cannot be provisioned because it's marked as abstract (metadata.type: abstract)",
				errUtils.ErrAbstractComponentCantBeProvisioned,
				filepath.Join(info.ComponentFolderPrefix, info.Component),
			)
		}
	}

	// Locked components may not be mutated.
	if info.ComponentIsLocked {
		switch info.SubCommand {
		case subcommandApply, "deploy", "destroy", "import", "state", "taint", "untaint":
			return fmt.Errorf(
				"%w: component '%s' cannot be modified (metadata.locked: true)",
				errUtils.ErrLockedComponentCantBeProvisioned,
				filepath.Join(info.ComponentFolderPrefix, info.Component),
			)
		}
	}

	// HTTP backend does not support workspace commands.
	if info.SubCommand == subcommandWorkspace && info.ComponentBackendType == "http" {
		return errUtils.ErrHTTPBackendWorkspaces
	}

	return nil
}

// printAndWriteVarFiles logs component variables and, when not using a pre-existing
// plan file, writes them to the varfile on disk (path derived from atmosConfig+info).
// Workspace subcommands do not use varfiles and are skipped, unless init.pass_vars is enabled: the init
// that runs before a workspace command then receives -var-file, so the varfile must exist even for a
// component that was never planned.
func printAndWriteVarFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if info.SubCommand == subcommandWorkspace && !atmosConfig.Components.Terraform.Init.PassVars {
		return nil
	}

	if err := logAndWriteComponentVars(atmosConfig, info); err != nil {
		return err
	}
	if err := writeTerraformTestVars(atmosConfig, info); err != nil {
		return err
	}

	return logCliVarsOverrides(atmosConfig, info)
}

// rejectComputedTerraformVars delegates to the Terraform component.
func rejectComputedTerraformVars(vars map[string]any) error {
	return RejectComputedTerraformVars(vars)
}

// diskSafeVars delegates to the Terraform component.
func diskSafeVars(info *schema.ConfigAndStacksInfo) map[string]any {
	return DiskSafeVars(info)
}

// terraformTestVars delegates to the Terraform component.
func terraformTestVars(info *schema.ConfigAndStacksInfo) map[string]any {
	return TerraformTestVars(info)
}

// hasTerraformTestVars delegates to the Terraform component.
func hasTerraformTestVars(info *schema.ConfigAndStacksInfo) bool {
	return HasTerraformTestVars(info)
}

// diskSafeTerraformTestVars delegates to the Terraform component.
func diskSafeTerraformTestVars(info *schema.ConfigAndStacksInfo) map[string]any {
	return DiskSafeTerraformTestVars(info)
}

// terraformTestSecretVarEnv delegates to the Terraform component.
func terraformTestSecretVarEnv(info *schema.ConfigAndStacksInfo) ([]string, error) {
	return TerraformTestSecretVarEnv(info)
}

// logAndWriteComponentVars logs component variables and writes the varfile to disk
// when not using a pre-existing plan. Secret-bearing variables are excluded from the
// varfile (they are injected as TF_VAR_<name> env vars in assembleComponentEnvVars).
func logAndWriteComponentVars(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	log.Debug("Variables for the component in the stack", logFieldComponent, info.ComponentFromArg, "stack", info.Stack)
	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		if err := u.PrintAsYAMLToFileDescriptor(atmosConfig, info.ComponentVarsSection); err != nil {
			return err
		}
	}

	if !info.UseTerraformPlan {
		varFilePath := ConstructTerraformComponentVarfilePath(atmosConfig, info)
		log.Debug("Writing the variables", "file", varFilePath)
		if !info.DryRun {
			if err := u.WriteToFileAsJSON(varFilePath, diskSafeVars(info), filePermissions); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeTerraformTestVars(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if !hasTerraformTestVars(info) {
		return nil
	}
	varFilePath := ConstructTerraformComponentTestVarfilePath(atmosConfig, info)
	log.Debug("Writing Terraform test variables", "file", varFilePath)
	if info.DryRun {
		return nil
	}
	return u.WriteToFileAsJSON(varFilePath, diskSafeTerraformTestVars(info), filePermissions)
}

// logCliVarsOverrides logs CLI variable overrides when present at debug/trace level.
func logCliVarsOverrides(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	cliVars, ok := info.ComponentSection[cfg.TerraformCliVarsSectionName].(map[string]any)
	if !ok || len(cliVars) == 0 {
		return nil
	}
	log.Debug("CLI variables (will override the variables defined in the stack manifests):")
	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		if err := u.PrintAsYAMLToFileDescriptor(atmosConfig, cliVars); err != nil {
			return err
		}
	}
	return nil
}

// validateTerraformComponent runs OPA/JSON-schema validation policies against the
// component's stack configuration section and returns an error if validation fails.
func (e *Executor) validateTerraformComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	valid, err := e.services.ValidateComponent(
		atmosConfig,
		info.ComponentFromArg,
		info.ComponentSection,
		"", "", nil, 0,
	)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("%w: the component '%s' did not pass the validation policies",
			errUtils.ErrInvalidComponent, info.ComponentFromArg)
	}
	return nil
}

// GenerateConfigFiles writes the backend configuration, generated files, and
// provider overrides for the component into the working directory.
//
// NOTE: GenerateFilesForComponent is also called by autoGenerateComponentFiles
// (inside resolveAndProvisionComponentPath) when AutoGenerateFiles=true. That call
// handles the generate: section from stack config, while this call handles the
// standard backend/provider override files. The two calls serve different purposes
// and both are needed. This is pre-existing behavior from the original terraform.go.
func (e *Executor) GenerateConfigFiles(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(nil, "terraform.Executor.GenerateConfigFiles")()

	return e.Operations.GenerateConfigFiles(config, info, workingDir)
}

// ComputeTerraformSecretVarKeys records the set of top-level variable keys whose value
// contains a resolved secret, so they can be kept out of the on-disk varfile and injected
// as TF_VAR_<name> environment variables instead. It must run after secret resolution
// (ProcessStacks) and before auth credentials are registered with the masker, so that the
// varfile-write and env-assembly steps partition the variables identically.
func ComputeTerraformSecretVarKeys(info *schema.ConfigAndStacksInfo) {
	defer perf.Track(nil, "terraform.ComputeTerraformSecretVarKeys")()

	_, secret := tfvars.Partition(info.ComponentVarsSection, atmosio.ContainsSecret)
	keys := make(map[string]bool, len(secret))
	for k := range secret {
		keys[k] = true
	}

	// Terraform's `sensitive = true` is an explicit declaration that an input must
	// not be persisted in a generated tfvars file. When masking is disabled, retain
	// the established compatibility behavior that keeps typed sensitive values in
	// JSON rather than coercing them through TF_VAR_ environment variables.
	if atmosio.MaskingEnabled() {
		for k := range terraformSensitiveVarKeys(info) {
			if _, supplied := info.ComponentVarsSection[k]; supplied {
				keys[k] = true
			}
		}
	}

	if len(keys) == 0 {
		info.TerraformSecretVarKeys = nil
		return
	}
	info.TerraformSecretVarKeys = keys
}

// terraformSensitiveVarKeys returns the root-module variable names declared
// `sensitive = true`. Component metadata is populated during stack processing
// with terraform-config-inspect, so this adds no extra filesystem parsing on
// the execution hot path.
func terraformSensitiveVarKeys(info *schema.ConfigAndStacksInfo) map[string]bool {
	if info == nil {
		return nil
	}
	componentInfo, ok := info.ComponentSection["component_info"].(map[string]any)
	if !ok {
		return nil
	}
	module, ok := componentInfo["terraform_config"].(*tfconfig.Module)
	if !ok || module == nil {
		return nil
	}

	keys := make(map[string]bool)
	for name, variable := range module.Variables {
		if variable != nil && variable.Sensitive {
			keys[name] = true
		}
	}
	return keys
}

// RejectComputedTerraformVars ensures display-only degraded values cannot cross
// into Terraform through either tfvars JSON or TF_VAR_ environment variables.
func RejectComputedTerraformVars(vars map[string]any) error {
	defer perf.Track(nil, "terraform.RejectComputedTerraformVars")()

	for key, value := range vars {
		if containsComputedTerraformValue(value) {
			return fmt.Errorf("%w: %q", errUtils.ErrUnresolvedComputedTerraformVar, key)
		}
	}
	return nil
}

// containsComputedTerraformValue recursively detects display-only placeholders in Terraform input values.
func containsComputedTerraformValue(value any) bool {
	switch v := value.(type) {
	case degradation.AtmosComputedValue:
		return true
	case *degradation.AtmosComputedValue:
		return v != nil
	case map[string]any:
		for _, child := range v {
			if containsComputedTerraformValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsComputedTerraformValue(child) {
				return true
			}
		}
	}
	return false
}

// DiskSafeVars returns ComponentVarsSection with secret-bearing top-level keys removed,
// so resolved secrets are never written to the on-disk varfile. Returns the original map
// unchanged when no secret-bearing variables were detected.
func DiskSafeVars(info *schema.ConfigAndStacksInfo) map[string]any {
	defer perf.Track(nil, "terraform.DiskSafeVars")()

	testSecretVars := terraformTestSecretVars(info)
	if len(info.TerraformSecretVarKeys) == 0 && len(testSecretVars) == 0 {
		return info.ComponentVarsSection
	}
	safe := make(map[string]any, len(info.ComponentVarsSection))
	for k, v := range info.ComponentVarsSection {
		if info.TerraformSecretVarKeys[k] {
			continue
		}
		if _, collidesWithSecretTestVar := testSecretVars[k]; collidesWithSecretTestVar {
			continue
		}
		safe[k] = v
	}
	return safe
}

// varsWithoutKeys removes excluded top-level variables, returning the original map when no exclusions exist.
func varsWithoutKeys(vars map[string]any, excluded map[string]bool) map[string]any {
	if len(excluded) == 0 {
		return vars
	}
	// `excluded` can include declared inputs that this stack does not supply, so
	// use the source size as the allocation hint rather than subtracting it.
	safe := make(map[string]any, len(vars))
	for k, v := range vars {
		if !excluded[k] {
			safe[k] = v
		}
	}
	return safe
}

// SecretVarEnv returns the TF_VAR_<name> environment entries for the component's
// secret-bearing variables, for injection into the subprocess environment.
func SecretVarEnv(info *schema.ConfigAndStacksInfo) ([]string, error) {
	defer perf.Track(nil, "terraform.SecretVarEnv")()

	if len(info.TerraformSecretVarKeys) == 0 {
		return nil, nil
	}
	secret := make(map[string]any, len(info.TerraformSecretVarKeys))
	for k := range info.TerraformSecretVarKeys {
		if v, ok := info.ComponentVarsSection[k]; ok {
			secret[k] = v
		}
	}
	return tfvars.SecretEnv(secret)
}

// TerraformTestVars returns test.vars for the test subcommand, or nil for other commands and absent values.
func TerraformTestVars(info *schema.ConfigAndStacksInfo) map[string]any {
	defer perf.Track(nil, "terraform.TerraformTestVars")()

	if info == nil || info.SubCommand != "test" {
		return nil
	}
	testSection, ok := info.ComponentSection[cfg.TestSectionName].(map[string]any)
	if !ok {
		return nil
	}
	vars, ok := testSection[cfg.VarsSectionName].(map[string]any)
	if !ok || len(vars) == 0 {
		return nil
	}
	return vars
}

// HasTerraformTestVars reports whether the current test command supplies additional test variables.
func HasTerraformTestVars(info *schema.ConfigAndStacksInfo) bool {
	defer perf.Track(nil, "terraform.HasTerraformTestVars")()

	return len(TerraformTestVars(info)) > 0
}

// DiskSafeTerraformTestVars excludes secret-bearing values from the variables written to the test varfile.
func DiskSafeTerraformTestVars(info *schema.ConfigAndStacksInfo) map[string]any {
	defer perf.Track(nil, "terraform.DiskSafeTerraformTestVars")()

	vars := TerraformTestVars(info)
	if len(vars) == 0 {
		return nil
	}
	safe, _ := tfvars.Partition(vars, atmosio.ContainsSecret)
	return safe
}

// terraformTestSecretVars selects test variables that must be passed through the subprocess environment.
func terraformTestSecretVars(info *schema.ConfigAndStacksInfo) map[string]any {
	vars := TerraformTestVars(info)
	if len(vars) == 0 {
		return nil
	}
	_, secret := tfvars.Partition(vars, atmosio.ContainsSecret)
	return secret
}

// TerraformTestSecretVarEnv encodes secret-bearing test variables as TF_VAR_ environment entries.
func TerraformTestSecretVarEnv(info *schema.ConfigAndStacksInfo) ([]string, error) {
	defer perf.Track(nil, "terraform.TerraformTestSecretVarEnv")()

	secret := terraformTestSecretVars(info)
	if len(secret) == 0 {
		return nil, nil
	}
	return tfvars.SecretEnv(secret)
}

// ConstructTerraformComponentWorkingDir constructs the working dir for a terraform component in a stack.
func ConstructTerraformComponentWorkingDir(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "terraform.ConstructTerraformComponentWorkingDir")()

	// Check if a provisioner (source or workdir) set a workdir path.
	// This takes precedence over the standard component path.
	// Use filepath.FromSlash to normalize path separators for cross-platform compatibility.
	if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
		return filepath.FromSlash(workdirPath)
	}

	// If we have a resolved absolute path, use GetComponentPath.
	// Otherwise, just construct the path directly (for tests and simple cases).
	if atmosConfig.TerraformDirAbsolutePath != "" {
		path, err := u.GetComponentPath(atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
		if err == nil {
			return path
		}
		// Log error but still try to return something.
		log.Debug("Failed to resolve component path, falling back to construction", "error", err)
	}

	// Direct path construction without converting to absolute.
	// This maintains compatibility with tests that use simple paths.
	return filepath.Join(
		atmosConfig.BasePath,
		atmosConfig.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
	)
}

// ConstructTerraformComponentPlanfileName constructs the planfile name for a terraform component in a stack.
func ConstructTerraformComponentPlanfileName(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "terraform.ConstructTerraformComponentPlanfileName")()

	var planFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		planFile = fmt.Sprintf("%s-%s.planfile", info.ContextPrefix, info.Component)
	} else {
		planFile = fmt.Sprintf("%s-%s-%s.planfile", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}

	return planFile
}

// ConstructTerraformComponentVarfileName constructs the varfile name for a terraform component in a stack.
func ConstructTerraformComponentVarfileName(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "terraform.ConstructTerraformComponentVarfileName")()

	var varFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		varFile = fmt.Sprintf("%s-%s.terraform.tfvars.json", info.ContextPrefix, info.Component)
	} else {
		varFile = fmt.Sprintf("%s-%s-%s.terraform.tfvars.json", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}

	return varFile
}

// ConstructTerraformComponentTestVarfileName constructs the test varfile name for a terraform component in a stack.
func ConstructTerraformComponentTestVarfileName(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "terraform.ConstructTerraformComponentTestVarfileName")()

	var varFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		varFile = fmt.Sprintf("%s-%s.terraform.test.tfvars.json", info.ContextPrefix, info.Component)
	} else {
		varFile = fmt.Sprintf("%s-%s-%s.terraform.test.tfvars.json", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}

	return varFile
}

// ConstructTerraformComponentVarfilePath constructs the varfile path for a terraform component in a stack.
func ConstructTerraformComponentVarfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "terraform.ConstructTerraformComponentVarfilePath")()

	return filepath.Join(
		ConstructTerraformComponentWorkingDir(atmosConfig, info),
		ConstructTerraformComponentVarfileName(info),
	)
}

// ConstructTerraformComponentTestVarfilePath constructs the test varfile path for a terraform component in a stack.
func ConstructTerraformComponentTestVarfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "terraform.ConstructTerraformComponentTestVarfilePath")()

	return filepath.Join(
		ConstructTerraformComponentWorkingDir(atmosConfig, info),
		ConstructTerraformComponentTestVarfileName(info),
	)
}

// ConstructTerraformComponentPlanfilePath constructs the planfile path for a terraform component in a stack.
func ConstructTerraformComponentPlanfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "terraform.ConstructTerraformComponentPlanfilePath")()

	return filepath.Join(
		ConstructTerraformComponentWorkingDir(atmosConfig, info),
		ConstructTerraformComponentPlanfileName(info),
	)
}
