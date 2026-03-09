package security

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentMapper maps AWS resources from security findings to Atmos components and stacks.
type ComponentMapper interface {
	// MapFinding attempts to map a finding's resource to an Atmos component/stack.
	// It tries Path A (tag-based) first, then falls back to Path B (heuristic pipeline).
	MapFinding(ctx context.Context, finding *Finding) (*ComponentMapping, error)

	// MapFindings maps multiple findings in batch, optimizing for shared lookups.
	MapFindings(ctx context.Context, findings []Finding) ([]Finding, error)
}

// NewComponentMapper creates a ComponentMapper that uses both tag-based and heuristic strategies.
func NewComponentMapper(atmosConfig *schema.AtmosConfiguration) ComponentMapper {
	defer perf.Track(nil, "security.NewComponentMapper")()

	return &dualPathMapper{
		atmosConfig: atmosConfig,
		tagMapping:  resolveTagMapping(atmosConfig),
		clients:     newAWSClientCache(),
		tagCache:    make(map[string]*tagLookupResult),
	}
}

// dualPathMapper implements the dual-path mapping algorithm from the PRD.
type dualPathMapper struct {
	atmosConfig *schema.AtmosConfiguration
	tagMapping  schema.AISecurityTagMapping
	clients     *awsClientCache
	tagCache    map[string]*tagLookupResult // Cache by resource ARN.
}

// tagLookupResult caches the result of a tag lookup for a resource.
type tagLookupResult struct {
	tags   map[string]string
	exists bool
}

// MapFinding maps a single finding to an Atmos component/stack.
func (m *dualPathMapper) MapFinding(ctx context.Context, finding *Finding) (*ComponentMapping, error) {
	// Path A: Try tag-based mapping first.
	mapping, err := m.mapByTags(ctx, finding)
	if err == nil && mapping != nil && mapping.Mapped {
		return mapping, nil
	}

	// Path B: Fall back to heuristic pipeline.
	return m.mapByHeuristics(ctx, finding)
}

// MapFindings maps multiple findings in batch.
func (m *dualPathMapper) MapFindings(ctx context.Context, findings []Finding) ([]Finding, error) {
	defer perf.Track(nil, "security.dualPathMapper.MapFindings")()

	// Pre-fetch tags for all resource ARNs in a single batch call.
	arns := make([]string, 0, len(findings))
	for i := range findings {
		if findings[i].ResourceARN != "" {
			arns = append(arns, findings[i].ResourceARN)
		}
	}
	if err := m.batchFetchTags(ctx, arns); err != nil {
		log.Debug("Batch tag fetch failed, falling back to individual lookups", "error", err)
	}

	for i := range findings {
		mapping, err := m.MapFinding(ctx, &findings[i])
		if err != nil {
			// Log error but continue with other findings.
			findings[i].Mapping = &ComponentMapping{
				Mapped:     false,
				Confidence: ConfidenceNone,
				Method:     "error: " + err.Error(),
			}
			continue
		}
		findings[i].Mapping = mapping
	}
	return findings, nil
}

// mapByTags implements Path A: tag-based mapping using cached resource tags.
func (m *dualPathMapper) mapByTags(ctx context.Context, finding *Finding) (*ComponentMapping, error) {
	if finding.ResourceARN == "" {
		return nil, nil
	}

	tags, err := m.getResourceTags(ctx, finding.ResourceARN, finding.Region)
	if err != nil {
		return nil, err
	}
	if tags == nil {
		return nil, nil
	}

	// Look for Atmos tags.
	stack := tags[m.tagMapping.StackTag]
	component := tags[m.tagMapping.ComponentTag]

	if stack == "" {
		stack = m.reconstructStackFromTags(tags)
	}

	if stack == "" && component == "" {
		return nil, nil
	}

	return &ComponentMapping{
		Stack:      stack,
		Component:  component,
		Mapped:     component != "",
		Confidence: ConfidenceExact,
		Method:     "tag",
	}, nil
}

// reconstructStackFromTags attempts to reconstruct a stack name from individual tenant/environment/stage tags.
func (m *dualPathMapper) reconstructStackFromTags(tags map[string]string) string {
	tenant := tags[m.tagMapping.TenantTag]
	environment := tags[m.tagMapping.EnvironmentTag]
	stage := tags[m.tagMapping.StageTag]

	if environment == "" && stage == "" {
		return ""
	}

	parts := []string{}
	if tenant != "" {
		parts = append(parts, tenant)
	}
	if environment != "" {
		parts = append(parts, environment)
	}
	if stage != "" {
		parts = append(parts, stage)
	}

	return strings.Join(parts, "-")
}

