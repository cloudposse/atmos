package cloudformation

import (
	"testing"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

const templateWithNoEcho = `
AWSTemplateFormatVersion: '2010-09-09'
Parameters:
  DbPassword:
    Type: String
    NoEcho: true
  Environment:
    Type: String
    NoEcho: false
  Name:
    Type: String
`

func TestNoEchoParameterNames(t *testing.T) {
	names := noEchoParameterNames(templateWithNoEcho)
	assert.True(t, names["DbPassword"])
	assert.False(t, names["Environment"])
	assert.False(t, names["Name"])
}

func TestNoEchoParameterNames_UnparsableTemplate(t *testing.T) {
	// Best-effort: an unparsable template must not panic or error the caller.
	names := noEchoParameterNames("{{{ not valid yaml or json")
	assert.Empty(t, names)
}

func TestIsTruthy(t *testing.T) {
	assert.True(t, isTruthy(true))
	assert.True(t, isTruthy("true"))
	assert.True(t, isTruthy("True"))
	assert.False(t, isTruthy(false))
	assert.False(t, isTruthy("false"))
	assert.False(t, isTruthy(nil))
	assert.False(t, isTruthy(123))
}

func TestRegisterNoEchoValues_OnlyRegistersNoEchoParams(t *testing.T) {
	t.Cleanup(iolib.Reset)

	spec := &stackSpec{
		Parameters: []cfntypes.Parameter{
			{ParameterKey: awsString("DbPassword"), ParameterValue: awsString("s3cr3t-value")},
			{ParameterKey: awsString("Environment"), ParameterValue: awsString("dev")},
		},
	}

	registerNoEchoValues(templateWithNoEcho, spec)

	assert.True(t, iolib.ContainsSecret("s3cr3t-value"), "NoEcho parameter value should be registered with the masker")
	assert.False(t, iolib.ContainsSecret("dev"), "non-NoEcho parameter value should not be registered")
}

func TestRegisterNoEchoValues_NoNoEchoParams(t *testing.T) {
	t.Cleanup(iolib.Reset)

	spec := &stackSpec{
		Parameters: []cfntypes.Parameter{
			{ParameterKey: awsString("Environment"), ParameterValue: awsString("dev-only-value")},
		},
	}

	registerNoEchoValues("Parameters: {}", spec)
	assert.False(t, iolib.ContainsSecret("dev-only-value"))
}
