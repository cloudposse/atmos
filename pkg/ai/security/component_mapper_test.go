package security

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// mockTaggingClient implements TaggingAPI for testing.
type mockTaggingClient struct {
	resources []tagtypes.ResourceTagMapping
	err       error
}

func (m *mockTaggingClient) GetResources(_ context.Context, _ *resourcegroupstaggingapi.GetResourcesInput, _ ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &resourcegroupstaggingapi.GetResourcesOutput{
		ResourceTagMappingList: m.resources,
	}, nil
}

func TestMapByTags_ExactMatch(t *testing.T) {
	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAISecurityTagMapping(),
		clients:     newAWSClientCache(),
		tagCache: map[string]*tagLookupResult{
			"arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0": {
				tags: map[string]string{
					"atmos:stack":     "tenant1-ue1-prod",
					"atmos:component": "vpc",
				},
				exists: true,
			},
		},
	}

	finding := &Finding{
		ResourceARN: "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0",
		Region:      "us-east-1",
	}

	mapping, err := mapper.mapByTags(context.Background(), finding)
	require.NoError(t, err)
	require.NotNil(t, mapping)
	assert.True(t, mapping.Mapped)
	assert.Equal(t, "tenant1-ue1-prod", mapping.Stack)
	assert.Equal(t, "vpc", mapping.Component)
	assert.Equal(t, ConfidenceExact, mapping.Confidence)
	assert.Equal(t, "tag", mapping.Method)
}

func TestMapByTags_IndividualTags(t *testing.T) {
	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAISecurityTagMapping(),
		clients:     newAWSClientCache(),
		tagCache: map[string]*tagLookupResult{
			"arn:aws:s3:::my-bucket": {
				tags: map[string]string{
					"atmos:tenant":      "acme",
					"atmos:environment": "ue1",
					"atmos:stage":       "prod",
					"atmos:component":   "s3-bucket",
				},
				exists: true,
			},
		},
	}

	finding := &Finding{
		ResourceARN: "arn:aws:s3:::my-bucket",
		Region:      "us-east-1",
	}

	mapping, err := mapper.mapByTags(context.Background(), finding)
	require.NoError(t, err)
	require.NotNil(t, mapping)
	assert.True(t, mapping.Mapped)
	assert.Equal(t, "acme-ue1-prod", mapping.Stack)
	assert.Equal(t, "s3-bucket", mapping.Component)
	assert.Equal(t, ConfidenceExact, mapping.Confidence)
}

func TestMapByTags_NoTags(t *testing.T) {
	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAISecurityTagMapping(),
		clients:     newAWSClientCache(),
		tagCache: map[string]*tagLookupResult{
			"arn:aws:ec2:us-east-1:123456789012:instance/i-abc": {
				tags:   map[string]string{"Name": "my-instance"},
				exists: true,
			},
		},
	}

	finding := &Finding{
		ResourceARN: "arn:aws:ec2:us-east-1:123456789012:instance/i-abc",
		Region:      "us-east-1",
	}

	mapping, err := mapper.mapByTags(context.Background(), finding)
	require.NoError(t, err)
	assert.Nil(t, mapping)
}

func TestMapByNamingConvention(t *testing.T) {
	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAISecurityTagMapping(),
	}

	tests := []struct {
		name          string
		arn           string
		wantComponent string
		wantStack     string
		wantMapped    bool
	}{
		{
			name:          "standard naming convention",
			arn:           "arn:aws:ec2:us-east-1:123456789012:instance/acme-ue1-prod-vpc",
			wantComponent: "vpc",
			wantStack:     "ue1-prod",
			wantMapped:    true,
		},
		{
			name:          "short name",
			arn:           "arn:aws:s3:::ab",
			wantComponent: "",
			wantStack:     "",
			wantMapped:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finding := &Finding{ResourceARN: tt.arn}
			mapping := mapper.mapByNamingConvention(finding)
			if !tt.wantMapped {
				assert.Nil(t, mapping)
				return
			}
			require.NotNil(t, mapping)
			assert.Equal(t, tt.wantComponent, mapping.Component)
			assert.Equal(t, tt.wantStack, mapping.Stack)
			assert.Equal(t, ConfidenceMedium, mapping.Confidence)
		})
	}
}

