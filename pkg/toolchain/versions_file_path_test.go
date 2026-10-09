package toolchain

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveVersionsFilePath_Precedence(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name   string
		config *schema.AtmosConfiguration
		want   string
	}{
		{
			name:   "nil config uses default",
			config: nil,
			want:   DefaultToolVersionsFilePath,
		},
		{
			name:   "neither set uses default anchored to base path",
			config: &schema.AtmosConfiguration{BasePathAbsolute: base},
			want:   filepath.Join(base, DefaultToolVersionsFilePath),
		},
		{
			name: "file_path only",
			config: &schema.AtmosConfiguration{
				BasePathAbsolute: base,
				Toolchain:        schema.Toolchain{FilePath: "config/file-path-versions"},
			},
			want: filepath.Join(base, "config", "file-path-versions"),
		},
		{
			name: "versions_file only",
			config: &schema.AtmosConfiguration{
				BasePathAbsolute: base,
				Toolchain:        schema.Toolchain{VersionsFile: "config/versions-file-versions"},
			},
			want: filepath.Join(base, "config", "versions-file-versions"),
		},
		{
			name: "both set file_path wins",
			config: &schema.AtmosConfiguration{
				BasePathAbsolute: base,
				Toolchain: schema.Toolchain{
					FilePath:     "primary-versions",
					VersionsFile: "alternative-versions",
				},
			},
			want: filepath.Join(base, "primary-versions"),
		},
		{
			name: "absolute file_path is not re-anchored",
			config: &schema.AtmosConfiguration{
				BasePathAbsolute: base,
				Toolchain:        schema.Toolchain{FilePath: filepath.Join(base, "abs", "versions")},
			},
			want: filepath.Join(base, "abs", "versions"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveVersionsFilePath(tt.config))
		})
	}
}

func TestGetToolVersionsFilePath_HonoursFilePath(t *testing.T) {
	original := GetAtmosConfig()
	t.Cleanup(func() { SetAtmosConfig(original) })

	SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{FilePath: "custom.tool-versions"},
	})

	assert.Equal(t, "custom.tool-versions", GetToolVersionsFilePath())
}
