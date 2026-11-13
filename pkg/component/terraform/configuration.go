package terraform

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	errUtils "github.com/cloudposse/atmos/errors"
	awsidentity "github.com/cloudposse/atmos/pkg/auth/identities/aws"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/backend"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// prepareConfiguration prepares a provider-owned component whose saved plan, if any, still has its
// retained working directory. Execution uses prepareConfigurationForExecution, which can also
// re-provision the directory of a plan restored on another runner.
func prepareConfiguration(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
	prepared, rebuild, err := prepareConfigurationForExecution(ctx, config, info)
	if err == nil && rebuild != nil {
		return nil, fmt.Errorf("%w: reviewed working directory changed", errUtils.ErrPlanVerificationFailed)
	}
	return prepared, err
}

// prepareConfigurationForExecution is prepareConfiguration that reports a pending rebuild: the
// returned reviewedRebuild is non-nil when the reviewed plan was verified but its working directory
// must be re-provisioned and checked before the plan can be applied.
func prepareConfigurationForExecution(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, *reviewedRebuild, error) {
	if _, owned := info.ComponentSettingsSection["configuration"]; !owned {
		return nil, nil, nil
	}
	if err := checkConfigurationPreconditions(config, info); err != nil {
		return nil, nil, err
	}
	if info.DryRun {
		return nil, nil, nil
	}
	if err := resolveConfigurationBinary(info); err != nil {
		return nil, nil, err
	}
	prepared, rebuild, err := obtainConfiguration(ctx, config, info)
	if err != nil || prepared == nil {
		return prepared, nil, err
	}
	if err = applyStackOverrides(config, info, prepared); err != nil {
		return nil, nil, err
	}
	// An operation the provider excludes never runs, so it must not need the unit's role.
	if !excludedFromOperation(info, prepared) {
		if err = applyIAMRole(ctx, info, prepared.IAM); err != nil {
			return nil, nil, err
		}
	}
	applyPreparedConfiguration(config, info, prepared)
	return prepared, rebuild, nil
}

// checkConfigurationPreconditions rejects what no preparation can fix: no provider, a restricted
// component, or a stack that overrides what the provider owns.
func checkConfigurationPreconditions(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if config.ComponentConfigurations == nil {
		return errUtils.ErrPluginUnsupportedExtension
	}
	dropCIPlanLocation(info)
	if err := checkComponentRestrictions(info); err != nil {
		return err
	}
	return checkStackOverrides(info)
}

// resolveConfigurationBinary resolves the selected Terraform executable to an absolute path.
// Plugins do not inherit PATH, so this happens before Terragrunt evaluates version constraints
// or dependency outputs.
func resolveConfigurationBinary(info *schema.ConfigAndStacksInfo) error {
	binary, err := exec.LookPath(info.Command)
	if err != nil {
		return errUtils.Build(fmt.Errorf("%w: Terraform executable of component %q in stack %q: %w", errUtils.ErrPluginInvalidConfig, info.ComponentFromArg, info.Stack, err)).
			WithContext("component", info.ComponentFromArg).
			WithContext("stack", info.Stack).
			WithContext("executable", info.Command).
			WithHint("The unit's `terraform_binary` or the component's `command` must name an installed executable, a toolchain tool from `dependencies.tools`, or an absolute path").
			Err()
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrPluginInvalidConfig, err)
	}
	info.Command = binary
	return nil
}

// obtainConfiguration evaluates the unit, or restores the reviewed preparation of a saved plan. The
// rebuild is non-nil when the saved plan lost its working directory and must be re-provisioned.
func obtainConfiguration(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, *reviewedRebuild, error) {
	if !usesConfigurationPlan(info) {
		prepared, err := config.ComponentConfigurations.Prepare(ctx, info)
		return prepared, nil, err
	}
	if err := useRetainedWorkdir(config, info); err != nil {
		return nil, nil, err
	}
	prepared, rebuild, err := restoreOrRebuildConfigurationPlan(ctx, config, info)
	if err != nil || rebuild == nil {
		return prepared, rebuild, err
	}
	rebuild.prepared, err = prepareForRebuild(ctx, config, info, rebuild)
	return rebuild.prepared, rebuild, err
}