func TestMapByResourceType(t *testing.T) {
	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAISecurityTagMapping(),
	}

	tests := []struct {
		name          string
		resourceType  string
		wantComponent string
		wantMapped    bool
	}{
		{"S3 bucket", "AwsS3Bucket", "s3-bucket", true},
		{"EC2 instance", "AwsEc2Instance", "ec2-instance", true},
		{"VPC", "AwsEc2Vpc", "vpc", true},
		{"Unknown type", "AwsSomeNewService", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finding := &Finding{ResourceType: tt.resourceType}
			mapping := mapper.mapByResourceType(finding)
			if !tt.wantMapped {
				assert.Nil(t, mapping)
				return
			}
			require.NotNil(t, mapping)
			assert.Equal(t, tt.wantComponent, mapping.Component)
			assert.Equal(t, ConfidenceLow, mapping.Confidence)
		})
	}
}

func TestExtractResourceName(t *testing.T) {
	tests := []struct {
		arn  string
		want string
	}{
		{"arn:aws:ec2:us-east-1:123456789012:instance/i-12345", "i-12345"},
		{"arn:aws:s3:::my-bucket", "my-bucket"},
		{"arn:aws:iam::123456789012:role/my-role", "my-role"},
	}

	for _, tt := range tests {
		t.Run(tt.arn, func(t *testing.T) {
			assert.Equal(t, tt.want, extractResourceName(tt.arn))
		})
	}
}

func TestExtractRegionFromARN(t *testing.T) {
	tests := []struct {
		arn  string
		want string
	}{
		{"arn:aws:ec2:us-east-1:123456789012:instance/i-12345", "us-east-1"},
		{"arn:aws:s3:::my-bucket", "us-east-1"},           // Global service, no region.
		{"arn:aws:iam::123456789012:role/r", "us-east-1"}, // IAM is global.
	}

	for _, tt := range tests {
		t.Run(tt.arn, func(t *testing.T) {
			assert.Equal(t, tt.want, extractRegionFromARN(tt.arn))
		})
	}
}

func TestBatchFetchTags(t *testing.T) {
	mockClient := &mockTaggingClient{
		resources: []tagtypes.ResourceTagMapping{
			{
				ResourceARN: aws.String("arn:aws:ec2:us-east-1:123:instance/i-abc"),
				Tags: []tagtypes.Tag{
					{Key: aws.String("atmos:stack"), Value: aws.String("prod-ue1")},
					{Key: aws.String("atmos:component"), Value: aws.String("vpc")},
				},
			},
		},
	}

	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAISecurityTagMapping(),
		clients:     newAWSClientCache(),
		tagCache:    make(map[string]*tagLookupResult),
	}

	// Override tagging client factory to return mock.
	mapper.clients.taggingFn = func(_ aws.Config) TaggingAPI {
		return mockClient
	}
	// Pre-populate cache to avoid actual AWS call for client creation.
	mapper.clients.tagging["us-east-1"] = mockClient

	arns := []string{
		"arn:aws:ec2:us-east-1:123:instance/i-abc",
		"arn:aws:ec2:us-east-1:123:instance/i-def",
	}

	err := mapper.batchFetchTags(context.Background(), arns)
	require.NoError(t, err)

	// First ARN should have tags.
	result, ok := mapper.tagCache["arn:aws:ec2:us-east-1:123:instance/i-abc"]
	assert.True(t, ok)
	assert.True(t, result.exists)
	assert.Equal(t, "prod-ue1", result.tags["atmos:stack"])

	// Second ARN should be marked as non-existent.
	result, ok = mapper.tagCache["arn:aws:ec2:us-east-1:123:instance/i-def"]
	assert.True(t, ok)
	assert.False(t, result.exists)
}
