package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLegacyActionRepo(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		envSet   bool
		wantRepo string
		wantOK   bool
	}{
		{
			name:   "unset",
			envSet: false,
			wantOK: false,
		},
		{
			name:     "non-matching repo",
			envSet:   true,
			envValue: "actions/checkout",
			wantRepo: "actions/checkout",
			wantOK:   false,
		},
		{
			name:     "legacy atmos action",
			envSet:   true,
			envValue: "cloudposse/github-action-atmos-terraform-plan",
			wantRepo: "cloudposse/github-action-atmos-terraform-plan",
			wantOK:   true,
		},
		{
			name:     "unrelated cloudposse action",
			envSet:   true,
			envValue: "cloudposse/github-action-setup-terraform",
			wantRepo: "cloudposse/github-action-setup-terraform",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envSet {
				t.Setenv("GITHUB_ACTION_REPOSITORY", tt.envValue)
			} else {
				t.Setenv("GITHUB_ACTION_REPOSITORY", "")
			}

			repo, ok := LegacyActionRepo()

			assert.Equal(t, tt.wantRepo, repo)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}
