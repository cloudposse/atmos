package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCopyGlobalAuthConfig(t *testing.T) {
	tests := []struct {
		name       string
		globalAuth *schema.AuthConfig
		verify     func(*testing.T, *schema.AuthConfig)
	}{
		{
			name:       "nil config returns empty",
			globalAuth: nil,
			verify: func(t *testing.T, result *schema.AuthConfig) {
				assert.NotNil(t, result)
				assert.Nil(t, result.Providers)
				assert.Nil(t, result.Identities)
			},
		},
		{
			name: "copies all fields including realm",
			globalAuth: &schema.AuthConfig{
				Realm:       "my-project",
				RealmSource: "config",
				Providers: map[string]schema.Provider{
					"test-provider": {
						Kind:   "aws/iam-identity-center",
						Region: "us-east-1",
					},
				},
				Identities: map[string]schema.Identity{
					"test-identity": {
						Kind:    "aws/user",
						Default: true,
					},
				},
				Logs: schema.Logs{
					Level: "Info",
				},
				Keyring: schema.KeyringConfig{
					Type: "system",
				},
				IdentityCaseMap: map[string]string{
					"test": "Test",
				},
			},
			verify: func(t *testing.T, result *schema.AuthConfig) {
				assert.Equal(t, "my-project", result.Realm)
				assert.Equal(t, "config", result.RealmSource)
				assert.Len(t, result.Providers, 1)
				assert.Contains(t, result.Providers, "test-provider")
				assert.Len(t, result.Identities, 1)
				assert.Contains(t, result.Identities, "test-identity")
				assert.Equal(t, "Info", result.Logs.Level)
				assert.Equal(t, "system", result.Keyring.Type)
				assert.Len(t, result.IdentityCaseMap, 1)
			},
		},
		{
			name: "copies realm from env source",
			globalAuth: &schema.AuthConfig{
				Realm:       "env-realm",
				RealmSource: "env",
			},
			verify: func(t *testing.T, result *schema.AuthConfig) {
				assert.Equal(t, "env-realm", result.Realm)
				assert.Equal(t, "env", result.RealmSource)
			},
		},
		{
			name: "empty realm is preserved",
			globalAuth: &schema.AuthConfig{
				Realm:       "",
				RealmSource: "",
			},
			verify: func(t *testing.T, result *schema.AuthConfig) {
				assert.Empty(t, result.Realm)
				assert.Empty(t, result.RealmSource)
			},
		},
		{
			name: "deep copies Keyring.Spec map",
			globalAuth: &schema.AuthConfig{
				Keyring: schema.KeyringConfig{
					Type: "file",
					Spec: map[string]interface{}{
						"path":     "/tmp/keyring",
						"password": "secret",
					},
				},
			},
			verify: func(t *testing.T, result *schema.AuthConfig) {
				// Verify Spec was copied.
				assert.NotNil(t, result.Keyring.Spec)
				assert.Len(t, result.Keyring.Spec, 2)
				assert.Equal(t, "/tmp/keyring", result.Keyring.Spec["path"])
				assert.Equal(t, "secret", result.Keyring.Spec["password"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CopyGlobalAuthConfig(tt.globalAuth)
			tt.verify(t, result)
		})
	}
}

func TestCopyGlobalAuthConfig_DeepCopyMutation(t *testing.T) {
	// Test that modifying the copy doesn't mutate the original.
	original := &schema.AuthConfig{
		Realm:       "original-realm",
		RealmSource: "config",
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": "/original/path",
			},
		},
		IdentityCaseMap: map[string]string{
			"original": "Original",
		},
	}

	// Create a copy.
	copy := CopyGlobalAuthConfig(original)

	// Modify the copy.
	copy.Realm = "modified-realm"
	copy.RealmSource = "env"
	copy.Keyring.Spec["path"] = "/modified/path"
	copy.Keyring.Spec["new_key"] = "new_value"
	copy.IdentityCaseMap["original"] = "Modified"
	copy.IdentityCaseMap["new"] = "New"

	// Verify original is unchanged.
	assert.Equal(t, "original-realm", original.Realm)
	assert.Equal(t, "config", original.RealmSource)
	assert.Equal(t, "/original/path", original.Keyring.Spec["path"])
	assert.Len(t, original.Keyring.Spec, 1)
	assert.NotContains(t, original.Keyring.Spec, "new_key")
	assert.Equal(t, "Original", original.IdentityCaseMap["original"])
	assert.Len(t, original.IdentityCaseMap, 1)
	assert.NotContains(t, original.IdentityCaseMap, "new")

	// Verify copy has the modifications.
	assert.Equal(t, "/modified/path", copy.Keyring.Spec["path"])
	assert.Equal(t, "new_value", copy.Keyring.Spec["new_key"])
	assert.Len(t, copy.Keyring.Spec, 2)
	assert.Equal(t, "Modified", copy.IdentityCaseMap["original"])
	assert.Equal(t, "New", copy.IdentityCaseMap["new"])
	assert.Len(t, copy.IdentityCaseMap, 2)
}

