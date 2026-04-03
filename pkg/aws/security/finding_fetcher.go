package security

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/securityhub"
	shtypes "github.com/aws/aws-sdk-go-v2/service/securityhub/types"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// FindingFetcher retrieves security findings from AWS security services.
type FindingFetcher interface {
	// FetchFindings retrieves findings matching the given options.
	FetchFindings(ctx context.Context, opts *QueryOptions) ([]Finding, error)

	// FetchComplianceStatus retrieves compliance status for a specific framework.
	FetchComplianceStatus(ctx context.Context, framework string, stack string) (*ComplianceReport, error)
}

// NewFindingFetcher creates a FindingFetcher based on the configured security sources.
// If authCtx is non-nil, AWS clients will use Atmos Auth credentials.
func NewFindingFetcher(atmosConfig *schema.AtmosConfiguration, authCtx *schema.AWSAuthContext) FindingFetcher {
	defer perf.Track(nil, "security.NewFindingFetcher")()

	clients := newAWSClientCache()
	if authCtx != nil {
		clients.WithAuthContext(authCtx)
	}

	return &awsFindingFetcher{
		atmosConfig: atmosConfig,
		clients:     clients,
		cache:       NewFindingsCache(),
	}
}

// Security fetcher constants.
const (
	securityHubPageSize   = 100 // Max findings per Security Hub GetFindings API call.
	complianceMaxFindings = 200 // Default max findings for compliance status.
	percentMultiplier     = 100 // Ratio-to-percentage multiplier.
	arnMinSegments        = 5   // Min colon-separated segments in an ARN.
)

// awsFindingFetcher implements FindingFetcher using AWS security services.
type awsFindingFetcher struct {
	atmosConfig *schema.AtmosConfiguration
	clients     *awsClientCache
	cache       *findingsCache
}

// FetchFindings retrieves security findings from Security Hub with the given filters.
// Results are cached by query options to reduce redundant AWS API calls.
func (f *awsFindingFetcher) FetchFindings(ctx context.Context, opts *QueryOptions) ([]Finding, error) {
	defer perf.Track(nil, "security.awsFindingFetcher.FetchFindings")()

	// Check cache first.
	if cached, hit := f.cache.GetFindings(opts); hit {
		log.Debug("Returning cached Security Hub findings", "count", len(cached))
		return cached, nil
	}

	region := f.resolveRegion(opts.Region)

	client, err := f.clients.getSecurityHubClient(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAISecurityFetchFailed, err)
	}

	filters := f.buildFindingFilters(opts)

	log.Debug("Fetching Security Hub findings",
		"region", region,
		"severity", opts.Severity,
		"source", opts.Source,
		"max_findings", opts.MaxFindings,
	)

	allFindings, err := f.paginateFindings(ctx, client, filters, opts.MaxFindings)
	if err != nil {
		return nil, err
	}

	log.Debug("Fetched Security Hub findings", "count", len(allFindings))

	// Store results in cache.
	f.cache.SetFindings(opts, allFindings)

	return allFindings, nil
}

// paginateFindings handles paginated retrieval of findings from Security Hub.
func (f *awsFindingFetcher) paginateFindings(
	ctx context.Context,
	client SecurityHubAPI,
	filters *shtypes.AwsSecurityFindingFilters,
	maxFindings int,
) ([]Finding, error) {
	pageSize := resolvePageSize(maxFindings)

	var allFindings []Finding
	var nextToken *string

	for {
		output, err := client.GetFindings(ctx, &securityhub.GetFindingsInput{
			Filters:    filters,
			MaxResults: aws.Int32(pageSize),
			NextToken:  nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrAISecurityFetchFailed, err)
		}

		for i := range output.Findings {
			allFindings = append(allFindings, normalizeSecurityHubFinding(&output.Findings[i]))
		}

		if output.NextToken == nil || reachedLimit(allFindings, maxFindings) {
			break
		}
		nextToken = output.NextToken
	}

	return trimToLimit(allFindings, maxFindings), nil
}

// resolvePageSize returns the appropriate page size for Security Hub queries.
func resolvePageSize(maxFindings int) int32 {
	pageSize := int32(securityHubPageSize)
	if maxFindings > 0 && maxFindings < securityHubPageSize {
		pageSize = int32(min(maxFindings, securityHubPageSize))
	}
	return pageSize
}