// applyPreparedConfiguration records the prepared values on the invocation.
func applyPreparedConfiguration(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) {
	info.ComponentVarsSection = prepared.Inputs
	info.PreparedSensitiveInputKeys = prepared.SensitiveInputKeys
	info.ComponentExecution = prepared.Execution
	info.ComponentBackendType, info.ComponentBackendSection = prepared.BackendType, prepared.Backend
	info.TerraformWorkspace = prepared.Workspace
	info.ComponentSection["vars"] = prepared.Inputs
	source, _ := info.ComponentSection["source"].(map[string]any)
	source = maps.Clone(source)
	if source == nil {
		source = make(map[string]any)
	}
	source["uri"] = prepared.Source
	// A local module changes without its location changing. The source provisioner stamps a local
	// directory and refreshes the retained workdir only when that stamp changes, so no TTL is set.
	info.ComponentSection["source"] = source
	info.ComponentSection["backend_type"] = prepared.BackendType
	info.ComponentSection["backend"] = prepared.Backend
	config.Components.Terraform.AutoGenerateBackendFile = !prepared.BackendGenerated
	warnOnCacheLocalState(info, prepared)
}

// prepareForRebuild evaluates the unit again for a plan restored without its working directory, the
// way `plan` did, so the directory is regenerated from the checkout's own configuration instead of
// from the restored record. The evaluation must reproduce the reviewed backend. The plan file already
// embodies the inputs, so none are kept.
//
// The evaluation runs as the reviewed `plan` operation, so a generate block that depends on
// get_terraform_command() writes the files that were reviewed. Its hooks, extra arguments, and error
// policies are not used: the reviewed execution metadata is, exactly as for a retained workdir, so a
// condition that evaluates differently on this runner cannot skip a reviewed hook. Atmos selects the
// hooks of the actual command from that metadata.
func prepareForRebuild(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, rebuild *reviewedRebuild) (*schema.PreparedComponentConfiguration, error) {
	fresh := *info
	fresh.UseTerraformPlan, fresh.PlanFile, fresh.StoredPlanFile = false, "", ""
	fresh.SubCommand, fresh.SubCommand2 = reviewedOperation, ""
	prepared, err := config.ComponentConfigurations.Prepare(ctx, &fresh)
	if err != nil || prepared == nil {
		return nil, err
	}
	digest, err := backendIdentityDigest(prepared.Backend)
	if err != nil {
		return nil, err
	}
	if digest != rebuild.plan.BackendDigest || prepared.BackendType != rebuild.plan.Prepared.BackendType || prepared.Workspace != rebuild.plan.Prepared.Workspace {
		return nil, fmt.Errorf("%w: reviewed backend identity changed", errUtils.ErrPlanVerificationFailed)
	}
	prepared.Inputs = map[string]any{}
	prepared.Execution = rebuild.plan.Prepared.Execution
	return prepared, nil
}

// reviewedOperation is the command a reviewed plan was evaluated for.
const reviewedOperation = "plan"

// terragruntCacheDirectory is where Terragrunt keeps its working copies of a unit.
const terragruntCacheDirectory = ".terragrunt-cache"

// cacheLocalStatePath returns the local state file path when it lives inside a Terragrunt cache.
func cacheLocalStatePath(prepared *schema.PreparedComponentConfiguration) (string, bool) {
	if prepared.BackendType != "local" {
		return "", false
	}
	path, _ := prepared.Backend["path"].(string)
	for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
		if segment == terragruntCacheDirectory {
			return path, true
		}
	}
	return "", false
}

// warnOnCacheLocalState tells the user that state kept inside .terragrunt-cache is easy to lose.
// The location is deliberate (it is Terragrunt's own backend identity, so existing state keeps
// working), but cleaning the cache, or checking out to another path, silently starts from empty state.
func warnOnCacheLocalState(info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) {
	path, inCache := cacheLocalStatePath(prepared)
	if !inCache {
		return
	}
	ui.Warningf("Component `%s` in stack `%s` keeps its local state at `%s`, inside `%s`. "+
		"The location depends on the checkout path, and deleting the cache (for example by cleaning untracked files) loses the state.",
		info.ComponentFromArg, info.Stack, path, terragruntCacheDirectory)
	ui.Hint("Configure `remote_state` with a remote backend, or a local backend with a stable path such as " +
		"`path = \"${get_terragrunt_dir()}/terraform.tfstate\"`, and move the existing state file there.")
}

// useRetainedWorkdir points the invocation at the directory the reviewed plan was created in.
// A new process has not provisioned anything, so the path is derived the way provisioning does.
func useRetainedWorkdir(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	// A downloaded plan and record can be the only files in an explicit workdir.
	// Resolve a relative override without source ownership checks here; the
	// reviewed-plan restore verifies the exact transport set before provisioning.
	if override := workingDirectoryOverride(info); override != "" && !filepath.IsAbs(override) {
		clean := filepath.Clean(override)
		if clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			base, err := filepath.Abs(config.BasePath)
			if err != nil {
				return err
			}
			info.ComponentSection[provWorkdir.WorkdirPathKey] = filepath.Join(base, clean)
			return nil
		}
	}
	fallback, err := u.GetComponentPath(config, cfg.TerraformComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return fmt.Errorf("failed to resolve component path: %w", err)
	}
	_, err = component.ResolveRetainedWorkdir(config, info, cfg.TerraformComponentType, fallback)
	return err
}

