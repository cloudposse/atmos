package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGitHubOwnerRepo(t *testing.T) {
	tests := []struct {
		name      string
		uri       string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		// Standard no-scheme GitHub URIs (go-getter style).
		{
			name:      "plain github.com URI",
			uri:       "github.com/cloudposse/terraform-null-label",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "github.com URI with subdirectory",
			uri:       "github.com/cloudposse/terraform-null-label//modules/vpc",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "github.com URI with subdirectory and query params",
			uri:       "github.com/cloudposse/terraform-null-label//modules/vpc?ref=v1.0.0",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},

		// HTTPS URLs.
		{
			name:      "https github URL",
			uri:       "https://github.com/cloudposse/terraform-null-label",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "https github URL with .git suffix",
			uri:       "https://github.com/cloudposse/terraform-null-label.git",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "https github URL with subdirectory",
			uri:       "https://github.com/cloudposse/terraform-null-label//modules/vpc?ref=v1.0.0",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},

		// go-getter force prefix.
		{
			name:      "git:: force prefix with https",
			uri:       "git::https://github.com/cloudposse/terraform-null-label.git",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "git:: force prefix without scheme",
			uri:       "git::github.com/cloudposse/terraform-null-label",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},

		// SCP-style Git URLs.
		{
			name:      "SCP-style git@github.com",
			uri:       "git@github.com:cloudposse/terraform-null-label.git",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},
		{
			name:      "SCP-style git@github.com with subdirectory",
			uri:       "git@github.com:cloudposse/terraform-null-label.git//modules/vpc",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-null-label",
			wantOK:    true,
		},

		// Non-GitHub URIs (should return ok=false).
		{
			name:   "gitlab URI",
			uri:    "gitlab.com/owner/repo",
			wantOK: false,
		},
		{
			name:   "s3 URI",
			uri:    "s3::s3://mybucket/path",
			wantOK: false,
		},
		{
			name:   "oci URI",
			uri:    "oci://registry.example.com/org/image:tag",
			wantOK: false,
		},
		{
			name:   "local path",
			uri:    "./local/path",
			wantOK: false,
		},
		{
			name:   "empty URI",
			uri:    "",
			wantOK: false,
		},
		{
			name:   "bitbucket URI",
			uri:    "bitbucket.org/owner/repo",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := ParseGitHubOwnerRepo(tt.uri)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantOwner, owner)
				assert.Equal(t, tt.wantRepo, repo)
			}
		})
	}
}
