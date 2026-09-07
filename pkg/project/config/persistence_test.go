package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/types"
)

func TestGetConfigPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	path, err := GetConfigPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(homeDir, ".atmos"), path)
}

func TestReadScaffoldConfig(t *testing.T) {
	t.Run("missing file returns empty map", func(t *testing.T) {
		tempDir := t.TempDir()

		config, err := ReadScaffoldConfig(tempDir)
		require.NoError(t, err)
		assert.Empty(t, config)
	})

	t.Run("valid atmos.yaml is parsed with casing preserved", func(t *testing.T) {
		tempDir := t.TempDir()
		content := `scaffold:
  projectName: my-project
  cloudProvider: aws
`
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(content), 0o644))

		config, err := ReadScaffoldConfig(tempDir)
		require.NoError(t, err)
		scaffold, ok := config["scaffold"].(map[string]interface{})
		require.True(t, ok)
		// Mixed-case keys must survive unmangled - this is the whole reason
		// yaml.v3 is used directly instead of Viper's AllSettings().
		assert.Equal(t, "my-project", scaffold["projectName"])
		assert.Equal(t, "aws", scaffold["cloudProvider"])
	})

	t.Run("empty file returns empty map", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(""), 0o644))

		config, err := ReadScaffoldConfig(tempDir)
		require.NoError(t, err)
		assert.Empty(t, config)
	})

	t.Run("invalid yaml returns error", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte("key: [unclosed"), 0o644))

		_, err := ReadScaffoldConfig(tempDir)
		require.Error(t, err)
	})
}

func TestReadAtmosScaffoldSection(t *testing.T) {
	t.Run("missing file returns empty map", func(t *testing.T) {
		tempDir := t.TempDir()

		section, err := ReadAtmosScaffoldSection(tempDir)
		require.NoError(t, err)
		assert.Empty(t, section)
	})

	t.Run("no scaffold key returns empty map", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte("other: value\n"), 0o644))

		section, err := ReadAtmosScaffoldSection(tempDir)
		require.NoError(t, err)
		assert.Empty(t, section)
	})

	t.Run("nil scaffold key returns empty map", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte("scaffold:\nother: value\n"), 0o644))

		section, err := ReadAtmosScaffoldSection(tempDir)
		require.NoError(t, err)
		assert.Empty(t, section)
	})

	t.Run("extracts only the scaffold section, preserving casing", func(t *testing.T) {
		tempDir := t.TempDir()
		content := `otherSection:
  ignored: true
scaffold:
  projectName: my-project
  regions:
    - us-east-1
`
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(content), 0o644))

		section, err := ReadAtmosScaffoldSection(tempDir)
		require.NoError(t, err)
		assert.Equal(t, "my-project", section["projectName"])
		_, hasOther := section["otherSection"]
		assert.False(t, hasOther)
	})

	t.Run("empty file returns empty map", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(""), 0o644))

		section, err := ReadAtmosScaffoldSection(tempDir)
		require.NoError(t, err)
		assert.Empty(t, section)
	})

	t.Run("invalid yaml returns error", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte("key: [unclosed"), 0o644))

		_, err := ReadAtmosScaffoldSection(tempDir)
		require.Error(t, err)
	})

	t.Run("non-map scaffold section returns ErrInvalidScaffoldSection", func(t *testing.T) {
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte("scaffold: not-a-map\n"), 0o644))

		_, err := ReadAtmosScaffoldSection(tempDir)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidScaffoldSection)
	})
}

func TestHasScaffoldConfig(t *testing.T) {
	tests := []struct {
		name  string
		files []types.File
		want  bool
	}{
		{
			name:  "no files",
			files: nil,
			want:  false,
		},
		{
			name: "files present but none match",
			files: []types.File{
				{Path: "README.md"},
				{Path: "main.tf"},
			},
			want: false,
		},
		{
			name: "scaffold.yaml present",
			files: []types.File{
				{Path: "README.md"},
				{Path: ScaffoldConfigFileName},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasScaffoldConfig(tt.files))
		})
	}
}

func TestHasUserConfig(t *testing.T) {
	t.Run("no record present", func(t *testing.T) {
		tempDir := t.TempDir()
		assert.False(t, HasUserConfig(tempDir))
	})

	t.Run("record present", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosDir := filepath.Join(tempDir, ScaffoldConfigDir)
		require.NoError(t, os.MkdirAll(atmosDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(atmosDir, ScaffoldConfigFileName), []byte("kind: AtmosScaffoldConfig\n"), 0o644))

		assert.True(t, HasUserConfig(tempDir))
	})
}
