package deferred

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestSecretValueComposesStoreAndAuthOnDemand(t *testing.T) {
	for _, scenario := range []string{"unused", "masked", "success", "failed auth", "undeclared"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			backend := store.NewMockIdentityAwareStore(ctrl)
			factory := authdeferred.NewMockAuthFactory(ctrl)
			ac := &schema.AtmosConfiguration{
				AuthManager:  authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory}),
				Stores:       store.StoreRegistry{"vault": backend},
				StoresConfig: store.StoresConfig{"vault": {Secret: true}},
			}
			info := secretInfo("store")
			info.Component = "app"
			info.SecretsMaskOnly = scenario == "masked"
			input := "!secret KEY"
			if scenario == "undeclared" {
				input = "!secret MISSING"
			}
			value := NewValue(ac, input, "dev", info)
			if scenario == "unused" {
				return
			}
			require.NoError(t, iolib.Initialize())
			switch scenario {
			case "success":
				gomock.InOrder(
					backend.EXPECT().ResetAuthContext(),
					factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(nil, nil),
					backend.EXPECT().Get("dev", "app", "KEY").Return("resolved-secret", nil),
				)
			case "failed auth":
				gomock.InOrder(
					backend.EXPECT().ResetAuthContext(),
					factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(nil, errUtils.ErrAuthenticationUnavailable),
				)
			}
			got, err := value.Resolve()
			switch scenario {
			case "failed auth":
				require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
			case "undeclared":
				require.ErrorIs(t, err, secrets.ErrSecretNotDeclared)
			case "masked":
				require.NoError(t, err)
				require.Equal(t, iolib.GetContext().Masker().Replacement(), got)
			case "success":
				require.NoError(t, err)
				require.Equal(t, "resolved-secret", got)
			}
		})
	}
}

func TestSecretDefaultDoesNotHideAuthenticationFailure(t *testing.T) {
	for _, failure := range []error{errUtils.ErrAuthenticationUnavailable, errUtils.ErrInvalidAuthConfig} {
		ctrl := gomock.NewController(t)
		backend := store.NewMockIdentityAwareStore(ctrl)
		factory := authdeferred.NewMockAuthFactory(ctrl)
		ac := &schema.AtmosConfiguration{
			AuthManager:  authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory}),
			Stores:       store.StoreRegistry{"vault": backend},
			StoresConfig: store.StoresConfig{"vault": {Secret: true}},
		}
		backend.EXPECT().ResetAuthContext()
		factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(nil, failure)
		value, err := NewValue(ac, "!secret KEY | default fallback", "dev", secretInfo("store")).Resolve()
		require.ErrorIs(t, err, failure)
		require.Nil(t, value)
	}
}
