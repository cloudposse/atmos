package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	workflowIdentity = "https://github.com/acme/images/.github/workflows/release.yml@refs/heads/main"
	githubIssuer     = "https://token.actions.githubusercontent.com"
)

func boolPtr(b bool) *bool { return &b }

func identityPolicy(requireSignature *bool, identity string) *schema.ArtifactVerificationConfig {
	return &schema.ArtifactVerificationConfig{
		RequireSignature:  requireSignature,
		TrustedIdentities: []schema.ArtifactTrustedIdentity{{Identity: identity, Issuer: githubIssuer}},
	}
}

func TestValidate_VerificationShape(t *testing.T) {
	keyDir := t.TempDir()
	tests := []struct {
		name    string
		policy  *schema.ArtifactVerificationConfig
		wantErr string
	}{
		{name: "PRD example", policy: identityPolicy(boolPtr(true), workflowIdentity)},
		{name: "empty key list is allowed", policy: &schema.ArtifactVerificationConfig{TrustedKeys: []string{}}},
		{name: "missing identity", policy: identityPolicy(nil, ""), wantErr: "identity is required"},
		{name: "issuer without https", policy: &schema.ArtifactVerificationConfig{TrustedIdentities: []schema.ArtifactTrustedIdentity{{Identity: "me", Issuer: "http://issuer.example"}}}, wantErr: "https URL"},
		{name: "missing issuer", policy: &schema.ArtifactVerificationConfig{TrustedIdentities: []schema.ArtifactTrustedIdentity{{Identity: "me"}}}, wantErr: "https URL"},
		{name: "issuer with credentials", policy: &schema.ArtifactVerificationConfig{TrustedIdentities: []schema.ArtifactTrustedIdentity{{Identity: "me", Issuer: "https://u:p@issuer.example"}}}, wantErr: "https URL"},
		{name: "absolute key", policy: &schema.ArtifactVerificationConfig{TrustedKeys: []string{filepath.Join(keyDir, "cosign.pub")}}},
		{name: "empty key path", policy: &schema.ArtifactVerificationConfig{TrustedKeys: []string{""}}, wantErr: "non-empty file path"},
		{name: "relative key without a declaring file", policy: &schema.ArtifactVerificationConfig{TrustedKeys: []string{"cosign.pub"}}, wantErr: "no declaring configuration file"},
		{name: "relative key from a remote import", policy: &schema.ArtifactVerificationConfig{TrustedKeys: []string{"cosign.pub"}, KeysOrigin: NewOrigin(filepath.Join(keyDir, "r.yaml"), true)}, wantErr: "remote import"},
		{name: "relative key from a local file", policy: &schema.ArtifactVerificationConfig{TrustedKeys: []string{"cosign.pub"}, KeysOrigin: NewOrigin(filepath.Join(keyDir, "atmos.yaml"), false)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(&schema.ArtifactConfig{Verification: tt.policy})
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidate_RepositoryCannotWeakenGlobalSignatureRequirement(t *testing.T) {
	repo := registryRepo()
	repo.Verification = &schema.ArtifactVerificationConfig{RequireSignature: boolPtr(false)}
	cfg := &schema.ArtifactConfig{
		Verification: &schema.ArtifactVerificationConfig{RequireSignature: boolPtr(true)},
		Repositories: map[string]schema.ArtifactRepositoryConfig{"shared": repo},
	}
	err := Validate(cfg)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.Contains(t, err.Error(), "cannot weaken")

	// Strengthening, or leaving it unset, is fine.
	repo.Verification.RequireSignature = boolPtr(true)
	cfg.Repositories["shared"] = repo
	require.NoError(t, Validate(cfg))
	repo.Verification.RequireSignature = nil
	cfg.Repositories["shared"] = repo
	require.NoError(t, Validate(cfg))

	// A repository may be stricter than a permissive global policy.
	cfg.Verification.RequireSignature = boolPtr(false)
	repo.Verification.RequireSignature = boolPtr(true)
	cfg.Repositories["shared"] = repo
	require.NoError(t, Validate(cfg))
}

func TestEffectivePolicy(t *testing.T) {
	repoIdentity := "https://github.com/acme/other/.github/workflows/release.yml@refs/heads/main"
	tests := []struct {
		name         string
		global, repo *schema.ArtifactVerificationConfig
		want         artifact.VerificationPolicy
	}{
		{name: "nothing configured", want: artifact.VerificationPolicy{}},
		{
			name:   "global only",
			global: identityPolicy(boolPtr(true), workflowIdentity),
			want: artifact.VerificationPolicy{RequireSignature: true, TrustSets: []artifact.TrustSet{
				{Identities: []artifact.TrustedIdentity{{Identity: workflowIdentity, Issuer: githubIssuer}}},
			}},
		},
		{
			name: "repository only",
			repo: &schema.ArtifactVerificationConfig{RequireSignature: boolPtr(true), TrustedKeys: []string{"/keys/a.pub"}},
			want: artifact.VerificationPolicy{RequireSignature: true, TrustSets: []artifact.TrustSet{{Keys: []string{"/keys/a.pub"}}}},
		},
		{
			name:   "both levels keep two trust sets, global first, never unioned",
			global: identityPolicy(boolPtr(true), workflowIdentity),
			repo:   identityPolicy(nil, repoIdentity),
			want: artifact.VerificationPolicy{RequireSignature: true, TrustSets: []artifact.TrustSet{
				{Identities: []artifact.TrustedIdentity{{Identity: workflowIdentity, Issuer: githubIssuer}}},
				{Identities: []artifact.TrustedIdentity{{Identity: repoIdentity, Issuer: githubIssuer}}},
			}},
		},
		{
			name:   "repository false cannot lower a global requirement",
			global: &schema.ArtifactVerificationConfig{RequireSignature: boolPtr(true)},
			repo:   &schema.ArtifactVerificationConfig{RequireSignature: boolPtr(false)},
			want:   artifact.VerificationPolicy{RequireSignature: true},
		},
		{
			name:   "empty trust level adds no trust set",
			global: &schema.ArtifactVerificationConfig{TrustedKeys: []string{}},
			want:   artifact.VerificationPolicy{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := registryRepo()
			repo.Verification = tt.repo
			cfg := &schema.ArtifactConfig{Verification: tt.global, Repositories: map[string]schema.ArtifactRepositoryConfig{"shared": repo}}
			got, err := EffectivePolicy(cfg, "shared")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := EffectivePolicy(&schema.ArtifactConfig{}, "missing")
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
}

func TestCombinePolicy_DoesNotAliasConfiguration(t *testing.T) {
	global := identityPolicy(boolPtr(true), workflowIdentity)
	global.TrustedKeys = []string{"/keys/a.pub"}
	policy := CombinePolicy(global, nil)
	policy.TrustSets[0].Keys[0] = "/mutated"
	policy.TrustSets[0].Identities[0].Identity = "mutated"
	assert.Equal(t, []string{"/keys/a.pub"}, global.TrustedKeys)
	assert.Equal(t, workflowIdentity, global.TrustedIdentities[0].Identity)
}
