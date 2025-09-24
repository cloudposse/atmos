package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeVendorURI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "triple-slash with query params converts to double-slash-dot",
			input:    "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git///?ref=v5.7.0",
			expected: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0",
		},
		{
			name:     "triple-slash with path and query params",
			input:    "github.com/cloudposse/terraform-aws-components.git///modules/vpc?ref=1.398.0",
			expected: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0",
		},
		{
			name:     "double-slash pattern unchanged",
			input:    "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0",
			expected: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0",
		},
		{
			name:     "no subdirectory pattern gets double-slash-dot added",
			input:    "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git?ref=v5.7.0",
			expected: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0",
		},
		{
			name:     "OCI registry URL unchanged",
			input:    "oci://public.ecr.aws/cloudposse/terraform-aws-components:latest",
			expected: "oci://public.ecr.aws/cloudposse/terraform-aws-components:latest",
		},
		{
			name:     "local file path unchanged",
			input:    "file:///path/to/local/components",
			expected: "file:///path/to/local/components",
		},
		{
			name:     "triple-slash without query params",
			input:    "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git///",
			expected: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.",
		},
		{
			name:     "multiple triple-slash patterns (only first is processed)",
			input:    "github.com/repo.git///path///subpath?ref=v1.0",
			expected: "github.com/repo.git//path///subpath?ref=v1.0",
		},
		{
			name:     "git URL without .git extension and no subdir",
			input:    "github.com/terraform-aws-modules/terraform-aws-s3-bucket?ref=v5.7.0",
			expected: "github.com/terraform-aws-modules/terraform-aws-s3-bucket//.?ref=v5.7.0",
		},
		{
			name:     "git URL with .git and existing double-slash-dot",
			input:    "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0",
			expected: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0",
		},
		{
			name:     "https git URL without subdir",
			input:    "https://github.com/cloudposse/atmos.git?ref=main",
			expected: "https://github.com/cloudposse/atmos.git//.?ref=main",
		},
		{
			name:     "git:: prefix URL without subdir",
			input:    "git::https://github.com/cloudposse/atmos.git?ref=main",
			expected: "git::https://github.com/cloudposse/atmos.git//.?ref=main",
		},
		{
			name:     "git:: prefix URL with subdir unchanged",
			input:    "git::https://github.com/cloudposse/atmos.git//examples?ref=main",
			expected: "git::https://github.com/cloudposse/atmos.git//examples?ref=main",
		},
		{
			name:     "local relative path unchanged",
			input:    "../../../components/terraform",
			expected: "../../../components/terraform",
		},
		{
			name:     "s3 URL unchanged",
			input:    "s3::https://s3.amazonaws.com/bucket/path",
			expected: "s3::https://s3.amazonaws.com/bucket/path",
		},
		{
			name:     "http URL (non-git) unchanged",
			input:    "https://example.com/archive.tar.gz",
			expected: "https://example.com/archive.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeVendorURI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
