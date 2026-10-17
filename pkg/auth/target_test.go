package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// targetOptions provides three independent scopes with observable overrides.
func targetOptions() *TargetAuthOptions {
	return &TargetAuthOptions{
		AtmosConfig: &schema.AtmosConfiguration{Auth: schema.AuthConfig{Identities: map[string]schema.Identity{
			"global":    {Kind: "aws/user", Default: true},
			"component": {Kind: "aws/user"}, "target": {Kind: "aws/user"},
		}}},
		Info: &schema.ConfigAndStacksInfo{
			Stack: "dev", Identity: "component",
			AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "component"}},
			ComponentSection: map[string]any{"auth": map[string]any{"identities": map[string]any{
				"component": map[string]any{"default": true},
			}}},
		},
		TargetConfig: map[string]any{"auth": map[string]any{"identities": map[string]any{
			"target": map[string]any{"default": true},
		}}},
	}
}

// TestResolveTargetAuthSelection verifies caller selection and target defaults
// across merged scopes while preserving the caller's credentials and inputs.
func TestResolveTargetAuthSelection(t *testing.T) {
	for _, tc := range []struct {
		name, requested, explicit, want string
		promptProfile                   bool
	}{
		{name: "target default", want: "target"},
		{name: "target explicit", explicit: "global", want: "global"},
		{name: "caller overrides target", requested: "component", explicit: "global", want: "component"},
		{name: "prompt selection", requested: cfg.IdentityFlagSelectValue, want: "component"},
		{name: "prompt profile fallback", requested: cfg.IdentityFlagSelectValue, promptProfile: true, want: "component"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := targetOptions()
			options.RequestedIdentity = tc.requested
			if tc.explicit != "" {
				options.TargetConfig["auth"].(map[string]any)["identity"] = tc.explicit
			}
			if tc.promptProfile {
				options.Info.Identity = cfg.IdentityFlagSelectValue
			}
			originalIdentity := options.Info.Identity
			manager := types.NewMockAuthManager(gomock.NewController(t))
			manager.EXPECT().GetChain().Return([]string{"upstream", tc.want})
			targetContext := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: tc.want}}
			manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: targetContext})
			var captured *schema.AuthConfig
			options.CreateManager = func(selected string, merged *schema.AuthConfig, selectValue string, ac *schema.AtmosConfiguration, stack string) (AuthManager, error) {
				assert.Equal(t, options.AtmosConfig, ac)
				assert.Equal(t, "dev", stack)
				assert.Equal(t, cfg.IdentityFlagSelectValue, selectValue)
				assert.False(t, merged.Identities["global"].Default)
				assert.False(t, merged.Identities["component"].Default)
				assert.True(t, merged.Identities["target"].Default)
				wantSelection := tc.want
				if tc.requested == "" && tc.explicit == "" {
					wantSelection = ""
				}
				assert.Equal(t, wantSelection, selected)
				captured = merged
				return manager, nil
			}
			resolved, err := ResolveTargetAuth(options)
			require.NoError(t, err)
			assert.Equal(t, tc.want, resolved.Identity)
			assert.Same(t, manager, resolved.AuthManager)
			assert.Same(t, targetContext, resolved.AuthContext)
			assert.Equal(t, originalIdentity, options.Info.Identity)
			assert.Nil(t, options.Info.AuthManager)
			assert.True(t, options.AtmosConfig.Auth.Identities["global"].Default)
			resolved.AuthContext.AWS.Profile = "changed-target"
			assert.Equal(t, "component", options.Info.AuthContext.AWS.Profile)
			options.Info.AuthContext.AWS.Profile = "changed-parent"
			assert.Equal(t, "changed-target", resolved.AuthContext.AWS.Profile)
			captured.Identities["global"] = schema.Identity{Kind: "changed"}
			assert.Equal(t, "aws/user", options.AtmosConfig.Auth.Identities["global"].Kind)
			options.AtmosConfig.Auth.Identities["component"] = schema.Identity{Kind: "changed-parent"}
			assert.Equal(t, "aws/user", captured.Identities["component"].Kind)
			options.TargetConfig["auth"].(map[string]any)["identities"].(map[string]any)["target"].(map[string]any)["default"] = false
			assert.True(t, captured.Identities["target"].Default)
		})
	}
}

// TestResolveTargetAuthNoAuthentication verifies absent and disabled scopes do
// not invoke authentication and disabled credentials never leak from the parent.
func TestResolveTargetAuthNoAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name      string
		target    map[string]any
		disabled  bool
		requested string
	}{
		{name: "absent"},
		{name: "nil", target: map[string]any{"auth": nil}},
		{name: "empty", target: map[string]any{"auth": map[string]any{}}},
		{name: "parent disabled", disabled: true},
		{name: "caller disabled", requested: cfg.IdentityFlagDisabledValue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := targetOptions()
			options.TargetConfig, options.Info.AuthDisabled, options.RequestedIdentity = tc.target, tc.disabled, tc.requested
			options.CreateManager = func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, string) (AuthManager, error) {
				t.Fatal("absent or disabled target must not authenticate")
				return nil, nil
			}
			resolved, err := ResolveTargetAuth(options)
			require.NoError(t, err)
			if tc.disabled || tc.requested != "" {
				assert.NotSame(t, options.Info, resolved)
				assert.True(t, resolved.AuthDisabled)
				assert.Empty(t, resolved.Identity)
				assert.Nil(t, resolved.AuthManager)
				assert.Nil(t, resolved.AuthContext)
				assert.Equal(t, "component", options.Info.AuthContext.AWS.Profile)
			} else {
				assert.Same(t, options.Info, resolved)
			}
		})
	}
}

