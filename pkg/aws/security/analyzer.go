package security

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ai "github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/registry"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// FindingAnalyzer provides AI-powered analysis of security findings.
type FindingAnalyzer interface {
	// AnalyzeFinding analyzes a single finding with component context.
	AnalyzeFinding(ctx context.Context, finding *Finding, componentSource string, stackConfig string) (*Remediation, error)

	// AnalyzeFindings analyzes multiple findings in batch, grouping by component.
	AnalyzeFindings(ctx context.Context, findings []Finding) ([]Finding, error)
}

// aiAnalyzer implements FindingAnalyzer using an AI client for root cause analysis and remediation.
type aiAnalyzer struct {
	client      registry.Client
	atmosConfig *schema.AtmosConfiguration
}

// NewFindingAnalyzer creates a FindingAnalyzer backed by the configured AI provider.
func NewFindingAnalyzer(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (FindingAnalyzer, error) {
	defer perf.Track(nil, "security.NewFindingAnalyzer")()

	client, err := ai.NewClientWithContext(ctx, atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create AI client for security analysis: %w", err)
	}

	return &aiAnalyzer{
		client:      client,
		atmosConfig: atmosConfig,
	}, nil
}

// newFindingAnalyzerWithClient creates a FindingAnalyzer with a pre-built client (for testing).
func newFindingAnalyzerWithClient(client registry.Client, atmosConfig *schema.AtmosConfiguration) FindingAnalyzer {
	return &aiAnalyzer{
		client:      client,
		atmosConfig: atmosConfig,
	}
}

// AnalyzeFinding analyzes a single finding with component context and returns AI-generated remediation.
func (a *aiAnalyzer) AnalyzeFinding(ctx context.Context, finding *Finding, componentSource string, stackConfig string) (*Remediation, error) {
	defer perf.Track(nil, "security.aiAnalyzer.AnalyzeFinding")()

	prompt := buildAnalysisPrompt(finding, componentSource, stackConfig)

	response, err := a.client.SendMessage(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("AI analysis failed for finding %s: %w", finding.ID, err)
	}

	return parseRemediationResponse(response, finding), nil
}

// AnalyzeFindings analyzes multiple findings in batch, skipping unmapped findings.
func (a *aiAnalyzer) AnalyzeFindings(ctx context.Context, findings []Finding) ([]Finding, error) {
	defer perf.Track(nil, "security.aiAnalyzer.AnalyzeFindings")()

	for i := range findings {
		// Skip findings that are not mapped to a component.
		if findings[i].Mapping == nil || !findings[i].Mapping.Mapped {
			continue
		}

		componentSource := readComponentSource(findings[i].Mapping.ComponentPath)
		stackConfig := formatStackInfo(findings[i].Mapping)

		remediation, err := a.AnalyzeFinding(ctx, &findings[i], componentSource, stackConfig)
		if err != nil {
			// Log error but continue with remaining findings.
			findings[i].Remediation = &Remediation{
				Description: fmt.Sprintf("AI analysis failed: %s", err.Error()),
				RiskLevel:   "unknown",
			}
			continue
		}
		findings[i].Remediation = remediation
	}

	return findings, nil
}

// buildAnalysisPrompt constructs the AI prompt for analyzing a security finding.
func buildAnalysisPrompt(finding *Finding, componentSource string, stackConfig string) string {
	var sb strings.Builder

	sb.WriteString("Analyze this AWS security finding and provide remediation guidance.\n\n")

	// Finding details.
	sb.WriteString("## Security Finding\n\n")
	fmt.Fprintf(&sb, "**ID:** %s\n", finding.ID)
	fmt.Fprintf(&sb, "**Title:** %s\n", finding.Title)
	fmt.Fprintf(&sb, "**Severity:** %s\n", finding.Severity)
	fmt.Fprintf(&sb, "**Source:** %s\n", finding.Source)
	fmt.Fprintf(&sb, "**Resource ARN:** %s\n", finding.ResourceARN)
	fmt.Fprintf(&sb, "**Resource Type:** %s\n", finding.ResourceType)
	fmt.Fprintf(&sb, "**Description:** %s\n\n", finding.Description)

	if finding.ComplianceStandard != "" {
		fmt.Fprintf(&sb, "**Compliance Standard:** %s\n\n", finding.ComplianceStandard)
	}

	// Component mapping info.
	if finding.Mapping != nil && finding.Mapping.Mapped {
		sb.WriteString("## Atmos Component Mapping\n\n")
		fmt.Fprintf(&sb, "**Component:** %s\n", finding.Mapping.Component)
		fmt.Fprintf(&sb, "**Stack:** %s\n", finding.Mapping.Stack)
		if finding.Mapping.ComponentPath != "" {
			fmt.Fprintf(&sb, "**Component Path:** %s\n", finding.Mapping.ComponentPath)
		}
		sb.WriteString("\n")
	}

	// Component source code.
	if componentSource != "" {
		sb.WriteString("## Component Source Code (Terraform)\n\n")
		sb.WriteString("```hcl\n")
		sb.WriteString(componentSource)
		sb.WriteString("\n```\n\n")
	}

	// Stack configuration.
	if stackConfig != "" {
		sb.WriteString("## Stack Configuration\n\n")
		sb.WriteString("```yaml\n")
		sb.WriteString(stackConfig)
		sb.WriteString("\n```\n\n")
	}

	// Instructions for structured response.
	sb.WriteString("## Response Format\n\n")
	sb.WriteString("Please provide your analysis using the following sections:\n\n")
	sb.WriteString("**Root Cause:** Explain the root cause tied to specific code or configuration.\n\n")
	sb.WriteString("**Remediation:** Describe whether the fix should be in stack vars or component source, ")
	sb.WriteString("and provide specific remediation steps.\n\n")
	sb.WriteString("**Deploy:** Provide the atmos command to deploy the fix ")
	sb.WriteString("(e.g., `atmos terraform apply <component> -s <stack>`).\n\n")
	sb.WriteString("**Risk:** Assess the risk level of applying this fix (low, medium, high).\n")

	return sb.String()
}

// parseRemediationResponse parses an AI response into a structured Remediation.
func parseRemediationResponse(response string, finding *Finding) *Remediation {
	remediation := &Remediation{
		Description: response,
	}

	// Extract Root Cause section (try bold markers first to avoid partial matches).
	if rootCause := extractSection(response, "**Root Cause:**"); rootCause != "" {
		remediation.RootCause = rootCause
	} else if rootCause := extractSection(response, "Root Cause:"); rootCause != "" {
		remediation.RootCause = rootCause
	}

	// Extract Deploy command (try bold markers first).
	if deploy := extractSection(response, "**Deploy:**"); deploy != "" {
		remediation.DeployCommand = extractAtmosCommand(deploy)
	} else if deploy := extractSection(response, "Deploy:"); deploy != "" {
		remediation.DeployCommand = extractAtmosCommand(deploy)
	}

	// Fall back to constructing the deploy command from the mapping.
	if remediation.DeployCommand == "" && finding.Mapping != nil && finding.Mapping.Mapped {
		remediation.DeployCommand = fmt.Sprintf("atmos terraform apply %s -s %s",
			finding.Mapping.Component, finding.Mapping.Stack)
	}

	// Extract Risk level (try bold markers first).
	if risk := extractSection(response, "**Risk:**"); risk != "" {
		remediation.RiskLevel = normalizeRiskLevel(risk)
	} else if risk := extractSection(response, "Risk:"); risk != "" {
		remediation.RiskLevel = normalizeRiskLevel(risk)
	}

	return remediation
}

// extractSection extracts the text following a header marker up to the next section header or end of text.
func extractSection(text string, header string) string {
	idx := strings.Index(text, header)
	if idx == -1 {
		return ""
	}

	content := text[idx+len(header):]

	// Find the end of this section (next header or end of text).
	// Look for common section markers.
	endMarkers := []string{
		"**Root Cause:**", "**Remediation:**", "**Deploy:**", "**Risk:**",
		"Root Cause:", "Remediation:", "Deploy:", "Risk:",
	}
	endIdx := len(content)
	for _, marker := range endMarkers {
		if marker == header {
			continue
		}
		if markerIdx := strings.Index(content, marker); markerIdx != -1 && markerIdx < endIdx {
			endIdx = markerIdx
		}
	}

	return strings.TrimSpace(content[:endIdx])
}

// extractAtmosCommand finds an atmos command in the text.
func extractAtmosCommand(text string) string {
	// Look for `atmos terraform apply ...` pattern.
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Strip backticks.
		line = strings.Trim(line, "`")
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "atmos ") {
			return line
		}
	}
	return strings.TrimSpace(text)
}