func TestMergeComponentAuthFromConfig(t *testing.T) {
	globalAuth := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"global-provider": {
				Kind:   "aws/iam-identity-center",
				Region: "us-west-2",
			},
		},
		Identities: map[string]schema.Identity{
			"global-identity": {
				Kind:    "aws/user",
				Default: true,
			},
		},
		IdentityCaseMap: map[string]string{
			"global-identity": "global-identity",
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	tests := []struct {
		name            string
		componentConfig map[string]any
		verify          func(*testing.T, *schema.AuthConfig, error)
	}{
		{
			name:            "nil component config returns global",
			componentConfig: nil,
			verify: func(t *testing.T, result *schema.AuthConfig, err error) {
				assert.NoError(t, err)
				assert.Len(t, result.Identities, 1)
				assert.Contains(t, result.Identities, "global-identity")
			},
		},
		{
			name: "no auth section returns global",
			componentConfig: map[string]any{
				"vars": map[string]any{
					"test": "value",
				},
			},
			verify: func(t *testing.T, result *schema.AuthConfig, err error) {
				assert.NoError(t, err)
				assert.Len(t, result.Identities, 1)
				assert.Contains(t, result.Identities, "global-identity")
			},
		},
		{
			name: "merges component auth section",
			componentConfig: map[string]any{
				cfg.AuthSectionName: map[string]any{
					"identities": map[string]any{
						"component-identity": map[string]any{
							"kind": "aws/assume-role",
						},
					},
				},
			},
			verify: func(t *testing.T, result *schema.AuthConfig, err error) {
				assert.NoError(t, err)
				assert.Len(t, result.Identities, 2)
				assert.Contains(t, result.Identities, "global-identity")
				assert.Contains(t, result.Identities, "component-identity")
			},
		},
		{
			name: "component identity added to IdentityCaseMap",
			componentConfig: map[string]any{
				cfg.AuthSectionName: map[string]any{
					"identities": map[string]any{
						"ComponentIdentity": map[string]any{
							"kind": "aws/assume-role",
						},
					},
				},
			},
			verify: func(t *testing.T, result *schema.AuthConfig, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result.IdentityCaseMap)
				assert.Contains(t, result.IdentityCaseMap, "componentidentity")
				assert.Equal(t, "ComponentIdentity", result.IdentityCaseMap["componentidentity"])
				// Global identity should still be present.
				assert.Contains(t, result.IdentityCaseMap, "global-identity")
			},
		},
		{
			name: "mixed-case component identity lookups work",
			componentConfig: map[string]any{
				cfg.AuthSectionName: map[string]any{
					"identities": map[string]any{
						"MyComponentIdentity": map[string]any{
							"kind": "aws/assume-role",
						},
					},
				},
			},
			verify: func(t *testing.T, result *schema.AuthConfig, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result.IdentityCaseMap)
				// Case-insensitive lookup should work.
				assert.Contains(t, result.IdentityCaseMap, "mycomponentidentity")
				assert.Equal(t, "MyComponentIdentity", result.IdentityCaseMap["mycomponentidentity"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MergeComponentAuthFromConfig(globalAuth, tt.componentConfig, atmosConfig, cfg.AuthSectionName)
			tt.verify(t, result, err)
		})
	}
}

