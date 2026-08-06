package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/manifest"
)

// templateForRecordTests returns a template manifest used as the source of a
// project record.
func templateForRecordTests() *ScaffoldConfig {
	return &ScaffoldConfig{
		APIVersion: manifest.DefaultAPIVersion,
		Kind:       ScaffoldKind,
		Metadata: manifest.Metadata{
			Name:    "simple",
			Version: "1.0.0",
		},
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "project_name", Type: "input", Default: "my-project"},
				{Name: "aws_region", Type: "input", Default: "us-east-1"},
			},
		},
	}
}

func TestSaveAndLoadProjectRecordWithBaseRef(t *testing.T) {
	tmpDir := t.TempDir()

	values := map[string]interface{}{
		"project_name": "test-project",
		"aws_region":   "us-east-1",
	}

	err := SaveProjectRecord(tmpDir, templateForRecordTests(), SourceEmbedded, "main", values)
	require.NoError(t, err)

	// Verify file was created.
	recordPath := filepath.Join(tmpDir, ScaffoldConfigDir, ScaffoldConfigFileName)
	assert.FileExists(t, recordPath)

	// Load record back.
	record, err := LoadProjectRecord(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, record)

	// The record is a full AtmosScaffoldConfig manifest.
	assert.Equal(t, manifest.DefaultAPIVersion, record.APIVersion)
	assert.Equal(t, ScaffoldKind, record.Kind)
	assert.Equal(t, "simple", record.Metadata.Name)
	assert.Equal(t, "1.0.0", record.Metadata.Version)
	assert.Equal(t, SourceEmbedded, record.Spec.Source)
	assert.Equal(t, "main", record.Spec.BaseRef)
	assert.Equal(t, "test-project", record.Spec.Values["project_name"])
	assert.Equal(t, "us-east-1", record.Spec.Values["aws_region"])

	// The questionnaire snapshot travels with the record.
	require.Len(t, record.Spec.Fields, 2)
	assert.Equal(t, "project_name", record.Spec.Fields[0].Name)
	assert.Equal(t, "aws_region", record.Spec.Fields[1].Name)
}

func TestSaveProjectRecord_EmptyBaseRef(t *testing.T) {
	tmpDir := t.TempDir()

	values := map[string]interface{}{
		"project_name": "test-project",
	}

	err := SaveProjectRecord(tmpDir, templateForRecordTests(), "", "", values)
	require.NoError(t, err)

	record, err := LoadProjectRecord(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, record)

	// Empty source and baseRef are omitted from the record.
	assert.Empty(t, record.Spec.Source)
	assert.Empty(t, record.Spec.BaseRef)
	assert.Equal(t, "test-project", record.Spec.Values["project_name"])

	raw, err := os.ReadFile(filepath.Join(tmpDir, ScaffoldConfigDir, ScaffoldConfigFileName))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "baseRef")
}

func TestSaveProjectRecord_PreservesValueKeyCasing(t *testing.T) {
	// The record is marshaled directly to YAML (never through viper), so
	// mixed-case value keys must survive a save/load round-trip.
	tmpDir := t.TempDir()

	values := map[string]interface{}{
		"projectName": "test-project",
		"awsRegion":   "us-east-1",
	}

	err := SaveProjectRecord(tmpDir, templateForRecordTests(), "", "", values)
	require.NoError(t, err)

	loaded, err := LoadUserValues(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "test-project", loaded["projectName"])
	assert.Equal(t, "us-east-1", loaded["awsRegion"])

	raw, err := os.ReadFile(filepath.Join(tmpDir, ScaffoldConfigDir, ScaffoldConfigFileName))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "projectName")
	assert.NotContains(t, string(raw), "projectname")
}

func TestLoadProjectRecord_NonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()

	record, err := LoadProjectRecord(tmpDir)
	require.NoError(t, err)
	assert.Nil(t, record) // Should return nil when file doesn't exist.
}

func TestSaveProjectRecord_NilTemplateConfig(t *testing.T) {
	// SaveProjectRecord rejects nil templateConfig immediately so that callers
	// cannot write a record that LoadProjectRecord would subsequently refuse to
	// reload (a nil config produces an empty metadata.name which fails schema
	// validation on load, leaving the project permanently broken).
	tmpDir := t.TempDir()

	err := SaveProjectRecord(tmpDir, nil, "", "", map[string]interface{}{"k": "v"})
	require.Error(t, err, "nil templateConfig must be rejected before writing")
}
