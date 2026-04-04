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
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
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
	assert.Equal(t, "tag-api", mapping.Method)
}

func TestMapByTags_ComponentOnlyNoStack(t *testing.T) {
	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
		clients:     newAWSClientCache(),
		tagCache: map[string]*tagLookupResult{
			"arn:aws:s3:::my-bucket": {
				tags: map[string]string{
					"atmos:component": "s3-bucket",
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
	assert.Equal(t, "", mapping.Stack)
	assert.Equal(t, "s3-bucket", mapping.Component)
	assert.Equal(t, ConfidenceExact, mapping.Confidence)
}

func TestMapByTags_CustomTagKeys(t *testing.T) {
	customMapping := schema.AWSSecurityTagMapping{
		StackTag:     "mycompany:stack",
		ComponentTag: "mycompany:component",
	}

	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  customMapping,
		clients:     newAWSClientCache(),
		tagCache: map[string]*tagLookupResult{
			"arn:aws:ec2:us-east-1:123456789012:instance/i-abc": {
				tags: map[string]string{
					"mycompany:stack":     "prod-us-east-1",
					"mycompany:component": "web-server",
				},
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
	require.NotNil(t, mapping)
	assert.True(t, mapping.Mapped)
	assert.Equal(t, "prod-us-east-1", mapping.Stack)
	assert.Equal(t, "web-server", mapping.Component)
	assert.Equal(t, ConfidenceExact, mapping.Confidence)
	assert.Equal(t, "tag-api", mapping.Method)
}

func TestMapByTags_NoTags(t *testing.T) {
	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
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
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
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
			assert.Equal(t, ConfidenceLow, mapping.Confidence)
		})
	}
}

func TestMapByResourceType(t *testing.T) {
	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
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

func TestMapFinding_TagsThenHeuristicsFallback(t *testing.T) {
	tests := []struct {
		name           string
		finding        Finding
		tagCache       map[string]*tagLookupResult
		wantMapped     bool
		wantMethod     string
		wantConfidence MappingConfidence
		wantComponent  string
	}{
		{
			name: "mapped via tags (Path A)",
			finding: Finding{
				ResourceARN:  "arn:aws:ec2:us-east-1:123:instance/i-abc",
				Region:       "us-east-1",
				ResourceType: "AwsEc2Instance",
			},
			tagCache: map[string]*tagLookupResult{
				"arn:aws:ec2:us-east-1:123:instance/i-abc": {
					tags:   map[string]string{"atmos:stack": "prod-ue1", "atmos:component": "vpc"},
					exists: true,
				},
			},
			wantMapped:     true,
			wantMethod:     "tag-api",
			wantConfidence: ConfidenceExact,
			wantComponent:  "vpc",
		},
		{
			name: "falls back to naming convention (Path B)",
			finding: Finding{
				ResourceARN:  "arn:aws:ec2:us-east-1:123:instance/acme-ue1-prod-alb",
				Region:       "us-east-1",
				ResourceType: "AwsElbv2LoadBalancer",
			},
			tagCache: map[string]*tagLookupResult{
				"arn:aws:ec2:us-east-1:123:instance/acme-ue1-prod-alb": {
					tags:   map[string]string{"Name": "acme-ue1-prod-alb"},
					exists: true,
				},
			},
			wantMapped:     true,
			wantMethod:     "naming-convention",
			wantConfidence: ConfidenceLow,
			wantComponent:  "alb",
		},
		{
			name: "falls back to resource type (Path B)",
			finding: Finding{
				ResourceARN:  "arn:aws:s3:::my-bucket",
				Region:       "us-east-1",
				ResourceType: "AwsS3Bucket",
			},
			tagCache: map[string]*tagLookupResult{
				"arn:aws:s3:::my-bucket": {exists: false},
			},
			wantMapped:     true,
			wantMethod:     "resource-type",
			wantConfidence: ConfidenceLow,
			wantComponent:  "s3-bucket",
		},
		{
			name: "unmatched - no tags no naming no resource type",
			finding: Finding{
				ResourceARN:  "arn:aws:custom:us-east-1:123:thing/x",
				Region:       "us-east-1",
				ResourceType: "AwsSomeUnknownService",
			},
			tagCache: map[string]*tagLookupResult{
				"arn:aws:custom:us-east-1:123:thing/x": {exists: false},
			},
			wantMapped:     false,
			wantMethod:     "unmatched",
			wantConfidence: ConfidenceNone,
			wantComponent:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := &dualPathMapper{
				atmosConfig: &schema.AtmosConfiguration{},
				tagMapping:  schema.DefaultAWSSecurityTagMapping(),
				clients:     newAWSClientCache(),
				tagCache:    tt.tagCache,
			}

			mapping, err := mapper.MapFinding(context.Background(), &tt.finding)
			require.NoError(t, err)
			require.NotNil(t, mapping)
			assert.Equal(t, tt.wantMapped, mapping.Mapped)
			assert.Equal(t, tt.wantMethod, mapping.Method)
			assert.Equal(t, tt.wantConfidence, mapping.Confidence)
			if tt.wantComponent != "" {
				assert.Equal(t, tt.wantComponent, mapping.Component)
			}
		})
	}
}

func TestMapFindings_Batch(t *testing.T) {
	mockClient := &mockTaggingClient{
		resources: []tagtypes.ResourceTagMapping{
			{
				ResourceARN: aws.String("arn:aws:ec2:us-east-1:123:instance/acme-ue1-prod-vpc"),
				Tags: []tagtypes.Tag{
					{Key: aws.String("atmos:stack"), Value: aws.String("prod-ue1")},
					{Key: aws.String("atmos:component"), Value: aws.String("vpc")},
				},
			},
		},
	}

	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
		clients:     newAWSClientCache(),
		tagCache:    make(map[string]*tagLookupResult),
	}
	// Set mock client so batch fetch populates cache correctly.
	mapper.clients.tagging["us-east-1"] = mockClient

	findings := []Finding{
		{
			ResourceARN:  "arn:aws:ec2:us-east-1:123:instance/acme-ue1-prod-vpc",
			Region:       "us-east-1",
			ResourceType: "AwsEc2Instance",
		},
		{
			ResourceARN:  "arn:aws:s3:::plain-bucket",
			Region:       "us-east-1",
			ResourceType: "AwsS3Bucket",
		},
		{
			ResourceARN:  "",
			ResourceType: "AwsSomeUnknownService",
		},
	}

	result, err := mapper.MapFindings(context.Background(), findings)
	require.NoError(t, err)
	require.Len(t, result, 3)

	// First finding: mapped via tags.
	require.NotNil(t, result[0].Mapping)
	assert.True(t, result[0].Mapping.Mapped)
	assert.Equal(t, "tag-api", result[0].Mapping.Method)

	// Second finding: no tags, falls to resource type.
	require.NotNil(t, result[1].Mapping)
	assert.True(t, result[1].Mapping.Mapped)
	assert.Equal(t, "resource-type", result[1].Mapping.Method)
	assert.Equal(t, "s3-bucket", result[1].Mapping.Component)

	// Third finding: empty ARN and unknown type → unmatched.
	require.NotNil(t, result[2].Mapping)
	assert.False(t, result[2].Mapping.Mapped)
	assert.Equal(t, "unmatched", result[2].Mapping.Method)
}

func TestMapByHeuristics(t *testing.T) {
	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
	}

	tests := []struct {
		name           string
		finding        Finding
		wantMapped     bool
		wantMethod     string
		wantConfidence MappingConfidence
	}{
		{
			name: "naming convention match",
			finding: Finding{
				ResourceARN:  "arn:aws:ec2:us-east-1:123:instance/acme-ue1-prod-eks",
				ResourceType: "AwsEc2Instance",
			},
			wantMapped:     true,
			wantMethod:     "naming-convention",
			wantConfidence: ConfidenceLow,
		},
		{
			name: "resource type match when naming fails",
			finding: Finding{
				ResourceARN:  "arn:aws:ec2:us-east-1:123:instance/i-12345",
				ResourceType: "AwsEksCluster",
			},
			wantMapped:     true,
			wantMethod:     "resource-type",
			wantConfidence: ConfidenceLow,
		},
		{
			name: "unmatched - no naming and unknown type",
			finding: Finding{
				ResourceARN:  "arn:aws:custom:us-east-1:123:thing/x",
				ResourceType: "AwsUnknownThing",
			},
			wantMapped:     false,
			wantMethod:     "unmatched",
			wantConfidence: ConfidenceNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping, err := mapper.mapByHeuristics(context.Background(), &tt.finding)
			require.NoError(t, err)
			require.NotNil(t, mapping)
			assert.Equal(t, tt.wantMapped, mapping.Mapped)
			assert.Equal(t, tt.wantMethod, mapping.Method)
			assert.Equal(t, tt.wantConfidence, mapping.Confidence)
		})
	}
}

