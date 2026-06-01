package downloader

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveToken_GitHubPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		settings   schema.AtmosSettings
		wantToken  string
		wantSource string
	}{
		{
			name:       "ATMOS_PRO_GITHUB_TOKEN wins",
			settings:   schema.AtmosSettings{AtmosProGithubToken: "pro", AtmosGithubToken: "atmos", GithubToken: "gh"},
			wantToken:  "pro",
			wantSource: "ATMOS_PRO_GITHUB_TOKEN",
		},
		{
			name:       "falls back to ATMOS_GITHUB_TOKEN",
			settings:   schema.AtmosSettings{AtmosGithubToken: "atmos", GithubToken: "gh"},
			wantToken:  "atmos",
			wantSource: "ATMOS_GITHUB_TOKEN",
		},
		{
			name:       "falls back to GITHUB_TOKEN",
			settings:   schema.AtmosSettings{GithubToken: "gh"},
			wantToken:  "gh",
			wantSource: "GITHUB_TOKEN",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := NewCustomGitDetector(&schema.AtmosConfiguration{Settings: tc.settings}, "")
			token, source := d.resolveToken(hostGitHub)
			assert.Equal(t, tc.wantToken, token)
			assert.Equal(t, tc.wantSource, source)
		})
	}
}
