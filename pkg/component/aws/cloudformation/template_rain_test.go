package cloudformation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestLoadTemplateBody_RejectsRainDirectives(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "template.yaml"), []byte("Resources:\n  M:\n    Properties:\n      Value: !Rain::Env OWNER\n      Name: !Sub \"${Rain::Prefix}/x\"\n"), 0o644))

	_, err := loadTemplateBody(dir, &stackSpec{TemplatePath: "template.yaml"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationRainDirective)
	directives, ok := errUtils.GetContext(err, "directives")
	require.True(t, ok)
	assert.Equal(t, "Env,Constant", directives, "in order of first appearance")
	template, ok := errUtils.GetContext(err, "template")
	require.True(t, ok)
	assert.Equal(t, filepath.Join(dir, "template.yaml"), template)
}

func TestLoadTemplateBody_PlainTemplatePasses(t *testing.T) {
	dir := t.TempDir()
	body := "Resources:\n  B:\n    Type: AWS::S3::Bucket\n    Properties:\n      BucketName: !Sub \"${AppName}-assets\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "template.yaml"), []byte(body), 0o644))

	got, err := loadTemplateBody(dir, &stackSpec{TemplatePath: "template.yaml"})
	require.NoError(t, err)
	assert.Equal(t, body, got, "a path: template is still passed through verbatim")
}
