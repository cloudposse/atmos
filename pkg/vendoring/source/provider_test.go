package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetProviderForSource(t *testing.T) {
	tests := []struct {
		name              string
		source            string
		expectedType      interface{}
		expectedSupported Operation
	}{
		{
			name:              "GitHub HTTPS URL",
			source:            "https://github.com/cloudposse/terraform-aws-vpc.git",
			expectedType:      &GitHubProvider{},
			expectedSupported: OperationGetDiff,
		},
		{
			name:              "GitHub shorthand",
			source:            "github.com/cloudposse/terraform-aws-vpc",
			expectedType:      &GitHubProvider{},
			expectedSupported: OperationGetDiff,
		},
		{
			name:              "GitHub SSH URL",
			source:            "git@github.com:cloudposse/terraform-aws-vpc.git",
			expectedType:      &GitHubProvider{},
			expectedSupported: OperationGetDiff,
		},
		{
			name:              "GitHub git:: prefix",
			source:            "git::https://github.com/cloudposse/terraform-aws-vpc.git",
			expectedType:      &GitHubProvider{},
			expectedSupported: OperationGetDiff,
		},
		{
			name:              "GitLab HTTPS URL",
			source:            "https://gitlab.com/example/repo.git",
			expectedType:      &GenericGitProvider{},
			expectedSupported: OperationListVersions,
		},
		{
			name:              "Generic Git HTTPS",
			source:            "https://git.example.com/repo.git",
			expectedType:      &GenericGitProvider{},
			expectedSupported: OperationListVersions,
		},
		{
			name:              "Generic Git SSH",
			source:            "git@git.example.com:repo.git",
			expectedType:      &GenericGitProvider{},
			expectedSupported: OperationListVersions,
		},
		{
			name:              "OCI registry",
			source:            "oci://registry.example.com/component",
			expectedType:      &UnsupportedProvider{},
			expectedSupported: "",
		},
		{
			name:              "Local path",
			source:            "/path/to/local/component",
			expectedType:      &UnsupportedProvider{},
			expectedSupported: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := GetProviderForSource(tt.source)
			assert.IsType(t, tt.expectedType, provider)
			if tt.expectedSupported != "" {
				assert.True(t, provider.SupportsOperation(tt.expectedSupported))
			}
		})
	}
}

func TestGitHubProvider_SupportsOperation(t *testing.T) {
	provider := NewGitHubProvider()

	assert.True(t, provider.SupportsOperation(OperationListVersions))
	assert.True(t, provider.SupportsOperation(OperationVerifyVersion))
	assert.True(t, provider.SupportsOperation(OperationGetDiff))
	assert.True(t, provider.SupportsOperation(OperationFetchSource))
	assert.False(t, provider.SupportsOperation(Operation("unknown")))
}

func TestGenericGitProvider_SupportsOperation(t *testing.T) {
	provider := NewGenericGitProvider()

	assert.True(t, provider.SupportsOperation(OperationListVersions))
	assert.True(t, provider.SupportsOperation(OperationVerifyVersion))
	assert.False(t, provider.SupportsOperation(OperationGetDiff))
	assert.True(t, provider.SupportsOperation(OperationFetchSource))
	assert.False(t, provider.SupportsOperation(Operation("unknown")))
}

func TestUnsupportedProvider_SupportsOperation(t *testing.T) {
	provider := NewUnsupportedProvider()

	assert.False(t, provider.SupportsOperation(OperationListVersions))
	assert.False(t, provider.SupportsOperation(OperationVerifyVersion))
	assert.False(t, provider.SupportsOperation(OperationGetDiff))
	assert.False(t, provider.SupportsOperation(OperationFetchSource))
}

func TestParseGitHubRepo(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "HTTPS URL",
			source:    "https://github.com/cloudposse/terraform-aws-vpc.git",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-aws-vpc",
			wantErr:   false,
		},
		{
			name:      "Shorthand",
			source:    "github.com/cloudposse/terraform-aws-vpc",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-aws-vpc",
			wantErr:   false,
		},
		{
			name:      "SSH URL",
			source:    "git@github.com:cloudposse/terraform-aws-vpc.git",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-aws-vpc",
			wantErr:   false,
		},
		{
			name:      "With git:: prefix",
			source:    "git::https://github.com/cloudposse/terraform-aws-vpc.git",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-aws-vpc",
			wantErr:   false,
		},
		{
			name:      "With module path",
			source:    "github.com/cloudposse/terraform-aws-vpc//modules/subnets",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-aws-vpc",
			wantErr:   false,
		},
		{
			name:      "With query params",
			source:    "github.com/cloudposse/terraform-aws-vpc?ref=tags/1.0.0",
			wantOwner: "cloudposse",
			wantRepo:  "terraform-aws-vpc",
			wantErr:   false,
		},
		{
			name:      "Invalid format",
			source:    "invalid",
			wantOwner: "",
			wantRepo:  "",
			wantErr:   true,
		},
		{
			name:      "Missing repo name",
			source:    "github.com/cloudposse",
			wantOwner: "",
			wantRepo:  "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ParseGitHubRepo(tt.source)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantOwner, owner)
				assert.Equal(t, tt.wantRepo, repo)
			}
		})
	}
}

func TestIsGitHubSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{
			name:   "GitHub HTTPS",
			source: "https://github.com/cloudposse/terraform-aws-vpc.git",
			want:   true,
		},
		{
			name:   "GitHub shorthand",
			source: "github.com/cloudposse/terraform-aws-vpc",
			want:   true,
		},
		{
			name:   "GitHub SSH",
			source: "git@github.com:cloudposse/terraform-aws-vpc.git",
			want:   true,
		},
		{
			name:   "GitLab",
			source: "https://gitlab.com/example/repo.git",
			want:   false,
		},
		{
			name:   "Generic Git",
			source: "https://git.example.com/repo.git",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsGitHubSource(tt.source)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsGitSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{
			name:   "HTTPS Git URL",
			source: "https://gitlab.com/example/repo.git",
			want:   true,
		},
		{
			name:   "SSH Git URL",
			source: "git@gitlab.com:example/repo.git",
			want:   true,
		},
		{
			name:   "git:: prefix",
			source: "git::https://example.com/repo.git",
			want:   true,
		},
		{
			name:   ".git suffix",
			source: "https://example.com/repo.git",
			want:   true,
		},
		{
			name:   "OCI registry",
			source: "oci://registry.example.com/component",
			want:   false,
		},
		{
			name:   "Local path",
			source: "/path/to/local/component",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsGitSource(tt.source)
			assert.Equal(t, tt.want, got)
		})
	}
}
