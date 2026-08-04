package vendor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

const vendorConfigFixture = `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: ["components/terraform/vpc"]
`

// --- vendorConfigGetCmd -------------------------------------------------------

func TestVendorConfigGetCmd_RunE(t *testing.T) {
	resetCommandFlags(t, vendorConfigGetCmd)
	initVendorTestWriter(t)

	file := writeCommandVendorManifest(t, vendorConfigFixture)
	require.NoError(t, vendorConfigGetCmd.Flags().Set("file", file))

	require.NoError(t, vendorConfigGetCmd.RunE(vendorConfigGetCmd, []string{"spec.sources[0].version"}))
}

func TestVendorConfigGetCmd_MissingPath(t *testing.T) {
	resetCommandFlags(t, vendorConfigGetCmd)

	file := writeCommandVendorManifest(t, vendorConfigFixture)
	require.NoError(t, vendorConfigGetCmd.Flags().Set("file", file))

	err := vendorConfigGetCmd.RunE(vendorConfigGetCmd, []string{"spec.sources[0].nope"})
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
}

func TestVendorConfigGetCmd_MissingFile(t *testing.T) {
	resetCommandFlags(t, vendorConfigGetCmd)

	missing := filepath.Join(t.TempDir(), "missing.yaml")
	require.NoError(t, vendorConfigGetCmd.Flags().Set("file", missing))

	err := vendorConfigGetCmd.RunE(vendorConfigGetCmd, []string{"spec.sources[0].version"})
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrReadFile)
}

// --- vendorConfigSetCmd -------------------------------------------------------

func TestVendorConfigSetCmd_RunE(t *testing.T) {
	resetCommandFlags(t, vendorConfigSetCmd)

	file := writeCommandVendorManifest(t, vendorConfigFixture)
	require.NoError(t, vendorConfigSetCmd.Flags().Set("file", file))

	require.NoError(t, vendorConfigSetCmd.RunE(vendorConfigSetCmd, []string{"spec.sources[0].version", "v0.9.0"}))

	got, err := atmosyaml.GetFile(file, "spec.sources[0].version")
	require.NoError(t, err)
	assert.Equal(t, "v0.9.0", got)
}

func TestVendorConfigSetCmd_CreatesNewPath(t *testing.T) {
	resetCommandFlags(t, vendorConfigSetCmd)

	file := writeCommandVendorManifest(t, vendorConfigFixture)
	require.NoError(t, vendorConfigSetCmd.Flags().Set("file", file))

	// spec.sources[0].description does not exist yet -- exercises the "created" branch.
	require.NoError(t, vendorConfigSetCmd.RunE(vendorConfigSetCmd, []string{"spec.sources[0].description", "VPC module"}))

	got, err := atmosyaml.GetFile(file, "spec.sources[0].description")
	require.NoError(t, err)
	assert.Equal(t, "VPC module", got)
}

func TestVendorConfigSetCmd_MissingFile(t *testing.T) {
	resetCommandFlags(t, vendorConfigSetCmd)

	missing := filepath.Join(t.TempDir(), "missing.yaml")
	require.NoError(t, vendorConfigSetCmd.Flags().Set("file", missing))

	err := vendorConfigSetCmd.RunE(vendorConfigSetCmd, []string{"spec.sources[0].version", "v0.9.0"})
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrReadFile)
}

func TestVendorConfigSetCmd_InvalidTypeValue(t *testing.T) {
	resetCommandFlags(t, vendorConfigSetCmd)

	file := writeCommandVendorManifest(t, vendorConfigFixture)
	require.NoError(t, vendorConfigSetCmd.Flags().Set("file", file))
	require.NoError(t, vendorConfigSetCmd.Flags().Set("type", atmosyaml.TypeBool))

	err := vendorConfigSetCmd.RunE(vendorConfigSetCmd, []string{"spec.sources[0].version", "not-a-bool"})
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrInvalidYAMLExpression)
}

// --- vendorConfigDeleteCmd ----------------------------------------------------

func TestVendorConfigDeleteCmd_RunE(t *testing.T) {
	resetCommandFlags(t, vendorConfigDeleteCmd)

	file := writeCommandVendorManifest(t, vendorConfigFixture)
	require.NoError(t, vendorConfigDeleteCmd.Flags().Set("file", file))

	require.NoError(t, vendorConfigDeleteCmd.RunE(vendorConfigDeleteCmd, []string{"spec.sources[0].targets"}))

	_, err := atmosyaml.GetFile(file, "spec.sources[0].targets")
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
}