func TestGetResourceTags(t *testing.T) {
	tests := []struct {
		name     string
		arn      string
		region   string
		tagCache map[string]*tagLookupResult
		mockTags []tagtypes.ResourceTagMapping
		mockErr  error
		wantTags map[string]string
		wantErr  bool
	}{
		{
			name:   "cache hit with tags",
			arn:    "arn:aws:ec2:us-east-1:123:instance/i-abc",
			region: "us-east-1",
			tagCache: map[string]*tagLookupResult{
				"arn:aws:ec2:us-east-1:123:instance/i-abc": {
					tags:   map[string]string{"atmos:stack": "prod"},
					exists: true,
				},
			},
			wantTags: map[string]string{"atmos:stack": "prod"},
		},
		{
			name:   "cache hit with no resource",
			arn:    "arn:aws:ec2:us-east-1:123:instance/i-gone",
			region: "us-east-1",
			tagCache: map[string]*tagLookupResult{
				"arn:aws:ec2:us-east-1:123:instance/i-gone": {exists: false},
			},
			wantTags: nil,
		},
		{
			name:     "cache miss - API returns tags",
			arn:      "arn:aws:ec2:us-east-1:123:instance/i-new",
			region:   "us-east-1",
			tagCache: map[string]*tagLookupResult{},
			mockTags: []tagtypes.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:ec2:us-east-1:123:instance/i-new"),
					Tags: []tagtypes.Tag{
						{Key: aws.String("atmos:component"), Value: aws.String("vpc")},
					},
				},
			},
			wantTags: map[string]string{"atmos:component": "vpc"},
		},
		{
			name:     "cache miss - API returns empty",
			arn:      "arn:aws:ec2:us-east-1:123:instance/i-empty",
			region:   "us-east-1",
			tagCache: map[string]*tagLookupResult{},
			mockTags: []tagtypes.ResourceTagMapping{},
			wantTags: nil,
		},
		{
			name:     "cache miss - API error",
			arn:      "arn:aws:ec2:us-east-1:123:instance/i-err",
			region:   "us-east-1",
			tagCache: map[string]*tagLookupResult{},
			mockErr:  assert.AnError,
			wantErr:  true,
		},
		{
			name:     "empty region defaults to us-east-1",
			arn:      "arn:aws:ec2:us-east-1:123:instance/i-noreg",
			region:   "",
			tagCache: map[string]*tagLookupResult{},
			mockTags: []tagtypes.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:ec2:us-east-1:123:instance/i-noreg"),
					Tags: []tagtypes.Tag{
						{Key: aws.String("env"), Value: aws.String("prod")},
					},
				},
			},
			wantTags: map[string]string{"env": "prod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockTaggingClient{
				resources: tt.mockTags,
				err:       tt.mockErr,
			}

			mapper := &dualPathMapper{
				atmosConfig: &schema.AtmosConfiguration{},
				tagMapping:  schema.DefaultAWSSecurityTagMapping(),
				clients:     newAWSClientCache(),
				tagCache:    tt.tagCache,
			}
			// Pre-populate the client for the resolved region.
			resolvedRegion := tt.region
			if resolvedRegion == "" {
				resolvedRegion = "us-east-1"
			}
			mapper.clients.tagging[resolvedRegion] = mock

			tags, err := mapper.getResourceTags(context.Background(), tt.arn, tt.region)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.wantTags == nil {
				assert.Nil(t, tags)
			} else {
				assert.Equal(t, tt.wantTags, tags)
			}
		})
	}
}

