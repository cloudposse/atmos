package deferred

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCredentialsAreLazyAndPreserveCaller(t *testing.T) {
	ctrl := gomock.NewController(t)
	factory := NewMockAuthFactory(ctrl)
	ac := &schema.AtmosConfiguration{
		AuthManager: NewManager(AuthOptions{Factory: factory}),
		Auth: schema.AuthConfig{Identities: map[string]schema.Identity{
			"original": {Kind: "aws/user", Default: true}, "requested": {Kind: "aws/user"},
		}},
	}
	prior := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "original"}}
	info := &schema.ConfigAndStacksInfo{Stack: "dev", AuthContext: prior}
	value := Credentials(ac, info, "requested")
	manager := types.NewMockAuthManager(ctrl)
	resolved := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "requested"}}
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: resolved}).AnyTimes()
	factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").DoAndReturn(func(_ *schema.AtmosConfiguration, config *schema.AuthConfig, _ string) (auth.AuthManager, error) {
		require.True(t, config.Identities["requested"].Default)
		require.False(t, config.Identities["original"].Default)
		return manager, nil
	}).Times(1)
	for range 2 {
		got, err := value.Resolve()
		require.NoError(t, err)
		require.Same(t, resolved, got.AuthContext)
		require.Same(t, prior, info.AuthContext)
		require.True(t, ac.Auth.Identities["original"].Default)
	}
}

func TestCredentialsFailuresAndDisabled(t *testing.T) {
	for _, scenario := range []string{"missing identity", "invalid config", "failed auth", "disabled", "nil info"} {
		t.Run(scenario, func(t *testing.T) {
			factory := NewMockAuthFactory(gomock.NewController(t))
			ac := &schema.AtmosConfiguration{AuthManager: NewManager(AuthOptions{Factory: factory, Disabled: scenario == "disabled"})}
			info := &schema.ConfigAndStacksInfo{Stack: "dev"}
			identity := "selected"
			switch scenario {
			case "invalid config":
				info.ComponentSection = map[string]any{"auth": map[string]any{"identities": "invalid"}}
			case "failed auth":
				identity = ""
				factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(nil, errUtils.ErrAuthenticationUnavailable).Times(1)
			case "nil info":
				identity, info = "", nil
				factory.EXPECT().Create(gomock.Any(), gomock.Any(), "").Return(nil, nil)
			}
			value := Credentials(ac, info, identity)
			got, err := value.Resolve()
			switch scenario {
			case "missing identity":
				require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
			case "invalid config":
				require.Error(t, err)
			case "failed auth":
				require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
				_, err = value.Resolve()
				require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
			default:
				require.NoError(t, err)
				require.Nil(t, got.AuthContext)
				require.Equal(t, scenario == "disabled", got.AuthDisabled)
			}
		})
	}
}

func TestDefaultIdentityDoesNotAuthenticate(t *testing.T) {
	for _, scenario := range []string{"empty", "sole", "default", "ambiguous", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			ac := &schema.AtmosConfiguration{AuthManager: NewManager(AuthOptions{Factory: NewMockAuthFactory(gomock.NewController(t))})}
			info := &schema.ConfigAndStacksInfo{}
			if scenario != "empty" {
				ac.Auth.Identities = map[string]schema.Identity{"selected": {Kind: "aws/user", Default: scenario == "default"}}
			}
			if scenario == "default" || scenario == "ambiguous" {
				ac.Auth.Identities["other"] = schema.Identity{Kind: "aws/user"}
			}
			if scenario == "invalid" {
				info.ComponentSection = map[string]any{"auth": map[string]any{"identities": "invalid"}}
			}
			got, err := DefaultIdentity(ac, info)
			if scenario == "invalid" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if scenario == "sole" || scenario == "default" {
				require.Equal(t, "selected", got)
			} else {
				require.Empty(t, got)
			}
		})
	}
}
