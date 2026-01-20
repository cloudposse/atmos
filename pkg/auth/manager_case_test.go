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
		{
			name: "provisioned identity - original case input works",
			identities: map[string]types.Identity{
				"core-artifacts/administratoraccess": nil, // lowercase key from Viper
			},
			identityCaseMap: map[string]string{
				"core-artifacts/administratoraccess": "core-artifacts/AdministratorAccess",
			},
			inputName:        "core-artifacts/AdministratorAccess", // user input with original case
			expectedResolved: "core-artifacts/administratoraccess", // returns lowercase for internal use
			expectedFound:    true,
		},
		{
			name: "provisioned identity - mixed case account name works",
			identities: map[string]types.Identity{
				"core-audit/billingadministratoraccess": nil,
			},
			identityCaseMap: map[string]string{
				"core-audit/billingadministratoraccess": "Core-Audit/BillingAdministratorAccess",
			},
			inputName:        "Core-Audit/BillingAdministratorAccess",
			expectedResolved: "core-audit/billingadministratoraccess",
			expectedFound:    true,
		},
		{
			name: "provisioned identity - all caps input works",
			identities: map[string]types.Identity{
				"myaccount/poweruser": nil,
			},
			identityCaseMap: map[string]string{
				"myaccount/poweruser": "MyAccount/PowerUser",
			},
			inputName:        "MYACCOUNT/POWERUSER", // user types all caps
			expectedResolved: "myaccount/poweruser",
			expectedFound:    true,
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

// TestGetIdentityDisplayName tests that identity display names preserve original case.
// This test reproduces the bug where provisioned identities are displayed in lowercase
// (e.g., "core-artifacts/terraformapplyaccess") instead of original case
// (e.g., "core-artifacts/TerraformApplyAccess").
func TestGetIdentityDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		identityCaseMap     map[string]string
		lowercaseKey        string
		expectedDisplayName string
	}{
		{
			name: "provisioned identity - preserves original case",
			identityCaseMap: map[string]string{
				"core-artifacts/terraformapplyaccess": "core-artifacts/TerraformApplyAccess",
			},
			lowercaseKey:        "core-artifacts/terraformapplyaccess",
			expectedDisplayName: "core-artifacts/TerraformApplyAccess",
		},
		{
			name: "provisioned identity - multiple identities",
			identityCaseMap: map[string]string{
				"core-artifacts/administratoraccess": "core-artifacts/AdministratorAccess",
				"core-artifacts/poweruseraccess":     "core-artifacts/PowerUserAccess",
				"inspatial aws cp/rootaccess":        "InSpatial AWS CP/RootAccess",
			},
			lowercaseKey:        "inspatial aws cp/rootaccess",
			expectedDisplayName: "InSpatial AWS CP/RootAccess",
		},
		{
			name:                "no case map - returns lowercase as-is",
			identityCaseMap:     nil,
			lowercaseKey:        "admin",
			expectedDisplayName: "admin",
		},
		{
			name: "not in case map - returns lowercase as-is",
			identityCaseMap: map[string]string{
				"other": "Other",
			},
			lowercaseKey:        "admin",
			expectedDisplayName: "admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := &manager{
				config: &schema.AuthConfig{
					IdentityCaseMap: tt.identityCaseMap,
				},
			}

			displayName := m.GetIdentityDisplayName(tt.lowercaseKey)

			assert.Equal(t, tt.expectedDisplayName, displayName,
				"display name should preserve original case from IdentityCaseMap")
		})
	}
}
