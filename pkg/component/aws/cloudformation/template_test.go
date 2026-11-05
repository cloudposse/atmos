package cloudformation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// resolveComponentPath must combine the configured aws/cloudformation base
// path with the component's folder prefix and name, resolving to an absolute
// path — the JIT-provisioning and template-loading steps downstream both
// depend on this being deterministic and absolute.
func TestResolveComponentPath(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{CloudFormationDirAbsolutePath: tempDir}
	info := &schema.ConfigAndStacksInfo{FinalComponent: "vpc"}

	path, err := resolveComponentPath(atmosConfig, info)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDir, "vpc"), path)
}

// resolveComponentPath must fold in a non-empty component folder prefix
// (e.g. a component nested under a category directory).
func TestResolveComponentPath_WithFolderPrefix(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{CloudFormationDirAbsolutePath: tempDir}
	info := &schema.ConfigAndStacksInfo{ComponentFolderPrefix: "networking", FinalComponent: "vpc"}

	path, err := resolveComponentPath(atmosConfig, info)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDir, "networking", "vpc"), path)
}

// loadTemplateBody must read the template file relative to componentPath and
// return its raw contents.
func TestLoadTemplateBody_RelativePath(t *testing.T) {
	tempDir := t.TempDir()
	body := "AWSTemplateFormatVersion: '2010-09-09'\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "template.yaml"), []byte(body), 0o644))

	spec := &stackSpec{TemplatePath: "template.yaml"}
	got, err := loadTemplateBody(tempDir, spec)
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

// loadTemplateBody must honor an already-absolute TemplatePath without
// joining it to componentPath.
func TestLoadTemplateBody_AbsolutePath(t *testing.T) {
	tempDir := t.TempDir()
	body := "AWSTemplateFormatVersion: '2010-09-09'\n"
	absPath := filepath.Join(tempDir, "abs-template.yaml")
	require.NoError(t, os.WriteFile(absPath, []byte(body), 0o644))

	spec := &stackSpec{TemplatePath: absPath}
	got, err := loadTemplateBody(filepath.Join(tempDir, "unrelated"), spec)
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

// loadTemplateBody must wrap a missing template file with
// ErrMissingAwsCloudFormationTemplate and the resolved path, so the error
// message tells the user exactly which file could not be found.
func TestLoadTemplateBody_MissingFile(t *testing.T) {
	tempDir := t.TempDir()
	spec := &stackSpec{TemplatePath: "does-not-exist.yaml"}

	_, err := loadTemplateBody(tempDir, spec)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMissingAwsCloudFormationTemplate)
	assert.Contains(t, err.Error(), "does-not-exist.yaml")
}

// loadStackPolicyBody returns an empty string (no error) when no
// stack_policy.file is configured — the common case.
func TestLoadStackPolicyBody_NotConfigured(t *testing.T) {
	body, err := loadStackPolicyBody(t.TempDir(), &stackSpec{})
	require.NoError(t, err)
	assert.Empty(t, body)
}

// loadStackPolicyBody must read the configured file relative to componentPath.
func TestLoadStackPolicyBody_RelativePath(t *testing.T) {
	tempDir := t.TempDir()
	body := `{"Statement": [{"Effect": "Allow", "Action": "Update:*", "Principal": "*", "Resource": "*"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "policy.json"), []byte(body), 0o644))

	spec := &stackSpec{StackPolicyFile: "policy.json"}
	got, err := loadStackPolicyBody(tempDir, spec)
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

// loadStackPolicyBody must return an error (not silently empty) when the
// configured file is missing.
func TestLoadStackPolicyBody_MissingFile(t *testing.T) {
	tempDir := t.TempDir()
	spec := &stackSpec{StackPolicyFile: "missing-policy.json"}

	_, err := loadStackPolicyBody(tempDir, spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing-policy.json")
}