func TestResolveTagMapping(t *testing.T) {
	tests := []struct {
		name    string
		config  schema.AtmosConfiguration
		wantTag string // Check ComponentTag as representative.
	}{
		{
			name:    "all defaults when empty",
			config:  schema.AtmosConfiguration{},
			wantTag: "atmos:component",
		},
		{
			name: "custom overrides",
			config: schema.AtmosConfiguration{
				AWS: schema.AWSSettings{
					Security: schema.AWSSecuritySettings{
						TagMapping: schema.AWSSecurityTagMapping{
							ComponentTag: "custom:component",
							StackTag:     "custom:stack",
						},
					},
				},
			},
			wantTag: "custom:component",
		},
		{
			name: "partial override fills remaining defaults",
			config: schema.AtmosConfiguration{
				AWS: schema.AWSSettings{
					Security: schema.AWSSecuritySettings{
						TagMapping: schema.AWSSecurityTagMapping{
							StackTag: "my:stack",
						},
					},
				},
			},
			wantTag: "atmos:component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveTagMapping(&tt.config)
			assert.Equal(t, tt.wantTag, result.ComponentTag)
			// Verify both fields are filled.
			assert.NotEmpty(t, result.StackTag)
			assert.NotEmpty(t, result.ComponentTag)
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
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
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

func TestMapByTags_FindingEmbeddedTags(t *testing.T) {
	// When the finding has ResourceTags, use them directly (no API call).
	atmosConfig := &schema.AtmosConfiguration{
		AWS: schema.AWSSettings{
			Security: schema.AWSSecuritySettings{
				TagMapping: schema.AWSSecurityTagMapping{
					StackTag:     "atmos_stack",
					ComponentTag: "atmos_component",
				},
			},
		},
	}

	mapper := NewComponentMapper(atmosConfig, nil)

	finding := &Finding{
		ID:          "embedded-tag-001",
		ResourceARN: "arn:aws:s3:::my-bucket",
		ResourceTags: map[string]string{
			"atmos_stack":     "plat-use2-prod",
			"atmos_component": "s3-bucket",
			"Environment":     "production",
		},
	}

	mapping, err := mapper.MapFinding(context.Background(), finding)
	require.NoError(t, err)
	require.NotNil(t, mapping)
	assert.True(t, mapping.Mapped)
	assert.Equal(t, "plat-use2-prod", mapping.Stack)
	assert.Equal(t, "s3-bucket", mapping.Component)
	assert.Equal(t, ConfidenceExact, mapping.Confidence)
	assert.Equal(t, "finding-tag", mapping.Method)
}

func TestMapByTags_FindingTagsFallbackToHeuristics(t *testing.T) {
	// When finding has no ResourceTags and no Tagging API, falls through to heuristics.
	finding := &Finding{
		ID:           "no-tag-001",
		ResourceARN:  "arn:aws:s3:::my-bucket",
		ResourceType: "AwsS3Bucket",
		ResourceTags: nil, // No embedded tags.
	}

	atmosConfig := &schema.AtmosConfiguration{}
	mapper := NewComponentMapper(atmosConfig, nil)

	mapping, err := mapper.MapFinding(context.Background(), finding)
	require.NoError(t, err)
	require.NotNil(t, mapping)
	// Without tags, should fall through to a heuristic method.
	assert.NotEqual(t, "finding-tag", mapping.Method)
	assert.NotEqual(t, "tag-api", mapping.Method)
	// The resource-type mapper should produce a component name from AwsS3Bucket.
	if mapping.Mapped {
		assert.NotEmpty(t, mapping.Component)
		assert.Equal(t, ConfidenceLow, mapping.Confidence)
	}
}

func TestMapByContextTags(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	m := NewComponentMapper(atmosConfig, nil).(*dualPathMapper)

	tests := []struct {
		name          string
		finding       *Finding
		wantComponent string
		wantStack     string
		wantMapped    bool
	}{
		{
			name: "full context tags",
			finding: &Finding{
				ResourceTags: map[string]string{
					"Name":        "ins-plat-use2-dev-example-static-app-origin",
					"Namespace":   "ins",
					"Tenant":      "plat",
					"Environment": "use2",
					"Stage":       "dev",
				},
			},
			wantComponent: "example-static-app-origin",
			wantStack:     "plat-use2-dev",
			wantMapped:    true,
		},
		{
			name: "no namespace",
			finding: &Finding{
				ResourceTags: map[string]string{
					"Name":        "plat-use2-prod-vpc",
					"Tenant":      "plat",
					"Environment": "use2",
					"Stage":       "prod",
				},
			},
			wantComponent: "vpc",
			wantStack:     "plat-use2-prod",
			wantMapped:    true,
		},
		{
			name: "no environment",
			finding: &Finding{
				ResourceTags: map[string]string{
					"Name":      "ins-core-security-guardduty",
					"Namespace": "ins",
					"Tenant":    "core",
					"Stage":     "security",
				},
			},
			wantComponent: "guardduty",
			wantStack:     "core-security",
			wantMapped:    true,
		},
		{
			name: "ecs task definition with version",
			finding: &Finding{
				ResourceTags: map[string]string{
					"Name":        "ins-plat-use2-prod-app",
					"Namespace":   "ins",
					"Tenant":      "plat",
					"Environment": "use2",
					"Stage":       "prod",
				},
			},
			wantComponent: "app",
			wantStack:     "plat-use2-prod",
			wantMapped:    true,
		},
		{
			name: "no tags",
			finding: &Finding{
				ResourceTags: nil,
			},
			wantMapped: false,
		},
		{
			name: "missing tenant",
			finding: &Finding{
				ResourceTags: map[string]string{
					"Name":  "something",
					"Stage": "dev",
				},
			},
			wantMapped: false,
		},
		{
			name: "name doesn't match prefix",
			finding: &Finding{
				ResourceTags: map[string]string{
					"Name":        "unrelated-resource-name",
					"Namespace":   "ins",
					"Tenant":      "plat",
					"Environment": "use2",
					"Stage":       "dev",
				},
			},
			wantMapped: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := m.mapByContextTags(tt.finding)
			if !tt.wantMapped {
				if mapping != nil {
					assert.False(t, mapping.Mapped)
				}
				return
			}
			require.NotNil(t, mapping)
			assert.True(t, mapping.Mapped)
			assert.Equal(t, tt.wantComponent, mapping.Component)
			assert.Equal(t, tt.wantStack, mapping.Stack)
			assert.Equal(t, ConfidenceHigh, mapping.Confidence)
			assert.Equal(t, "context-tags", mapping.Method)
		})
	}
}

func TestMapByAccountID(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AWS: schema.AWSSettings{
			Security: schema.AWSSecuritySettings{
				AccountMap: map[string]string{
					"452379801773": "plat-prod",
					"344349181611": "plat-dev",
				},
			},
		},
	}
	m := NewComponentMapper(atmosConfig, nil).(*dualPathMapper)

	t.Run("account-level finding with known account", func(t *testing.T) {
		finding := &Finding{
			ResourceARN: "AWS::::Account:452379801773",
			AccountID:   "452379801773",
		}
		mapping := m.mapByAccountID(finding)
		require.NotNil(t, mapping)
		assert.True(t, mapping.Mapped)
		assert.Equal(t, "plat-prod", mapping.Stack)
		assert.Equal(t, "account", mapping.Component)
		assert.Equal(t, "account-map", mapping.Method)
	})

	t.Run("account-level finding with unknown account", func(t *testing.T) {
		finding := &Finding{
			ResourceARN: "AWS::::Account:999999999999",
			AccountID:   "999999999999",
		}
		mapping := m.mapByAccountID(finding)
		require.NotNil(t, mapping)
		assert.False(t, mapping.Mapped)
		assert.Equal(t, "999999999999", mapping.Stack)
		assert.Equal(t, "account-level", mapping.Method)
	})

	t.Run("non-account finding returns nil", func(t *testing.T) {
		finding := &Finding{
			ResourceARN: "arn:aws:s3:::my-bucket",
		}
		mapping := m.mapByAccountID(finding)
		assert.Nil(t, mapping)
	})
}

func TestMapByECRRepo(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		AWS: schema.AWSSettings{
			Security: schema.AWSSecuritySettings{
				AccountMap: map[string]string{
					"982674173972": "core-artifacts",
					"101071483060": "core-auto",
				},
			},
		},
	}
	m := NewComponentMapper(atmosConfig, nil).(*dualPathMapper)

	t.Run("ECR with sha256 and account map", func(t *testing.T) {
		finding := &Finding{
			ResourceARN: "arn:aws:ecr:us-east-2:982674173972:repository/inspatial/example-app-on-ecs/sha256:abc123",
			AccountID:   "982674173972",
		}
		mapping := m.mapByECRRepo(finding)
		require.NotNil(t, mapping)
		assert.True(t, mapping.Mapped)
		assert.Equal(t, "example-app-on-ecs", mapping.Component)
		assert.Equal(t, "core-artifacts", mapping.Stack)
		assert.Equal(t, "ecr-repo", mapping.Method)
	})

	t.Run("ECR without account map", func(t *testing.T) {
		finding := &Finding{
			ResourceARN: "arn:aws:ecr:us-east-2:999:repository/myorg/myapp",
			AccountID:   "999",
		}
		mapping := m.mapByECRRepo(finding)
		require.NotNil(t, mapping)
		assert.Equal(t, "myapp", mapping.Component)
		assert.Empty(t, mapping.Stack) // Unknown account.
	})

	t.Run("non-ECR resource returns nil", func(t *testing.T) {
		finding := &Finding{
			ResourceARN: "arn:aws:s3:::my-bucket",
		}
		mapping := m.mapByECRRepo(finding)
		assert.Nil(t, mapping)
	})
}