func configurationArtifactName(info *schema.ConfigAndStacksInfo, suffix string) (string, bool) {
	if _, owned := info.ComponentSettingsSection["configuration"]; !owned {
		return "", false
	}
	return "atmos-" + configurationIdentity(info.Stack, info.ComponentFromArg) + suffix, true
}

// configurationIdentity names a stack and component pair for artifact files. The name must be the same in
// every process (a plan in one run is applied by another), so it is a plain content digest with no secret.
// It identifies a component for file naming only: it is not a credential hash and gives no security
// guarantee, which is why a non-cryptographic 128-bit FNV-1a digest is used instead of SHA-256.
func configurationIdentity(stack, component string) string {
	digest := fnv.New128a()
	// Writing to a hash never fails.
	_, _ = digest.Write([]byte(stack + "\x00" + component))
	return hex.EncodeToString(digest.Sum(nil))
}

// prepareConfigurationEnvironment applies the same authentication, RC, cache,
// and toolchain environment as Terraform before the evaluator can spawn children.
func (e *Executor) prepareConfigurationEnvironment(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, shouldProcess bool) (*dependencies.ToolchainEnvironment, error) {
	if _, owned := info.ComponentSettingsSection["configuration"]; !owned || info.DryRun {
		return nil, nil
	}
	if err := checkComponentRestrictions(info); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := e.prepareAuthEnvironmentFn(config, info); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var tenv *dependencies.ToolchainEnvironment
	if shouldProcess {
		var err error
		tenv, err = e.resolveConfigurationToolsFn(config, info)
		if err != nil {
			return nil, err
		}
		info.Command = tenv.Resolve(info.Command)
	}
	disableTerraformPluginCacheForExecution(config, info)
	if err := assembleComponentEnvVars(config, info, tenv); err != nil {
		return nil, err
	}
	return tenv, nil
}

// assumeInlineRole is replaced by tests that must not call STS.
var assumeInlineRole = awsidentity.AssumeInlineRole

// applyIAMRole assumes the role a component asks for, on top of the credentials of the invocation, and
// exports the session to every command the component runs. An Atmos identity that the user selected
// explicitly decides who the component runs as, so it replaces the role instead of being layered under it.
// The session lives only in the environment of this invocation: it is neither written to a file nor to a
// saved plan, and it is registered with the output masker.
func applyIAMRole(ctx context.Context, info *schema.ConfigAndStacksInfo, iam *schema.ComponentIAM) error {
	if iam == nil {
		return nil
	}
	if requested := requestedIdentity(info); requested != "" {
		ui.Infof("Component `%s` in stack `%s` runs as Atmos identity `%s`; its Terragrunt `iam_role` `%s` is not assumed.", info.ComponentFromArg, info.Stack, info.Identity, iam.Role)
		return nil
	}
	token, err := webIdentityToken(iam)
	if err != nil {
		return iamRoleError(info, iam, err)
	}
	role := awsidentity.InlineRole{Role: iam.Role, SessionName: iam.SessionName, Duration: time.Duration(iam.DurationSeconds) * time.Second, WebIdentityToken: token}
	base := info.SanitizedEnv
	if base == nil {
		base = os.Environ()
	}
	session, err := assumeInlineRole(ctx, role, append(slices.Clone(base), info.ComponentEnvList...))
	if err != nil {
		return iamRoleError(info, iam, err)
	}
	for _, key := range slices.Sorted(maps.Keys(session)) {
		info.ComponentEnvList = append(info.ComponentEnvList, key+"="+session[key])
	}
	return nil
}

// webIdentityToken returns the token to assume the role with: read from its file when one is set,
// otherwise the literal value. A literal token is not stored with a saved plan, so applying such a
// plan fails here instead of assuming the role with the base credentials.
func webIdentityToken(iam *schema.ComponentIAM) (string, error) {
	token := iam.WebIdentityToken
	if iam.WebIdentityTokenFile != "" {
		content, err := os.ReadFile(iam.WebIdentityTokenFile)
		if err != nil {
			return "", fmt.Errorf("%w: web identity token file: %w", errUtils.ErrAuthenticationFailed, err)
		}
		token = strings.TrimSpace(string(content))
		if token == "" {
			return "", fmt.Errorf("%w: web identity token file %s is empty", errUtils.ErrAuthenticationFailed, iam.WebIdentityTokenFile)
		}
	}
	if iam.WebIdentityTokenLiteral && token == "" {
		return "", fmt.Errorf("%w: the web identity token is not stored with a saved plan", errUtils.ErrAuthenticationFailed)
	}
	return token, nil
}

