package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

// TestMergeComponentAuthFromConfig_ComponentIdentitySelectorDroppedByDecoder
// documents the fact that the `pkg/auth` struct-decode layer silently drops a
// component-level `auth.identity: <name>` selector because `schema.AuthConfig`
// has no `Identity string` field.
//
// This is an intentional property of `pkg/auth` — the auth config struct
// exists to describe identity pools and providers, not to carry component-
// level selectors. The fix for the Slack-reported component-identity-selector
// bug lives in the exec layer (`internal/exec/utils_auth.go`:
// `extractComponentIdentitySelector`), which reads the selector from the raw
// componentConfig map BEFORE it reaches this decode path and propagates it
// to `info.Identity` directly.
//
// This test guards that pkg/auth continues to decode cleanly and the default
// identity is unaffected — a regression here would mean the struct gained a
// field that changes the merge semantics.
//
// See docs/fixes/2026-04-08-atmos-auth-identity-resolution-fixes.md §"Issue 3"
// for the full fix rationale.
func TestMergeComponentAuthFromConfig_ComponentIdentitySelectorDroppedByDecoder(t *testing.T) {
	// Global auth: two identities, backend-role is the default.
	globalAuth := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"mock-provider": {Kind: "mock/aws"},
		},
		Identities: map[string]schema.Identity{
			"backend-role": {
				Kind:    "mock/aws",
				Default: true,
				Via:     &schema.IdentityVia{Provider: "mock-provider"},
			},
			"provider-role": {
				Kind: "mock/aws",
				Via:  &schema.IdentityVia{Provider: "mock-provider"},
			},
		},
	}

	// Component config as it would come out of ExecuteDescribeComponent when
	// the stack YAML contains:
	//   components.terraform.s3-bucket.auth.identity: provider-role
	componentConfig := map[string]any{
		"auth": map[string]any{
			"identity": "provider-role",
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}
	merged, err := MergeComponentAuthFromConfig(globalAuth, componentConfig, atmosConfig, cfg.AuthSectionName)
	assert.NoError(t, err)
	assert.NotNil(t, merged)

	// Both identities remain present; the struct decoder drops the
	// unknown `identity` key silently. This is the current and intended
	// behavior of the pkg/auth struct layer.
	assert.Len(t, merged.Identities, 2,
		"pkg/auth merge preserves global identities — the component-level "+
			"auth.identity selector is not a pkg/auth concept.")

	backendRole, ok := merged.Identities["backend-role"]
	assert.True(t, ok, "backend-role must still be present after merge")
	assert.True(t, backendRole.Default,
		"backend-role is still marked as default in the merged struct — "+
			"the exec layer is responsible for overriding info.Identity "+
			"before the auth manager resolves the default.")

	providerRole, ok := merged.Identities["provider-role"]
	assert.True(t, ok, "provider-role must still be present after merge")
	assert.False(t, providerRole.Default,
		"provider-role is not elevated in the pkg/auth struct. The exec "+
			"layer's extractComponentIdentitySelector handles the selector "+
			"propagation separately.")
}
