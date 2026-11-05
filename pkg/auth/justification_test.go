package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// The embedded types.Identity is never called by chainConsumesJustification (it only
// type-asserts to JustificationConsumer), so leaving it nil is safe for these tests.
type plainTestIdentity struct{ types.Identity }

type consumerTestIdentity struct{ types.Identity }

func (consumerTestIdentity) ConsumesJustification() bool { return true }

type nonConsumerTestIdentity struct{ types.Identity }

func (nonConsumerTestIdentity) ConsumesJustification() bool { return false }

func TestChainConsumesJustification(t *testing.T) {
	tests := []struct {
		name       string
		chain      []string
		identities map[string]types.Identity
		want       bool
	}{
		{
			name:       "chain with a consumer identity",
			chain:      []string{"azure-interactive", "azure-dev", "prod-contributor"},
			identities: map[string]types.Identity{"azure-dev": plainTestIdentity{}, "prod-contributor": consumerTestIdentity{}},
			want:       true,
		},
		{
			name:       "chain with no consumer",
			chain:      []string{"azure-interactive", "azure-dev"},
			identities: map[string]types.Identity{"azure-dev": plainTestIdentity{}},
			want:       false,
		},
		{
			name:       "consumer that opts out returns false",
			chain:      []string{"p", "x"},
			identities: map[string]types.Identity{"x": nonConsumerTestIdentity{}},
			want:       false,
		},
		{
			name:       "provider-only names not in identities map",
			chain:      []string{"azure-interactive"},
			identities: map[string]types.Identity{},
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, chainConsumesJustification(tt.chain, tt.identities))
		})
	}
}
