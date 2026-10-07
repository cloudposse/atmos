package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPublishTargetIdentityIsolation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, requested, want string }{
		{"target overrides inherited", "", "writer"},
		{"explicit overrides target", "caller", "caller"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vars := NewVariables()
			vars.AtmosConfig = &schema.AtmosConfiguration{Auth: schema.AuthConfig{Identities: map[string]schema.Identity{
				"caller": {Kind: "aws/user"}, "writer": {Kind: "aws/user"},
			}}}
			original := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "component"}}
			vars.PublishInfo = &schema.ConfigAndStacksInfo{Identity: "component", AuthContext: original}
			block := map[string]any{"kind": "aws/s3", "auth": map[string]any{"identity": "writer"}}
			_, resolvedBlock, err := resolvePublishTarget(block, vars)
			require.NoError(t, err)
			options := publishAuthOptions(&schema.WorkflowStep{Identity: tc.requested}, vars, "assets", resolvedBlock)
			manager := types.NewMockAuthManager(gomock.NewController(t))
			manager.EXPECT().GetChain().Return([]string{tc.want})
			targetAuth := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: tc.want}}
			manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: targetAuth})
			options.CreateManager = func(identity string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration, _ string) (auth.AuthManager, error) {
				assert.Equal(t, tc.want, identity)
				return manager, nil
			}
			in := &target.PublishInput{AtmosConfig: vars.AtmosConfig, TargetConfig: resolvedBlock}
			require.NoError(t, authenticatePublish(in, options))
			assert.Same(t, targetAuth, in.AuthContext)
			assert.Same(t, original, vars.PublishInfo.AuthContext)
			assert.Equal(t, "writer", block["auth"].(map[string]any)["identity"])
		})
	}
}

func TestPublishDisabledIdentity(t *testing.T) {
	t.Parallel()
	vars := NewVariables()
	vars.PublishInfo = &schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "parent"}}}
	block := map[string]any{"kind": "aws/s3", "auth": map[string]any{"identity": "writer"}}
	options := publishAuthOptions(&schema.WorkflowStep{Identity: "false"}, vars, "disabled", block)
	options.CreateManager = func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, string) (auth.AuthManager, error) {
		t.Fatal("disabled auth must not authenticate")
		return nil, nil
	}
	in := &target.PublishInput{TargetConfig: block}
	require.NoError(t, authenticatePublish(in, options))
	assert.Nil(t, in.AuthContext)
	assert.Nil(t, in.EnvProvider)
	assert.NotNil(t, vars.PublishInfo.AuthContext)
}

func TestPublishRepositoryIdentityFallback(t *testing.T) {
	t.Parallel()
	vars := NewVariables()
	vars.AtmosConfig = &schema.AtmosConfiguration{Git: schema.GitConfig{Repositories: map[string]schema.GitRepository{
		"deployment": {URI: "https://example.com/repo.git", Auth: schema.GitAuthConfig{Identity: "repository-writer"}},
	}}, Auth: schema.AuthConfig{Identities: map[string]schema.Identity{"repository-writer": {Kind: "aws/user"}}}}
	block := map[string]any{"kind": "git", "repository": "deployment"}
	options := publishAuthOptions(&schema.WorkflowStep{}, vars, "git", block)
	manager := types.NewMockAuthManager(gomock.NewController(t))
	manager.EXPECT().GetChain().Return([]string{"repository-writer"})
	manager.EXPECT().GetStackInfo().Return(nil)
	options.CreateManager = func(identity string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration, _ string) (auth.AuthManager, error) {
		assert.Equal(t, "repository-writer", identity)
		return manager, nil
	}
	in := &target.PublishInput{AtmosConfig: vars.AtmosConfig, TargetConfig: block}
	require.NoError(t, authenticatePublish(in, options))
	assert.Same(t, manager, in.EnvProvider)
}

func TestPublishAuthPreservesValidationAndPrompt(t *testing.T) {
	t.Parallel()
	vars := NewVariables()
	invalid := map[string]any{"auth": "invalid"}
	options := publishAuthOptions(&schema.WorkflowStep{Identity: "caller"}, vars, "assets", invalid)
	require.Error(t, auth.ValidateTargetAuth(options), "explicit identity must not hide invalid target auth")
	options = publishAuthOptions(&schema.WorkflowStep{Identity: cfg.IdentityFlagSelectValue}, vars, "assets", map[string]any{})
	// Use the configured sentinel, which may differ from its display spelling.
	options.RequestedIdentity = cfg.IdentityFlagSelectValue
	prepared := preparePublishAuth(options)
	assert.Empty(t, prepared.RequestedIdentity)
	assert.Equal(t, cfg.IdentityFlagSelectValue, prepared.TargetConfig["auth"].(map[string]any)["identity"])
	assert.Empty(t, options.TargetConfig, "preparing auth must not mutate caller's target")
}