func TestGroupByTitle(t *testing.T) {
	findings := []Finding{
		{Title: "AWS Config should be enabled", AccountID: "111"},
		{Title: "AWS Config should be enabled", AccountID: "222"},
		{Title: "AWS Config should be enabled", AccountID: "333"},
		{Title: "S3 bucket public", AccountID: "111"},
		{Title: "S3 bucket public", AccountID: "222"},
	}
	groups := groupByTitle(findings)
	require.Len(t, groups, 2)
	assert.Len(t, groups[0], 3)
	assert.Equal(t, "AWS Config should be enabled", groups[0][0].Title)
	assert.Len(t, groups[1], 2)
	assert.Equal(t, "S3 bucket public", groups[1][0].Title)
}

func TestGroupByTitle_NoDuplicates(t *testing.T) {
	findings := []Finding{
		{Title: "A"},
		{Title: "B"},
		{Title: "C"},
	}
	groups := groupByTitle(findings)
	require.Len(t, groups, 3)

	gotTitles := make([]string, 0, len(groups))
	for _, group := range groups {
		require.Len(t, group, 1)
		gotTitles = append(gotTitles, group[0].Title)
	}
	assert.ElementsMatch(t, []string{"A", "B", "C"}, gotTitles)
}

func TestMapByAccountID_EmptyAccountID(t *testing.T) {
	// When the resource ARN has "AWS::::Account:" but the extracted ID is empty.
	m := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
		accountMap:  map[string]string{"123": "prod"},
	}
	finding := &Finding{
		ResourceARN: "AWS::::Account:",
	}
	mapping := m.mapByAccountID(finding)
	assert.Nil(t, mapping, "empty account ID should return nil")
}

