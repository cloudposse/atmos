package aws

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	atmosTools "github.com/cloudposse/atmos/pkg/ai/tools/atmos"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/aws/security"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// defaultMaxFindings is the default maximum number of security findings to fetch.
// Set high enough to capture findings across all accounts in a multi-account org.
// AI cost is controlled separately (only mapped findings are sent to AI).
const defaultMaxFindings = 500

//go:embed markdown/atmos_aws_security.md
var securityLongMarkdown string

// securityParser handles flag parsing with Viper precedence for the security command.
var securityParser *flags.StandardParser

// securityCmd is the parent command for security subcommands.
var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "AWS security commands",
	Long:  "Commands for analyzing AWS security findings and mapping them to Atmos components.",
	Args:  cobra.NoArgs,
}

// securityAnalyzeCmd represents the aws security analyze command.
var securityAnalyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze AWS security findings for Atmos stacks",
	Long:  securityLongMarkdown,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bind parsed flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := securityParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flags from Viper (supports CLI > ENV > config > defaults).
		stack := v.GetString("stack")
		component := v.GetString("component")
		severityStr := v.GetString("severity")
		sourceStr := v.GetString("source")
		formatStr := v.GetString("format")
		fileOutput := v.GetString("file")
		maxFindings := v.GetInt("max-findings")
		useAI := v.GetBool("ai")
		region := v.GetString("region")
		identityFlag := v.GetString("identity")
		noGroup := v.GetBool("no-group")

		// Initialize configuration.
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return err
		}

		// Check if AWS security features are enabled.
		if !atmosConfig.AWS.Security.Enabled {
			return errUtils.Build(errUtils.ErrAISecurityNotEnabled).
				WithHint("Add `aws.security.enabled: true` to your `atmos.yaml`").
				WithHint("See https://atmos.tools/cli/configuration/aws for configuration reference").
				WithExitCode(2).
				Err()
		}

		// If --ai flag is passed, check that AI is enabled in configuration.
		if useAI && !atmosConfig.AI.Enabled {
			return errUtils.Build(errUtils.ErrAINotEnabled).
				WithExplanation("The `--ai` flag enables AI-powered analysis but requires an AI provider to be configured.").
				WithHint("Add `ai.enabled: true` to your `atmos.yaml`").
				WithHint("Configure a provider under `ai.providers` (e.g. `anthropic`, `bedrock`, `openai`)").
				WithHint("See https://atmos.tools/cli/configuration/ai for provider setup").
				WithExitCode(2).
				Err()
		}

		// Validate and parse flags.
		outputFormat, err := parseOutputFormat(formatStr)
		if err != nil {
			return err
		}

		source, err := parseSource(sourceStr)
		if err != nil {
			return err
		}

		severities, err := parseSeverities(severityStr, atmosConfig.AWS.Security.DefaultSeverity)
		if err != nil {
			return err
		}

		if maxFindings <= 0 {
			maxFindings = atmosConfig.AWS.Security.MaxFindings
			if maxFindings <= 0 {
				maxFindings = defaultMaxFindings
			}
		}

		// Resolve region (from --region flag or config).
		if region == "" {
			region = atmosConfig.AWS.Security.Region
		}

		// Resolve Atmos Auth identity (from --identity flag or config).
		identityName := identityFlag
		if identityName == "" {
			identityName = atmosConfig.AWS.Security.Identity
		}
		authCtx, err := resolveAuthContext(&atmosConfig, identityName)
		if err != nil {
			return err
		}

		// Validate AWS credentials early before attempting any API calls.
		credCtx, credCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer credCancel()
		if err := validateAWSCredentials(credCtx, region, authCtx); err != nil {
			return err
		}

		// Note: stack and component are NOT passed to QueryOptions because
		// Security Hub has no concept of Atmos stacks. Filtering by stack/component
		// happens AFTER mapping (see filterByStackAndComponent below).
		opts := security.QueryOptions{
			Severity:    severities,
			Source:      source,
			MaxFindings: maxFindings,
			Region:      region,
			NoAI:        !useAI,
		}

		log.Debug("Running security analysis",
			"stack", stack,
			"component", component,
			"severity", severityStr,
			"source", sourceStr,
			"format", formatStr,
			"max_findings", maxFindings,
			"ai", useAI,
		)

		// Create context with timeout.
		// AI analysis with multi-turn tools and retries needs more time than simple API calls.
		defaultTimeout := 120
		if useAI {
			defaultTimeout = 300
		}
		timeoutSeconds := defaultTimeout
		if atmosConfig.AI.TimeoutSeconds > 0 {
			timeoutSeconds = atmosConfig.AI.TimeoutSeconds
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()

		// Fetch findings.
		if outputFormat == security.FormatMarkdown {
			ui.Info("Fetching security findings...")
		}
		fetcher := security.NewFindingFetcher(&atmosConfig, authCtx)
		findings, err := fetcher.FetchFindings(ctx, &opts)
		if err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrAISecurityFetchFailed, err)
		}

		if len(findings) == 0 {
			ui.Success("No security findings match the specified filters. No report written.")
			return nil
		}

		// Map findings to Atmos components.
		if outputFormat == security.FormatMarkdown {
			ui.Infof("Mapping %d findings to Atmos components...", len(findings))
		}
		mapper := security.NewComponentMapper(&atmosConfig, authCtx)
		findings, err = mapper.MapFindings(ctx, findings)
		if err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrAISecurityMappingFailed, err)
		}

		// Filter by stack and component AFTER mapping.
		// Security Hub doesn't know about Atmos stacks — we can only filter after
		// findings are mapped to components/stacks via tags or heuristics.
		if stack != "" || component != "" {
			findings = filterByStackAndComponent(findings, stack, component)
			ui.Infof("Filtered to %d findings matching stack=%q component=%q", len(findings), stack, component)
			if len(findings) == 0 {
				ui.Success("No findings match the specified stack/component after mapping. No report written.")
				return nil
			}
		}

		// AI analysis (only when --ai flag is set).
		if useAI {
			if outputFormat == security.FormatMarkdown {
				ui.Info("Analyzing findings with AI...")
			}

			// Initialize read-only tools for multi-turn analysis (API providers).
			// CLI providers fall back to single-prompt mode automatically.
			var toolReg *tools.Registry
			var toolExec *tools.Executor
			if atmosConfig.AI.Tools.Enabled {
				toolReg, toolExec = initReadOnlyTools(&atmosConfig)
			}

			analyzer, analyzerErr := security.NewFindingAnalyzer(ctx, &atmosConfig, toolReg, toolExec)
			if analyzerErr != nil {
				log.Debug("AI analyzer creation failed, skipping AI analysis", "error", analyzerErr)
			} else {
				findings, err = analyzer.AnalyzeFindings(ctx, findings)
				if err != nil {
					return fmt.Errorf("%w: %w", errUtils.ErrAISecurityAnalysisFailed, err)
				}
			}
		}

		// Build report.
		tagMapping := &atmosConfig.AWS.Security.TagMapping
		report := buildSecurityReport(findings, stack, component, tagMapping)
		report.GroupFindings = !noGroup

		// Determine output destination.
		output := os.Stdout
		if fileOutput != "" {
			dir := filepath.Dir(fileOutput)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("failed to create output directory %q: %w", dir, err)
			}
			f, err := os.Create(fileOutput)
			if err != nil {
				return fmt.Errorf("failed to create output file %q: %w", fileOutput, err)
			}
			defer f.Close()
			output = f
		}

		// Render output.
		renderer := security.NewReportRenderer(outputFormat)

		// For Markdown to stdout, render with colors via ui.Markdown().
		if outputFormat == security.FormatMarkdown && fileOutput == "" {
			var buf strings.Builder
			if err := renderer.RenderSecurityReport(&buf, report); err != nil {
				return err
			}
			ui.Markdown(buf.String())
		} else {
			if err := renderer.RenderSecurityReport(output, report); err != nil {
				return err
			}
			if fileOutput != "" {
				ui.Successf("Report saved to %s", fileOutput)
			}
		}

		return nil
	},
}

