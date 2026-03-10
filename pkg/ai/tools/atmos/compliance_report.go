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

// ComplianceReportTool generates compliance posture reports for frameworks.
type ComplianceReportTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewComplianceReportTool creates a new compliance report tool.
func NewComplianceReportTool(atmosConfig *schema.AtmosConfiguration) *ComplianceReportTool {
	return &ComplianceReportTool{atmosConfig: atmosConfig}
}

// Name returns the tool name.
func (t *ComplianceReportTool) Name() string {
	return "atmos_compliance_report"
}

// Description returns the tool description.
func (t *ComplianceReportTool) Description() string {
	return "Generate a compliance posture report for a specific framework (CIS AWS, PCI DSS, SOC2, HIPAA, NIST). " +
		"Returns the compliance score, passing/failing controls, and remediation guidance."
}

// Parameters returns the tool parameters.
func (t *ComplianceReportTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "framework",
			Description: "Compliance framework: cis-aws, pci-dss, soc2, hipaa, nist",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "stack",
			Description: "Filter by Atmos stack name",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute runs the tool.
func (t *ComplianceReportTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	framework, ok := params["framework"].(string)
	if !ok || framework == "" {
		return &tools.Result{Success: false, Output: "framework parameter is required"}, nil
	}

	stack := ""
	if s, ok := params["stack"].(string); ok {
		stack = s
	}

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

	// Fetch compliance status.
	fetcher := security.NewFindingFetcher(&atmosConfig)
	report, err := fetcher.FetchComplianceStatus(ctx, framework, stack)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if report == nil {
		return &tools.Result{
			Success: true,
			Output:  fmt.Sprintf("No compliance data available for framework: %s", framework),
		}, nil
	}

	// Format output.
	output := formatComplianceOutput(report)

	data, _ := json.Marshal(report)

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"framework":        report.Framework,
			"total_controls":   report.TotalControls,
			"passing_controls": report.PassingControls,
			"failing_controls": report.FailingControls,
			"score_percent":    report.ScorePercent,
			"report":           string(data),
		},
	}, nil
}

// formatComplianceOutput creates a readable compliance report.
func formatComplianceOutput(report *security.ComplianceReport) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Compliance Report: %s\n", report.FrameworkTitle)
	fmt.Fprintf(&sb, "Score: %d/%d Controls Passing (%.0f%%)\n\n",
		report.PassingControls, report.TotalControls, report.ScorePercent)

	if len(report.FailingDetails) > 0 {
		sb.WriteString("Failing Controls:\n")
		for _, ctrl := range report.FailingDetails {
			fmt.Fprintf(&sb, "  - [%s] %s: %s\n", ctrl.Severity, ctrl.ControlID, ctrl.Title)
			if ctrl.Component != "" {
				fmt.Fprintf(&sb, "    Component: %s\n", ctrl.Component)
			}
		}
	} else {
		sb.WriteString("All controls are passing.\n")
	}

	return sb.String()
}

// RequiresPermission returns whether this tool needs permission.
func (t *ComplianceReportTool) RequiresPermission() bool {
	return false
}

// IsRestricted returns whether this tool is always restricted.
func (t *ComplianceReportTool) IsRestricted() bool {
	return false
}
