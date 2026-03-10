package aws

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/aws/security"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// defaultMaxFindings is the default maximum number of security findings to analyze.
const defaultMaxFindings = 50

//go:embed markdown/atmos_aws_security.md
var securityLongMarkdown string

// securityParser handles flag parsing with Viper precedence for the security command.
var securityParser *flags.StandardParser

// securityCmd represents the aws security command.
var securityCmd = &cobra.Command{
	Use:   "security",
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
		maxFindings := v.GetInt("max-findings")
		useAI := v.GetBool("ai")
		region := v.GetString("region")

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

		opts := security.QueryOptions{
			Stack:       stack,
			Component:   component,
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
		timeoutSeconds := 120
		if atmosConfig.AI.TimeoutSeconds > 0 {
			timeoutSeconds = atmosConfig.AI.TimeoutSeconds
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()

		// Fetch findings.
		if outputFormat == security.FormatMarkdown {
			ui.Writef("🔍 Fetching security findings...\n")
		}
		fetcher := security.NewFindingFetcher(&atmosConfig)
		findings, err := fetcher.FetchFindings(ctx, &opts)
		if err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrAISecurityFetchFailed, err)
		}

		if len(findings) == 0 {
			if outputFormat == security.FormatMarkdown {
				ui.Writef("✅ No security findings match the specified filters.\n")
			}
			return nil
		}

		// Map findings to Atmos components.
		if outputFormat == security.FormatMarkdown {
			ui.Writef("🗺️  Mapping %d findings to Atmos components...\n", len(findings))
		}
		mapper := security.NewComponentMapper(&atmosConfig)
		findings, err = mapper.MapFindings(ctx, findings)
		if err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrAISecurityMappingFailed, err)
		}

		// AI analysis (only when --ai flag is set).
		if useAI {
			if outputFormat == security.FormatMarkdown {
				ui.Writef("🤖 Analyzing findings with AI...\n")
			}
			analyzer, analyzerErr := security.NewFindingAnalyzer(ctx, &atmosConfig)
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
		report := buildSecurityReport(findings, stack, component)

		// Render output.
		renderer := security.NewReportRenderer(outputFormat)
		return renderer.RenderSecurityReport(os.Stdout, report)
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
		flags.WithIntFlag("max-findings", "", defaultMaxFindings, "Maximum findings to analyze"),
		flags.WithStringFlag("region", "", "", "AWS region override"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("format", "ATMOS_AWS_SECURITY_FORMAT"),
		flags.WithEnvVars("max-findings", "ATMOS_AWS_SECURITY_MAX_FINDINGS"),
		flags.WithEnvVars("region", "ATMOS_AWS_SECURITY_REGION"),
	)

	// Register flags on the command.
	securityParser.RegisterFlags(securityCmd)

	// Bind flags to Viper for environment variable support.
	if err := securityParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	awsCmd.AddCommand(securityCmd)
}

// buildSecurityReport constructs a Report from mapped findings.
func buildSecurityReport(findings []Finding, stack, component string) *security.Report {
	report := &security.Report{
		GeneratedAt:    time.Now().UTC(),
		Stack:          stack,
		Component:      component,
		TotalFindings:  len(findings),
		SeverityCounts: make(map[security.Severity]int),
		Findings:       findings,
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