// writePreparedConfiguration verifies retained files before relinquishing the host backend
// and applying provider file policies.
func writePreparedConfiguration(directory string, prepared *schema.PreparedComponentConfiguration) error {
	if err := validateRetainedConfiguration(directory, prepared.Files); err != nil {
		return err
	}
	if prepared.BackendGenerated {
		if err := backend.RemoveHostBackend(directory); err != nil {
			return err
		}
	}
	return configuration.WriteFiles(directory, prepared.Files)
}

// validateRetainedConfiguration runs before any backend cleanup or file writes.
// A skipped Terraform file may disagree with the provider's resolved state identity.
// Accept different bytes only when both versions are provably free of backend/cloud
// declarations; otherwise fail closed without replacing the user's retained file.
func validateRetainedConfiguration(directory string, files []schema.ComponentConfigurationFile) error {
	defer perf.Track(nil, "terraform.validateRetainedConfiguration")()

	for _, file := range files {
		if file.IfExists != "skip" || !terraformConfigurationPath(file.Path) {
			continue
		}
		if err := validateRetainedFile(directory, file); err != nil {
			return err
		}
	}
	return nil
}

// requestedIdentity returns what the user asked for with --identity or ATMOS_IDENTITY, or empty when no
// identity was requested. A default identity that authentication selected on its own is not a request, so
// the role is assumed on top of it. Disabling authentication requests nothing either.
func requestedIdentity(info *schema.ConfigAndStacksInfo) string {
	requested := info.Identity
	if info.RequestedIdentity != nil {
		requested = *info.RequestedIdentity
	}
	if requested == cfg.IdentityFlagDisabledValue {
		return ""
	}
	return requested
}

func iamRoleError(info *schema.ConfigAndStacksInfo, iam *schema.ComponentIAM, cause error) error {
	return errUtils.Build(errUtils.ErrAuthenticationFailed).
		WithCause(cause).
		WithExplanationf("Component `%s` in stack `%s` sets the Terragrunt `iam_role` `%s`, and Atmos could not assume it", info.ComponentFromArg, info.Stack, iam.Role).
		WithHint("Check that the credentials in use may assume the role, and that its trust policy allows them").
		WithHint("To run as an Atmos identity instead of assuming `iam_role`, select it with `--identity <name>`").
		WithHint("A web identity token must be a file path when a plan is saved and applied later").
		WithContext("component", info.ComponentFromArg).
		WithContext("stack", info.Stack).
		Err()
}

func validateRetainedFile(directory string, file schema.ComponentConfigurationFile) error {
	if !filepath.IsLocal(file.Path) {
		return errUtils.ErrPluginProtocol
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	stat, err := root.Lstat(file.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() {
		return retainedBackendError(file.Path)
	}
	retained, err := root.ReadFile(file.Path)
	if err != nil {
		return err
	}
	if bytes.Equal(retained, file.Content) {
		return nil
	}
	if configurationHasBackend(file.Path, retained) || configurationHasBackend(file.Path, file.Content) {
		return retainedBackendError(file.Path)
	}
	return nil
}

func retainedBackendError(path string) error {
	return fmt.Errorf("%w: cannot verify backend identity for retained generated file %s (if_exists = \"skip\"); use matching generated content or an explicit overwrite policy", errUtils.ErrPluginInvalidConfig, path)
}

func terraformConfigurationPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".tf") || strings.HasSuffix(lower, ".tf.json")
}

// configurationHasBackend also returns true for unparseable configuration: absence
// of a backend cannot then be established. Diagnostics never expose retained secrets.
func configurationHasBackend(path string, content []byte) bool {
	parser := hclparse.NewParser()
	var file *hcl.File
	var diagnostics hcl.Diagnostics
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		file, diagnostics = parser.ParseJSON(content, path)
	} else {
		file, diagnostics = parser.ParseHCL(content, path)
	}
	if diagnostics.HasErrors() {
		return true
	}
	body, _, diagnostics := file.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}}})
	if diagnostics.HasErrors() {
		return true
	}
	for _, block := range body.Blocks {
		nested, _, diags := block.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "backend", LabelNames: []string{"type"}}, {Type: "cloud"}}})
		if diags.HasErrors() || len(nested.Blocks) != 0 {
			return true
		}
	}
	return false
}