// reachedLimit checks if enough findings have been collected.
func reachedLimit(findings []Finding, maxFindings int) bool {
	return maxFindings > 0 && len(findings) >= maxFindings
}

// trimToLimit trims findings to the max limit if exceeded.
func trimToLimit(findings []Finding, maxFindings int) []Finding {
	if maxFindings > 0 && len(findings) > maxFindings {
		return findings[:maxFindings]
	}
	return findings
}

// FetchComplianceStatus retrieves compliance status for a specific framework from Security Hub.
// Results are cached by framework and stack to reduce redundant AWS API calls.
func (f *awsFindingFetcher) FetchComplianceStatus(ctx context.Context, framework string, stack string) (*ComplianceReport, error) {
	defer perf.Track(nil, "security.awsFindingFetcher.FetchComplianceStatus")()

	// Check cache first.
	if cached, hit := f.cache.GetCompliance(framework, stack); hit {
		log.Debug("Returning cached compliance report", "framework", framework, "stack", stack)
		return cached, nil
	}

	region := f.resolveRegion("")

	client, err := f.clients.getSecurityHubClient(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAISecurityFetchFailed, err)
	}

	// Find the enabled standard ARN matching the framework.
	standardARN, title, err := f.resolveFrameworkStandard(ctx, client, framework)
	if err != nil {
		return nil, err
	}
	if standardARN == "" {
		return nil, nil
	}

	// Fetch findings for this framework's compliance standard.
	opts := QueryOptions{
		Framework:   framework,
		MaxFindings: complianceMaxFindings,
		Severity:    []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInformational},
	}
	if stack != "" {
		opts.Stack = stack
	}

	findings, err := f.FetchFindings(ctx, &opts)
	if err != nil {
		return nil, err
	}

	// Count total controls for this standard to compute accurate compliance score.
	totalControls, err := f.countTotalControls(ctx, client, standardARN)
	if err != nil {
		log.Debug("Failed to count total controls, falling back to failing count", "error", err)
		totalControls = 0
	}

	// Build compliance report from findings.
	report := buildComplianceReport(findings, framework, title, stack, totalControls)

	// Store report in cache.
	f.cache.SetCompliance(framework, stack, report)

	return report, nil
}

// resolveRegion returns the region to use, falling back to config and then a default.
func (f *awsFindingFetcher) resolveRegion(override string) string {
	if override != "" {
		return override
	}
	// Fall back to config region, then auth context region, then default.
	if f.atmosConfig.AWS.Security.Region != "" {
		return f.atmosConfig.AWS.Security.Region
	}
	return "us-east-1"
}

// buildFindingFilters constructs Security Hub finding filters from query options.
func (f *awsFindingFetcher) buildFindingFilters(opts *QueryOptions) *shtypes.AwsSecurityFindingFilters {
	filters := &shtypes.AwsSecurityFindingFilters{
		// Only active findings (not archived/suppressed).
		WorkflowStatus: []shtypes.StringFilter{
			{
				Value:      aws.String(string(shtypes.WorkflowStatusNew)),
				Comparison: shtypes.StringFilterComparisonEquals,
			},
			{
				Value:      aws.String(string(shtypes.WorkflowStatusNotified)),
				Comparison: shtypes.StringFilterComparisonEquals,
			},
		},
		RecordState: []shtypes.StringFilter{
			{
				Value:      aws.String(string(shtypes.RecordStateActive)),
				Comparison: shtypes.StringFilterComparisonEquals,
			},
		},
	}

	// Severity filter.
	if len(opts.Severity) > 0 {
		for _, sev := range opts.Severity {
			filters.SeverityLabel = append(filters.SeverityLabel, shtypes.StringFilter{
				Value:      aws.String(string(sev)),
				Comparison: shtypes.StringFilterComparisonEquals,
			})
		}
	}

	// Source filter — map Source enum to product ARN prefixes.
	if opts.Source != "" && opts.Source != SourceAll {
		productFilters := sourceToProductFilters(opts.Source)
		filters.ProductName = productFilters
	}

	// Framework filter — use compliance standard.
	if opts.Framework != "" {
		standardID := frameworkToStandardID(opts.Framework)
		if standardID != "" {
			filters.ComplianceAssociatedStandardsId = []shtypes.StringFilter{
				{
					Value:      aws.String(standardID),
					Comparison: shtypes.StringFilterComparisonPrefix,
				},
			}
		}
	}

	return filters
}

