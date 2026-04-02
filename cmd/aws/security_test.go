package aws

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/aws/security"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParseOutputFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected security.OutputFormat
		wantErr  bool
	}{
		{"markdown", "markdown", security.FormatMarkdown, false},
		{"md alias", "md", security.FormatMarkdown, false},
		{"empty defaults to markdown", "", security.FormatMarkdown, false},
		{"json", "json", security.FormatJSON, false},
		{"yaml", "yaml", security.FormatYAML, false},
		{"yml alias", "yml", security.FormatYAML, false},
		{"csv", "csv", security.FormatCSV, false},
		{"case insensitive", "JSON", security.FormatJSON, false},
		{"mixed case", "Yaml", security.FormatYAML, false},
		{"invalid", "xml", "", true},
		{"invalid format", "html", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseOutputFormat(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseOutputFormat_ErrorType(t *testing.T) {
	// Verify that invalid format errors wrap the correct sentinel.
	tests := []struct {
		name  string
		input string
	}{
		{"xml format", "xml"},
		{"html format", "html"},
		{"text format", "text"},
		{"pdf format", "pdf"},
		{"whitespace only", "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseOutputFormat(tt.input)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrAISecurityInvalidFormat),
				"expected ErrAISecurityInvalidFormat, got: %v", err)
		})
	}
}

func TestParseSource(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected security.Source
		wantErr  bool
	}{
		{"all", "all", security.SourceAll, false},
		{"empty defaults to all", "", security.SourceAll, false},
		{"security-hub", "security-hub", security.SourceSecurityHub, false},
		{"securityhub alias", "securityhub", security.SourceSecurityHub, false},
		{"config", "config", security.SourceConfig, false},
		{"inspector", "inspector", security.SourceInspector, false},
		{"guardduty", "guardduty", security.SourceGuardDuty, false},
		{"macie", "macie", security.SourceMacie, false},
		{"access-analyzer", "access-analyzer", security.SourceAccessAnalyzer, false},
		{"accessanalyzer alias", "accessanalyzer", security.SourceAccessAnalyzer, false},
		{"case insensitive", "SecurityHub", security.SourceSecurityHub, false},
		{"invalid", "unknown", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSource(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseSource_ErrorType(t *testing.T) {
	// Verify that invalid source errors wrap the correct sentinel.
	tests := []struct {
		name  string
		input string
	}{
		{"unknown service", "unknown"},
		{"cloudwatch", "cloudwatch"},
		{"iam", "iam"},
		{"whitespace only", "  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSource(tt.input)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrAISecurityInvalidSource),
				"expected ErrAISecurityInvalidSource, got: %v", err)
		})
	}
}

func TestParseSeverities(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		defaults []string
		expected []security.Severity
		wantErr  bool
	}{
		{
			"empty with no defaults returns critical+high",
			"", nil,
			[]security.Severity{security.SeverityCritical, security.SeverityHigh},
			false,
		},
		{
			"single severity",
			"critical", nil,
			[]security.Severity{security.SeverityCritical},
			false,
		},
		{
			"multiple severities",
			"critical,high,medium", nil,
			[]security.Severity{security.SeverityCritical, security.SeverityHigh, security.SeverityMedium},
			false,
		},
		{
			"case insensitive",
			"Critical,HIGH,low", nil,
			[]security.Severity{security.SeverityCritical, security.SeverityHigh, security.SeverityLow},
			false,
		},
		{
			"with whitespace",
			" critical , high ", nil,
			[]security.Severity{security.SeverityCritical, security.SeverityHigh},
			false,
		},
		{
			"informational",
			"informational", nil,
			[]security.Severity{security.SeverityInformational},
			false,
		},
		{
			"empty with defaults",
			"",
			[]string{"MEDIUM", "LOW"},
			[]security.Severity{security.SeverityMedium, security.SeverityLow},
			false,
		},
		{
			"invalid severity",
			"critical,unknown", nil,
			nil, true,
		},
		{
			"invalid default",
			"",
			[]string{"invalid"},
			nil, true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSeverities(tt.input, tt.defaults)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseSeverities_AllSeverities(t *testing.T) {
	// Parse all five severity levels at once.
	result, err := parseSeverities("critical,high,medium,low,informational", nil)
	require.NoError(t, err)
	require.Len(t, result, 5)
	assert.Equal(t, security.SeverityCritical, result[0])
	assert.Equal(t, security.SeverityHigh, result[1])
	assert.Equal(t, security.SeverityMedium, result[2])
	assert.Equal(t, security.SeverityLow, result[3])
	assert.Equal(t, security.SeverityInformational, result[4])
}

func TestParseSeverities_ErrorType(t *testing.T) {
	// Verify that invalid severity errors wrap the correct sentinel.
	tests := []struct {
		name     string
		input    string
		defaults []string
	}{
		{"unknown severity in input", "critical,bogus", nil},
		{"empty string severity in list", "critical,,high", nil},
		{"invalid default severity", "", []string{"CRITICAL", "BOGUS"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSeverities(tt.input, tt.defaults)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrAISecurityInvalidSeverity),
				"expected ErrAISecurityInvalidSeverity, got: %v", err)
		})
	}
}

func TestParseSeverities_DefaultsOverrideBuiltin(t *testing.T) {
	// When input is empty but defaults are provided, defaults take precedence over the builtin critical+high.
	result, err := parseSeverities("", []string{"LOW"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, security.SeverityLow, result[0])
}

func TestParseSeverities_InputOverridesDefaults(t *testing.T) {
	// When input is provided, defaults are ignored.
	result, err := parseSeverities("medium", []string{"CRITICAL", "HIGH"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, security.SeverityMedium, result[0])
}

func TestBuildSecurityReport(t *testing.T) {
	t.Run("empty findings", func(t *testing.T) {
		report := buildSecurityReport(nil, "prod", "vpc")

		assert.Equal(t, "prod", report.Stack)
		assert.Equal(t, "vpc", report.Component)
		assert.Equal(t, 0, report.TotalFindings)
		assert.Equal(t, 0, report.MappedCount)
		assert.Equal(t, 0, report.UnmappedCount)
		assert.NotNil(t, report.SeverityCounts)
	})

	t.Run("mixed mapped and unmapped", func(t *testing.T) {
		findings := []security.Finding{
			{
				ID:       "f1",
				Severity: security.SeverityCritical,
				Mapping:  &security.ComponentMapping{Mapped: true},
			},
			{
				ID:       "f2",
				Severity: security.SeverityHigh,
				Mapping:  &security.ComponentMapping{Mapped: false},
			},
			{
				ID:       "f3",
				Severity: security.SeverityCritical,
				Mapping:  nil,
			},
		}

		report := buildSecurityReport(findings, "staging", "")

		assert.Equal(t, "staging", report.Stack)
		assert.Equal(t, 3, report.TotalFindings)
		assert.Equal(t, 1, report.MappedCount)
		assert.Equal(t, 2, report.UnmappedCount)
		assert.Equal(t, 2, report.SeverityCounts[security.SeverityCritical])
		assert.Equal(t, 1, report.SeverityCounts[security.SeverityHigh])
	})

	t.Run("all mapped", func(t *testing.T) {
		findings := []security.Finding{
			{
				ID:       "f1",
				Severity: security.SeverityLow,
				Mapping:  &security.ComponentMapping{Mapped: true},
			},
			{
				ID:       "f2",
				Severity: security.SeverityLow,
				Mapping:  &security.ComponentMapping{Mapped: true},
			},
		}

		report := buildSecurityReport(findings, "", "")

		assert.Equal(t, 2, report.TotalFindings)
		assert.Equal(t, 2, report.MappedCount)
		assert.Equal(t, 0, report.UnmappedCount)
	})

	t.Run("generated at is set", func(t *testing.T) {
		report := buildSecurityReport(nil, "", "")
		assert.False(t, report.GeneratedAt.IsZero())
	})

	t.Run("all severities represented", func(t *testing.T) {
		findings := []security.Finding{
			{ID: "f1", Severity: security.SeverityCritical, Mapping: &security.ComponentMapping{Mapped: true}},
			{ID: "f2", Severity: security.SeverityHigh, Mapping: &security.ComponentMapping{Mapped: true}},
			{ID: "f3", Severity: security.SeverityMedium, Mapping: &security.ComponentMapping{Mapped: true}},
			{ID: "f4", Severity: security.SeverityLow, Mapping: &security.ComponentMapping{Mapped: false}},
			{ID: "f5", Severity: security.SeverityInformational, Mapping: nil},
		}

		report := buildSecurityReport(findings, "dev", "rds")

		assert.Equal(t, "dev", report.Stack)
		assert.Equal(t, "rds", report.Component)
		assert.Equal(t, 5, report.TotalFindings)
		assert.Equal(t, 3, report.MappedCount)
		assert.Equal(t, 2, report.UnmappedCount)
		assert.Equal(t, 1, report.SeverityCounts[security.SeverityCritical])
		assert.Equal(t, 1, report.SeverityCounts[security.SeverityHigh])
		assert.Equal(t, 1, report.SeverityCounts[security.SeverityMedium])
		assert.Equal(t, 1, report.SeverityCounts[security.SeverityLow])
		assert.Equal(t, 1, report.SeverityCounts[security.SeverityInformational])
	})

	t.Run("all unmapped with nil mappings", func(t *testing.T) {
		findings := []security.Finding{
			{ID: "f1", Severity: security.SeverityHigh, Mapping: nil},
			{ID: "f2", Severity: security.SeverityHigh, Mapping: nil},
			{ID: "f3", Severity: security.SeverityMedium, Mapping: nil},
		}

		report := buildSecurityReport(findings, "prod", "")

		assert.Equal(t, 3, report.TotalFindings)
		assert.Equal(t, 0, report.MappedCount)
		assert.Equal(t, 3, report.UnmappedCount)
		assert.Equal(t, 2, report.SeverityCounts[security.SeverityHigh])
		assert.Equal(t, 1, report.SeverityCounts[security.SeverityMedium])
	})

	t.Run("single finding mapped", func(t *testing.T) {
		findings := []security.Finding{
			{
				ID:       "f1",
				Severity: security.SeverityCritical,
				Title:    "Critical vulnerability",
				Source:   security.SourceSecurityHub,
				Mapping:  &security.ComponentMapping{Mapped: true, Stack: "prod", Component: "vpc"},
			},
		}

		report := buildSecurityReport(findings, "prod", "vpc")

		assert.Equal(t, 1, report.TotalFindings)
		assert.Equal(t, 1, report.MappedCount)
		assert.Equal(t, 0, report.UnmappedCount)
		assert.Equal(t, 1, report.SeverityCounts[security.SeverityCritical])
		// Verify findings are preserved in the report.
		require.Len(t, report.Findings, 1)
		assert.Equal(t, "f1", report.Findings[0].ID)
		assert.Equal(t, "Critical vulnerability", report.Findings[0].Title)
		assert.Equal(t, security.SourceSecurityHub, report.Findings[0].Source)
	})

	t.Run("empty findings slice vs nil", func(t *testing.T) {
		// Empty slice should behave the same as nil.
		report := buildSecurityReport([]security.Finding{}, "prod", "vpc")

		assert.Equal(t, 0, report.TotalFindings)
		assert.Equal(t, 0, report.MappedCount)
		assert.Equal(t, 0, report.UnmappedCount)
		assert.NotNil(t, report.SeverityCounts)
	})

	t.Run("unmapped finding with Mapped false", func(t *testing.T) {
		// A finding with a non-nil mapping but Mapped=false should count as unmapped.
		findings := []security.Finding{
			{
				ID:       "f1",
				Severity: security.SeverityLow,
				Mapping:  &security.ComponentMapping{Mapped: false, Confidence: "none"},
			},
		}

		report := buildSecurityReport(findings, "", "")

		assert.Equal(t, 1, report.TotalFindings)
		assert.Equal(t, 0, report.MappedCount)
		assert.Equal(t, 1, report.UnmappedCount)
	})

	t.Run("severity counts do not include missing severities", func(t *testing.T) {
		// Only the severities present in findings should appear in the counts map.
		findings := []security.Finding{
			{ID: "f1", Severity: security.SeverityCritical, Mapping: nil},
		}

		report := buildSecurityReport(findings, "", "")

		assert.Equal(t, 1, report.SeverityCounts[security.SeverityCritical])
		assert.Equal(t, 0, report.SeverityCounts[security.SeverityHigh])
		assert.Equal(t, 0, report.SeverityCounts[security.SeverityMedium])
	})
}

func TestSecurityAnalyzeFileFlag(t *testing.T) {
	// Verify the --file flag is registered on the analyze subcommand.
	flag := securityAnalyzeCmd.Flags().Lookup("file")
	require.NotNil(t, flag, "securityAnalyzeCmd should have --file flag")
	assert.Equal(t, "", flag.DefValue, "--file default should be empty")
	assert.Equal(t, "string", flag.Value.Type(), "--file should be a string flag")
}

func TestSecuritySubcommandRegistered(t *testing.T) {
	cmd := awsCmd
	var foundSecurity bool
	for _, sub := range cmd.Commands() {
		if sub.Use != "security" {
			continue
		}

		foundSecurity = true
		// Verify the analyze subcommand exists under security.
		var foundAnalyze bool
		for _, subSub := range sub.Commands() {
			if subSub.Use == "analyze" {
				foundAnalyze = true
				break
			}
		}
		assert.True(t, foundAnalyze, "security command should have analyze subcommand")
		break
	}
	assert.True(t, foundSecurity, "aws command should have security subcommand")
}

func TestSecurityAnalyzeAllFlagsRegistered(t *testing.T) {
	// Verify all expected flags are registered on securityAnalyzeCmd.
	tests := []struct {
		name     string
		flagName string
		defValue string
		flagType string
	}{
		{"stack flag", "stack", "", "string"},
		{"component flag", "component", "", "string"},
		{"severity flag", "severity", "critical,high", "string"},
		{"source flag", "source", "all", "string"},
		{"framework flag", "framework", "", "string"},
		{"format flag", "format", "markdown", "string"},
		{"file flag", "file", "", "string"},
		{"max-findings flag", "max-findings", "50", "int"},
		{"region flag", "region", "", "string"},
		{"identity flag", "identity", "", "string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := securityAnalyzeCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, f, "flag %q should be registered", tt.flagName)
			assert.Equal(t, tt.defValue, f.DefValue, "flag %q default", tt.flagName)
			assert.Equal(t, tt.flagType, f.Value.Type(), "flag %q type", tt.flagName)
		})
	}
}

func TestSecurityAnalyzeFlagShorthand(t *testing.T) {
	// Verify shorthand aliases for key flags.
	tests := []struct {
		name      string
		flagName  string
		shorthand string
	}{
		{"stack shorthand", "stack", "s"},
		{"component shorthand", "component", "c"},
		{"format shorthand", "format", "f"},
		{"identity shorthand", "identity", "i"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := securityAnalyzeCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, f, "flag %q should be registered", tt.flagName)
			assert.Equal(t, tt.shorthand, f.Shorthand, "flag %q shorthand", tt.flagName)
		})
	}
}

func TestSecurityCmdUsesNoArgs(t *testing.T) {
	// Both security and security analyze commands should accept no positional args.
	assert.NotNil(t, securityCmd.Args, "securityCmd should have Args set")
	assert.NotNil(t, securityAnalyzeCmd.Args, "securityAnalyzeCmd should have Args set")
}

func TestSeverityMapCompleteness(t *testing.T) {
	// Verify that the severityMap covers all expected severity levels.
	expectedSeverities := []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "INFORMATIONAL"}
	for _, sev := range expectedSeverities {
		_, ok := severityMap[sev]
		assert.True(t, ok, "severityMap should contain %q", sev)
	}
	assert.Len(t, severityMap, len(expectedSeverities), "severityMap should have exactly %d entries", len(expectedSeverities))
}

func TestDefaultMaxFindings(t *testing.T) {
	// Verify the default constant matches expectations.
	assert.Equal(t, 50, defaultMaxFindings, "defaultMaxFindings should be 50")
}

func TestResolveAuthContext_EmptyIdentity(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	authCtx, err := resolveAuthContext(atmosConfig, "")
	require.NoError(t, err)
	assert.Nil(t, authCtx)
}
