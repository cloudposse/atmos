package plugin

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestParseSpec(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantName    string
		wantURL     string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "alias without version",
			raw:         "diff",
			wantName:    "diff",
			wantURL:     "https://github.com/databus23/helm-diff",
			wantVersion: "",
		},
		{
			name:        "alias with version",
			raw:         "diff@v3.9.4",
			wantName:    "diff",
			wantURL:     "https://github.com/databus23/helm-diff",
			wantVersion: "v3.9.4",
		},
		{
			name:        "owner/repo",
			raw:         "databus23/helm-diff",
			wantName:    "helm-diff",
			wantURL:     "https://github.com/databus23/helm-diff",
			wantVersion: "",
		},
		{
			name:        "owner/repo with version",
			raw:         "jkroepke/helm-secrets@v4.6.0",
			wantName:    "helm-secrets",
			wantURL:     "https://github.com/jkroepke/helm-secrets",
			wantVersion: "v4.6.0",
		},
		{
			name:        "full url with version",
			raw:         "https://github.com/databus23/helm-diff@v3.9.4",
			wantName:    "helm-diff",
			wantURL:     "https://github.com/databus23/helm-diff",
			wantVersion: "v3.9.4",
		},
		{
			name:        "scp-style url is not split on @",
			raw:         "git@github.com:databus23/helm-diff",
			wantName:    "helm-diff",
			wantURL:     "git@github.com:databus23/helm-diff",
			wantVersion: "",
		},
		{
			name:    "unknown alias",
			raw:     "totally-unknown-plugin",
			wantErr: true,
		},
		{
			name:    "empty",
			raw:     "   ",
			wantErr: true,
		},
		{
			name:    "constraint version rejected",
			raw:     "diff@3.9.x",
			wantErr: true,
		},
		{
			name:    "caret constraint rejected",
			raw:     "diff@^3.9.0",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSpec(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrInvalidHelmPluginSpec))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, got.Name)
			assert.Equal(t, tt.wantURL, got.URL)
			assert.Equal(t, tt.wantVersion, got.Version)
			assert.Equal(t, tt.raw, got.Raw)
		})
	}
}

func TestParseSpecs(t *testing.T) {
	specs, err := ParseSpecs([]string{"diff@v3.9.4", "secrets"})
	require.NoError(t, err)
	require.Len(t, specs, 2)
	assert.Equal(t, "https://github.com/databus23/helm-diff", specs[0].URL)
	assert.Equal(t, "v3.9.4", specs[0].Version)
	assert.Equal(t, "https://github.com/jkroepke/helm-secrets", specs[1].URL)
	assert.True(t, specs[1].IsLatest())

	_, err = ParseSpecs([]string{"diff", "nope-unknown"})
	require.Error(t, err)
}

func TestSpecIsLatest(t *testing.T) {
	assert.True(t, Spec{Version: ""}.IsLatest())
	assert.True(t, Spec{Version: "latest"}.IsLatest())
	assert.True(t, Spec{Version: "LATEST"}.IsLatest())
	assert.False(t, Spec{Version: "v3.9.4"}.IsLatest())
}

func TestSpecCandidateNames(t *testing.T) {
	assert.Equal(t, []string{"diff", "helm-diff"}, Spec{Name: "diff"}.candidateNames())
	assert.Equal(t, []string{"helm-git", "git"}, Spec{Name: "helm-git"}.candidateNames())
}

func TestKnownAliases(t *testing.T) {
	aliases := KnownAliases()
	assert.Equal(t, []string{"diff", "git", "s3", "secrets", "unittest"}, aliases)
}
