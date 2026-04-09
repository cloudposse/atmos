package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewSTSClientWithCredentials_RegionResolution(t *testing.T) {
	testCases := []struct {
		name           string
		inputRegion    string
		baseRegion     string
		expectedRegion string
	}{
		{
			name:           "uses provided region",
			inputRegion:    "eu-west-1",
			baseRegion:     "us-east-1",
			expectedRegion: "eu-west-1",
		},
		{
			name:           "falls back to base region",
			inputRegion:    "",
			baseRegion:     "ap-south-1",
			expectedRegion: "ap-south-1",
		},
		{
			name:           "defaults to us-east-1 when both empty",
			inputRegion:    "",
			baseRegion:     "",
			expectedRegion: "us-east-1",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			baseCreds := &types.AWSCredentials{
				AccessKeyID:     "AKIAEXAMPLE",
				SecretAccessKey: "secret",
				SessionToken:    "token",
				Region:          tt.baseRegion,
			}

			client, resolvedRegion, err := NewSTSClientWithCredentials(
				context.Background(),
				baseCreds,
				tt.inputRegion,
				nil,
			)

			assert.NoError(t, err)
			assert.NotNil(t, client)
			assert.Equal(t, tt.expectedRegion, resolvedRegion)
		})
	}
}

func TestNewSTSClientWithCredentials_WithIdentityConfig(t *testing.T) {
	baseCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIAEXAMPLE",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Region:          "us-east-1",
	}

	identityConfig := &schema.Identity{
		Kind: "aws/assume-root",
		Principal: map[string]any{
			"target_principal": "123456789012",
			"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		},
	}

	client, resolvedRegion, err := NewSTSClientWithCredentials(
		context.Background(),
		baseCreds,
		"eu-central-1",
		identityConfig,
	)

	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "eu-central-1", resolvedRegion)
}

func TestNewSTSClientWithCredentials_WithCustomResolver(t *testing.T) {
	baseCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIAEXAMPLE",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Region:          "us-east-1",
	}

	// Config with custom resolver.
	identityConfig := &schema.Identity{
		Kind: "aws/assume-root",
		Principal: map[string]any{
			"target_principal": "123456789012",
			"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		},
		Credentials: map[string]any{
			"aws": map[string]any{
				"resolver": map[string]any{
					"url": "http://localhost:4566",
				},
			},
		},
	}

	client, resolvedRegion, err := NewSTSClientWithCredentials(
		context.Background(),
		baseCreds,
		"us-west-2",
		identityConfig,
	)

	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "us-west-2", resolvedRegion)
}

func TestNewSTSClientWithCredentials_NilIdentityConfig(t *testing.T) {
	baseCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIAEXAMPLE",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Region:          "us-east-1",
	}

	client, resolvedRegion, err := NewSTSClientWithCredentials(
		context.Background(),
		baseCreds,
		"ap-northeast-1",
		nil,
	)

	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "ap-northeast-1", resolvedRegion)
}
