package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveVersionConstraints(t *testing.T) {
	tests := []struct {
		name        string
		versions    []string
		constraints *schema.VendorConstraints
		want        string
		wantErr     bool
	}{
		{
			name:        "no constraints returns latest",
			versions:    []string{"1.0.0", "1.1.0", "1.2.0"},
			constraints: nil,
			want:        "1.2.0",
			wantErr:     false,
		},
		{
			name:     "caret constraint allows minor updates",
			versions: []string{"1.0.0", "1.1.0", "1.2.0", "2.0.0"},
			constraints: &schema.VendorConstraints{
				Version: "^1.0.0",
			},
			want:    "1.2.0",
			wantErr: false,
		},
		{
			name:     "tilde constraint allows patch updates",
			versions: []string{"1.2.0", "1.2.1", "1.2.2", "1.3.0"},
			constraints: &schema.VendorConstraints{
				Version: "~1.2.0",
			},
			want:    "1.2.2",
			wantErr: false,
		},
		{
			name:     "range constraint",
			versions: []string{"1.0.0", "1.5.0", "2.0.0", "2.5.0"},
			constraints: &schema.VendorConstraints{
				Version: ">=1.0.0 <2.0.0",
			},
			want:    "1.5.0",
			wantErr: false,
		},
		{
			name:     "excluded versions filter specific versions",
			versions: []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0"},
			constraints: &schema.VendorConstraints{
				ExcludedVersions: []string{"1.2.0"},
			},
			want:    "1.3.0",
			wantErr: false,
		},
		{
			name:     "excluded versions with wildcard",
			versions: []string{"1.5.0", "1.5.1", "1.5.2", "1.6.0"},
			constraints: &schema.VendorConstraints{
				ExcludedVersions: []string{"1.5.*"},
			},
			want:    "1.6.0",
			wantErr: false,
		},
		{
			name:     "no prereleases filter",
			versions: []string{"1.0.0-alpha", "1.0.0-beta", "1.0.0", "1.1.0"},
			constraints: &schema.VendorConstraints{
				NoPrereleases: true,
			},
			want:    "1.1.0",
			wantErr: false,
		},
		{
			name:     "combined constraints - version and excluded",
			versions: []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0", "2.0.0"},
			constraints: &schema.VendorConstraints{
				Version:          "^1.0.0",
				ExcludedVersions: []string{"1.2.0"},
			},
			want:    "1.3.0",
			wantErr: false,
		},
		{
			name:     "combined constraints - all filters",
			versions: []string{"1.0.0", "1.1.0-rc1", "1.2.0", "1.3.0", "2.0.0"},
			constraints: &schema.VendorConstraints{
				Version:          "^1.0.0",
				ExcludedVersions: []string{"1.2.0"},
				NoPrereleases:    true,
			},
			want:    "1.3.0",
			wantErr: false,
		},
		{
			name:     "no versions match constraints",
			versions: []string{"1.0.0", "1.1.0"},
			constraints: &schema.VendorConstraints{
				Version: "^2.0.0",
			},
			want:    "",
			wantErr: true,
		},
		{
			name:        "empty version list",
			versions:    []string{},
			constraints: nil,
			want:        "",
			wantErr:     true,
		},
		{
			name:     "invalid semver constraint",
			versions: []string{"1.0.0"},
			constraints: &schema.VendorConstraints{
				Version: "invalid",
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveVersionConstraints(tt.versions, tt.constraints)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestFilterBySemverConstraint(t *testing.T) {
	tests := []struct {
		name       string
		versions   []string
		constraint string
		want       []string
		wantErr    bool
	}{
		{
			name:       "caret allows compatible versions",
			versions:   []string{"1.0.0", "1.1.0", "1.2.0", "2.0.0"},
			constraint: "^1.0.0",
			want:       []string{"1.0.0", "1.1.0", "1.2.0"},
			wantErr:    false,
		},
		{
			name:       "tilde allows patch versions",
			versions:   []string{"1.2.0", "1.2.1", "1.2.2", "1.3.0"},
			constraint: "~1.2.0",
			want:       []string{"1.2.0", "1.2.1", "1.2.2"},
			wantErr:    false,
		},
		{
			name:       "greater than or equal",
			versions:   []string{"1.0.0", "1.5.0", "2.0.0"},
			constraint: ">=1.5.0",
			want:       []string{"1.5.0", "2.0.0"},
			wantErr:    false,
		},
		{
			name:       "range constraint",
			versions:   []string{"1.0.0", "1.5.0", "2.0.0", "2.5.0"},
			constraint: ">=1.0.0 <2.0.0",
			want:       []string{"1.0.0", "1.5.0"},
			wantErr:    false,
		},
		{
			name:       "invalid constraint",
			versions:   []string{"1.0.0"},
			constraint: "not-a-constraint",
			want:       nil,
			wantErr:    true,
		},
		{
			name:       "filters non-semver versions",
			versions:   []string{"1.0.0", "latest", "1.1.0", "main"},
			constraint: "^1.0.0",
			want:       []string{"1.0.0", "1.1.0"},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FilterBySemverConstraint(tt.versions, tt.constraint)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestFilterExcludedVersions(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		excluded []string
		want     []string
	}{
		{
			name:     "exact match exclusion",
			versions: []string{"1.0.0", "1.1.0", "1.2.0"},
			excluded: []string{"1.1.0"},
			want:     []string{"1.0.0", "1.2.0"},
		},
		{
			name:     "wildcard exclusion",
			versions: []string{"1.5.0", "1.5.1", "1.5.2", "1.6.0"},
			excluded: []string{"1.5.*"},
			want:     []string{"1.6.0"},
		},
		{
			name:     "multiple exact exclusions",
			versions: []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0"},
			excluded: []string{"1.0.0", "1.2.0"},
			want:     []string{"1.1.0", "1.3.0"},
		},
		{
			name:     "mixed exact and wildcard",
			versions: []string{"1.0.0", "1.5.0", "1.5.1", "1.6.0"},
			excluded: []string{"1.0.0", "1.5.*"},
			want:     []string{"1.6.0"},
		},
		{
			name:     "no exclusions",
			versions: []string{"1.0.0", "1.1.0"},
			excluded: []string{},
			want:     []string{"1.0.0", "1.1.0"},
		},
		{
			name:     "no matches",
			versions: []string{"1.0.0", "1.1.0"},
			excluded: []string{"2.0.0"},
			want:     []string{"1.0.0", "1.1.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterExcludedVersions(tt.versions, tt.excluded)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchesWildcard(t *testing.T) {
	tests := []struct {
		name    string
		version string
		pattern string
		want    bool
	}{
		{
			name:    "exact match",
			version: "1.2.3",
			pattern: "1.2.3",
			want:    true,
		},
		{
			name:    "wildcard match patch",
			version: "1.2.3",
			pattern: "1.2.*",
			want:    true,
		},
		{
			name:    "wildcard match minor",
			version: "1.2.3",
			pattern: "1.*",
			want:    true,
		},
		{
			name:    "wildcard no match",
			version: "2.0.0",
			pattern: "1.*",
			want:    false,
		},
		{
			name:    "no wildcard no match",
			version: "1.2.3",
			pattern: "1.2.4",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesWildcard(tt.version, tt.pattern)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilterPrereleases(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		want     []string
	}{
		{
			name:     "filters alpha versions",
			versions: []string{"1.0.0-alpha", "1.0.0"},
			want:     []string{"1.0.0"},
		},
		{
			name:     "filters beta versions",
			versions: []string{"1.0.0-beta.1", "1.0.0"},
			want:     []string{"1.0.0"},
		},
		{
			name:     "filters rc versions",
			versions: []string{"1.0.0-rc1", "1.0.0"},
			want:     []string{"1.0.0"},
		},
		{
			name:     "keeps stable versions",
			versions: []string{"1.0.0", "1.1.0", "1.2.0"},
			want:     []string{"1.0.0", "1.1.0", "1.2.0"},
		},
		{
			name:     "mixed prereleases and stable",
			versions: []string{"1.0.0-alpha", "1.0.0", "1.1.0-beta", "1.1.0"},
			want:     []string{"1.0.0", "1.1.0"},
		},
		{
			name:     "keeps non-semver strings",
			versions: []string{"latest", "main", "1.0.0-alpha", "1.0.0"},
			want:     []string{"latest", "main", "1.0.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterPrereleases(tt.versions)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSelectLatestVersion(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		want     string
		wantErr  bool
	}{
		{
			name:     "selects highest semver",
			versions: []string{"1.0.0", "1.2.0", "1.1.0"},
			want:     "1.2.0",
			wantErr:  false,
		},
		{
			name:     "handles major version differences",
			versions: []string{"1.9.0", "2.0.0", "1.10.0"},
			want:     "2.0.0",
			wantErr:  false,
		},
		{
			name:     "handles patch versions",
			versions: []string{"1.0.0", "1.0.1", "1.0.2"},
			want:     "1.0.2",
			wantErr:  false,
		},
		{
			name:     "fallback to first for non-semver",
			versions: []string{"latest", "main"},
			want:     "latest",
			wantErr:  false,
		},
		{
			name:     "empty list",
			versions: []string{},
			want:     "",
			wantErr:  true,
		},
		{
			name:     "single version",
			versions: []string{"1.0.0"},
			want:     "1.0.0",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectLatestVersion(tt.versions)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
