package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/aws/security"
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
}

func TestSecuritySubcommandRegistered(t *testing.T) {
	cmd := awsCmd
	var found bool
	for _, sub := range cmd.Commands() {
		if sub.Use == "security" {
			found = true
			break
		}
	}
	assert.True(t, found, "aws command should have security subcommand")
}