func init() {
	// Create parser with security-specific flags using functional options.
	securityParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Target stack to analyze"),
		flags.WithStringFlag("component", "c", "", "Target component within the stack"),
		flags.WithStringFlag("severity", "", "critical,high", "Comma-separated severity filter"),
		flags.WithStringFlag("source", "", "all", "Finding source: security-hub, config, inspector, guardduty, macie, access-analyzer, all"),
		flags.WithStringFlag("framework", "", "", "Compliance framework filter"),
		flags.WithStringFlag("format", "f", "markdown", "Output format: markdown, json, yaml, csv"),
		flags.WithStringFlag("file", "", "", "Write output to file instead of stdout"),
		flags.WithIntFlag("max-findings", "", defaultMaxFindings, "Maximum findings to analyze"),
		flags.WithStringFlag("region", "", "", "AWS region override"),
		flags.WithStringFlag("identity", "i", "", "Atmos Auth identity for AWS credentials"),
		flags.WithBoolFlag("no-group", "", false, "Disable grouping of duplicate findings"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("identity", "ATMOS_AWS_SECURITY_IDENTITY"),
		flags.WithEnvVars("format", "ATMOS_AWS_SECURITY_FORMAT"),
		flags.WithEnvVars("max-findings", "ATMOS_AWS_SECURITY_MAX_FINDINGS"),
		flags.WithEnvVars("region", "ATMOS_AWS_SECURITY_REGION"),
	)

	// Register flags on the analyze subcommand.
	securityParser.RegisterFlags(securityAnalyzeCmd)

	// Bind flags to Viper for environment variable support.
	if err := securityParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	securityCmd.AddCommand(securityAnalyzeCmd)
	awsCmd.AddCommand(securityCmd)
}

