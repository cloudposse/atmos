package realm

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestGetRealm_EnvVarPrecedence(t *testing.T) {
	// Environment variable should have highest priority.
	t.Setenv(EnvVarName, "env-realm")

	info, err := GetRealm("config-realm", "/path/to/config")
	require.NoError(t, err)
	assert.Equal(t, "env-realm", info.Value)
	assert.Equal(t, SourceEnv, info.Source)
}

func TestGetRealm_ConfigPrecedence(t *testing.T) {
	// Config should be used when env var is not set.
	info, err := GetRealm("config-realm", "/path/to/config")
	require.NoError(t, err)
	assert.Equal(t, "config-realm", info.Value)
	assert.Equal(t, SourceConfig, info.Source)
}

func TestGetRealm_AutoEmpty(t *testing.T) {
	// When no explicit realm is configured, GetRealm returns empty.
	info, err := GetRealm("", "/path/to/config")
	require.NoError(t, err)
	assert.Equal(t, SourceAuto, info.Source)
	assert.Empty(t, info.Value, "auto realm should be empty — identity types handle fallback")
}

func TestGetRealm_AutoEmptyConsistency(t *testing.T) {
	// Both paths should produce the same empty realm.
	info1, err1 := GetRealm("", "/path/to/customer-a")
	info2, err2 := GetRealm("", "/path/to/customer-b")
	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, info1.Value, info2.Value, "auto realm should be empty regardless of path")
	assert.Empty(t, info1.Value)
}

func TestGetRealm_EmptyPath(t *testing.T) {
	// Empty path should produce empty realm.
	info, err := GetRealm("", "")
	require.NoError(t, err)
	assert.Equal(t, SourceAuto, info.Source)
	assert.Empty(t, info.Value)
}

func TestGetRealm_InvalidEnvVar(t *testing.T) {
	t.Setenv(EnvVarName, "Invalid/Realm")

	_, err := GetRealm("", "/path/to/config")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidRealm), "error should wrap ErrInvalidRealm")
	assert.Contains(t, err.Error(), EnvVarName)
	assert.Contains(t, err.Error(), "path traversal")
}

func TestGetRealm_InvalidConfig(t *testing.T) {
	_, err := GetRealm("My Realm", "/path/to/config")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidRealm), "error should wrap ErrInvalidRealm")
	assert.Contains(t, err.Error(), "auth.realm")
	assert.Contains(t, err.Error(), "invalid characters")
}

func TestValidate_ValidRealms(t *testing.T) {
	validRealms := []string{
		"customer-acme",
		"customer_acme",
		"customer123",
		"a",
		"ab",
		"a1b2c3d4",
		"my-project_v2",
		strings.Repeat("a", MaxLength),
	}

	for _, realm := range validRealms {
		t.Run(realm, func(t *testing.T) {
			err := Validate(realm)
			assert.NoError(t, err, "realm %q should be valid", realm)
		})
	}
}

func TestValidate_Empty(t *testing.T) {
	err := Validate("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestValidate_TooLong(t *testing.T) {
	longRealm := strings.Repeat("a", MaxLength+1)
	err := Validate(longRealm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum length")
}

func TestValidate_InvalidCharacters(t *testing.T) {
	tests := []struct {
		name     string
		realm    string
		contains string
	}{
		{"uppercase", "Customer", "invalid characters"},
		{"space", "my realm", "invalid characters"},
		{"dot", "my.realm", "invalid characters"},
		{"at sign", "my@realm", "invalid characters"},
		{"colon", "my:realm", "invalid characters"},
		{"forward slash", "my/realm", "path traversal"},
		{"backslash", "my\\realm", "path traversal"},
		{"double dot", "my..realm", "path traversal"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.realm)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.contains)
		})
	}
}

func TestValidate_StartsWithHyphenOrUnderscore(t *testing.T) {
	tests := []string{"-realm", "_realm"}
	for _, realm := range tests {
		t.Run(realm, func(t *testing.T) {
			err := Validate(realm)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot start with")
		})
	}
}

func TestValidate_EndsWithHyphenOrUnderscore(t *testing.T) {
	tests := []string{"realm-", "realm_"}
	for _, realm := range tests {
		t.Run(realm, func(t *testing.T) {
			err := Validate(realm)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot end with")
		})
	}
}

func TestValidate_ConsecutiveHyphensOrUnderscores(t *testing.T) {
	tests := []string{"my--realm", "my__realm", "my-_realm", "my_-realm"}
	for _, realm := range tests {
		t.Run(realm, func(t *testing.T) {
			err := Validate(realm)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "consecutive")
		})
	}
}

func TestRealmInfo_SourceDescription(t *testing.T) {
	tests := []struct {
		name          string
		info          RealmInfo
		cliConfigPath string
		contains      string
	}{
		{
			name:     "env source",
			info:     RealmInfo{Value: "test", Source: SourceEnv},
			contains: EnvVarName,
		},
		{
			name:     "config source",
			info:     RealmInfo{Value: "test", Source: SourceConfig},
			contains: "atmos.yaml",
		},
		{
			name:          "auto source with path",
			info:          RealmInfo{Value: "", Source: SourceAuto},
			cliConfigPath: "/path/to/config",
			contains:      "default (no realm isolation)",
		},
		{
			name:          "auto source without path",
			info:          RealmInfo{Value: "", Source: SourceAuto},
			cliConfigPath: "",
			contains:      "default (no realm isolation)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			desc := tc.info.SourceDescription(tc.cliConfigPath)
			assert.Contains(t, desc, tc.contains)
		})
	}
}
