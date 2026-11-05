package step

import (
	"maps"

	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	publishAuthField     = "auth"
	publishIdentityField = "identity"
)

func publishAuthOptions(step *schema.WorkflowStep, vars *Variables, name string, block map[string]any) *auth.TargetAuthOptions {
	info := vars.PublishInfo
	if info == nil {
		info = &schema.ConfigAndStacksInfo{Stack: step.Stack}
	}
	requested := step.Identity
	if requested == "" {
		requested = vars.Flags[publishIdentityField]
	}
	if requested == "" && info.RequestedIdentity != nil {
		requested = *info.RequestedIdentity
	}
	config := vars.AtmosConfig
	if config == nil {
		config = &schema.AtmosConfiguration{}
	}
	return &auth.TargetAuthOptions{AtmosConfig: config, Info: info, TargetName: name, TargetConfig: block, RequestedIdentity: requested}
}

func authenticatePublish(in *target.PublishInput, options *auth.TargetAuthOptions) error {
	options = preparePublishAuth(options)
	resolved, err := auth.ResolveTargetAuth(options)
	if err != nil {
		return err
	}
	in.AuthContext = resolved.AuthContext
	in.EnvProvider, _ = resolved.AuthManager.(target.IdentityEnvironmentProvider)
	if resolved.AuthDisabled {
		// Suppress target and repository identity selection when authentication is disabled.
		delete(in.TargetConfig, publishAuthField)
		in.EnvProvider = nil
		return nil
	}
	if _, hasAuth := options.TargetConfig[publishAuthField]; hasAuth && resolved.Identity != "" {
		block, _ := in.TargetConfig[publishAuthField].(map[string]any)
		block = maps.Clone(block)
		if block == nil {
			block = make(map[string]any)
		}
		block[publishIdentityField] = resolved.Identity
		in.TargetConfig[publishAuthField] = block
	}
	return ensureGitPublishIdentity(in, options, resolved)
}

// ensureGitPublishIdentity resolves the repository identity when the target has no override.
func ensureGitPublishIdentity(in *target.PublishInput, options *auth.TargetAuthOptions, info *schema.ConfigAndStacksInfo) error {
	kind, _ := in.TargetConfig["kind"].(string)
	if kind != "git" || in.EnvProvider != nil {
		return nil
	}
	repo, _ := in.TargetConfig["repository"].(string)
	repository, err := atmosgit.ResolveRepository(&in.AtmosConfig.Git, repo)
	if err != nil {
		return err
	}
	identity := repository.Identity
	if block, ok := in.TargetConfig[publishAuthField].(map[string]any); ok {
		if selected, ok := block[publishIdentityField].(string); ok && selected != "" {
			identity = selected
		}
	}
	if identity == "" {
		return nil
	}
	copyOptions := *options
	copyOptions.Info = info
	copyOptions.TargetConfig = map[string]any{publishAuthField: map[string]any{publishIdentityField: identity}}
	resolved, err := auth.ResolveTargetAuth(&copyOptions)
	if err != nil {
		return err
	}
	in.EnvProvider, _ = resolved.AuthManager.(target.IdentityEnvironmentProvider)
	return nil
}

// preparePublishAuth selects an identity only after the original target block was validated.
func preparePublishAuth(options *auth.TargetAuthOptions) *auth.TargetAuthOptions {
	requested := cfg.NormalizeIdentityValue(options.RequestedIdentity)
	if requested == "" || requested == cfg.IdentityFlagDisabledValue {
		return options
	}
	cloned := *options
	cloned.TargetConfig = maps.Clone(options.TargetConfig)
	block, _ := cloned.TargetConfig[publishAuthField].(map[string]any)
	block = maps.Clone(block)
	if block == nil {
		block = make(map[string]any)
	}
	// Standalone steps have not prompted for a bare --identity yet. Let the
	// shared authenticator resolve the prompt instead of treating it as resolved.
	unresolvedPrompt := requested == cfg.IdentityFlagSelectValue && options.Info.Identity == "" && options.Info.AuthContext == nil
	if len(block) == 0 || unresolvedPrompt {
		block[publishIdentityField] = requested
	}
	if unresolvedPrompt {
		cloned.RequestedIdentity = ""
	}
	cloned.TargetConfig[publishAuthField] = block
	return &cloned
}
