package security

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/inspector2"
	itypes "github.com/aws/aws-sdk-go-v2/service/inspector2/types"
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
	securityHubPageSize      = 100 // Max findings per Security Hub GetFindings API call.
	inspector2PageSize       = 100 // Max findings per Inspector2 ListFindings API call.
	complianceMaxFindings    = 200 // Default max findings for compliance status.
	percentMultiplier        = 100 // Ratio-to-percentage multiplier.
	arnMinSegments           = 5   // Min colon-separated segments in an ARN.
	securityHubSeverityScale = 10.0
	awsErrorWrapFormat       = "%w: %w"
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

	if cached, hit := f.cache.GetFindings(opts); hit {
		log.Debug("Returning cached security findings", "count", len(cached))
		return cached, nil
	}

	var allFindings []Finding
	var err error
	switch opts.Source {
	case SourceInspector:
		allFindings, err = f.fetchInspector2Findings(ctx, opts)
	case SourceAll:
		securityHubFindings, fetchErr := f.fetchSecurityHubFindings(ctx, opts)
		if fetchErr != nil {
			return nil, fetchErr
		}
		inspectorFindings, fetchErr := f.fetchInspector2Findings(ctx, opts)
		if fetchErr != nil {
			return nil, fetchErr
		}
		allFindings = dedupeFindingsPreferNativeInspector(append(securityHubFindings, inspectorFindings...))
		allFindings = sortedFindings(allFindings)
		allFindings = trimToLimit(allFindings, opts.MaxFindings)
	default:
		allFindings, err = f.fetchSecurityHubFindings(ctx, opts)
	}
	if err != nil {
		return nil, err
	}

	f.cache.SetFindings(opts, allFindings)
	return allFindings, nil
}

// fetchSecurityHubFindings retrieves findings from Security Hub with the given filters.
func (f *awsFindingFetcher) fetchSecurityHubFindings(ctx context.Context, opts *QueryOptions) ([]Finding, error) {
	region := f.resolveRegion(opts.Region)
	client, err := f.clients.getSecurityHubClient(ctx, region)
	if err != nil {
		return nil, fmt.Errorf(awsErrorWrapFormat, errUtils.ErrAWSSecurityFetchFailed, err)
	}

	filters := f.buildFindingFilters(opts)

	log.Debug(
		"Fetching Security Hub findings",
		"region", region,
		"severity", opts.Severity,
		"source", opts.Source,
		"max_findings", opts.MaxFindings,
	)

	allFindings, err := f.paginateFindings(ctx, client, filters, opts.MaxFindings)
	if err != nil {
		return nil, wrapAWSServiceError("GetFindings", err)
	}

	log.Debug("Fetched Security Hub findings", "count", len(allFindings))
	return allFindings, nil
}

