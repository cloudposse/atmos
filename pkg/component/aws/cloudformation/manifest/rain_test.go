package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const rainTemplate = `
Rain:
  Constants:
    AppName: acme
Resources:
  Marker:
    Properties:
      Name: !Sub "${Rain::AppName}/${Stage}"
      Value: !Rain::Env DEPLOY_OWNER
  Bucket:
    Properties:
      Tags: !Rain::Include fragments/tags.yaml
  Function:
    Properties:
      Code:
        ZipFile: !Rain::Embed ../src/handler.py
  Worker:
    Properties:
      Code: !Rain::S3
        Path: ../dist/worker
`

func TestDetectRainDirectives(t *testing.T) {
	assert.Equal(t, []string{"Constant", "Env", "Include", "Embed", "S3"}, DetectRainDirectives(rainTemplate))
	assert.Empty(t, DetectRainDirectives("Resources:\n  A:\n    Type: AWS::S3::Bucket\n"))
	assert.Empty(t, DetectRainDirectives("Description: it rains a lot\nMetadata: {Rain: no}\n"), "the word Rain alone is not a directive")
	assert.Equal(t, []string{"Constant"}, DetectRainDirectives(`x: "${Rain::Thing}"`))
}

func TestRainDirectiveHint(t *testing.T) {
	assert.Nil(t, RainDirectiveHint("!env"))
	assert.Nil(t, RainDirectiveHint("!Ref"))

	hints := RainDirectiveHint("!Rain::Env")
	require.Len(t, hints, 2)
	assert.Contains(t, hints[0], "Replace !Rain::Env with `!env NAME`")
	assert.Contains(t, hints[1], "https://atmos.tools/migration/rain")

	unknown := RainDirectiveHint("!Rain::Teleport")
	assert.Contains(t, unknown[0], "Replace !Rain::Teleport with not a directive Rain documented")
}

func TestRainDirectiveError(t *testing.T) {
	err := RainDirectiveError("components/cloudformation/app/template.yaml", []string{"Env", "S3"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationRainDirective)
	template, ok := errUtils.GetContext(err, "template")
	require.True(t, ok)
	assert.Equal(t, "components/cloudformation/app/template.yaml", template)
	directives, ok := errUtils.GetContext(err, "directives")
	require.True(t, ok)
	assert.Equal(t, "Env,S3", directives)
	assert.True(t, errUtils.HasHint(err, "Replace !Rain::Env with `!env NAME`"))
	assert.True(t, errUtils.HasHint(err, "Replace !Rain::S3 with an `archive` + `publish` hook step"))
	assert.True(t, errUtils.HasHint(err, "https://atmos.tools/migration/rain"))
}

func TestRainDirective_InsideIncludedOrInlineTemplateIsRejectedWithHint(t *testing.T) {
	_, err := u.UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{}, "template:\n  Value: !Rain::Env OWNER\n", "stack.yaml")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
	assert.True(t, errUtils.HasHint(err, "Replace !Rain::Env with `!env NAME`"))
	assert.NotContains(t, err.Error(), "Supported tags are")
}
