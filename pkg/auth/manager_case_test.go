package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestResolveIdentityName_CaseSensitivity tests case-insensitive identity name resolution.
func TestResolveIdentityName_CaseSensitivity(t *testing.T) {
	tests := []struct {
		name             string
		identities       map[string]types.Identity
		identityCaseMap  map[string]string
		inputName        string
		expectedResolved string
		expectedFound    bool
	}{
		{
			name: "exact match lowercase",
			identities: map[string]types.Identity{
				"admin": nil, // nil is fine, we don't call methods on it
			},
			identityCaseMap: map[string]string{
				"admin": "Admin",
			},
			inputName:        "admin",
			expectedResolved: "admin",
			expectedFound:    true,
		},
		{
			name: "case insensitive match - uppercase input",
			identities: map[string]types.Identity{
				"superadmin": nil,
			},
			identityCaseMap: map[string]string{
				"superadmin": "SuperAdmin",
			},
			inputName:        "SuperAdmin",
			expectedResolved: "superadmin",
			expectedFound:    true,
		},
		{
			name: "case insensitive match - mixed case input",
			identities: map[string]types.Identity{
				"devteam": nil,
			},
			identityCaseMap: map[string]string{
				"devteam": "DevTeam",
			},
			inputName:        "DevTeam",
			expectedResolved: "devteam",
			expectedFound:    true,
		},
		{
			name: "not found",
			identities: map[string]types.Identity{
				"admin": nil,
			},
			identityCaseMap: map[string]string{
				"admin": "Admin",
			},
			inputName:        "nonexistent",
			expectedResolved: "",
			expectedFound:    false,
		},
		{
			name: "no case map - exact match only",
			identities: map[string]types.Identity{
				"admin": nil,
			},
			identityCaseMap:  nil,
			inputName:        "admin",
			expectedResolved: "admin",
			expectedFound:    true,
		},
		{
			name: "no case map - case mismatch fails",
			identities: map[string]types.Identity{
				"admin": nil,
			},
			identityCaseMap:  nil,
			inputName:        "Admin",
			expectedResolved: "",
			expectedFound:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manager{
				config: &schema.AuthConfig{
					IdentityCaseMap: tt.identityCaseMap,
				},
				identities: tt.identities,
			}

			resolved, found := m.resolveIdentityName(tt.inputName)

			assert.Equal(t, tt.expectedFound, found, "found mismatch")
			assert.Equal(t, tt.expectedResolved, resolved, "resolved name mismatch")
		})
	}
}
