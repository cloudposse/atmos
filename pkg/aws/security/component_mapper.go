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

// minNamingParts is the minimum hyphen-separated segments for naming convention matching.
const minNamingParts = 3

// nameSeparator is the hyphen used as a separator in Cloud Posse naming conventions.
const nameSeparator = "-"

// ComponentMapper maps AWS resources from security findings to Atmos components and stacks.
type ComponentMapper interface {
	// MapFinding attempts to map a finding's resource to an Atmos component/stack.
	// It tries Path A (tag-based) first, then falls back to Path B (heuristic pipeline).
	MapFinding(ctx context.Context, finding *Finding) (*ComponentMapping, error)

	// MapFindings maps multiple findings in batch, optimizing for shared lookups.
	MapFindings(ctx context.Context, findings []Finding) ([]Finding, error)
}

// NewComponentMapper creates a ComponentMapper that uses both tag-based and heuristic strategies.
// If authCtx is non-nil, AWS clients will use Atmos Auth credentials.
func NewComponentMapper(atmosConfig *schema.AtmosConfiguration, authCtx *schema.AWSAuthContext) ComponentMapper {
	defer perf.Track(nil, "security.NewComponentMapper")()

	clients := newAWSClientCache()
	if authCtx != nil {
		clients.WithAuthContext(authCtx)
	}

	return &dualPathMapper{
		atmosConfig: atmosConfig,
		tagMapping:  resolveTagMapping(atmosConfig),
		accountMap:  atmosConfig.AWS.Security.AccountMap,
		clients:     clients,
		tagCache:    make(map[string]*tagLookupResult),
	}
}

