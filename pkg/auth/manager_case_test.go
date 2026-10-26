package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestResolveIdentityName_CaseSensitivity tests case-insensitive identity name resolution.
func TestResolveIdentityName_CaseSensitivity(t *testing.T) {
	t.Parallel()

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
			// Regression for cloudposse/atmos#3185: namespaced identity names containing ':'
			// are opaque map keys and must resolve at runtime, matching the relaxed manifest schema.
			name: "namespaced identity with colon - exact match",
			identities: map[string]types.Identity{
				"example/prod:terraform_applier": nil,
			},
			identityCaseMap: map[string]string{
				"example/prod:terraform_applier": "example/prod:terraform_applier",
			},
			inputName:        "example/prod:terraform_applier",
			expectedResolved: "example/prod:terraform_applier",
			expectedFound:    true,
		},
		{
			name: "namespaced identity with colon - case insensitive match",
			identities: map[string]types.Identity{
				"example/prod:terraform_applier": nil,
			},
			identityCaseMap: map[string]string{
				"example/prod:terraform_applier": "Example/Prod:Terraform_Applier",
			},
			inputName:        "Example/Prod:Terraform_Applier",
			expectedResolved: "example/prod:terraform_applier",
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
			t.Parallel()

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
