package atmos

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/aws/security"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// defaultToolMaxFindings is the default max findings for AI tool queries.
const defaultToolMaxFindings = 20

// ListFindingsTool lists security findings from AWS Security Hub.
type ListFindingsTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewListFindingsTool creates a new list findings tool.
func NewListFindingsTool(atmosConfig *schema.AtmosConfiguration) *ListFindingsTool {
	return &ListFindingsTool{atmosConfig: atmosConfig}
}

// Name returns the tool name.
func (t *ListFindingsTool) Name() string {
	return "atmos_list_findings"
}

// Description returns the tool description.
func (t *ListFindingsTool) Description() string {
	return "List security findings from AWS Security Hub for Atmos stacks. " +
		"Returns findings filtered by severity, source service, stack, and component. " +
		"Use this to understand the security posture of your infrastructure."
}

// Parameters returns the tool parameters.
func (t *ListFindingsTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "stack",
			Description: "Filter findings by Atmos stack name",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "severity",
			Description: "Comma-separated severity filter (CRITICAL, HIGH, MEDIUM, LOW, INFORMATIONAL)",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     "CRITICAL,HIGH",
		},
		{
			Name:        "source",
			Description: "Filter by source service (security-hub, config, inspector, guardduty, macie, access-analyzer, all)",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     "all",
		},
		{
			Name:        "max_findings",
			Description: "Maximum number of findings to return",
			Type:        tools.ParamTypeInt,
			Required:    false,
			Default:     defaultToolMaxFindings,
		},
	}
}

// Execute runs the tool.
func (t *ListFindingsTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if !atmosConfig.AWS.Security.Enabled {
		return &tools.Result{
			Success: false,
			Error: errUtils.Build(errUtils.ErrAISecurityNotEnabled).
				WithHint("Add `aws.security.enabled: true` to your `atmos.yaml`").
				WithExitCode(2).
				Err(),
		}, nil
	}

	opts := parseFindingsQueryParams(params)

	// Fetch and map findings.
	fetcher := security.NewFindingFetcher(&atmosConfig)
	findings, err := fetcher.FetchFindings(ctx, &opts)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if len(findings) == 0 {
		return &tools.Result{
			Success: true,
			Output:  "No security findings match the specified filters.",
		}, nil
	}

	mapper := security.NewComponentMapper(&atmosConfig)
	findings, err = mapper.MapFindings(ctx, findings)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	output := formatFindingsOutput(findings)
	data, _ := json.Marshal(findings)

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"total":    len(findings),
			"findings": string(data),
		},
	}, nil
}

// parseFindingsQueryParams extracts security query options from tool parameters.
func parseFindingsQueryParams(params map[string]interface{}) security.QueryOptions {
	opts := security.QueryOptions{
		Source:      security.SourceAll,
		MaxFindings: defaultToolMaxFindings,
	}

	if stack, ok := params["stack"].(string); ok && stack != "" {
		opts.Stack = stack
	}
	if maxFindings, ok := params["max_findings"].(float64); ok && maxFindings > 0 {
		opts.MaxFindings = int(maxFindings)
	}

	opts.Severity = parseSeverityParam(params)
	opts.Source = parseSourceParam(params, opts.Source)

	return opts
}

// severityLookup maps severity strings to typed constants.
var severityLookup = map[string]security.Severity{
	"CRITICAL":      security.SeverityCritical,
	"HIGH":          security.SeverityHigh,
	"MEDIUM":        security.SeverityMedium,
	"LOW":           security.SeverityLow,
	"INFORMATIONAL": security.SeverityInformational,
}

// parseSeverityParam parses the severity parameter from tool params.
func parseSeverityParam(params map[string]interface{}) []security.Severity {
	severityStr := "CRITICAL,HIGH"
	if s, ok := params["severity"].(string); ok && s != "" {
		severityStr = s
	}

	var severities []security.Severity
	for _, s := range strings.Split(severityStr, ",") {
		if sev, ok := severityLookup[strings.ToUpper(strings.TrimSpace(s))]; ok {
			severities = append(severities, sev)
		}
	}
	return severities
}

// sourceLookup maps source strings to typed constants.
var sourceLookup = map[string]security.Source{
	"security-hub":    security.SourceSecurityHub,
	"config":          security.SourceConfig,
	"inspector":       security.SourceInspector,
	"guardduty":       security.SourceGuardDuty,
	"macie":           security.SourceMacie,
	"access-analyzer": security.SourceAccessAnalyzer,
}

// parseSourceParam parses the source parameter from tool params.
func parseSourceParam(params map[string]interface{}, defaultSource security.Source) security.Source {
	if source, ok := params["source"].(string); ok && source != "" {
		if src, ok := sourceLookup[strings.ToLower(source)]; ok {
			return src
		}
	}
	return defaultSource
}

// formatFindingsOutput creates a readable text summary of findings.
func formatFindingsOutput(findings []security.Finding) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Security Findings (%d total):\n\n", len(findings))

	for i := range findings {
		f := &findings[i]
		fmt.Fprintf(&sb, "%d. [%s] %s\n", i+1, f.Severity, f.Title)
		fmt.Fprintf(&sb, "   Resource: %s\n", f.ResourceARN)
		fmt.Fprintf(&sb, "   Source: %s\n", f.Source)
		if f.Mapping != nil && f.Mapping.Mapped {
			fmt.Fprintf(&sb, "   Component: %s (stack: %s, confidence: %s)\n",
				f.Mapping.Component, f.Mapping.Stack, f.Mapping.Confidence)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// RequiresPermission returns whether this tool needs permission.
func (t *ListFindingsTool) RequiresPermission() bool {
	return false
}

// IsRestricted returns whether this tool is always restricted.
func (t *ListFindingsTool) IsRestricted() bool {
	return false
}
