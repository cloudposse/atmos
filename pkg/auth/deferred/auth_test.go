package deferred

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthCacheIncludesStackAndConfiguration(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	factory := NewMockAuthFactory(gomock.NewController(t))
	ac.DeferredAuth = NewAuthResolver(AuthOptions{Factory: factory})
	failure := errors.Join(errUtils.ErrAuthenticationUnavailable, errUtils.ErrExpiredCredentials)
	for _, stack := range []string{"dev", "prod"} {
		factory.EXPECT().Create(ac, gomock.Any(), stack).Return(nil, failure).Times(2)
		for _, name := range []string{"first", "second"} {
			info := &schema.ConfigAndStacksInfo{Stack: stack, ComponentSection: map[string]any{"auth": map[string]any{"identities": map[string]any{name: map[string]any{"kind": "aws/user", "default": true}}}}}
			for range 2 {
				require.ErrorIs(t, ResolveAuth(ac, info), errUtils.ErrAuthenticationUnavailable)
				assert.Nil(t, info.AuthContext)
				assert.Nil(t, info.AuthManager)
			}
		}
	}
}

func TestResolveAuthClearsStaleContext(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "no manager", true: "disabled"}[disabled], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			factory := NewMockAuthFactory(ctrl)
			ac := &schema.AtmosConfiguration{DeferredAuth: NewAuthResolver(AuthOptions{Factory: factory, Disabled: disabled})}
			info := &schema.ConfigAndStacksInfo{AuthManager: types.NewMockAuthManager(ctrl), AuthContext: &schema.AuthContext{}}
			if !disabled {
				factory.EXPECT().Create(ac, gomock.Any(), "").Return(nil, nil)
			}
			require.NoError(t, ResolveAuth(ac, info))
			require.Nil(t, info.AuthManager)
			require.Nil(t, info.AuthContext)
		})
	}
}