// resolveFrameworkStandard finds the enabled Security Hub standard matching a framework name.
func (f *awsFindingFetcher) resolveFrameworkStandard(ctx context.Context, client SecurityHubAPI, framework string) (string, string, error) {
	defer perf.Track(nil, "security.awsFindingFetcher.resolveFrameworkStandard")()

	output, err := client.GetEnabledStandards(ctx, &securityhub.GetEnabledStandardsInput{})
	if err != nil {
		return "", "", fmt.Errorf("%w: GetEnabledStandards: %w", errUtils.ErrAISecurityFetchFailed, err)
	}

	targetID := frameworkToStandardID(framework)
	if targetID == "" {
		return "", "", nil
	}

	for _, std := range output.StandardsSubscriptions {
		if std.StandardsArn != nil && strings.Contains(*std.StandardsArn, targetID) {
			title := frameworkToTitle(framework)
			return *std.StandardsArn, title, nil
		}
	}

	return "", "", nil
}

// normalizeSecurityHubFinding converts an AWS Security Hub finding to our normalized Finding type.
func normalizeSecurityHubFinding(f *shtypes.AwsSecurityFinding) Finding {
	finding := Finding{
		ID:          aws.ToString(f.Id),
		Title:       aws.ToString(f.Title),
		Description: aws.ToString(f.Description),
		Source:      detectSource(f),
		AccountID:   aws.ToString(f.AwsAccountId),
	}

	// Severity.
	if f.Severity != nil {
		finding.Severity = normalizeSeverityLabel(f.Severity.Label)
	}

	// Resource info (use first resource if multiple).
	if len(f.Resources) > 0 {
		res := f.Resources[0]
		finding.ResourceARN = aws.ToString(res.Id)
		finding.ResourceType = aws.ToString(res.Type)
		finding.Region = aws.ToString(res.Region)
		// Extract resource tags directly from the finding — no separate API call needed.
		if len(res.Tags) > 0 {
			finding.ResourceTags = res.Tags
		}
	}

	// Compliance standard.
	if f.Compliance != nil && len(f.Compliance.AssociatedStandards) > 0 {
		finding.ComplianceStandard = aws.ToString(f.Compliance.AssociatedStandards[0].StandardsId)
	}

	// Timestamps (Security Hub returns ISO 8601 strings).
	if f.CreatedAt != nil {
		if t, err := time.Parse(time.RFC3339, *f.CreatedAt); err == nil {
			finding.CreatedAt = t
		}
	}
	if f.UpdatedAt != nil {
		if t, err := time.Parse(time.RFC3339, *f.UpdatedAt); err == nil {
			finding.UpdatedAt = t
		}
	}

	return finding
}

// detectSource determines the AWS service that produced a Security Hub finding.
func detectSource(f *shtypes.AwsSecurityFinding) Source {
	productName := strings.ToLower(aws.ToString(f.ProductName))

	switch {
	case strings.Contains(productName, "security hub"):
		return SourceSecurityHub
	case strings.Contains(productName, "config"):
		return SourceConfig
	case strings.Contains(productName, "inspector"):
		return SourceInspector
	case strings.Contains(productName, "guardduty"):
		return SourceGuardDuty
	case strings.Contains(productName, "macie"):
		return SourceMacie
	case strings.Contains(productName, "access analyzer"):
		return SourceAccessAnalyzer
	default:
		return SourceSecurityHub
	}
}

// normalizeSeverityLabel converts AWS severity label to our Severity type.
func normalizeSeverityLabel(label shtypes.SeverityLabel) Severity {
	switch label {
	case shtypes.SeverityLabelCritical:
		return SeverityCritical
	case shtypes.SeverityLabelHigh:
		return SeverityHigh
	case shtypes.SeverityLabelMedium:
		return SeverityMedium
	case shtypes.SeverityLabelLow:
		return SeverityLow
	case shtypes.SeverityLabelInformational:
		return SeverityInformational
	default:
		return SeverityInformational
	}
}