func TestMapByAccountID_FallbackToFindingAccountID(t *testing.T) {
	// When the ARN account ID doesn't match but finding.AccountID does.
	m := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
		accountMap:  map[string]string{"999": "special-account"},
	}
	finding := &Finding{
		ResourceARN: "AWS::::Account:888", // 888 not in map.
		AccountID:   "999",                // 999 is in map.
	}
	mapping := m.mapByAccountID(finding)
	require.NotNil(t, mapping)
	assert.True(t, mapping.Mapped)
	assert.Equal(t, "special-account", mapping.Stack)
	assert.Equal(t, "account", mapping.Component)
	assert.Equal(t, "account-map", mapping.Method)
}

func TestMapByAccountID_NilAccountMap(t *testing.T) {
	// When accountMap is nil, should return unmapped for account-level findings.
	m := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
		accountMap:  nil,
	}
	finding := &Finding{
		ResourceARN: "AWS::::Account:123456",
		AccountID:   "123456",
	}
	mapping := m.mapByAccountID(finding)
	require.NotNil(t, mapping)
	assert.False(t, mapping.Mapped)
	assert.Equal(t, "123456", mapping.Stack)
	assert.Equal(t, "account-level", mapping.Method)
}

func TestMapByECRRepo_EmptyComponent(t *testing.T) {
	// When the repo path ends with /, the component would be empty.
	m := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
	}
	finding := &Finding{
		ResourceARN: "arn:aws:ecr:us-east-1:123:repository/",
	}
	mapping := m.mapByECRRepo(finding)
	assert.Nil(t, mapping, "empty component from ECR path should return nil")
}