// mapByHeuristics implements Path B: multi-strategy heuristic pipeline.
func (m *dualPathMapper) mapByHeuristics(_ context.Context, finding *Finding) (*ComponentMapping, error) {
	// Strategy 1: Resource naming convention analysis.
	if mapping := m.mapByNamingConvention(finding); mapping != nil {
		return mapping, nil
	}

	// Strategy 2: Resource type to component mapping.
	if mapping := m.mapByResourceType(finding); mapping != nil {
		return mapping, nil
	}

	// No match found.
	return &ComponentMapping{
		Mapped:     false,
		Confidence: ConfidenceNone,
		Method:     "unmatched",
	}, nil
}

// mapByNamingConvention attempts to extract component/stack info from resource naming patterns.
func (m *dualPathMapper) mapByNamingConvention(finding *Finding) *ComponentMapping {
	arn := finding.ResourceARN
	if arn == "" {
		return nil
	}

	// Extract resource name from ARN (last segment after / or :).
	name := extractResourceName(arn)
	if name == "" {
		return nil
	}

	// Common Cloud Posse naming convention: {namespace}-{tenant}-{environment}-{stage}-{component}.
	// Try to detect pattern with at least 3 hyphen-separated segments.
	parts := strings.Split(name, "-")
	if len(parts) < 3 {
		return nil
	}

	// Heuristic: the last segment is often the component type.
	component := parts[len(parts)-1]
	// The middle segments form the stack identifier.
	stack := strings.Join(parts[1:len(parts)-1], "-")

	return &ComponentMapping{
		Stack:      stack,
		Component:  component,
		Mapped:     true,
		Confidence: ConfidenceMedium,
		Method:     "naming-convention",
	}
}

// mapByResourceType maps well-known AWS resource types to common Atmos component names.
func (m *dualPathMapper) mapByResourceType(finding *Finding) *ComponentMapping {
	resourceTypeMap := map[string]string{
		"AwsEc2Instance":            "ec2-instance",
		"AwsEc2SecurityGroup":       "security-group",
		"AwsEc2Vpc":                 "vpc",
		"AwsEc2Subnet":              "vpc",
		"AwsS3Bucket":               "s3-bucket",
		"AwsIamRole":                "iam-role",
		"AwsIamPolicy":              "iam-policy",
		"AwsIamUser":                "iam-user",
		"AwsRdsDbInstance":          "rds",
		"AwsRdsDbCluster":           "aurora",
		"AwsLambdaFunction":         "lambda",
		"AwsElbv2LoadBalancer":      "alb",
		"AwsEcsCluster":             "ecs-cluster",
		"AwsEcsService":             "ecs-service",
		"AwsEksCluster":             "eks-cluster",
		"AwsCloudTrailTrail":        "cloudtrail",
		"AwsKmsKey":                 "kms",
		"AwsDynamoDbTable":          "dynamodb",
		"AwsSqsQueue":               "sqs",
		"AwsSnsTopicSubscription":   "sns",
		"AwsElasticSearchDomain":    "elasticsearch",
		"AwsCloudFrontDistribution": "cloudfront",
		"AwsWafWebAcl":              "waf",
	}

	if component, ok := resourceTypeMap[finding.ResourceType]; ok {
		return &ComponentMapping{
			Component:  component,
			Mapped:     true,
			Confidence: ConfidenceLow,
			Method:     "resource-type",
		}
	}

	return nil
}

