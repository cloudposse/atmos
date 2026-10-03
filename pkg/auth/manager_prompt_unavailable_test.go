package auth

import (
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the schema field the fixtures below rely on.
var _ = schema.Identity{Default: true}

func disableInteractivePrompts(t *testing.T) {
	t.Helper()
	original := viper.GetBool("interactive")
	viper.Set("interactive", false)
	t.Cleanup(func() { viper.Set("interactive", original) })
}

// When a selector is needed but cannot be shown (for example stderr is captured by the AWS CLI
// running a credential_process helper, so the huh form would be invisible and hang), the error must
// say so and point at the way out.
func TestGetDefaultIdentity_PromptUnavailableHintsLogin(t *testing.T) {
	disableInteractivePrompts(t)

	tests := []struct {
		name        string
		identities  map[string]schema.Identity
		forceSelect bool
		wantErr     error
	}{
		{
			name:        "explicit selection without a terminal",
			identities:  map[string]schema.Identity{"dev": {Kind: "aws/user"}},
			forceSelect: true,
			wantErr:     errUtils.ErrIdentitySelectionRequiresTTY,
		},
		{
			name:       "no default identity",
			identities: map[string]schema.Identity{"dev": {Kind: "aws/user"}},
			wantErr:    errUtils.ErrNoDefaultIdentity,
		},
		{
			name: "multiple default identities",
			identities: map[string]schema.Identity{
				"dev":  {Kind: "aws/user", Default: true},
				"prod": {Kind: "aws/user", Default: true},
			},
			wantErr: errUtils.ErrMultipleDefaultIdentities,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manager{config: &schema.AuthConfig{Identities: tt.identities}}

			name, err := m.GetDefaultIdentity(tt.forceSelect)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, name)
			hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
			assert.Contains(t, hints, "atmos auth login --identity=")
			assert.Contains(t, hints, "--identity=<name>")
		})
	}
}

func TestGetDefaultIdentity_SingleDefaultNeedsNoPrompt(t *testing.T) {
	disableInteractivePrompts(t)
	m := &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{
		"dev": {Kind: "aws/user", Default: true},
	}}}

	name, err := m.GetDefaultIdentity(false)

	require.NoError(t, err)
	assert.Equal(t, "dev", name)
}
