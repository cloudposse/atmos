package atmos

import (
	"context"
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/aws/security"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AnalyzeFindingTool performs AI-powered analysis of a specific security finding.
type AnalyzeFindingTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewAnalyzeFindingTool creates a new analyze finding tool.
func NewAnalyzeFindingTool(atmosConfig *schema.AtmosConfiguration) *AnalyzeFindingTool {
	return &AnalyzeFindingTool{atmosConfig: atmosConfig}
}

// Name returns the tool name.
func (t *AnalyzeFindingTool) Name() string {
	return "atmos_analyze_finding"
}

// Description returns the tool description.
func (t *AnalyzeFindingTool) Description() string {
	return "Analyze a security finding using AI to determine root cause, remediation steps, " +
		"and deployment commands. Maps the finding to its Atmos component and reads the " +
		"component source code for context-aware analysis. Use atmos_list_findings first to get finding IDs."
}

// Parameters returns the tool parameters.
func (t *AnalyzeFindingTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "finding_id",
			Description: "The security finding ID to analyze",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "component_source",
			Description: "Optional component source code to include in the analysis context",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "stack_config",
			Description: "Optional stack configuration YAML to include in the analysis context",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute runs the tool.
func (t *AnalyzeFindingTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	findingID, ok := params["finding_id"].(string)
	if !ok || findingID == "" {
		return &tools.Result{Success: false, Output: "finding_id parameter is required"}, nil
	}

	componentSource, _ := params["component_source"].(string)
	stackConfig, _ := params["stack_config"].(string)

	// Re-initialize config.
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

	// Fetch the finding by ID.
	finding, err := fetchFindingByID(ctx, &atmosConfig, findingID)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if finding == nil {
		return &tools.Result{
			Success: true,
			Output:  fmt.Sprintf("Finding with ID %q not found.", findingID),
		}, nil
	}

	// Map and analyze the finding.
	return mapAndAnalyzeFinding(ctx, &atmosConfig, finding, componentSource, stackConfig)
}

// mapAndAnalyzeFinding maps a finding to its component and runs AI analysis.
func mapAndAnalyzeFinding(ctx context.Context, atmosConfig *schema.AtmosConfiguration, finding *security.Finding, componentSource, stackConfig string) (*tools.Result, error) {
	// Map the finding to a component.
	mapper := security.NewComponentMapper(atmosConfig)
	mapping, _ := mapper.MapFinding(ctx, finding)
	finding.Mapping = mapping

	// Run AI analysis.
	analyzer, err := security.NewFindingAnalyzer(ctx, atmosConfig)
	if err != nil {
		return &tools.Result{
			Success: false,
			Output:  fmt.Sprintf("Failed to create AI analyzer: %s", err.Error()),
		}, nil
	}

	remediation, err := analyzer.AnalyzeFinding(ctx, finding, componentSource, stackConfig)
	if err != nil {
		return &tools.Result{
			Success: false,
			Output:  fmt.Sprintf("AI analysis failed: %s", err.Error()),
		}, nil
	}

	finding.Remediation = remediation

	// Format output.
	output := formatAnalysisOutput(finding)
	data, _ := json.Marshal(finding)

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"finding": string(data),
		},
	}, nil
}

// fetchFindingByID fetches a specific finding by its ID.
func fetchFindingByID(ctx context.Context, atmosConfig *schema.AtmosConfiguration, findingID string) (*security.Finding, error) {
	fetcher := security.NewFindingFetcher(atmosConfig)
	opts := security.QueryOptions{
		Severity:    []security.Severity{security.SeverityCritical, security.SeverityHigh, security.SeverityMedium, security.SeverityLow, security.SeverityInformational},
		MaxFindings: security.MaxFindingsForLookup,
	}
	findings, err := fetcher.FetchFindings(ctx, &opts)
	if err != nil {
		return nil, err
	}

	for i := range findings {
		if findings[i].ID == findingID {
			return &findings[i], nil
		}
	}

	return nil, nil
}

// formatAnalysisOutput creates a readable text summary of the AI analysis.
func formatAnalysisOutput(f *security.Finding) string {
	output := formatFindingDetail(f)

	if f.Remediation != nil {
		output += "\nAI Analysis:\n"
		if f.Remediation.RootCause != "" {
			output += fmt.Sprintf("  Root Cause: %s\n", f.Remediation.RootCause)
		}
		output += fmt.Sprintf("  Remediation: %s\n", f.Remediation.Description)
		if f.Remediation.DeployCommand != "" {
			output += fmt.Sprintf("  Deploy: %s\n", f.Remediation.DeployCommand)
		}
		if f.Remediation.RiskLevel != "" {
			output += fmt.Sprintf("  Risk: %s\n", f.Remediation.RiskLevel)
		}
	}

	return output
}

// RequiresPermission returns whether this tool needs permission.
func (t *AnalyzeFindingTool) RequiresPermission() bool {
	return false
}

// IsRestricted returns whether this tool is always restricted.
func (t *AnalyzeFindingTool) IsRestricted() bool {
	return false
}
