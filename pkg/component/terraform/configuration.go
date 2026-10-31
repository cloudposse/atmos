package terraform

import (
	"context"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	awsidentity "github.com/cloudposse/atmos/pkg/auth/identities/aws"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func prepareConfiguration(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
	if _, owned := info.ComponentSettingsSection["configuration"]; !owned {
		return nil, nil
	}
	if config.ComponentConfigurations == nil {
		return nil, errUtils.ErrPluginUnsupportedExtension
	}
	if err := checkComponentRestrictions(info); err != nil {
		return nil, err
	}
	if err := checkStackOverrides(info); err != nil {
		return nil, err
	}
	if info.DryRun {
		return nil, nil
	}
	// Plugins do not inherit PATH. Resolve the selected host executable before
	// Terragrunt evaluates version constraints or dependency outputs.
	binary, err := exec.LookPath(info.Command)
	if err != nil {
		return nil, errUtils.Build(fmt.Errorf("%w: Terraform executable of component %q in stack %q: %w", errUtils.ErrPluginInvalidConfig, info.ComponentFromArg, info.Stack, err)).
			WithContext("component", info.ComponentFromArg).
			WithContext("stack", info.Stack).
			WithContext("executable", info.Command).
			WithHint("The unit's `terraform_binary` or the component's `command` must name an installed executable, a toolchain tool from `dependencies.tools`, or an absolute path").
			Err()
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPluginInvalidConfig, err)
	}
	info.Command = binary
	var prepared *schema.PreparedComponentConfiguration
	if usesConfigurationPlan(info) {
		if err = useRetainedWorkdir(config, info); err != nil {
			return nil, err
		}
		prepared, err = restoreConfigurationPlan(ctx, config, info)
	} else {
		prepared, err = config.ComponentConfigurations.Prepare(ctx, info)
	}
	if err != nil || prepared == nil {
		return prepared, err
	}
	if err = applyStackOverrides(config, info, prepared); err != nil {
		return nil, err
	}
	if err = applyIAMRole(ctx, info, prepared.IAM); err != nil {
		return nil, err
	}
	info.ComponentVarsSection = prepared.Inputs
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
	return prepared, nil
}

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
	role := awsidentity.InlineRole{Role: iam.Role, SessionName: iam.SessionName, Duration: time.Duration(iam.DurationSeconds) * time.Second, WebIdentityToken: iam.WebIdentityToken}
	if iam.WebIdentityTokenFile != "" {
		token, err := os.ReadFile(iam.WebIdentityTokenFile)
		if err != nil {
			return iamRoleError(info, iam, fmt.Errorf("%w: web identity token file: %w", errUtils.ErrAuthenticationFailed, err))
		}
		role.WebIdentityToken = strings.TrimSpace(string(token))
	}
	if iam.WebIdentityTokenLiteral && role.WebIdentityToken == "" {
		return iamRoleError(info, iam, fmt.Errorf("%w: the web identity token is not stored with a saved plan", errUtils.ErrAuthenticationFailed))
	}
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
