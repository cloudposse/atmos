package source

import (
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveSourceURI(t *testing.T) {
	tests := []struct {
		name       string
		sourceSpec *schema.VendorComponentSource
		expected   string
	}{
		{
			name: "URI with version already in ref",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
				Version: "",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
		},
		{
			name: "URI without ref, version specified",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc",
				Version: "v1.0.0",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
		},
		{
			name: "URI with query params, version specified",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc?depth=1",
				Version: "v1.0.0",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc?depth=1&ref=v1.0.0",
		},
		{
			name: "URI with ref, version also specified (ref takes priority)",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
				Version: "v2.0.0",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
		},
		{
			name: "URI with &ref in query params",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc?depth=1&ref=v1.0.0",
				Version: "v2.0.0",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc?depth=1&ref=v1.0.0",
		},
		{
			name: "empty version",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc",
				Version: "",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveSourceURI(tt.sourceSpec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "triple slash to double slash dot",
			uri:      "github.com/cloudposse/terraform-aws-vpc///",
			expected: "github.com/cloudposse/terraform-aws-vpc//.",
		},
		{
			name:     "triple slash with query params",
			uri:      "github.com/cloudposse/terraform-aws-vpc///?ref=v1.0.0",
			expected: "github.com/cloudposse/terraform-aws-vpc//.?ref=v1.0.0",
		},
		{
			name:     "no triple slash",
			uri:      "github.com/cloudposse/terraform-aws-vpc//modules/vpc",
			expected: "github.com/cloudposse/terraform-aws-vpc//modules/vpc",
		},
		{
			name:     "empty URI",
			uri:      "",
			expected: "",
		},
		{
			name:     "multiple triple slashes (only first replaced)",
			uri:      "github.com/cloudposse/terraform-aws-vpc//////",
			expected: "github.com/cloudposse/terraform-aws-vpc//.///",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURI(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateSkipFunc(t *testing.T) {
	tests := []struct {
		name       string
		sourceSpec *schema.VendorComponentSource
		fileName   string
		isDir      bool
		expected   bool
	}{
		{
			name: "skip .git directory",
			sourceSpec: &schema.VendorComponentSource{
				Uri: "github.com/example/repo",
			},
			fileName: ".git",
			isDir:    true,
			expected: true,
		},
		{
			name: "no patterns - don't skip regular file",
			sourceSpec: &schema.VendorComponentSource{
				Uri: "github.com/example/repo",
			},
			fileName: "main.tf",
			isDir:    false,
			expected: false,
		},
		{
			name: "no patterns - don't skip regular directory",
			sourceSpec: &schema.VendorComponentSource{
				Uri: "github.com/example/repo",
			},
			fileName: "modules",
			isDir:    true,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock FileInfo.
			info := &mockFileInfo{
				name:  tt.fileName,
				isDir: tt.isDir,
			}

			skipFunc := createSkipFunc("/tmp/src", tt.sourceSpec)
			result, err := skipFunc(info, "/tmp/src/"+tt.fileName, "/tmp/dst/"+tt.fileName)

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockFileInfo implements fs.FileInfo for testing.
type mockFileInfo struct {
	name  string
	isDir bool
}

// Ensure mockFileInfo implements fs.FileInfo.
var _ fs.FileInfo = (*mockFileInfo)(nil)

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() fs.FileMode  { return 0o644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() any           { return nil }