func TestVendorConfigDeleteCmd_NothingToDelete(t *testing.T) {
	resetCommandFlags(t, vendorConfigDeleteCmd)

	file := writeCommandVendorManifest(t, vendorConfigFixture)
	require.NoError(t, vendorConfigDeleteCmd.Flags().Set("file", file))

	before, err := os.ReadFile(file)
	require.NoError(t, err)

	require.NoError(t, vendorConfigDeleteCmd.RunE(vendorConfigDeleteCmd, []string{"spec.sources[0].does_not_exist"}))

	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "an absent-path delete must not touch the file")
}

func TestVendorConfigDeleteCmd_MissingFile(t *testing.T) {
	resetCommandFlags(t, vendorConfigDeleteCmd)

	missing := filepath.Join(t.TempDir(), "missing.yaml")
	require.NoError(t, vendorConfigDeleteCmd.Flags().Set("file", missing))

	err := vendorConfigDeleteCmd.RunE(vendorConfigDeleteCmd, []string{"spec.sources[0].targets"})
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrReadFile)
}

// --- vendorConfigFormatCmd ----------------------------------------------------

func TestVendorConfigFormatCmd_RunE(t *testing.T) {
	resetCommandFlags(t, vendorConfigFormatCmd)

	file := writeCommandVendorManifest(t, vendorConfigFixture)
	require.NoError(t, vendorConfigFormatCmd.Flags().Set("file", file))

	require.NoError(t, vendorConfigFormatCmd.RunE(vendorConfigFormatCmd, nil))

	got, err := atmosyaml.GetFile(file, "spec.sources[0].component")
	require.NoError(t, err)
	assert.Equal(t, "vpc", got)
}

func TestVendorConfigFormatCmd_MissingFile(t *testing.T) {
	resetCommandFlags(t, vendorConfigFormatCmd)

	missing := filepath.Join(t.TempDir(), "missing.yaml")
	require.NoError(t, vendorConfigFormatCmd.Flags().Set("file", missing))

	err := vendorConfigFormatCmd.RunE(vendorConfigFormatCmd, nil)
	require.Error(t, err)
}

// --- vendorConfigListCmd -------------------------------------------------------

func TestVendorConfigListCmd_Formats(t *testing.T) {
	tests := []struct {
		name      string
		format    string
		delimiter string
		contains  string
	}{
		{"json", "json", "", `"path"`},
		{"yaml", "yaml", "", "path:"},
		{"csv", "csv", "", "file,path,type,value"},
		{"tsv", "tsv", "", "file\tpath\ttype\tvalue"},
		{"paths", "paths", "", "vendor.yaml"},
		{"csv custom delimiter", "csv", ";", "file;path;type;value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetCommandFlags(t, vendorConfigListCmd)
			initVendorTestWriter(t)

			file := writeCommandVendorManifest(t, vendorConfigFixture)
			require.NoError(t, vendorConfigListCmd.Flags().Set("file", file))
			require.NoError(t, vendorConfigListCmd.Flags().Set("format", tt.format))
			if tt.delimiter != "" {
				require.NoError(t, vendorConfigListCmd.Flags().Set("delimiter", tt.delimiter))
			}

			require.NoError(t, vendorConfigListCmd.RunE(vendorConfigListCmd, nil))
		})
	}
}

func TestVendorConfigListCmd_MissingFile(t *testing.T) {
	resetCommandFlags(t, vendorConfigListCmd)

	missing := filepath.Join(t.TempDir(), "missing.yaml")
	require.NoError(t, vendorConfigListCmd.Flags().Set("file", missing))

	err := vendorConfigListCmd.RunE(vendorConfigListCmd, nil)
	require.Error(t, err)
}

// --- independent per-subcommand flags ------------------------------------------

