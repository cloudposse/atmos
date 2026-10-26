package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSecretsDefaultIdentity covers the precedence used to pick the default identity for cloud-KMS
// SOPS providers: an explicit, real `--identity` wins; the select/disabled sentinels and the empty
// value fall back to the tail of the authenticated chain; an empty chain yields an empty string.
func TestSecretsDefaultIdentity(t *testing.T) {
	t.Run("explicit identity wins without consulting the chain", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		am := authtypes.NewMockAuthManager(ctrl)
		// No GetChain expectation: an explicit identity must short-circuit before the chain lookup.
		assert.Equal(t, "explicit", secretsDefaultIdentity(secretScope{Identity: "explicit"}, am))
	})

	sentinels := map[string]string{
		"empty":    "",
		"select":   cfg.IdentityFlagSelectValue,
		"disabled": cfg.IdentityFlagDisabledValue,
	}
	for name, id := range sentinels {
		t.Run(name+" identity falls back to the authenticated chain tail", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			am := authtypes.NewMockAuthManager(ctrl)
			am.EXPECT().GetChain().Return([]string{"base", "role-a", "role-b"})
			assert.Equal(t, "role-b", secretsDefaultIdentity(secretScope{Identity: id}, am))
		})
	}

	t.Run("empty identity with empty chain yields empty string", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		am := authtypes.NewMockAuthManager(ctrl)
		am.EXPECT().GetChain().Return(nil)
		assert.Empty(t, secretsDefaultIdentity(secretScope{}, am))
	})
}

// TestInjectSecretStoreAuthResolver_NilArguments verifies the documented no-op behavior: a nil
// atmosConfig or nil authManager neither panics nor populates SecretsAuth.
func TestInjectSecretStoreAuthResolver_NilArguments(t *testing.T) {
	t.Run("nil atmosConfig is a no-op", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		am := authtypes.NewMockAuthManager(ctrl)
		injectSecretStoreAuthResolver(nil, am, secretScope{})
	})

	t.Run("nil authManager leaves SecretsAuth unset", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		injectSecretStoreAuthResolver(atmosConfig, nil, secretScope{})
		assert.Nil(t, atmosConfig.SecretsAuth)
	})
}
