package security

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	ai "github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/registry"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed skill_prompt.md
var skillPrompt string

// FindingAnalyzer provides AI-powered analysis of security findings.
type FindingAnalyzer interface {
	// AnalyzeFinding analyzes a single finding with component context.
	AnalyzeFinding(ctx context.Context, finding *Finding, componentSource string, stackConfig string) (*Remediation, error)

	// AnalyzeFindings analyzes multiple findings in batch, grouping by component.
	AnalyzeFindings(ctx context.Context, findings []Finding) ([]Finding, error)
}

// maxAnalysisIterations is the maximum tool call iterations for multi-turn analysis.
const maxAnalysisIterations = 10

// aiAnalyzer implements FindingAnalyzer using an AI client for root cause analysis and remediation.
type aiAnalyzer struct {
	client       registry.Client
	atmosConfig  *schema.AtmosConfiguration
	toolRegistry *tools.Registry // Tool registry for multi-turn analysis (API providers).
	toolExecutor *tools.Executor // Tool executor for running tool calls.
}

// NewFindingAnalyzer creates a FindingAnalyzer backed by the configured AI provider.
// If toolRegistry and toolExecutor are provided, API providers use multi-turn tool analysis.
// CLI providers always fall back to single-prompt mode with pre-fetched context.
func NewFindingAnalyzer(ctx context.Context, atmosConfig *schema.AtmosConfiguration, toolRegistry *tools.Registry, toolExecutor *tools.Executor) (FindingAnalyzer, error) {
	defer perf.Track(nil, "security.NewFindingAnalyzer")()

	client, err := ai.NewClientWithContext(ctx, atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create AI client for security analysis: %w", err)
	}

	return &aiAnalyzer{
		client:       client,
		atmosConfig:  atmosConfig,
		toolRegistry: toolRegistry,
		toolExecutor: toolExecutor,
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

	// Try multi-turn tool analysis for API providers.
	if a.toolRegistry != nil && a.toolExecutor != nil {
		return a.analyzeWithTools(ctx, finding, prompt)
	}

	// Fall back to single-prompt analysis (CLI providers or no tools).
	return a.analyzeSimple(ctx, finding, prompt)
}

// analyzeWithTools uses multi-turn tool execution for API providers.
// The AI can call atmos_describe_component, read_component_file, etc. to gather more data.
func (a *aiAnalyzer) analyzeWithTools(ctx context.Context, finding *Finding, prompt string) (*Remediation, error) {
	availableTools := a.toolExecutor.ListTools()
	if len(availableTools) == 0 {
		return a.analyzeSimple(ctx, finding, prompt)
	}

	log.Debug("Using multi-turn tool analysis", "tools", len(availableTools), "finding", finding.ID)

	messages := []types.Message{
		{Role: types.RoleUser, Content: prompt},
	}

	var finalResponse string
	for iteration := 0; iteration < maxAnalysisIterations; iteration++ {
		response, err := a.client.SendMessageWithSystemPromptAndTools(ctx, skillPrompt, "", messages, availableTools)
		if err != nil {
			// If tools are not supported (CLI provider), fall back to simple.
			if isToolsNotSupported(err) {
				log.Debug("Provider does not support tools, falling back to simple analysis")
				return a.analyzeSimple(ctx, finding, prompt)
			}
			return nil, fmt.Errorf("AI analysis failed for finding %s (iteration %d): %w", finding.ID, iteration, err)
		}

		// If the AI wants to call tools, execute them and continue.
		if response.StopReason == types.StopReasonToolUse && len(response.ToolCalls) > 0 {
			messages = a.handleToolCalls(ctx, response, messages)
			continue
		}

		// Final response — AI is done.
		finalResponse = response.Content
		break
	}

	if finalResponse == "" {
		return nil, fmt.Errorf("%w: empty response for finding %s", errUtils.ErrAISecurityAnalysisFailed, finding.ID)
	}

	return parseRemediationResponse(finalResponse, finding), nil
}

// handleToolCalls executes tool calls and appends results to the conversation.
func (a *aiAnalyzer) handleToolCalls(ctx context.Context, response *types.Response, messages []types.Message) []types.Message {
	// Add the assistant's response (with tool calls) to history.
	messages = append(messages, types.Message{
		Role:    types.RoleAssistant,
		Content: response.Content,
	})

	// Execute each tool call and add results.
	for _, tc := range response.ToolCalls {
		log.Debug("Executing tool call", "tool", tc.Name, "finding_analysis", true)

		result, err := a.toolExecutor.Execute(ctx, tc.Name, tc.Input)
		var resultContent string
		if err != nil {
			resultContent = fmt.Sprintf("Tool execution failed: %s", err)
		} else if result != nil {
			resultContent = result.Output
		}

		// Add tool result as a user message with tool call context.
		messages = append(messages, types.Message{
			Role:    types.RoleUser,
			Content: fmt.Sprintf("[Tool result for %s (call %s)]\n\n%s", tc.Name, tc.ID, resultContent),
		})
	}

	return messages
}

// isToolsNotSupported checks if the error indicates the provider doesn't support tools.
func isToolsNotSupported(err error) bool {
	return err != nil && (errors.Is(err, errUtils.ErrCLIProviderToolsNotSupported) ||
		strings.Contains(err.Error(), "not supported"))
}

// analyzeSimple uses a single prompt with pre-fetched context (CLI providers or no tools).
func (a *aiAnalyzer) analyzeSimple(ctx context.Context, finding *Finding, prompt string) (*Remediation, error) {
	response, err := a.client.SendMessage(ctx, skillPrompt+"\n\n---\n\n"+prompt)
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

// buildAnalysisPrompt constructs the data portion of the AI prompt for analyzing a security finding.
func buildAnalysisPrompt(finding *Finding, componentSource string, stackConfig string) string {
	var sb strings.Builder

	sb.WriteString("Analyze this AWS security finding and provide structured remediation.\n\n")
	sb.WriteString("You have access to Atmos tools. Use them to gather more context:\n")
	sb.WriteString("- `atmos_describe_component` — get full resolved config for a component in a stack\n")
	sb.WriteString("- `read_component_file` — read any file from a Terraform component\n")
	sb.WriteString("- `read_stack_file` — read a stack configuration file\n")
	sb.WriteString("- `atmos_list_stacks` — list all stacks\n\n")

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

	// Component source code (pre-fetched for single-prompt mode).
	if componentSource != "" {
		sb.WriteString("## Component Source Code (Terraform)\n\n")
		sb.WriteString("```hcl\n")
		sb.WriteString(componentSource)
		sb.WriteString("\n```\n\n")
	}

	// Stack configuration (pre-fetched for single-prompt mode).
	if stackConfig != "" {
		sb.WriteString("## Stack Configuration\n\n")
		sb.WriteString("```yaml\n")
		sb.WriteString(stackConfig)
		sb.WriteString("\n```\n\n")
	}

	return sb.String()
}

// parseRemediationResponse parses an AI response into a structured Remediation.
func parseRemediationResponse(response string, finding *Finding) *Remediation {
	remediation := &Remediation{
		Description:   response,
		RootCause:     extractFirstMatch(response, "### Root Cause", "**Root Cause:**", "Root Cause:"),
		StackChanges:  extractFirstMatch(response, "### Stack Changes"),
		RiskLevel:     normalizeRiskLevel(extractFirstMatch(response, "### Risk", "**Risk:**", "Risk:")),
		DeployCommand: extractAtmosCommand(extractFirstMatch(response, "### Deploy", "**Deploy:**", "Deploy:")),
	}

	// Parse steps from structured or legacy format.
	if steps := extractFirstMatch(response, "### Steps", "**Remediation:**"); steps != "" {
		remediation.Steps = parseListItems(steps)
	}

	// Parse references.
	if refs := extractFirstMatch(response, "### References"); refs != "" {
		remediation.References = parseListItems(refs)
	}

	// Fall back to constructing the deploy command from the mapping.
	if remediation.DeployCommand == "" && finding.Mapping != nil && finding.Mapping.Mapped {
		remediation.DeployCommand = fmt.Sprintf("atmos terraform apply %s -s %s",
			finding.Mapping.Component, finding.Mapping.Stack)
	}

	return remediation
}

// extractFirstMatch tries multiple section headers and returns the first non-empty match.
func extractFirstMatch(text string, headers ...string) string {
	for _, header := range headers {
		if section := extractSection(text, header); section != "" {
			return section
		}
	}
	return ""
}

// maxNumberedPrefixLen is the max digits before ". " in a numbered list item.
const maxNumberedPrefixLen = 5

// newlineSep is the newline separator used for splitting text.
const newlineSep = "\n"

// parseListItems extracts items from numbered lists or bullet lists.
func parseListItems(text string) []string {
	var items []string
	for _, line := range strings.Split(text, newlineSep) {
		if item := extractListItem(strings.TrimSpace(line)); item != "" {
			items = append(items, item)
		}
	}
	return items
}

// extractListItem strips list prefixes from a line. Returns empty for non-list lines.
func extractListItem(line string) string {
	if line == "" {
		return ""
	}
	if len(line) > 2 && line[0] >= '0' && line[0] <= '9' {
		if idx := strings.Index(line, ". "); idx != -1 && idx < maxNumberedPrefixLen {
			return strings.TrimSpace(line[idx+2:])
		}
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return strings.TrimSpace(line[2:])
	}
	return ""
}

// extractSection extracts text following a header up to the next section header.
func extractSection(text string, header string) string {
	idx := strings.Index(text, header)
	if idx == -1 {
		return ""
	}

	content := text[idx+len(header):]
	endMarkers := []string{
		"### Root Cause", "### Steps", "### Code Changes", "### Stack Changes",
		"### Deploy", "### Risk", "### References",
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
	lines := strings.Split(text, newlineSep)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "`")
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "atmos ") {
			return line
		}
	}
	return strings.TrimSpace(text)
}

// normalizeRiskLevel normalizes a risk level string to low, medium, or high.
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

// readComponentSource reads main.tf from a component path.
func readComponentSource(componentPath string) string {
	if componentPath == "" {
		return ""
	}
	mainTF := filepath.Join(componentPath, "main.tf")
	content, err := readFileContent(mainTF)
	if err != nil {
		return ""
	}
	const maxSourceLength = 10000
	if len(content) > maxSourceLength {
		content = content[:maxSourceLength] + "\n... (truncated)"
	}
	return content
}

// readFileContent reads a file and returns its content as a string.
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