func TestMapByNamingConvention_EmptyARN(t *testing.T) {
	// Empty ARN should return nil.
	m := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
	}
	finding := &Finding{ResourceARN: ""}
	mapping := m.mapByNamingConvention(finding)
	assert.Nil(t, mapping)
}

func TestMapByResourceType_WithAccountMap(t *testing.T) {
	// Resource type mapping should resolve stack from account map.
	m := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
		accountMap:  map[string]string{"111222333444": "plat-prod"},
	}
	finding := &Finding{
		ResourceType: "AwsEc2Vpc",
		AccountID:    "111222333444",
	}
	mapping := m.mapByResourceType(finding)
	require.NotNil(t, mapping)
	assert.True(t, mapping.Mapped)
	assert.Equal(t, "vpc", mapping.Component)
	assert.Equal(t, "plat-prod", mapping.Stack)
	assert.Equal(t, "resource-type", mapping.Method)
}

func TestMapByResourceType_NoAccountMap(t *testing.T) {
	// When accountMap is nil, stack should be empty.
	m := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
		accountMap:  nil,
	}
	finding := &Finding{
		ResourceType: "AwsS3Bucket",
		AccountID:    "123",
	}
	mapping := m.mapByResourceType(finding)
	require.NotNil(t, mapping)
	assert.True(t, mapping.Mapped)
	assert.Equal(t, "s3-bucket", mapping.Component)
	assert.Empty(t, mapping.Stack)
}