// dualPathMapper implements the dual-path mapping algorithm from the PRD.
type dualPathMapper struct {
	atmosConfig *schema.AtmosConfiguration
	tagMapping  schema.AWSSecurityTagMapping
	accountMap  map[string]string // Account ID → name for account-level findings.
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

// mapByTags implements Path A: tag-based mapping.
// First checks tags embedded in the Security Hub finding (no API call needed).
// Falls back to the Resource Groups Tagging API if finding tags are empty.
func (m *dualPathMapper) mapByTags(ctx context.Context, finding *Finding) (*ComponentMapping, error) {
	if finding.ResourceARN == "" {
		return nil, nil
	}

	// Try tags from the finding itself (embedded in Security Hub ASFF response).
	if mapping := m.matchTags(finding.ResourceTags, "finding-tag"); mapping != nil {
		return mapping, nil
	}

	// Fall back to Resource Groups Tagging API (only works in the same account).
	tags, err := m.getResourceTags(ctx, finding.ResourceARN, finding.Region)
	if err != nil {
		return nil, err
	}
	return m.matchTags(tags, "tag-api"), nil
}

// matchTags checks a tag map for Atmos stack/component tags.
func (m *dualPathMapper) matchTags(tags map[string]string, method string) *ComponentMapping {
	if len(tags) == 0 {
		return nil
	}

	stack := tags[m.tagMapping.StackTag]
	component := tags[m.tagMapping.ComponentTag]

	if stack == "" && component == "" {
		return nil
	}

	return &ComponentMapping{
		Stack:      stack,
		Component:  component,
		Mapped:     component != "",
		Confidence: ConfidenceExact,
		Method:     method,
	}
}

// mapByHeuristics implements Path B: multi-strategy heuristic pipeline.
func (m *dualPathMapper) mapByHeuristics(_ context.Context, finding *Finding) (*ComponentMapping, error) {
	// Strategy 1: Context tags — use Namespace/Tenant/Environment/Stage tags.
	if mapping := m.mapByContextTags(finding); mapping != nil {
		return mapping, nil
	}

	// Strategy 2: Account-level findings (AWS::::Account:123456789012).
	if mapping := m.mapByAccountID(finding); mapping != nil {
		return mapping, nil
	}

	// Strategy 3: ECR repository findings — extract repo/image name.
	if mapping := m.mapByECRRepo(finding); mapping != nil {
		return mapping, nil
	}

	// Strategy 4: Resource naming convention analysis (last segment heuristic).
	if mapping := m.mapByNamingConvention(finding); mapping != nil {
		return mapping, nil
	}

	// Strategy 5: Resource type to component mapping.
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

// mapByAccountID maps account-level findings (AWS::::Account:ID) to account names.
func (m *dualPathMapper) mapByAccountID(finding *Finding) *ComponentMapping {
	if !strings.HasPrefix(finding.ResourceARN, "AWS::::Account:") {
		return nil
	}

	// Extract account ID from the ARN.
	accountID := strings.TrimPrefix(finding.ResourceARN, "AWS::::Account:")
	if accountID == "" {
		return nil
	}

	// Look up account name from config.
	accountName := ""
	if m.accountMap != nil {
		accountName = m.accountMap[accountID]
	}
	if accountName == "" && finding.AccountID != "" {
		// Try finding.AccountID as well.
		accountName = m.accountMap[finding.AccountID]
	}

	if accountName == "" {
		return &ComponentMapping{
			Stack:      accountID,
			Mapped:     false,
			Confidence: ConfidenceNone,
			Method:     "account-level",
		}
	}

	return &ComponentMapping{
		Stack:      accountName,
		Component:  "account",
		Mapped:     true,
		Confidence: ConfidenceLow,
		Method:     "account-map",
	}
}

// mapByECRRepo extracts ECR repository name as component and resolves stack from account map.
// ARN format: arn:aws:ecr:region:account:repository/org/image-name/sha256:hash.
func (m *dualPathMapper) mapByECRRepo(finding *Finding) *ComponentMapping {
	if !strings.Contains(finding.ResourceARN, ":repository/") {
		return nil
	}

	// Extract everything after "repository/".
	idx := strings.Index(finding.ResourceARN, ":repository/")
	if idx == -1 {
		return nil
	}
	repoPath := finding.ResourceARN[idx+len(":repository/"):]

	// Remove SHA256 suffix if present.
	if shaIdx := strings.Index(repoPath, "/sha256:"); shaIdx != -1 {
		repoPath = repoPath[:shaIdx]
	}

	// The last segment of the repo path is the image/component name.
	parts := strings.Split(repoPath, "/")
	component := parts[len(parts)-1]
	if component == "" {
		return nil
	}

	// Resolve stack from account map using the finding's account ID.
	stack := ""
	if m.accountMap != nil && finding.AccountID != "" {
		stack = m.accountMap[finding.AccountID]
	}

	return &ComponentMapping{
		Stack:      stack,
		Component:  component,
		Mapped:     true,
		Confidence: ConfidenceLow,
		Method:     "ecr-repo",
	}
}

// mapByContextTags uses Cloud Posse context tags (Namespace, Tenant, Environment, Stage)
// from the finding's resource tags to reconstruct the naming prefix and extract the component.
// This is more reliable than the basic naming convention because it uses explicit tag values.
func (m *dualPathMapper) mapByContextTags(finding *Finding) *ComponentMapping {
	tags := finding.ResourceTags
	if len(tags) == 0 {
		return nil
	}

	name := tags["Name"]
	if name == "" {
		return nil
	}

	// Build the context prefix from tags: {namespace}-{tenant}-{environment}-{stage}-.
	namespace := tags["Namespace"]
	tenant := tags["Tenant"]
	environment := tags["Environment"]
	stage := tags["Stage"]

	// Need at least tenant + stage to construct a meaningful prefix.
	if tenant == "" || stage == "" {
		return nil
	}

	// Build prefix: namespace-tenant-environment-stage-.
	var prefixParts []string
	if namespace != "" {
		prefixParts = append(prefixParts, namespace)
	}
	prefixParts = append(prefixParts, tenant)
	if environment != "" {
		prefixParts = append(prefixParts, environment)
	}
	prefixParts = append(prefixParts, stage)
	prefix := strings.Join(prefixParts, nameSeparator) + nameSeparator

	// Strip the prefix from the Name to get the component name.
	if !strings.HasPrefix(name, prefix) {
		return nil
	}
	component := name[len(prefix):]
	if component == "" {
		return nil
	}

	// Build stack name from context: tenant-environment-stage.
	var stackParts []string
	stackParts = append(stackParts, tenant)
	if environment != "" {
		stackParts = append(stackParts, environment)
	}
	stackParts = append(stackParts, stage)
	stack := strings.Join(stackParts, nameSeparator)

	return &ComponentMapping{
		Stack:      stack,
		Component:  component,
		Mapped:     true,
		Confidence: ConfidenceHigh,
		Method:     "context-tags",
	}
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
	parts := strings.Split(name, nameSeparator)
	if len(parts) < minNamingParts {
		return nil
	}

	// Heuristic: the last segment is often the component type.
	// But multi-word components (e.g., "example-static-app-origin") break this.
	// Only use this heuristic at LOW confidence since it's unreliable.
	component := parts[len(parts)-1]
	// The middle segments form the stack identifier.
	stack := strings.Join(parts[1:len(parts)-1], nameSeparator)

	return &ComponentMapping{
		Stack:      stack,
		Component:  component,
		Mapped:     true,
		Confidence: ConfidenceLow,
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
		// Resolve stack from account map.
		stack := ""
		if m.accountMap != nil && finding.AccountID != "" {
			stack = m.accountMap[finding.AccountID]
		}
		return &ComponentMapping{
			Stack:      stack,
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
func resolveTagMapping(atmosConfig *schema.AtmosConfiguration) schema.AWSSecurityTagMapping {
	mapping := atmosConfig.AWS.Security.TagMapping
	defaults := schema.DefaultAWSSecurityTagMapping()

	if mapping.StackTag == "" {
		mapping.StackTag = defaults.StackTag
	}
	if mapping.ComponentTag == "" {
		mapping.ComponentTag = defaults.ComponentTag
	}

	return mapping
}
