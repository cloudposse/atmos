package cloudformation

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newClient must target the custom endpoint when one is configured — this is
// the behavior an emulator-backed identity (aws/emulator) depends on (see
// environment.go's resolveEndpointURL). We assert the real SDK client's
// resolved Options().BaseEndpoint rather than just checking newClient doesn't
// panic, per the no-coverage-theater rule.
func TestNewClient_SetsCustomEndpoint(t *testing.T) {
	client := newClient(aws.Config{Region: "us-east-1"}, "http://localhost:4566")

	real, ok := client.(*cloudformation.Client)
	require.True(t, ok, "newClient must return the real SDK client")

	opts := real.Options()
	require.NotNil(t, opts.BaseEndpoint, "BaseEndpoint must be set when an endpoint override is configured")
	assert.Equal(t, "http://localhost:4566", *opts.BaseEndpoint)
}

// Without an endpoint override, BaseEndpoint must be left nil so the client
// targets real AWS via the SDK's normal region-based endpoint resolution.
func TestNewClient_NoEndpointOverride(t *testing.T) {
	client := newClient(aws.Config{Region: "us-east-1"}, "")

	real, ok := client.(*cloudformation.Client)
	require.True(t, ok)

	opts := real.Options()
	assert.Nil(t, opts.BaseEndpoint, "BaseEndpoint must stay nil (real AWS) when no override is configured")
}
