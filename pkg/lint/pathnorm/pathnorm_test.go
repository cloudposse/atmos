package pathnorm_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/lint/pathnorm"
)

func TestNormalizeRelNoExt(t *testing.T) {
	t.Parallel()

	base := "/stacks"

	tests := []struct {
		name     string
		path     string
		basePath string
		want     string
	}{
		{
			name:     "absolute with base and yaml ext",
			path:     "/stacks/catalog/vpc.yaml",
			basePath: base,
			want:     "catalog/vpc",
		},
		{
			name:     "absolute with base and yml ext",
			path:     "/stacks/catalog/vpc.yml",
			basePath: base,
			want:     "catalog/vpc",
		},
		{
			name:     "absolute with base no ext",
			path:     "/stacks/catalog/vpc",
			basePath: base,
			want:     "catalog/vpc",
		},
		{
			name:     "relative with base",
			path:     "catalog/vpc.yaml",
			basePath: base,
			want:     "catalog/vpc",
		},
		{
			name:     "absolute no base",
			path:     "/stacks/catalog/vpc.yaml",
			basePath: "",
			want:     "/stacks/catalog/vpc",
		},
		{
			name:     "relative no base",
			path:     "deploy/prod.yaml",
			basePath: "",
			want:     "deploy/prod",
		},
		{
			name:     "nested absolute",
			path:     "/stacks/deploy/us-east-1/prod.yaml",
			basePath: base,
			want:     "deploy/us-east-1/prod",
		},
		{
			name:     "dot-segment is cleaned",
			path:     "/stacks/./catalog/vpc.yaml",
			basePath: base,
			want:     "catalog/vpc",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := pathnorm.NormalizeRelNoExt(tc.path, tc.basePath)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestNormalizeRelNoExtWindowsSeparators verifies that path separators are
// normalised to forward slashes on all platforms (important for Windows CI).
func TestNormalizeRelNoExtWindowsSeparators(t *testing.T) {
	t.Parallel()

	// filepath.Join uses the OS separator, so this test verifies cross-platform output.
	path := filepath.Join("/", "stacks", "catalog", "vpc.yaml")
	got := pathnorm.NormalizeRelNoExt(path, "/stacks")
	assert.Equal(t, "catalog/vpc", got)
}