// sourceToProductFilters returns Security Hub product name filters for a source.
func sourceToProductFilters(source Source) []shtypes.StringFilter {
	productNames := map[Source]string{
		SourceSecurityHub:    "Security Hub",
		SourceConfig:         "Config",
		SourceInspector:      "Inspector",
		SourceGuardDuty:      "GuardDuty",
		SourceMacie:          "Macie",
		SourceAccessAnalyzer: "Access Analyzer",
	}

	if name, ok := productNames[source]; ok {
		return []shtypes.StringFilter{
			{
				Value:      aws.String(name),
				Comparison: shtypes.StringFilterComparisonEquals,
			},
		}
	}
	return nil
}

// frameworkToStandardID maps framework names to Security Hub standard ID prefixes.
func frameworkToStandardID(framework string) string {
	standards := map[string]string{
		"cis-aws": "cis-aws-foundations-benchmark",
		"pci-dss": "pci-dss",
		"nist":    "nist-800-53",
		"soc2":    "soc2",
		"hipaa":   "hipaa",
	}
	return standards[strings.ToLower(framework)]
}

// frameworkToTitle returns a human-readable title for a compliance framework.
func frameworkToTitle(framework string) string {
	titles := map[string]string{
		"cis-aws": "CIS AWS Foundations Benchmark",
		"pci-dss": "PCI DSS",
		"nist":    "NIST 800-53",
		"soc2":    "SOC 2",
		"hipaa":   "HIPAA",
	}
	if title, ok := titles[strings.ToLower(framework)]; ok {
		return title
	}
	return framework
}

// controlsPageSize is the max results per DescribeStandardsControls API call.
const controlsPageSize = 100

// countTotalControls paginates through DescribeStandardsControls to count all controls for a standard.
func (f *awsFindingFetcher) countTotalControls(ctx context.Context, client SecurityHubAPI, subscriptionARN string) (int, error) {
	defer perf.Track(nil, "security.awsFindingFetcher.countTotalControls")()

	var total int
	var nextToken *string

	for {
		output, err := client.DescribeStandardsControls(ctx, &securityhub.DescribeStandardsControlsInput{
			StandardsSubscriptionArn: &subscriptionARN,
			MaxResults:               aws.Int32(controlsPageSize),
			NextToken:                nextToken,
		})
		if err != nil {
			return 0, fmt.Errorf("%w: DescribeStandardsControls: %w", errUtils.ErrAISecurityFetchFailed, err)
		}

		total += len(output.Controls)

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return total, nil
}

// buildComplianceReport constructs a ComplianceReport from Security Hub findings.
func buildComplianceReport(findings []Finding, framework, title, stack string, totalControls int) *ComplianceReport {
	report := &ComplianceReport{
		GeneratedAt:    time.Now().UTC(),
		Stack:          stack,
		Framework:      framework,
		FrameworkTitle: title,
	}

	// Deduplicate by compliance control ID.
	controlMap := make(map[string]*ComplianceControl)
	for i := range findings {
		f := &findings[i]
		controlID := f.ComplianceStandard
		if controlID == "" {
			controlID = f.ID
		}
		if _, exists := controlMap[controlID]; !exists {
			controlMap[controlID] = &ComplianceControl{
				ControlID: controlID,
				Title:     f.Title,
				Severity:  f.Severity,
			}
		}
	}

	// All fetched findings are failing controls.
	for _, ctrl := range controlMap {
		report.FailingDetails = append(report.FailingDetails, *ctrl)
	}

	report.FailingControls = len(report.FailingDetails)

	// Use the actual total from DescribeStandardsControls when available.
	// Fall back to failing count if the API total is unavailable or less than failing.
	if totalControls >= report.FailingControls {
		report.TotalControls = totalControls
	} else {
		report.TotalControls = report.FailingControls
	}

	report.PassingControls = report.TotalControls - report.FailingControls
	if report.TotalControls > 0 {
		report.ScorePercent = float64(report.PassingControls) / float64(report.TotalControls) * percentMultiplier
	}

	return report
}
