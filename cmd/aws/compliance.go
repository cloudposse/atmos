package aws

import (
	"context"
	_ "embed"
	"fmt"
	"os"
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

// complianceCmd represents the aws compliance command.
var complianceCmd = &cobra.Command{
	Use:   "compliance",
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

		log.Debug("Running compliance report",
			"stack", stack,
			"framework", framework,
			"format", formatStr,
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
			ui.Writef("📊 Generating compliance report...\n")
		}

		fetcher := security.NewFindingFetcher(&atmosConfig)
		renderer := security.NewReportRenderer(outputFormat)

		for _, fw := range frameworks {
			report, err := fetcher.FetchComplianceStatus(ctx, fw, stack)
			if err != nil {
				return fmt.Errorf("%w: %w", errUtils.ErrAISecurityFetchFailed, err)
			}

			if report == nil {
				if outputFormat == security.FormatMarkdown {
					ui.Writef("No compliance data available for framework: %s\n", fw)
				}
				continue
			}

			if err := renderer.RenderComplianceReport(os.Stdout, report); err != nil {
				return err
			}
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
		flags.WithStringFlag("controls", "", "", "Specific control IDs to check"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("framework", "ATMOS_AWS_COMPLIANCE_FRAMEWORK"),
		flags.WithEnvVars("format", "ATMOS_AWS_COMPLIANCE_FORMAT"),
	)

	// Register flags on the command.
	complianceParser.RegisterFlags(complianceCmd)

	// Bind flags to Viper for environment variable support.
	if err := complianceParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	awsCmd.AddCommand(complianceCmd)
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