func TestMergeComponentAuthFromConfig_NilGlobalAuth(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	tests := []struct {
		name            string
		globalAuth      *schema.AuthConfig
		componentConfig map[string]any
		verify          func(*testing.T, *schema.AuthConfig, error)
	}{
		{
			name:       "nil global auth with component identities",
			globalAuth: nil,
			componentConfig: map[string]any{
				cfg.AuthSectionName: map[string]any{
					"identities": map[string]any{
						"ComponentIdentity": map[string]any{
							"kind": "aws/assume-role",
						},
					},
				},
			},
			verify: func(t *testing.T, result *schema.AuthConfig, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result.Identities, 1)
				assert.Contains(t, result.Identities, "ComponentIdentity")
				// IdentityCaseMap should be built for component identities.
				assert.NotNil(t, result.IdentityCaseMap)
				assert.Contains(t, result.IdentityCaseMap, "componentidentity")
				assert.Equal(t, "ComponentIdentity", result.IdentityCaseMap["componentidentity"])
			},
		},
		{
			name: "empty global auth with component identities",
			globalAuth: &schema.AuthConfig{
				Identities:      map[string]schema.Identity{},
				IdentityCaseMap: nil,
			},
			componentConfig: map[string]any{
				cfg.AuthSectionName: map[string]any{
					"identities": map[string]any{
						"ComponentIdentity": map[string]any{
							"kind": "aws/assume-role",
						},
					},
				},
			},
			verify: func(t *testing.T, result *schema.AuthConfig, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result.Identities, 1)
				// IdentityCaseMap should be built even when global config has nil map.
				assert.NotNil(t, result.IdentityCaseMap)
				assert.Contains(t, result.IdentityCaseMap, "componentidentity")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MergeComponentAuthFromConfig(tt.globalAuth, tt.componentConfig, atmosConfig, cfg.AuthSectionName)
			tt.verify(t, result, err)
		})
	}
}

func TestMergeComponentAuthFromConfig_RealmSurvivesMapstructureRoundTrip(t *testing.T) {
	// Realm has mapstructure:"realm" so it must survive the struct→map→merge→map→struct round-trip.
	// RealmSource has mapstructure:"-" so it is lost — this is expected because GetRealm() recomputes it.
	globalAuth := &schema.AuthConfig{
		Realm:       "my-project",
		RealmSource: "config",
		Providers: map[string]schema.Provider{
			"aws": {Kind: "aws/iam-identity-center", Region: "us-east-1"},
		},
		Identities: map[string]schema.Identity{
			"global-id": {Kind: "aws/user", Default: true},
		},
		IdentityCaseMap: map[string]string{
			"global-id": "global-id",
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	// Component with its own auth section that overrides some fields.
	componentConfig := map[string]any{
		cfg.AuthSectionName: map[string]any{
			"identities": map[string]any{
				"component-id": map[string]any{
					"kind": "aws/assume-role",
				},
			},
		},
	}

	result, err := MergeComponentAuthFromConfig(globalAuth, componentConfig, atmosConfig, cfg.AuthSectionName)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Realm must survive the merge round-trip.
	assert.Equal(t, "my-project", result.Realm)

	// RealmSource is lost (mapstructure:"-") — this is expected.
	// GetRealm() recomputes it from the Realm value when creating the manager.
	assert.Empty(t, result.RealmSource)

	// Other fields still work.
	assert.Len(t, result.Identities, 2)
	assert.Contains(t, result.Identities, "global-id")
	assert.Contains(t, result.Identities, "component-id")
}

func TestMergeComponentAuthFromConfig_NoRealmConfigured(t *testing.T) {
	// When no realm is configured, everything should work as before — no realm in result.
	globalAuth := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"aws": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"dev": {Kind: "aws/user", Default: true},
		},
		IdentityCaseMap: map[string]string{
			"dev": "dev",
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	componentConfig := map[string]any{
		cfg.AuthSectionName: map[string]any{
			"identities": map[string]any{
				"staging": map[string]any{
					"kind": "aws/assume-role",
				},
			},
		},
	}

	result, err := MergeComponentAuthFromConfig(globalAuth, componentConfig, atmosConfig, cfg.AuthSectionName)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Realm)
	assert.Empty(t, result.RealmSource)
	assert.Len(t, result.Identities, 2)
}

func TestCopyGlobalAuthConfig_NoRealmConfigured(t *testing.T) {
	// When no realm is configured at all, CopyGlobalAuthConfig should produce a valid config
	// with empty realm fields — same as before this fix was applied.
	globalAuth := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"aws": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"dev": {Kind: "aws/user", Default: true},
		},
		Logs: schema.Logs{Level: "Info"},
	}

	result := CopyGlobalAuthConfig(globalAuth)
	assert.NotNil(t, result)
	assert.Empty(t, result.Realm)
	assert.Empty(t, result.RealmSource)
	assert.Len(t, result.Providers, 1)
	assert.Len(t, result.Identities, 1)
	assert.Equal(t, "Info", result.Logs.Level)
}

