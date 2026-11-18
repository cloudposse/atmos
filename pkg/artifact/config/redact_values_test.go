package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestEchoedNames_DoNotEchoCredentials checks that a repository alias, kind, lock scope or lock
// delegate that is a pasted credential-bearing URL never reaches a validation error.
func TestEchoedNames_DoNotEchoCredentials(t *testing.T) {
	const credential = "https://usr7:S3cr3tA@example.com"
	local := schema.ArtifactRepositoryConfig{Kind: "local/archive"}
	delegating := func(target string) *schema.ArtifactConfig {
		return &schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
			"a": {Kind: "local/archive", Locking: &schema.ArtifactLockingConfig{Repository: target}},
		}}
	}
	_, missingAlias := ResolveLockPlan(&schema.ArtifactConfig{}, credential)
	_, missingDelegate := ResolveLockPlan(delegating(credential), "a")
	_, scope := ResolveLockPlan(&schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"a": {Kind: "local/archive", Locking: &schema.ArtifactLockingConfig{RequiredScope: credential}},
	}}, "a")
	_, cycle := ResolveLockPlan(&schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"a":        {Kind: "local/archive", Locking: &schema.ArtifactLockingConfig{Repository: credential}},
		credential: {Kind: "local/archive", Locking: &schema.ArtifactLockingConfig{Repository: "a"}},
	}}, "a")
	withAlias := &schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{credential: local}}

	tests := []struct {
		name string
		err  error
	}{
		{name: "lock plan of an unknown alias", err: missingAlias},
		{name: "lock delegate that is not configured", err: missingDelegate},
		{name: "lock scope", err: scope},
		{name: "lock delegation cycle", err: cycle},
		{name: "alias that is not valid", err: NewValidator().ValidateRepository(withAlias, credential)},
		{name: "configured aliases listed beside an unknown one", err: NewValidator().ValidateRepository(withAlias, "nope")},
		{name: "unknown kind", err: NewValidator().ValidateRepository(&schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{"a": {Kind: credential}}}, "a")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.err)
			assert.ErrorIs(t, tt.err, errUtils.ErrArtifactConfigInvalid)
			assert.NotContains(t, tt.err.Error(), "S3cr3tA")
			assert.NotContains(t, tt.err.Error(), "usr7")
		})
	}
}

// TestEchoedNames_KeepNamesWithoutCredentials is the negative path of the test above: a plain alias
// is still named in the error.
func TestEchoedNames_KeepNamesWithoutCredentials(t *testing.T) {
	err := NewValidator().ValidateRepository(&schema.ArtifactConfig{}, "prod-eu")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `repository "prod-eu" is not configured`)
}

// TestDecode_ErrorsDoNotEchoCredentials checks that the keys and values a decode error quotes are
// scrubbed, and that a non-string permissions value is shown without its credential.
func TestDecode_ErrorsDoNotEchoCredentials(t *testing.T) {
	const credential = "https://usr7:S3cr3tA@example.com"
	tests := []struct {
		name string
		raw  map[string]any
	}{
		{name: "unknown repository field", raw: map[string]any{"repositories": map[string]any{"a": map[string]any{"kind": "local/archive", credential: "x"}}}},
		{name: "unknown top-level field", raw: map[string]any{credential: "x"}},
		{name: "credential-like unknown field", raw: map[string]any{"repositories": map[string]any{"a": map[string]any{"usr7:S3cr3tA@password": "x"}}}},
		{name: "permissions holding a credential", raw: map[string]any{"repositories": map[string]any{credential: map[string]any{"kind": "local/archive", "permissions": map[string]any{"k": credential}}}}},
		{name: "permissions holding a scheme-less credential", raw: map[string]any{"repositories": map[string]any{"a": map[string]any{"kind": "local/archive", "permissions": map[string]any{"k": "usr7:S3cr3tA@example.com"}}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(tt.raw)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.NotContains(t, err.Error(), "S3cr3tA")
			assert.NotContains(t, err.Error(), "usr7")
		})
	}
}