// buildSecurityReport constructs a Report from mapped findings.
func buildSecurityReport(findings []Finding, stack, component string, tagMapping *schema.AWSSecurityTagMapping) *security.Report {
	report := &security.Report{
		GeneratedAt:    time.Now().UTC(),
		Stack:          stack,
		Component:      component,
		TotalFindings:  len(findings),
		SeverityCounts: make(map[security.Severity]int),
		Findings:       findings,
		TagMapping:     tagMapping,
	}

	for i := range findings {
		report.SeverityCounts[findings[i].Severity]++
		if findings[i].Mapping != nil && findings[i].Mapping.Mapped {
			report.MappedCount++
		} else {
			report.UnmappedCount++
		}
	}

	return report
}

// parseOutputFormat validates and returns the output format.
func parseOutputFormat(format string) (security.OutputFormat, error) {
	switch strings.ToLower(format) {
	case "markdown", "md", "":
		return security.FormatMarkdown, nil
	case "json":
		return security.FormatJSON, nil
	case "yaml", "yml":
		return security.FormatYAML, nil
	case "csv":
		return security.FormatCSV, nil
	default:
		return "", errUtils.ErrAISecurityInvalidFormat
	}
}

// parseSource validates and returns the finding source.
func parseSource(source string) (security.Source, error) {
	switch strings.ToLower(source) {
	case "all", "":
		return security.SourceAll, nil
	case "security-hub", "securityhub":
		return security.SourceSecurityHub, nil
	case "config":
		return security.SourceConfig, nil
	case "inspector":
		return security.SourceInspector, nil
	case "guardduty":
		return security.SourceGuardDuty, nil
	case "macie":
		return security.SourceMacie, nil
	case "access-analyzer", "accessanalyzer":
		return security.SourceAccessAnalyzer, nil
	default:
		return "", errUtils.ErrAISecurityInvalidSource
	}
}

// severityMap maps severity name strings to their typed constants.
var severityMap = map[string]security.Severity{
	"CRITICAL":      security.SeverityCritical,
	"HIGH":          security.SeverityHigh,
	"MEDIUM":        security.SeverityMedium,
	"LOW":           security.SeverityLow,
	"INFORMATIONAL": security.SeverityInformational,
}

// parseSeverities parses and validates the severity filter string.
func parseSeverities(severityStr string, defaults []string) ([]security.Severity, error) {
	if severityStr == "" && len(defaults) == 0 {
		return []security.Severity{security.SeverityCritical, security.SeverityHigh}, nil
	}

	parts := strings.Split(severityStr, ",")
	if severityStr == "" {
		parts = defaults
	}

	var severities []security.Severity
	for _, p := range parts {
		sev, ok := severityMap[strings.ToUpper(strings.TrimSpace(p))]
		if !ok {
			return nil, errUtils.ErrAISecurityInvalidSeverity
		}
		severities = append(severities, sev)
	}

	return severities, nil
}

// Finding is a type alias for the security package Finding type (used in buildSecurityReport).
type Finding = security.Finding

// filterByStackAndComponent filters findings to those matching the specified stack and/or component.
// Matching is done on the mapped stack/component names (after finding-to-code mapping).
// If stack is empty, all stacks match. If component is empty, all components match.
// Unmapped findings are excluded when filtering by stack or component.
func filterByStackAndComponent(findings []Finding, stack, component string) []Finding {
	var filtered []Finding
	for i := range findings {
		f := &findings[i]

		// Unmapped findings can't match stack/component filters.
		if f.Mapping == nil || !f.Mapping.Mapped {
			continue
		}

		// Stack filter: exact match or prefix match (e.g., "plat-use2-prod" matches "plat-use2-prod-vpc").
		if stack != "" && f.Mapping.Stack != stack && !strings.HasPrefix(f.Mapping.Stack, stack+nameSep) {
			continue
		}

		// Component filter: exact match.
		if component != "" && f.Mapping.Component != component {
			continue
		}

		filtered = append(filtered, *f)
	}
	return filtered
}

// nameSep is the separator for stack name prefix matching.
const nameSep = "-"

// initReadOnlyTools creates a read-only tool registry and executor for AI security analysis.
// Returns nil, nil if tool setup fails (analysis falls back to single-prompt mode).
func initReadOnlyTools(atmosConfig *schema.AtmosConfiguration) (*tools.Registry, *tools.Executor) {
	registry := tools.NewRegistry()
	if err := atmosTools.RegisterTools(registry, atmosConfig, nil); err != nil {
		log.Debug("Failed to register tools for security analysis", "error", err)
		return nil, nil
	}

	// Read-only tools don't require permissions.
	permConfig := &permission.Config{Mode: permission.ModeAllow}
	permChecker := permission.NewChecker(permConfig, nil)
	executor := tools.NewExecutor(registry, permChecker, tools.DefaultTimeout)

	log.Debug("Initialized tools for security analysis", "count", registry.Count())
	return registry, executor
}