func TestAuthConfigToMap(t *testing.T) {
	tests := []struct {
		name       string
		authConfig *schema.AuthConfig
		wantErr    bool
	}{
		{
			name:       "nil config returns empty map",
			authConfig: nil,
			wantErr:    false,
		},
		{
			name: "converts auth config to map",
			authConfig: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"test": {Kind: "aws/user"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AuthConfigToMap(tt.authConfig)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

// ============================================================================
// Issue #3 — component-level default does not override global default.
//
// When a component declares auth.identities.<name>.default: true in its stack
// config, and the global atmos.yaml also has a different identity with
// default: true, the raw deep merge preserves BOTH defaults. The user then
// gets prompted to choose (interactive) or errors out (CI). The fix clears
// global defaults before merging when the component declares its own.
// ============================================================================

func TestMergeComponentAuthConfig_ComponentDefaultOverridesGlobalDefault(t *testing.T) {
	// The core Issue #3 scenario: global has `tf-state.default: true`,
	// component declares `create-resources.default: true`. After merge,
	// only `create-resources` should be default.
	globalAuth := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"tf-state": {
				Kind:    "aws/permission-set",
				Default: true,
			},
			"create-resources": {
				Kind:    "aws/permission-set",
				Default: false,
			},
		},
	}

	componentAuth := map[string]any{
		"identities": map[string]any{
			"create-resources": map[string]any{
				"default": true,
			},
		},
	}

	result, err := MergeComponentAuthConfig(&schema.AtmosConfiguration{}, globalAuth, componentAuth)
	require.NoError(t, err)

	// Component default wins — global default must be cleared.
	assert.False(t, result.Identities["tf-state"].Default,
		"global default must be cleared when component declares its own default")
	assert.True(t, result.Identities["create-resources"].Default,
		"component-level default must survive the merge")
}

func TestMergeComponentAuthConfig_NoComponentDefault_PreservesGlobalDefault(t *testing.T) {
	// When the component auth section does NOT declare any default, the
	// global default must be preserved unchanged.
	globalAuth := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"tf-state": {
				Kind:    "aws/permission-set",
				Default: true,
			},
		},
	}

	// Component adds an identity but without default: true.
	componentAuth := map[string]any{
		"identities": map[string]any{
			"deploy": map[string]any{
				"kind": "aws/assume-role",
			},
		},
	}

	result, err := MergeComponentAuthConfig(&schema.AtmosConfiguration{}, globalAuth, componentAuth)
	require.NoError(t, err)

	assert.True(t, result.Identities["tf-state"].Default,
		"global default must be preserved when component has no default")
}

