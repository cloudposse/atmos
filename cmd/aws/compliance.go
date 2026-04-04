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
	"github.com/cloudposse/atmos/pkg/aws/security"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_aws_compliance.md
var complianceLongMarkdown string

// complianceParser handles flag parsing with Viper precedence for the compliance command.
var complianceParser *flags.StandardParser

// complianceCmd is the parent command for compliance subcommands.
var complianceCmd = &cobra.Command{
	Use:   "compliance",
	Short: "AWS compliance commands",
	Long:  "Commands for generating compliance posture reports against industry frameworks.",
	Args:  cobra.NoArgs,
}

// complianceReportCmd represents the aws compliance report command.
var complianceReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate compliance posture reports",
	Long:  complianceLongMarkdown,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bind parsed flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := complianceParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flags from Viper (supports CLI > ENV > config > defaults).
		stack := v.GetString("stack")
		framework := v.GetString("framework")
		formatStr := v.GetString("format")
		fileOutput := v.GetString("file")
		controlsStr := v.GetString("controls")
		identityFlag := v.GetString("identity")

		// Parse comma-separated control IDs into a set for filtering.
		controlFilter := parseControlFilter(controlsStr)

		// Initialize configuration with global flags (--base-path, --config, etc.).
		globalFlags := flags.ParseGlobalFlags(cmd, v)
		configAndStacksInfo := schema.ConfigAndStacksInfo{
			BasePath:                globalFlags.BasePath,
			AtmosConfigFilesFromArg: globalFlags.Config,
			AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
			ProfilesFromArg:         globalFlags.Profile,
		}
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

		// Validate output format.
		outputFormat, err := parseOutputFormat(formatStr)
		if err != nil {
			return err
		}

		// Validate framework if specified.
		if framework != "" {
			if err := validateFramework(framework); err != nil {
				return err
			}
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
		if err := validateAWSCredentials(credCtx, "", authCtx); err != nil {
			return err
		}

		log.Debug("Running compliance report",
			"stack", stack,
			"framework", framework,
			"format", formatStr,
			"controls", controlsStr,
		)

		// Create context with timeout.
		timeoutSeconds := 120
		if atmosConfig.AI.TimeoutSeconds > 0 {
			timeoutSeconds = atmosConfig.AI.TimeoutSeconds
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()

		// Determine which frameworks to report on.
		frameworks := atmosConfig.AWS.Security.Frameworks
		if framework != "" {
			frameworks = []string{framework}
		}
		if len(frameworks) == 0 {
			frameworks = []string{"cis-aws"}
		}

		// Fetch compliance status for each framework.
		if outputFormat == security.FormatMarkdown {
			ui.Info("Generating compliance report...")
		}

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

		fetcher := security.NewFindingFetcher(&atmosConfig, authCtx)
		renderer := security.NewReportRenderer(outputFormat)

		for _, fw := range frameworks {
			report, err := fetcher.FetchComplianceStatus(ctx, fw, stack)
			if err != nil {
				return fmt.Errorf("%w: %w", errUtils.ErrAISecurityFetchFailed, err)
			}

			if report == nil {
				if outputFormat == security.FormatMarkdown {
					ui.Warningf("No compliance data available for framework: %s", fw)
				}
				continue
			}

			// Filter report to specific control IDs if --controls was provided.
			if len(controlFilter) > 0 {
				report = filterComplianceReport(report, controlFilter)
			}

			// For Markdown to stdout, render with colors via ui.Markdown().
			if outputFormat == security.FormatMarkdown && fileOutput == "" {
				var buf strings.Builder
				if err := renderer.RenderComplianceReport(&buf, report); err != nil {
					return err
				}
				ui.Markdown(buf.String())
			} else {
				if err := renderer.RenderComplianceReport(output, report); err != nil {
					return err
				}
			}
		}

		if fileOutput != "" {
			ui.Successf("Report saved to %s", fileOutput)
		}

		return nil
	},
}

func init() {
	// Create parser with compliance-specific flags using functional options.
	complianceParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Target stack"),
		flags.WithStringFlag("framework", "", "", "Compliance framework: cis-aws, pci-dss, soc2, hipaa, nist"),
		flags.WithStringFlag("format", "f", "markdown", "Output format: markdown, json, yaml, csv"),
		flags.WithStringFlag("file", "", "", "Write output to file instead of stdout"),
		flags.WithStringFlag("controls", "", "", "Specific control IDs to check"),
		flags.WithStringFlag("identity", "i", "", "Atmos Auth identity for AWS credentials"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("identity", "ATMOS_AWS_SECURITY_IDENTITY"),
		flags.WithEnvVars("framework", "ATMOS_AWS_COMPLIANCE_FRAMEWORK"),
		flags.WithEnvVars("format", "ATMOS_AWS_COMPLIANCE_FORMAT"),
	)

	// Register flags on the report subcommand.
	complianceParser.RegisterFlags(complianceReportCmd)

	// Bind flags to Viper for environment variable support.
	if err := complianceParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	complianceCmd.AddCommand(complianceReportCmd)
	awsCmd.AddCommand(complianceCmd)
}

// parseControlFilter parses a comma-separated list of control IDs into a set.
// Returns nil if the input is empty, meaning no filtering should be applied.
func parseControlFilter(controlsStr string) map[string]bool {
	controlsStr = strings.TrimSpace(controlsStr)
	if controlsStr == "" {
		return nil
	}
	filter := make(map[string]bool)
	for _, id := range strings.Split(controlsStr, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			filter[id] = true
		}
	}
	return filter
}

// filterComplianceReport returns a copy of the report containing only the controls
// whose IDs match the given filter set. Counts are recalculated accordingly.
func filterComplianceReport(report *security.ComplianceReport, controlFilter map[string]bool) *security.ComplianceReport {
	filtered := make([]security.ComplianceControl, 0, len(report.FailingDetails))
	for _, ctrl := range report.FailingDetails {
		if controlFilter[ctrl.ControlID] {
			filtered = append(filtered, ctrl)
		}
	}

	// Build a new report with recalculated counts.
	filteredReport := *report
	filteredReport.FailingDetails = filtered
	filteredReport.FailingControls = len(filtered)
	filteredReport.TotalControls = len(filtered) + report.PassingControls
	if filteredReport.TotalControls > 0 {
		const percentMultiplier = 100
		filteredReport.ScorePercent = float64(report.PassingControls) / float64(filteredReport.TotalControls) * percentMultiplier
	} else {
		filteredReport.ScorePercent = 0
	}
	return &filteredReport
}

// validateFramework checks that the framework name is valid.
func validateFramework(framework string) error {
	validFrameworks := map[string]bool{
		"cis-aws": true,
		"pci-dss": true,
		"soc2":    true,
		"hipaa":   true,
		"nist":    true,
	}
	if !validFrameworks[framework] {
		return errUtils.ErrAISecurityInvalidFramework
	}
	return nil
}