// getResourceTags retrieves tags for a resource, using cache if available.
func (m *dualPathMapper) getResourceTags(ctx context.Context, arn string, region string) (map[string]string, error) {
	// Check cache first.
	if result, ok := m.tagCache[arn]; ok {
		if !result.exists {
			return nil, nil
		}
		return result.tags, nil
	}

	if region == "" {
		region = "us-east-1"
	}

	client, err := m.clients.getTaggingClient(ctx, region)
	if err != nil {
		return nil, err
	}

	output, err := client.GetResources(ctx, &resourcegroupstaggingapi.GetResourcesInput{
		ResourceARNList: []string{arn},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get resource tags for %s: %w", arn, err)
	}

	if len(output.ResourceTagMappingList) == 0 {
		m.tagCache[arn] = &tagLookupResult{exists: false}
		return nil, nil
	}

	tags := make(map[string]string)
	for _, tag := range output.ResourceTagMappingList[0].Tags {
		tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	m.tagCache[arn] = &tagLookupResult{tags: tags, exists: true}
	return tags, nil
}

// batchFetchTags fetches tags for multiple ARNs in a single API call (up to 100 at a time).
func (m *dualPathMapper) batchFetchTags(ctx context.Context, arns []string) error {
	defer perf.Track(nil, "security.dualPathMapper.batchFetchTags")()

	if len(arns) == 0 {
		return nil
	}

	// Group ARNs by region.
	arnsByRegion := make(map[string][]string)
	for _, arn := range arns {
		region := extractRegionFromARN(arn)
		arnsByRegion[region] = append(arnsByRegion[region], arn)
	}

	batchSize := securityHubPageSize // AWS API limit for GetResources.

	for region, regionARNs := range arnsByRegion {
		client, err := m.clients.getTaggingClient(ctx, region)
		if err != nil {
			return err
		}

		for i := 0; i < len(regionARNs); i += batchSize {
			end := i + batchSize
			if end > len(regionARNs) {
				end = len(regionARNs)
			}
			m.fetchTagBatch(ctx, client, regionARNs[i:end], region)
		}
	}

	return nil
}

// fetchTagBatch fetches tags for a single batch of ARNs and caches the results.
func (m *dualPathMapper) fetchTagBatch(ctx context.Context, client TaggingAPI, batch []string, region string) {
	output, err := client.GetResources(ctx, &resourcegroupstaggingapi.GetResourcesInput{
		ResourceARNList: batch,
	})
	if err != nil {
		log.Debug("Batch tag fetch failed", "region", region, "error", err)
		return
	}

	// Cache results.
	fetchedARNs := make(map[string]bool)
	for _, mapping := range output.ResourceTagMappingList {
		arn := aws.ToString(mapping.ResourceARN)
		fetchedARNs[arn] = true
		tags := tagsToMap(mapping.Tags)
		m.tagCache[arn] = &tagLookupResult{tags: tags, exists: true}
	}

	// Mark unfound ARNs as non-existent in cache.
	for _, arn := range batch {
		if !fetchedARNs[arn] {
			m.tagCache[arn] = &tagLookupResult{exists: false}
		}
	}
}

// tagsToMap converts AWS tag list to a simple map.
func tagsToMap(tags []tagtypes.Tag) map[string]string {
	result := make(map[string]string, len(tags))
	for _, tag := range tags {
		result[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return result
}

// extractResourceName extracts the resource name from an ARN.
func extractResourceName(arn string) string {
	// ARN format: arn:partition:service:region:account:resource-type/resource-id.
	// or: arn:partition:service:region:account:resource-type:resource-id.
	parts := strings.Split(arn, "/")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	parts = strings.Split(arn, ":")
	if len(parts) > arnMinSegments {
		return parts[len(parts)-1]
	}
	return ""
}

// extractRegionFromARN extracts the AWS region from an ARN string.
func extractRegionFromARN(arn string) string {
	// ARN format: arn:partition:service:region:account:...
	parts := strings.Split(arn, ":")
	if len(parts) > 3 && parts[3] != "" {
		return parts[3]
	}
	return "us-east-1"
}

// resolveTagMapping returns the tag mapping config with defaults applied.
func resolveTagMapping(atmosConfig *schema.AtmosConfiguration) schema.AISecurityTagMapping {
	mapping := atmosConfig.AI.Security.TagMapping
	defaults := schema.DefaultAISecurityTagMapping()

	if mapping.StackTag == "" {
		mapping.StackTag = defaults.StackTag
	}
	if mapping.ComponentTag == "" {
		mapping.ComponentTag = defaults.ComponentTag
	}
	if mapping.TenantTag == "" {
		mapping.TenantTag = defaults.TenantTag
	}
	if mapping.EnvironmentTag == "" {
		mapping.EnvironmentTag = defaults.EnvironmentTag
	}
	if mapping.StageTag == "" {
		mapping.StageTag = defaults.StageTag
	}

	return mapping
}