// TestVendorConfigSubcommands_FlagsIndependent proves each vendor config subcommand's flags
// (--file, --type, --format, --delimiter) are backed by fully independent per-command
// StandardParsers, not the shared package-level vars this migrated from: setting one
// subcommand's flag must not leak into another subcommand's default value, and a flag that only
// exists on one subcommand must not be registered on the others at all.
func TestVendorConfigSubcommands_FlagsIndependent(t *testing.T) {
	resetCommandFlags(t, vendorConfigGetCmd)
	resetCommandFlags(t, vendorConfigSetCmd)
	resetCommandFlags(t, vendorConfigDeleteCmd)
	resetCommandFlags(t, vendorConfigFormatCmd)
	resetCommandFlags(t, vendorConfigListCmd)

	require.NoError(t, vendorConfigGetCmd.Flags().Set("file", "get-only.yaml"))
	require.NoError(t, vendorConfigSetCmd.Flags().Set("file", "set-only.yaml"))
	require.NoError(t, vendorConfigSetCmd.Flags().Set("type", atmosyaml.TypeInt))
	require.NoError(t, vendorConfigListCmd.Flags().Set("format", "json"))
	require.NoError(t, vendorConfigListCmd.Flags().Set("delimiter", ";"))

	// Each command's own flag holds exactly the value set on it.
	assert.Equal(t, "get-only.yaml", vendorConfigGetCmd.Flags().Lookup("file").Value.String())
	assert.Equal(t, "set-only.yaml", vendorConfigSetCmd.Flags().Lookup("file").Value.String())
	assert.Equal(t, atmosyaml.TypeInt, vendorConfigSetCmd.Flags().Lookup("type").Value.String())
	assert.Equal(t, "json", vendorConfigListCmd.Flags().Lookup("format").Value.String())
	assert.Equal(t, ";", vendorConfigListCmd.Flags().Lookup("delimiter").Value.String())

	// Unrelated commands' --file remain at their own untouched defaults.
	assert.Equal(t, "", vendorConfigDeleteCmd.Flags().Lookup("file").Value.String(), "delete's --file must not pick up get's or set's value")
	assert.Equal(t, "", vendorConfigFormatCmd.Flags().Lookup("file").Value.String(), "format's --file must not pick up get's or set's value")
	assert.Equal(t, "", vendorConfigListCmd.Flags().Lookup("file").Value.String(), "list's --file must not pick up get's or set's value")

	// Flags that only exist on one subcommand must not be registered on the others at all.
	assert.Nil(t, vendorConfigGetCmd.Flags().Lookup("type"), "get must not register set's --type flag")
	assert.Nil(t, vendorConfigDeleteCmd.Flags().Lookup("format"), "delete must not register list's --format flag")
	assert.Nil(t, vendorConfigFormatCmd.Flags().Lookup("delimiter"), "format must not register list's --delimiter flag")
}

// --- buildVendorConfigPathRows error paths ------------------------------------

func TestBuildVendorConfigPathRows_BadRootFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")

	_, err := buildVendorConfigPathRows(missing)
	require.Error(t, err)
}

func TestBuildVendorConfigPathRows_UnreadableCollectedFile(t *testing.T) {
	dir := t.TempDir()
	rootFile := filepath.Join(dir, DefaultVendorManifest)
	importFile := filepath.Join(dir, "imports", "common.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(importFile), 0o755))
	require.NoError(t, os.WriteFile(rootFile, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  imports:
    - imports/common.yaml
  sources:
    - component: vpc
      version: v0.1.0
`), 0o644))
	require.NoError(t, os.WriteFile(importFile, []byte(`spec:
  sources:
    - component: eks
      version: v0.2.0
`), 0o644))

	// Remove the imported file after CollectManifestFiles would have listed it,
	// so the read in buildVendorConfigPathRows itself fails.
	require.NoError(t, os.Remove(importFile))

	_, err := buildVendorConfigPathRows(rootFile)
	require.Error(t, err)
}

func TestBuildVendorConfigPathRows_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	rootFile := filepath.Join(dir, DefaultVendorManifest)
	importFile := filepath.Join(dir, "imports", "common.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(importFile), 0o755))
	require.NoError(t, os.WriteFile(rootFile, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  imports:
    - imports/common.yaml
  sources:
    - component: vpc
      version: v0.1.0
`), 0o644))
	// Malformed YAML (unbalanced flow mapping) in the collected import file.
	require.NoError(t, os.WriteFile(importFile, []byte("spec: {sources: [component: eks\n"), 0o644))

	_, err := buildVendorConfigPathRows(rootFile)
	require.Error(t, err)
}

// --- formatVendorConfigFiles error path ---------------------------------------

func TestFormatVendorConfigFiles_BadRootFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")

	_, err := formatVendorConfigFiles(missing)
	require.Error(t, err)
}

// --- relativeVendorPathForDisplay ---------------------------------------------

func TestRelativeVendorPathForDisplay(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		basePath string
		want     string
	}{
		{
			name:     "normal relative",
			file:     filepath.Join("root", "imports", "common.yaml"),
			basePath: "root",
			want:     "imports/common.yaml",
		},
		{
			name:     "rel is dot falls back to file path",
			file:     "root",
			basePath: "root",
			want:     filepath.ToSlash("root"),
		},
		{
			name:     "rel is dotdot falls back to file path",
			file:     filepath.Dir("root"),
			basePath: "root",
			want:     filepath.ToSlash(filepath.Dir("root")),
		},
		{
			name:     "rel has dotdot prefix falls back to file path",
			file:     filepath.Join("other", "vendor.yaml"),
			basePath: filepath.Join("root", "nested"),
			want:     filepath.ToSlash(filepath.Join("other", "vendor.yaml")),
		},
		{
			name:     "absolute fallback when no relative path exists across volumes",
			file:     "/abs/vendor.yaml",
			basePath: "relative/base",
			want:     filepath.ToSlash("/abs/vendor.yaml"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := relativeVendorPathForDisplay(tt.file, tt.basePath)
			assert.Equal(t, tt.want, got)
		})
	}
}