// fetchInspector2Findings retrieves native Amazon Inspector2 findings.
func (f *awsFindingFetcher) fetchInspector2Findings(ctx context.Context, opts *QueryOptions) ([]Finding, error) {
	if opts.Framework != "" {
		return nil, nil
	}

	region := f.resolveRegion(opts.Region)
	client, err := f.clients.getInspector2Client(ctx, region)
	if err != nil {
		return nil, fmt.Errorf(awsErrorWrapFormat, errUtils.ErrAWSSecurityFetchFailed, err)
	}

	filters := buildInspector2Filters(opts)
	log.Debug(
		"Fetching Inspector2 findings",
		"region", region,
		"severity", opts.Severity,
		"max_findings", opts.MaxFindings,
	)

	findings, err := f.paginateInspector2Findings(ctx, client, filters, opts.MaxFindings)
	if err != nil {
		return nil, wrapAWSServiceError("ListFindings", err)
	}

	log.Debug("Fetched Inspector2 findings", "count", len(findings))
	return findings, nil
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
			return nil, fmt.Errorf(awsErrorWrapFormat, errUtils.ErrAWSSecurityFetchFailed, err)
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

// paginateInspector2Findings handles paginated retrieval of native Inspector2 findings.
func (f *awsFindingFetcher) paginateInspector2Findings(
	ctx context.Context,
	client Inspector2API,
	filters *itypes.FilterCriteria,
	maxFindings int,
) ([]Finding, error) {
	pageSize := int32(inspector2PageSize)
	if maxFindings > 0 && maxFindings < inspector2PageSize {
		pageSize = int32(maxFindings)
	}

	var allFindings []Finding
	var nextToken *string
	for {
		output, err := client.ListFindings(ctx, &inspector2.ListFindingsInput{
			FilterCriteria: filters,
			MaxResults:     aws.Int32(pageSize),
			NextToken:      nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf(awsErrorWrapFormat, errUtils.ErrAWSSecurityFetchFailed, err)
		}

		for i := range output.Findings {
			allFindings = append(allFindings, normalizeInspector2Finding(&output.Findings[i]))
		}

		if output.NextToken == nil || reachedLimit(allFindings, maxFindings) {
			break
		}
		nextToken = output.NextToken
	}

	return trimToLimit(allFindings, maxFindings), nil
}

// buildInspector2Filters constructs Inspector2 filters from query options.
func buildInspector2Filters(opts *QueryOptions) *itypes.FilterCriteria {
	filters := &itypes.FilterCriteria{
		FindingStatus: []itypes.StringFilter{
			{
				Value:      aws.String(string(itypes.FindingStatusActive)),
				Comparison: itypes.StringComparisonEquals,
			},
		},
	}
	for _, sev := range opts.Severity {
		filters.Severity = append(filters.Severity, itypes.StringFilter{
			Value:      aws.String(string(sev)),
			Comparison: itypes.StringComparisonEquals,
		})
	}
	return filters
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
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAWSSecurityFetchFailed, err)
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
	if f.clients != nil && f.clients.authContext != nil && f.clients.authContext.Region != "" {
		return f.clients.authContext.Region
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
	// Security Hub standard IDs include a type prefix like "ruleset/" or "standards/"
	// (e.g., "ruleset/cis-aws-foundations-benchmark/v/1.2.0"). We use PREFIX matching
	// with the full path including the type prefix.
	if opts.Framework != "" {
		standardIDs := frameworkToStandardIDs(opts.Framework)
		for _, id := range standardIDs {
			filters.ComplianceAssociatedStandardsId = append(
				filters.ComplianceAssociatedStandardsId,
				shtypes.StringFilter{
					Value:      aws.String(id),
					Comparison: shtypes.StringFilterComparisonPrefix,
				},
			)
		}
	}

	return filters
}

// resolveFrameworkStandard finds the enabled Security Hub standard matching a framework name.
func (f *awsFindingFetcher) resolveFrameworkStandard(ctx context.Context, client SecurityHubAPI, framework string) (string, string, error) {
	defer perf.Track(nil, "security.awsFindingFetcher.resolveFrameworkStandard")()

	output, err := client.GetEnabledStandards(ctx, &securityhub.GetEnabledStandardsInput{})
	if err != nil {
		return "", "", wrapAWSServiceError("GetEnabledStandards", err)
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
		Region:      aws.ToString(f.Region),
		SourceURL:   aws.ToString(f.SourceUrl),
	}

	// Severity.
	if f.Severity != nil {
		finding.Severity = normalizeSeverityLabel(f.Severity.Label)
		finding.SourceSeverity = normalizeSecurityHubSourceSeverity(f.Severity)
	}

	// Resource info (use first resource if multiple).
	if len(f.Resources) > 0 {
		res := f.Resources[0]
		finding.ResourceARN = aws.ToString(res.Id)
		finding.ResourceType = aws.ToString(res.Type)
		if res.Region != nil {
			finding.Region = aws.ToString(res.Region)
		}
		// Extract resource tags directly from the finding — no separate API call needed.
		if len(res.Tags) > 0 {
			finding.ResourceTags = res.Tags
		}
	}

	// Compliance standard and control ID.
	if f.Compliance != nil {
		if len(f.Compliance.AssociatedStandards) > 0 {
			finding.ComplianceStandard = aws.ToString(f.Compliance.AssociatedStandards[0].StandardsId)
			finding.ComplianceStandards = make([]ComplianceStandard, 0, len(f.Compliance.AssociatedStandards))
			for _, standard := range f.Compliance.AssociatedStandards {
				finding.ComplianceStandards = append(finding.ComplianceStandards, parseComplianceStandard(aws.ToString(standard.StandardsId)))
			}
		}
		finding.SecurityControlID = aws.ToString(f.Compliance.SecurityControlId)
	}

	finding.SourceLifecycle = normalizeSecurityHubLifecycle(f)
	finding.SourceTimestamps = normalizeSecurityHubTimestamps(f)
	if finding.SourceTimestamps != nil {
		if finding.SourceTimestamps.CreatedAt != nil {
			finding.CreatedAt = *finding.SourceTimestamps.CreatedAt
		}
		if finding.SourceTimestamps.UpdatedAt != nil {
			finding.UpdatedAt = *finding.SourceTimestamps.UpdatedAt
		}
	}
	finding.SourceRemediation = normalizeSecurityHubRemediation(f.Remediation)
	finding.Vulnerability = normalizeSecurityHubVulnerability(f.Vulnerabilities)

	return finding
}

// normalizeInspector2Finding converts a native Amazon Inspector2 finding.
func normalizeInspector2Finding(f *itypes.Finding) Finding {
	finding := Finding{
		ID:          aws.ToString(f.FindingArn),
		Title:       aws.ToString(f.Title),
		Description: aws.ToString(f.Description),
		Severity:    normalizeInspector2Severity(f.Severity),
		Source:      SourceInspector,
		AccountID:   aws.ToString(f.AwsAccountId),
		SourceSeverity: &SourceSeverity{
			Label: string(f.Severity),
		},
		SourceLifecycle: &SourceLifecycle{
			InspectorStatus: string(f.Status),
		},
		SourceTimestamps: &SourceTimestamps{
			FirstObservedAt: f.FirstObservedAt,
			LastObservedAt:  f.LastObservedAt,
			UpdatedAt:       f.UpdatedAt,
		},
	}
	if f.InspectorScore != nil {
		finding.SourceSeverity.Score = f.InspectorScore
	}
	if len(f.Resources) > 0 {
		res := f.Resources[0]
		finding.ResourceARN = aws.ToString(res.Id)
		finding.ResourceType = string(res.Type)
		finding.Region = aws.ToString(res.Region)
		if len(res.Tags) > 0 {
			finding.ResourceTags = res.Tags
		}
	}
	if f.FirstObservedAt != nil {
		finding.CreatedAt = *f.FirstObservedAt
	}
	if f.UpdatedAt != nil {
		finding.UpdatedAt = *f.UpdatedAt
	}
	finding.SourceRemediation = normalizeInspector2Remediation(f.Remediation)
	finding.Vulnerability = normalizeInspector2Vulnerability(f)
	if finding.SourceRemediation != nil {
		finding.SourceURL = finding.SourceRemediation.URL
	}
	if finding.SourceURL == "" && f.PackageVulnerabilityDetails != nil {
		finding.SourceURL = aws.ToString(f.PackageVulnerabilityDetails.SourceUrl)
	}
	if finding.SourceURL == "" {
		finding.SourceURL = inspector2FindingURL(finding.Region, finding.ID)
	}
	return finding
}

func normalizeSecurityHubSourceSeverity(sev *shtypes.Severity) *SourceSeverity {
	if sev == nil {
		return nil
	}
	out := &SourceSeverity{Label: aws.ToString(sev.Original)}
	if sev.Normalized != nil {
		score := float64(*sev.Normalized) / securityHubSeverityScale
		out.Score = &score
	}
	if out.Score == nil && out.Label == "" {
		return nil
	}
	return out
}

func normalizeSecurityHubLifecycle(f *shtypes.AwsSecurityFinding) *SourceLifecycle {
	out := &SourceLifecycle{
		RecordState: string(f.RecordState),
	}
	if f.Workflow != nil {
		out.WorkflowStatus = string(f.Workflow.Status)
	}
	if f.Compliance != nil {
		out.ComplianceStatus = string(f.Compliance.Status)
	}
	if out.WorkflowStatus == "" && out.RecordState == "" && out.ComplianceStatus == "" {
		return nil
	}
	return out
}

func normalizeSecurityHubTimestamps(f *shtypes.AwsSecurityFinding) *SourceTimestamps {
	out := &SourceTimestamps{
		FirstObservedAt: parseAWSRFC3339Time(aws.ToString(f.FirstObservedAt)),
		LastObservedAt:  parseAWSRFC3339Time(aws.ToString(f.LastObservedAt)),
		UpdatedAt:       parseAWSRFC3339Time(aws.ToString(f.UpdatedAt)),
		CreatedAt:       parseAWSRFC3339Time(aws.ToString(f.CreatedAt)),
	}
	if out.FirstObservedAt == nil && out.LastObservedAt == nil && out.UpdatedAt == nil && out.CreatedAt == nil {
		return nil
	}
	return out
}

func parseAWSRFC3339Time(value string) *time.Time {
	if value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &t
}

func normalizeSecurityHubRemediation(remediation *shtypes.Remediation) *SourceRemediation {
	if remediation == nil || remediation.Recommendation == nil {
		return nil
	}
	out := &SourceRemediation{
		Text: aws.ToString(remediation.Recommendation.Text),
		URL:  aws.ToString(remediation.Recommendation.Url),
	}
	if out.Text == "" && out.URL == "" {
		return nil
	}
	return out
}

func normalizeInspector2Remediation(remediation *itypes.Remediation) *SourceRemediation {
	if remediation == nil || remediation.Recommendation == nil {
		return nil
	}
	out := &SourceRemediation{
		Text: aws.ToString(remediation.Recommendation.Text),
		URL:  aws.ToString(remediation.Recommendation.Url),
	}
	if out.Text == "" && out.URL == "" {
		return nil
	}
	return out
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

func normalizeInspector2Severity(sev itypes.Severity) Severity {
	switch sev {
	case itypes.SeverityCritical:
		return SeverityCritical
	case itypes.SeverityHigh:
		return SeverityHigh
	case itypes.SeverityMedium:
		return SeverityMedium
	case itypes.SeverityLow:
		return SeverityLow
	case itypes.SeverityInformational:
		return SeverityInformational
	default:
		return SeverityInformational
	}
}

func normalizeSecurityHubVulnerability(vulnerabilities []shtypes.Vulnerability) *VulnerabilityDetails { //nolint:revive // cyclomatic: preserves independent optional vulnerability fields from ASFF.
	if len(vulnerabilities) == 0 {
		return nil
	}
	v := vulnerabilities[0]
	out := &VulnerabilityDetails{
		ID:            aws.ToString(v.Id),
		ReferenceURLs: append([]string(nil), v.ReferenceUrls...),
		CWEIDs:        cweIDsFromRelated(v.RelatedVulnerabilities),
	}
	if strings.HasPrefix(strings.ToUpper(out.ID), "CVE-") {
		out.CVEID = out.ID
	}
	if v.EpssScore != nil {
		out.EPSSScore = *v.EpssScore
	}
	for _, cvss := range v.Cvss {
		score := CVSSScore{
			Vector:  aws.ToString(cvss.BaseVector),
			Source:  aws.ToString(cvss.Source),
			Version: aws.ToString(cvss.Version),
		}
		if cvss.BaseScore != nil {
			score.BaseScore = *cvss.BaseScore
		}
		out.CVSS = append(out.CVSS, score)
	}
	for _, pkg := range v.VulnerablePackages {
		out.Packages = append(out.Packages, VulnerablePackage{
			Name:           aws.ToString(pkg.Name),
			Version:        aws.ToString(pkg.Version),
			FixedInVersion: aws.ToString(pkg.FixedInVersion),
			PackageManager: aws.ToString(pkg.PackageManager),
			Remediation:    aws.ToString(pkg.Remediation),
			FilePath:       aws.ToString(pkg.FilePath),
		})
	}
	populatePrimaryPackage(out)
	if out.ID == "" && out.EPSSScore == 0 && len(out.Packages) == 0 && len(out.CWEIDs) == 0 && len(out.ReferenceURLs) == 0 && len(out.CVSS) == 0 {
		return nil
	}
	return out
}

func normalizeInspector2Vulnerability(f *itypes.Finding) *VulnerabilityDetails { //nolint:revive,cyclop // Preserves independent optional Inspector vulnerability fields.
	if f.PackageVulnerabilityDetails == nil && f.Epss == nil {
		return nil
	}
	out := &VulnerabilityDetails{}
	if f.PackageVulnerabilityDetails != nil {
		pkgDetails := f.PackageVulnerabilityDetails
		out.ID = aws.ToString(pkgDetails.VulnerabilityId)
		if strings.HasPrefix(strings.ToUpper(out.ID), "CVE-") {
			out.CVEID = out.ID
		}
		out.CWEIDs = cweIDsFromRelated(pkgDetails.RelatedVulnerabilities)
		out.ReferenceURLs = append([]string(nil), pkgDetails.ReferenceUrls...)
		if sourceURL := aws.ToString(pkgDetails.SourceUrl); sourceURL != "" {
			out.ReferenceURLs = appendUniqueString(out.ReferenceURLs, sourceURL)
		}
		for _, cvss := range pkgDetails.Cvss {
			score := CVSSScore{
				Vector:  aws.ToString(cvss.ScoringVector),
				Source:  aws.ToString(cvss.Source),
				Version: aws.ToString(cvss.Version),
			}
			if cvss.BaseScore != nil {
				score.BaseScore = *cvss.BaseScore
			}
			out.CVSS = append(out.CVSS, score)
		}
		for _, pkg := range pkgDetails.VulnerablePackages {
			out.Packages = append(out.Packages, VulnerablePackage{
				Name:           aws.ToString(pkg.Name),
				Version:        aws.ToString(pkg.Version),
				FixedInVersion: aws.ToString(pkg.FixedInVersion),
				PackageManager: string(pkg.PackageManager),
				Remediation:    aws.ToString(pkg.Remediation),
				FilePath:       aws.ToString(pkg.FilePath),
			})
		}
	}
	if f.Epss != nil {
		out.EPSSScore = f.Epss.Score
	}
	populatePrimaryPackage(out)
	if out.ID == "" && out.EPSSScore == 0 && len(out.Packages) == 0 && len(out.CWEIDs) == 0 && len(out.ReferenceURLs) == 0 && len(out.CVSS) == 0 {
		return nil
	}
	return out
}

func populatePrimaryPackage(v *VulnerabilityDetails) {
	if v == nil || len(v.Packages) == 0 {
		return
	}
	v.PackageName = v.Packages[0].Name
	v.PackageVersion = v.Packages[0].Version
	v.FixedInVersion = v.Packages[0].FixedInVersion
}

func cweIDsFromRelated(values []string) []string {
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, value := range values {
		upper := strings.ToUpper(value)
		if !strings.HasPrefix(upper, "CWE-") {
			continue
		}
		if _, ok := seen[upper]; ok {
			continue
		}
		seen[upper] = struct{}{}
		out = append(out, upper)
	}
	sort.Strings(out)
	return out
}

func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func parseComplianceStandard(id string) ComplianceStandard {
	if id == "" {
		return ComplianceStandard{}
	}
	out := ComplianceStandard{ID: id}
	parts := strings.Split(id, "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "v" {
			out.Version = parts[i+1]
			break
		}
	}
	for _, part := range parts {
		if part == "ruleset" || part == "standards" || part == "v" || part == out.Version {
			continue
		}
		out.Name = part
		break
	}
	if out.Name == "" {
		out.Name = id
	}
	return out
}

func inspector2FindingURL(region, findingARN string) string {
	if region == "" || findingARN == "" {
		return ""
	}
	return fmt.Sprintf("https://%s.console.aws.amazon.com/inspector/v2/home?region=%s#/findings/%s",
		region, url.QueryEscape(region), url.PathEscape(findingARN))
}

func dedupeFindingsPreferNativeInspector(findings []Finding) []Finding {
	if len(findings) == 0 {
		return nil
	}
	byKey := make(map[string]Finding, len(findings))
	order := make([]string, 0, len(findings))
	for i := range findings {
		finding := &findings[i]
		key := dedupeFindingKey(finding)
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
			byKey[key] = *finding
			continue
		}
		existing := byKey[key]
		if isNativeInspectorFinding(finding) && !isNativeInspectorFinding(&existing) {
			byKey[key] = *finding
		}
	}
	out := make([]Finding, 0, len(order))
	for _, key := range order {
		out = append(out, byKey[key])
	}
	return out
}

func dedupeFindingKey(f *Finding) string {
	if f == nil {
		return ""
	}
	if f.Source == SourceInspector && f.Vulnerability != nil && f.Vulnerability.ID != "" && f.ResourceARN != "" {
		return "inspector:" + f.Vulnerability.ID + ":" + f.ResourceARN
	}
	return string(f.Source) + ":" + f.ID
}

func isNativeInspectorFinding(f *Finding) bool {
	return f != nil && f.Source == SourceInspector && f.SourceLifecycle != nil && f.SourceLifecycle.InspectorStatus != ""
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

// Framework name constants used across mapping, filtering, and display functions.
const (
	frameworkCISAWS = "cis-aws"
	frameworkPCIDSS = "pci-dss"
	frameworkNIST   = "nist"
	frameworkSOC2   = "soc2"
	frameworkHIPAA  = "hipaa"
)

// frameworkToStandardID maps framework names to Security Hub standard ID prefixes.
// Used by resolveFrameworkStandard for ARN matching (no type prefix needed).
func frameworkToStandardID(framework string) string {
	standards := map[string]string{
		frameworkCISAWS: "cis-aws-foundations-benchmark",
		frameworkPCIDSS: frameworkPCIDSS,
		frameworkNIST:   "nist-800-53",
		frameworkSOC2:   frameworkSOC2,
		frameworkHIPAA:  frameworkHIPAA,
	}
	return standards[strings.ToLower(framework)]
}

// frameworkToStandardIDs maps framework names to full Security Hub standard ID prefixes
// including the type prefix (ruleset/ or standards/). Some frameworks appear under both
// prefixes, so multiple entries are returned for OR matching.
func frameworkToStandardIDs(framework string) []string {
	standards := map[string][]string{
		frameworkCISAWS: {"ruleset/cis-aws-foundations-benchmark", "standards/cis-aws-foundations-benchmark"},
		frameworkPCIDSS: {"standards/pci-dss"},
		frameworkNIST:   {"standards/nist-800-53"},
		frameworkSOC2:   {"standards/soc2"},
		frameworkHIPAA:  {"standards/hipaa"},
	}
	return standards[strings.ToLower(framework)]
}

// frameworkToTitle returns a human-readable title for a compliance framework.
func frameworkToTitle(framework string) string {
	titles := map[string]string{
		frameworkCISAWS: "CIS AWS Foundations Benchmark",
		frameworkPCIDSS: "PCI DSS",
		frameworkNIST:   "NIST 800-53",
		frameworkSOC2:   "SOC 2",
		frameworkHIPAA:  "HIPAA",
	}
	if title, ok := titles[strings.ToLower(framework)]; ok {
		return title
	}
	return framework
}

// controlsPageSize is the max results per ListSecurityControlDefinitions API call.
const controlsPageSize = 100

// countTotalControls paginates through ListSecurityControlDefinitions to count all controls
// for a standard. Uses the standards ARN (not subscription ARN) which works in delegated admin mode.
func (f *awsFindingFetcher) countTotalControls(ctx context.Context, client SecurityHubAPI, standardsARN string) (int, error) {
	defer perf.Track(nil, "security.awsFindingFetcher.countTotalControls")()

	var total int
	var nextToken *string

	for {
		output, err := client.ListSecurityControlDefinitions(ctx, &securityhub.ListSecurityControlDefinitionsInput{
			StandardsArn: &standardsARN,
			MaxResults:   aws.Int32(controlsPageSize),
			NextToken:    nextToken,
		})
		if err != nil {
			return 0, fmt.Errorf("%w: ListSecurityControlDefinitions: %w", errUtils.ErrAWSSecurityFetchFailed, err)
		}

		total += len(output.SecurityControlDefinitions)

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

	// Deduplicate by security control ID (e.g., "EC2.18", "IAM.4").
	// SecurityControlID is per-control; ComplianceStandard is per-framework and
	// would collapse multiple failing controls under the same framework.
	controlMap := make(map[string]*ComplianceControl)
	for i := range findings {
		f := &findings[i]
		controlID := f.SecurityControlID
		if controlID == "" {
			controlID = f.ComplianceStandard
		}
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

// wrapAWSServiceError detects common AWS service errors and returns user-friendly messages
// with actionable hints. Falls back to generic error wrapping for unknown errors.
func wrapAWSServiceError(operation string, err error) error {
	if err == nil {
		return nil
	}

	msg := err.Error()

	// Security Hub not enabled.
	if strings.Contains(msg, "InvalidAccessException") || strings.Contains(msg, "not subscribed") ||
		strings.Contains(msg, "Security Hub is not enabled") {
		return errUtils.Build(errUtils.ErrAWSSecurityFetchFailed).
			WithCause(err).
			WithExplanation("AWS Security Hub is not enabled in this account/region").
			WithHint("Enable Security Hub: `aws securityhub enable-security-hub --region <region>`").
			WithHint("Or deploy the `aws-security-hub` component via Atmos").
			WithHint("See https://docs.aws.amazon.com/securityhub/latest/userguide/securityhub-enable.html").
			Err()
	}

	// Access denied / insufficient permissions.
	if strings.Contains(msg, "AccessDeniedException") || strings.Contains(msg, "is not authorized") ||
		strings.Contains(msg, "Access Denied") {
		return errUtils.Build(errUtils.ErrAWSSecurityFetchFailed).
			WithCause(err).
			WithExplanationf("Insufficient permissions for %s", operation).
			WithHint("Ensure the IAM role has `securityhub:GetFindings`, `securityhub:GetEnabledStandards`, and `securityhub:ListSecurityControlDefinitions` permissions").
			WithHint("If using delegated admin, verify the `identity` in `aws.security` targets the correct account").
			Err()
	}

	// Invalid region or endpoint.
	if strings.Contains(msg, "UnrecognizedClientException") || strings.Contains(msg, "Could not connect") {
		return errUtils.Build(errUtils.ErrAWSSecurityFetchFailed).
			WithCause(err).
			WithExplanation("Cannot connect to AWS Security Hub in the configured region").
			WithHint("Check `aws.security.region` in atmos.yaml — it should be the Security Hub aggregation region").
			WithHint("Verify the region with: `aws securityhub describe-hub --region <region>`").
			Err()
	}

	// Generic fallback.
	return fmt.Errorf("%w: %s: %w", errUtils.ErrAWSSecurityFetchFailed, operation, err)
}