func TestMapByContextTags_EmptyComponentAfterPrefix(t *testing.T) {
	// When the name exactly equals the prefix (no remaining component), should return nil.
	m := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
	}
	finding := &Finding{
		ResourceTags: map[string]string{
			"Name":        "plat-use2-prod-",
			"Tenant":      "plat",
			"Environment": "use2",
			"Stage":       "prod",
		},
	}
	// The prefix would be "plat-use2-prod-", and stripping it leaves empty string.
	mapping := m.mapByContextTags(finding)
	assert.Nil(t, mapping, "empty component after prefix strip should return nil")
}

func TestMapByContextTags_MissingStage(t *testing.T) {
	// Without Stage, should return nil.
	m := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
	}
	finding := &Finding{
		ResourceTags: map[string]string{
			"Name":   "something",
			"Tenant": "plat",
		},
	}
	mapping := m.mapByContextTags(finding)
	assert.Nil(t, mapping)
}

func TestMapByTags_EmptyARN(t *testing.T) {
	// Empty ARN should return nil without error.
	mapper := &dualPathMapper{
		atmosConfig: &schema.AtmosConfiguration{},
		tagMapping:  schema.DefaultAWSSecurityTagMapping(),
		clients:     newAWSClientCache(),
		tagCache:    map[string]*tagLookupResult{},
	}
	finding := &Finding{ResourceARN: ""}
	mapping, err := mapper.mapByTags(context.Background(), finding)
	require.NoError(t, err)
	assert.Nil(t, mapping)
}

func TestMatchTags_BothEmpty(t *testing.T) {
	// When tags are present but neither stack nor component tag exists.
	mapper := &dualPathMapper{
		tagMapping: schema.AWSSecurityTagMapping{
			StackTag:     "atmos:stack",
			ComponentTag: "atmos:component",
		},
	}
	result := mapper.matchTags(map[string]string{"Name": "test", "Env": "prod"}, "test-method")
	assert.Nil(t, result, "should return nil when neither stack nor component tag found")
}

func TestMatchTags_StackOnlyNoComponent(t *testing.T) {
	// When only stack tag is present, Mapped should be false (component is empty).
	mapper := &dualPathMapper{
		tagMapping: schema.AWSSecurityTagMapping{
			StackTag:     "atmos:stack",
			ComponentTag: "atmos:component",
		},
	}
	result := mapper.matchTags(map[string]string{"atmos:stack": "prod-ue1"}, "test-method")
	require.NotNil(t, result)
	assert.False(t, result.Mapped, "mapped should be false when component is empty")
	assert.Equal(t, "prod-ue1", result.Stack)
	assert.Empty(t, result.Component)
}

func TestExtractResourceName_ColonSeparated(t *testing.T) {
	// Resource name extracted from colon-separated ARN (no slash).
	name := extractResourceName("arn:aws:iam::123456789012:user:my-user")
	assert.Equal(t, "my-user", name)
}

func TestExtractResourceName_ShortARN(t *testing.T) {
	// ARN with too few segments should return empty.
	name := extractResourceName("arn:aws:s3")
	assert.Equal(t, "", name)
}

func TestTruncateMiddle(t *testing.T) {
	assert.Equal(t, "short", truncateMiddle("short"))
	long := "arn:aws:ecr:us-east-2:982674173972:repository/inspatial/example-app-on-ecs/sha256:876f27531c79965bc6e3a5492e2ccdd3ca4532b0ebef80f2b5c2063e2db712c7"
	truncated := truncateMiddle(long)
	assert.LessOrEqual(t, len(truncated), maxARNDisplayLen)
	assert.Contains(t, truncated, "...")
}