func TestMergeComponentAuthConfig_ComponentDefaultForSameIdentity(t *testing.T) {
	// Edge case: global has `foo.default: true`, component also declares
	// `foo.default: true` (same identity). Should still work — no conflict.
	globalAuth := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"foo": {Kind: "aws/assume-role", Default: true},
		},
	}

	componentAuth := map[string]any{
		"identities": map[string]any{
			"foo": map[string]any{
				"default": true,
			},
		},
	}

	result, err := MergeComponentAuthConfig(&schema.AtmosConfiguration{}, globalAuth, componentAuth)
	require.NoError(t, err)

	assert.True(t, result.Identities["foo"].Default,
		"same identity declared as default in both global and component — should stay default")
}

func TestMergeComponentAuthFromConfig_ComponentDefaultOverridesGlobal(t *testing.T) {
	// End-to-end via MergeComponentAuthFromConfig (the wrapper used by
	// the exec-layer in getMergedAuthConfigWithFetcher). Verifies the fix
	// flows through the full call chain.
	globalAuth := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"global-default": {Kind: "aws/assume-role", Default: true},
			"component-id":   {Kind: "aws/assume-role", Default: false},
		},
	}

	componentConfig := map[string]any{
		"auth": map[string]any{
			"identities": map[string]any{
				"component-id": map[string]any{
					"default": true,
				},
			},
		},
	}

	result, err := MergeComponentAuthFromConfig(globalAuth, componentConfig, &schema.AtmosConfiguration{}, "auth")
	require.NoError(t, err)

	assert.False(t, result.Identities["global-default"].Default,
		"global default cleared by component-level override")
	assert.True(t, result.Identities["component-id"].Default,
		"component-level default must win")
}

func TestComponentAuthHasDefault_True(t *testing.T) {
	section := map[string]any{
		"identities": map[string]any{
			"foo": map[string]any{"default": true},
		},
	}
	assert.True(t, componentAuthHasDefault(section))
}

func TestComponentAuthHasDefault_False(t *testing.T) {
	section := map[string]any{
		"identities": map[string]any{
			"foo": map[string]any{"kind": "aws/assume-role"},
		},
	}
	assert.False(t, componentAuthHasDefault(section))
}

func TestComponentAuthHasDefault_NoIdentities(t *testing.T) {
	assert.False(t, componentAuthHasDefault(map[string]any{}))
	assert.False(t, componentAuthHasDefault(map[string]any{"identities": "invalid"}))
}

func TestComponentAuthHasDefault_InvalidIdentityType(t *testing.T) {
	// Identity value is not a map — must return false gracefully.
	section := map[string]any{
		"identities": map[string]any{
			"foo": "invalid-string-instead-of-map",
		},
	}
	assert.False(t, componentAuthHasDefault(section))
}

func TestComponentAuthHasDefault_ExplicitFalse(t *testing.T) {
	// default: false is NOT a "has default" — it's an explicit opt-out.
	section := map[string]any{
		"identities": map[string]any{
			"foo": map[string]any{"default": false},
		},
	}
	assert.False(t, componentAuthHasDefault(section))
}

func TestClearExistingIdentityDefaults_Nil(t *testing.T) {
	// Must not panic on nil.
	clearExistingIdentityDefaults(nil)
}

func TestClearExistingIdentityDefaults_ClearsAll(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"a": {Kind: "aws/assume-role", Default: true},
			"b": {Kind: "aws/assume-role", Default: true},
			"c": {Kind: "aws/assume-role", Default: false},
		},
	}
	clearExistingIdentityDefaults(authConfig)
	assert.False(t, authConfig.Identities["a"].Default)
	assert.False(t, authConfig.Identities["b"].Default)
	assert.False(t, authConfig.Identities["c"].Default)
}