// normalizeRiskLevel normalizes a risk level string to one of: low, medium, high.
func normalizeRiskLevel(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(lower, "high"):
		return "high"
	case strings.Contains(lower, "medium"):
		return "medium"
	case strings.Contains(lower, "low"):
		return "low"
	default:
		return strings.TrimSpace(text)
	}
}

// readComponentSource reads the main.tf file from a component path.
// Returns empty string if the path is empty or the file cannot be read.
func readComponentSource(componentPath string) string {
	if componentPath == "" {
		return ""
	}

	// Read main.tf as the primary source file.
	mainTF := filepath.Join(componentPath, "main.tf")
	content, err := readFileContent(mainTF)
	if err != nil {
		return ""
	}

	// Truncate very large files to avoid exceeding token limits.
	const maxSourceLength = 10000
	if len(content) > maxSourceLength {
		content = content[:maxSourceLength] + "\n... (truncated)"
	}

	return content
}

// readFileContent reads the content of a file and returns it as a string.
func readFileContent(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	data, err := readFile(cleanPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// readFile is a variable to allow test overrides.
var readFile = os.ReadFile

// formatStackInfo formats component mapping into a stack configuration summary.
func formatStackInfo(mapping *ComponentMapping) string {
	if mapping == nil {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "component: %s\n", mapping.Component)
	fmt.Fprintf(&sb, "stack: %s\n", mapping.Stack)
	if mapping.Workspace != "" {
		fmt.Fprintf(&sb, "workspace: %s\n", mapping.Workspace)
	}
	fmt.Fprintf(&sb, "confidence: %s\n", mapping.Confidence)
	fmt.Fprintf(&sb, "method: %s\n", mapping.Method)

	return sb.String()
}
