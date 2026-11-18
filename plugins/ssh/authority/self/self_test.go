package self_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/authority/authoritytest"
	"github.com/cloudposse/atmos/plugins/ssh/authority/self"
)

func TestConformance(t *testing.T) {
	authoritytest.Run(t, func(*testing.T) *authoritytest.Environment {
		return &authoritytest.Environment{
			Authority:   self.New(),
			Credentials: map[string]string{},
		}
	})
}

func TestEnsureMakesNoClaims(t *testing.T) {
	key := authoritytest.NewKey(t)
	a := self.New()
	receipt, err := a.Ensure(context.Background(), nil, authority.Request{
		KeyName: "ops", Key: key,
		Uses: []authority.Use{authority.UseSigning, authority.UseSigning},
	})
	require.NoError(t, err)
	assert.Equal(t, "self", receipt.Authority)
	assert.Empty(t, receipt.References)
	assert.Empty(t, receipt.Certificate)
	assert.True(t, receipt.ExpiresAt.IsZero())
	assert.Equal(t, []authority.Use{authority.UseSigning}, receipt.Uses)
}

func TestFactoryBuildsSelf(t *testing.T) {
	registry := authority.NewRegistry()
	require.NoError(t, registry.Register(self.Factory))
	a, err := registry.New("self", authority.Options{})
	require.NoError(t, err)
	assert.Equal(t, self.Name, a.Name())
}
