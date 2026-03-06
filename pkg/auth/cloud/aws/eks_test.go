package aws

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// mockEKSClient implements EKSClient for testing.
type mockEKSClient struct {
	describeClusterFn func(ctx context.Context, input *eks.DescribeClusterInput, opts ...func(*eks.Options)) (*eks.DescribeClusterOutput, error)
}

func (m *mockEKSClient) DescribeCluster(ctx context.Context, input *eks.DescribeClusterInput, opts ...func(*eks.Options)) (*eks.DescribeClusterOutput, error) {
	return m.describeClusterFn(ctx, input, opts...)
}

func TestDescribeCluster_Success(t *testing.T) {
	client := &mockEKSClient{
		describeClusterFn: func(_ context.Context, input *eks.DescribeClusterInput, _ ...func(*eks.Options)) (*eks.DescribeClusterOutput, error) {
			return &eks.DescribeClusterOutput{
				Cluster: &ekstypes.Cluster{
					Name:     input.Name,
					Endpoint: aws.String("https://XXXX.gr7.us-east-2.eks.amazonaws.com"),
					CertificateAuthority: &ekstypes.Certificate{
						Data: aws.String("LS0tLS1CRUdJTi..."),
					},
					Arn: aws.String("arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"),
				},
			}, nil
		},
	}

	info, err := DescribeCluster(context.Background(), client, "dev-cluster", "us-east-2")
	require.NoError(t, err)
	assert.Equal(t, "dev-cluster", info.Name)
	assert.Equal(t, "https://XXXX.gr7.us-east-2.eks.amazonaws.com", info.Endpoint)
	assert.Equal(t, "LS0tLS1CRUdJTi...", info.CertificateAuthorityData)
	assert.Equal(t, "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster", info.ARN)
	assert.Equal(t, "us-east-2", info.Region)
}

func TestDescribeCluster_NilCluster(t *testing.T) {
	client := &mockEKSClient{
		describeClusterFn: func(_ context.Context, _ *eks.DescribeClusterInput, _ ...func(*eks.Options)) (*eks.DescribeClusterOutput, error) {
			return &eks.DescribeClusterOutput{
				Cluster: nil,
			}, nil
		},
	}

	_, err := DescribeCluster(context.Background(), client, "missing-cluster", "us-east-2")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSClusterNotFound)
}

func TestDescribeCluster_APIError(t *testing.T) {
	client := &mockEKSClient{
		describeClusterFn: func(_ context.Context, _ *eks.DescribeClusterInput, _ ...func(*eks.Options)) (*eks.DescribeClusterOutput, error) {
			return nil, assert.AnError
		},
	}

	_, err := DescribeCluster(context.Background(), client, "dev-cluster", "us-east-2")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSDescribeCluster)
}

func TestDescribeCluster_NilCertificateAuthority(t *testing.T) {
	client := &mockEKSClient{
		describeClusterFn: func(_ context.Context, input *eks.DescribeClusterInput, _ ...func(*eks.Options)) (*eks.DescribeClusterOutput, error) {
			return &eks.DescribeClusterOutput{
				Cluster: &ekstypes.Cluster{
					Name:                 input.Name,
					Endpoint:             aws.String("https://example.eks.amazonaws.com"),
					CertificateAuthority: nil,
					Arn:                  aws.String("arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"),
				},
			}, nil
		},
	}

	info, err := DescribeCluster(context.Background(), client, "dev-cluster", "us-east-2")
	require.NoError(t, err)
	assert.Equal(t, "", info.CertificateAuthorityData)
}

func TestGetToken_InvalidCredentials(t *testing.T) {
	// Non-AWS credentials should fail.
	_, _, err := GetToken(context.Background(), nil, "dev-cluster", "us-east-2")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSTokenGeneration)
}

func TestGetToken_Success(t *testing.T) {
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBYaDH...",
		Region:          "us-east-2",
	}

	token, expiresAt, err := GetToken(context.Background(), creds, "dev-cluster", "us-east-2")
	require.NoError(t, err)

	// Token should start with k8s-aws-v1. prefix.
	assert.True(t, strings.HasPrefix(token, eksTokenPrefix), "token should start with %s", eksTokenPrefix)

	// Token should not be empty after prefix.
	assert.Greater(t, len(token), len(eksTokenPrefix))

	// Expiration should be in the future.
	assert.True(t, expiresAt.After(time.Now()))
}

func TestNewEKSClient_InvalidCredentials(t *testing.T) {
	// Non-AWS credentials should fail.
	_, err := NewEKSClient(context.Background(), nil, "us-east-2")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEKSDescribeCluster)
}
