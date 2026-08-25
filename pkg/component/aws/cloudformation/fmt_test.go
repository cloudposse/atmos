package cloudformation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// formatTemplate must round-trip a valid YAML template, preserving comments
// and CFN's short-form intrinsic function tags (Ref, Sub, GetAtt) — plain
// yaml.v3 rather than the Atmos custom-tag-aware unmarshaler is what makes
// those tags survive instead of being rejected as unknown.
func TestFormatTemplate_ValidYAML_PreservesCommentsAndIntrinsicTags(t *testing.T) {
	body := "# top-level comment\n" +
		"AWSTemplateFormatVersion: '2010-09-09'\n" +
		"Resources:\n" +
		"  Bucket:\n" +
		"    Type: AWS::S3::Bucket\n" +
		"    Properties:\n" +
		"      BucketName: !Sub '${AWS::StackName}-bucket'\n" +
		"Outputs:\n" +
		"  BucketArn:\n" +
		"    Value: !GetAtt Bucket.Arn\n" +
		"  BucketRef:\n" +
		"    Value: !Ref Bucket\n"

	got, err := formatTemplate(body)
	require.NoError(t, err)
	assert.Contains(t, got, "# top-level comment")
	assert.Contains(t, got, "!Sub")
	assert.Contains(t, got, "!GetAtt")
	assert.Contains(t, got, "!Ref")

	// Re-formatting the already-formatted output must be a no-op (idempotent).
	again, err := formatTemplate(got)
	require.NoError(t, err)
	assert.Equal(t, got, again)
}

// formatTemplate must reject malformed YAML with
// ErrInvalidAwsCloudFormationSettings, not a bare yaml.v3 error.
func TestFormatTemplate_MalformedYAML(t *testing.T) {
	_, err := formatTemplate("Resources:\n  Bucket: [unterminated\n")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

// runFmt on an already-formatted file must be a no-op: no error, and
// summary["formatted"] reports false.
func TestRunFmt_CleanNoCheck_NoOp(t *testing.T) {
	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "template.yaml")
	body := "AWSTemplateFormatVersion: \"2010-09-09\"\n"
	require.NoError(t, os.WriteFile(templatePath, []byte(body), 0o644))

	// The formatted output of body must equal body itself for this to
	// actually exercise the "clean" branch instead of accidentally the dirty one.
	formatted, err := formatTemplate(body)
	require.NoError(t, err)
	require.Equal(t, body, formatted, "test fixture must already be in formatTemplate's canonical form")

	spec := &stackSpec{TemplateBody: body, TemplateAbsPath: templatePath}
	summary, err := runFmt(spec, map[string]any{}, map[string]any{})
	require.NoError(t, err)
	assert.False(t, summary["formatted"].(bool))

	// The file on disk must be untouched.
	onDisk, readErr := os.ReadFile(templatePath)
	require.NoError(t, readErr)
	assert.Equal(t, body, string(onDisk))
}

// runFmt with --check on a dirty (not-yet-formatted) file must return
// ErrAwsCloudFormationFmtNotClean without writing anything to disk.
func TestRunFmt_Check_Dirty_ReturnsErrorWithoutWriting(t *testing.T) {
	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "template.yaml")
	dirty := "AWSTemplateFormatVersion:   '2010-09-09'\nResources: {}\n"
	require.NoError(t, os.WriteFile(templatePath, []byte(dirty), 0o644))

	spec := &stackSpec{TemplateBody: dirty, TemplateAbsPath: templatePath}
	_, err := runFmt(spec, map[string]any{"check": true}, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationFmtNotClean)

	onDisk, readErr := os.ReadFile(templatePath)
	require.NoError(t, readErr)
	assert.Equal(t, dirty, string(onDisk), "--check must never write changes to disk")
}

// runFmt with --check on an already-clean file must return nil.
func TestRunFmt_Check_Clean_ReturnsNil(t *testing.T) {
	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "template.yaml")
	body := "AWSTemplateFormatVersion: \"2010-09-09\"\n"
	formatted, err := formatTemplate(body)
	require.NoError(t, err)
	require.Equal(t, body, formatted)
	require.NoError(t, os.WriteFile(templatePath, []byte(body), 0o644))

	spec := &stackSpec{TemplateBody: body, TemplateAbsPath: templatePath}
	summary, err := runFmt(spec, map[string]any{"check": true}, map[string]any{})
	require.NoError(t, err)
	assert.False(t, summary["formatted"].(bool))
}

// runFmt in non-check mode on a dirty file must actually write the formatted
// content to disk.
func TestRunFmt_NonCheck_Dirty_WritesFormattedContent(t *testing.T) {
	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "template.yaml")
	dirty := "AWSTemplateFormatVersion:   '2010-09-09'\nResources: {}\n"
	require.NoError(t, os.WriteFile(templatePath, []byte(dirty), 0o644))

	spec := &stackSpec{TemplateBody: dirty, TemplateAbsPath: templatePath}
	summary, err := runFmt(spec, map[string]any{}, map[string]any{})
	require.NoError(t, err)
	assert.True(t, summary["formatted"].(bool))

	wantFormatted, err := formatTemplate(dirty)
	require.NoError(t, err)

	onDisk, readErr := os.ReadFile(templatePath)
	require.NoError(t, readErr)
	assert.Equal(t, wantFormatted, string(onDisk))
	assert.NotEqual(t, dirty, string(onDisk), "the write must have actually changed the file's contents")
}

// runFmt must propagate a formatTemplate error (malformed template body)
// without touching disk.
func TestRunFmt_FormatTemplateError(t *testing.T) {
	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "template.yaml")
	malformed := "Resources:\n  Bucket: [unterminated\n"
	require.NoError(t, os.WriteFile(templatePath, []byte(malformed), 0o644))

	spec := &stackSpec{TemplateBody: malformed, TemplateAbsPath: templatePath}
	_, err := runFmt(spec, map[string]any{}, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)

	onDisk, readErr := os.ReadFile(templatePath)
	require.NoError(t, readErr)
	assert.Equal(t, malformed, string(onDisk), "a formatTemplate failure must never write to disk")
}

// runFmt must propagate a write failure (e.g. the resolved path's parent
// directory does not exist) wrapped in ErrAwsCloudFormationFmtNotClean.
func TestRunFmt_WriteFailure(t *testing.T) {
	tempDir := t.TempDir()
	// A path whose parent directory does not exist: os.WriteFile fails the
	// same deterministic way on both Unix and Windows, unlike a
	// permissions-based approach.
	unwritablePath := filepath.Join(tempDir, "missing-subdir", "template.yaml")
	dirty := "AWSTemplateFormatVersion:   '2010-09-09'\n"

	spec := &stackSpec{TemplateBody: dirty, TemplateAbsPath: unwritablePath}
	_, err := runFmt(spec, map[string]any{}, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationFmtNotClean)
}