// TestResolveTargetAuthInvalidScopes rejects malformed scope and identity data
// before invoking a manager or changing the component credentials.
func TestResolveTargetAuthInvalidScopes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*TargetAuthOptions)
		want      error
	}{
		{"non-mapping auth", func(o *TargetAuthOptions) { o.TargetConfig["auth"] = "target" }, errUtils.ErrInvalidAuthConfig},
		{"non-string identity", func(o *TargetAuthOptions) { o.TargetConfig["auth"] = map[string]any{"identity": 123} }, errUtils.ErrInvalidAuthConfig},
		{"empty identity", func(o *TargetAuthOptions) { o.TargetConfig["auth"] = map[string]any{"identity": ""} }, errUtils.ErrInvalidAuthConfig},
		{"undefined component default", func(o *TargetAuthOptions) {
			o.Info.ComponentSection["auth"] = map[string]any{"identities": map[string]any{"invalid": map[string]any{"default": true}}}
		}, errUtils.ErrInvalidIdentityConfig},
		{"undefined target default", func(o *TargetAuthOptions) {
			o.TargetConfig["auth"] = map[string]any{"identities": map[string]any{"invalid": map[string]any{"default": true}}}
		}, errUtils.ErrInvalidIdentityConfig},
		{"missing prompt selection", func(o *TargetAuthOptions) {
			o.RequestedIdentity = cfg.IdentityFlagSelectValue
			o.Info.Identity = ""
			o.Info.AuthContext = nil
		}, errUtils.ErrFailedToInitializeAuthManager},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := targetOptions()
			tc.configure(options)
			options.CreateManager = func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, string) (AuthManager, error) {
				t.Error("invalid scope must not authenticate")
				return nil, nil
			}
			resolved, err := ResolveTargetAuth(options)
			require.Error(t, err)
			if tc.want != nil {
				assert.ErrorIs(t, err, tc.want)
			}
			assert.Nil(t, resolved)
			assert.Nil(t, options.Info.AuthManager)
		})
	}
}

// TestResolveTargetAuthManagerOutcomes ensures authentication failures cannot
// fall back to parent credentials and incomplete managers do not reuse them.
func TestResolveTargetAuthManagerOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name               string
		failure            error
		nilManager, noInfo bool
	}{
		{name: "authentication denied", failure: errUtils.ErrAuthenticationFailed},
		{name: "no manager", nilManager: true},
		{name: "no stack context", noInfo: true},
		{name: "empty identity chain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := targetOptions()
			options.RequestedIdentity = "global"
			manager := types.NewMockAuthManager(gomock.NewController(t))
			options.CreateManager = func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, string) (AuthManager, error) {
				if tc.failure != nil || tc.nilManager {
					return nil, tc.failure
				}
				return manager, nil
			}
			if tc.failure == nil && !tc.nilManager {
				manager.EXPECT().GetChain().Return(nil)
				var info *schema.ConfigAndStacksInfo
				if !tc.noInfo {
					info = &schema.ConfigAndStacksInfo{}
				}
				manager.EXPECT().GetStackInfo().Return(info)
			}
			resolved, err := ResolveTargetAuth(options)
			if tc.failure != nil || tc.nilManager {
				require.ErrorIs(t, err, errUtils.ErrFailedToInitializeAuthManager)
				if tc.failure != nil {
					assert.ErrorIs(t, err, tc.failure)
				}
				assert.Nil(t, resolved)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "global", resolved.Identity)
				assert.Nil(t, resolved.AuthContext)
			}
			assert.Equal(t, "component", options.Info.AuthContext.AWS.Profile)
		})
	}
}

// TestResolveTargetAuthDefaultFactory reports missing configuration through the
// production authenticator without silently using the parent's credentials.
func TestResolveTargetAuthDefaultFactory(t *testing.T) {
	options := &TargetAuthOptions{
		AtmosConfig:  &schema.AtmosConfiguration{},
		Info:         &schema.ConfigAndStacksInfo{Stack: "dev"},
		TargetConfig: map[string]any{"auth": map[string]any{"identity": "missing"}},
	}
	resolved, err := ResolveTargetAuth(options)
	require.ErrorIs(t, err, errUtils.ErrFailedToInitializeAuthManager)
	assert.ErrorIs(t, err, errUtils.ErrAuthNotConfigured)
	assert.Nil(t, resolved)
}
