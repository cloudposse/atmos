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

// DescribeFindingTool retrieves detailed information about a specific security finding.
type DescribeFindingTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewDescribeFindingTool creates a new describe finding tool.
func NewDescribeFindingTool(atmosConfig *schema.AtmosConfiguration) *DescribeFindingTool {
	return &DescribeFindingTool{atmosConfig: atmosConfig}
}

// Name returns the tool name.
func (t *DescribeFindingTool) Name() string {
	return "atmos_describe_finding"
}

// Description returns the tool description.
func (t *DescribeFindingTool) Description() string {
	return "Get detailed information about a specific security finding by ID. " +
		"Returns the finding details including severity, resource, component mapping, " +
		"and description. Use atmos_list_findings first to get finding IDs."
}

// Parameters returns the tool parameters.
func (t *DescribeFindingTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "finding_id",
			Description: "The security finding ID to look up",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
	}
}

// Execute runs the tool.
func (t *DescribeFindingTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	findingID, ok := params["finding_id"].(string)
	if !ok || findingID == "" {
		return &tools.Result{Success: false, Output: "finding_id parameter is required"}, nil
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

	return fetchAndDescribeFinding(ctx, &atmosConfig, findingID)
}

// fetchAndDescribeFinding fetches a finding by ID and returns a detailed description.
func fetchAndDescribeFinding(ctx context.Context, atmosConfig *schema.AtmosConfiguration, findingID string) (*tools.Result, error) {
	finding, err := fetchFindingByID(ctx, atmosConfig, findingID)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if finding == nil {
		return &tools.Result{
			Success: true,
			Output:  fmt.Sprintf("Finding with ID %q not found.", findingID),
		}, nil
	}

	// Map the finding to component.
	mapper := security.NewComponentMapper(atmosConfig, nil)
	mapping, _ := mapper.MapFinding(ctx, finding)
	finding.Mapping = mapping

	// Format detailed output.
	output := formatFindingDetail(finding)
	data, _ := json.Marshal(finding)

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"finding": string(data),
		},
	}, nil
}

// formatFindingDetail formats a single finding with full details.
func formatFindingDetail(f *security.Finding) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Finding: %s\n", f.Title)
	fmt.Fprintf(&sb, "ID: %s\n", f.ID)
	fmt.Fprintf(&sb, "Severity: %s\n", f.Severity)
	fmt.Fprintf(&sb, "Source: %s\n", f.Source)
	fmt.Fprintf(&sb, "Resource: %s\n", f.ResourceARN)
	fmt.Fprintf(&sb, "Resource Type: %s\n", f.ResourceType)
	fmt.Fprintf(&sb, "Account: %s\n", f.AccountID)
	fmt.Fprintf(&sb, "Region: %s\n", f.Region)

	if f.ComplianceStandard != "" {
		fmt.Fprintf(&sb, "Compliance Standard: %s\n", f.ComplianceStandard)
	}

	if f.Description != "" {
		fmt.Fprintf(&sb, "\nDescription:\n%s\n", f.Description)
	}

	if f.Mapping != nil && f.Mapping.Mapped {
		sb.WriteString("\nAtmos Mapping:\n")
		fmt.Fprintf(&sb, "  Component: %s\n", f.Mapping.Component)
		fmt.Fprintf(&sb, "  Stack: %s\n", f.Mapping.Stack)
		fmt.Fprintf(&sb, "  Confidence: %s\n", f.Mapping.Confidence)
		fmt.Fprintf(&sb, "  Method: %s\n", f.Mapping.Method)
		if f.Mapping.ComponentPath != "" {
			fmt.Fprintf(&sb, "  Path: %s\n", f.Mapping.ComponentPath)
		}
	} else {
		sb.WriteString("\nAtmos Mapping: Not mapped to any component\n")
	}

	return sb.String()
}

// RequiresPermission returns whether this tool needs permission.
func (t *DescribeFindingTool) RequiresPermission() bool {
	return false
}

// IsRestricted returns whether this tool is always restricted.
func (t *DescribeFindingTool) IsRestricted() bool {
	return false
}
