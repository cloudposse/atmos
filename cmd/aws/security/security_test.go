package security

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgsecurity "github.com/cloudposse/atmos/pkg/aws/security"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParseSource(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected pkgsecurity.Source
		wantErr  bool
	}{
		{"all", "all", pkgsecurity.SourceAll, false},
		{"empty defaults to all", "", pkgsecurity.SourceAll, false},
		{"security-hub", "security-hub", pkgsecurity.SourceSecurityHub, false},
		{"securityhub alias", "securityhub", pkgsecurity.SourceSecurityHub, false},
		{"config", "config", pkgsecurity.SourceConfig, false},
		{"inspector", "inspector", pkgsecurity.SourceInspector, false},
		{"guardduty", "guardduty", pkgsecurity.SourceGuardDuty, false},
		{"macie", "macie", pkgsecurity.SourceMacie, false},
		{"access-analyzer", "access-analyzer", pkgsecurity.SourceAccessAnalyzer, false},
		{"accessanalyzer alias", "accessanalyzer", pkgsecurity.SourceAccessAnalyzer, false},
		{"case insensitive", "SecurityHub", pkgsecurity.SourceSecurityHub, false},
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
		expected []pkgsecurity.Severity
		wantErr  bool
	}{
		{
			"empty with no defaults returns critical+high",
			"", nil,
			[]pkgsecurity.Severity{pkgsecurity.SeverityCritical, pkgsecurity.SeverityHigh},
			false,
		},
		{
			"single severity",
			"critical", nil,
			[]pkgsecurity.Severity{pkgsecurity.SeverityCritical},
			false,
		},
		{
			"multiple severities",
			"critical,high,medium", nil,
			[]pkgsecurity.Severity{pkgsecurity.SeverityCritical, pkgsecurity.SeverityHigh, pkgsecurity.SeverityMedium},
			false,
		},
		{
			"case insensitive",
			"Critical,HIGH,low", nil,
			[]pkgsecurity.Severity{pkgsecurity.SeverityCritical, pkgsecurity.SeverityHigh, pkgsecurity.SeverityLow},
			false,
		},
		{
			"with whitespace",
			" critical , high ", nil,
			[]pkgsecurity.Severity{pkgsecurity.SeverityCritical, pkgsecurity.SeverityHigh},
			false,
		},
		{
			"informational",
			"informational", nil,
			[]pkgsecurity.Severity{pkgsecurity.SeverityInformational},
			false,
		},
		{
			"empty with defaults",
			"",
			[]string{"MEDIUM", "LOW"},
			[]pkgsecurity.Severity{pkgsecurity.SeverityMedium, pkgsecurity.SeverityLow},
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
	assert.Equal(t, pkgsecurity.SeverityCritical, result[0])
	assert.Equal(t, pkgsecurity.SeverityHigh, result[1])
	assert.Equal(t, pkgsecurity.SeverityMedium, result[2])
	assert.Equal(t, pkgsecurity.SeverityLow, result[3])
	assert.Equal(t, pkgsecurity.SeverityInformational, result[4])
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
	assert.Equal(t, pkgsecurity.SeverityLow, result[0])
}

func TestParseSeverities_InputOverridesDefaults(t *testing.T) {
	// When input is provided, defaults are ignored.
	result, err := parseSeverities("medium", []string{"CRITICAL", "HIGH"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, pkgsecurity.SeverityMedium, result[0])
}

func TestBuildSecurityReport(t *testing.T) {
	t.Run("empty findings", func(t *testing.T) {
		report := buildSecurityReport(nil, "prod", "vpc", nil)

		assert.Equal(t, "prod", report.Stack)
		assert.Equal(t, "vpc", report.Component)
		assert.Equal(t, 0, report.TotalFindings)
		assert.Equal(t, 0, report.MappedCount)
		assert.Equal(t, 0, report.UnmappedCount)
		assert.NotNil(t, report.SeverityCounts)
	})

	t.Run("mixed mapped and unmapped", func(t *testing.T) {
		findings := []pkgsecurity.Finding{
			{
				ID:       "f1",
				Severity: pkgsecurity.SeverityCritical,
				Mapping:  &pkgsecurity.ComponentMapping{Mapped: true},
			},
			{
				ID:       "f2",
				Severity: pkgsecurity.SeverityHigh,
				Mapping:  &pkgsecurity.ComponentMapping{Mapped: false},
			},
			{
				ID:       "f3",
				Severity: pkgsecurity.SeverityCritical,
				Mapping:  nil,
			},
		}

		report := buildSecurityReport(findings, "staging", "", nil)

		assert.Equal(t, "staging", report.Stack)
		assert.Equal(t, 3, report.TotalFindings)
		assert.Equal(t, 1, report.MappedCount)
		assert.Equal(t, 2, report.UnmappedCount)
		assert.Equal(t, 2, report.SeverityCounts[pkgsecurity.SeverityCritical])
		assert.Equal(t, 1, report.SeverityCounts[pkgsecurity.SeverityHigh])
	})

	t.Run("all mapped", func(t *testing.T) {
		findings := []pkgsecurity.Finding{
			{
				ID:       "f1",
				Severity: pkgsecurity.SeverityLow,
				Mapping:  &pkgsecurity.ComponentMapping{Mapped: true},
			},
			{
				ID:       "f2",
				Severity: pkgsecurity.SeverityLow,
				Mapping:  &pkgsecurity.ComponentMapping{Mapped: true},
			},
		}

		report := buildSecurityReport(findings, "", "", nil)

		assert.Equal(t, 2, report.TotalFindings)
		assert.Equal(t, 2, report.MappedCount)
		assert.Equal(t, 0, report.UnmappedCount)
	})

	t.Run("generated at is set", func(t *testing.T) {
		report := buildSecurityReport(nil, "", "", nil)
		assert.False(t, report.GeneratedAt.IsZero())
	})

	t.Run("all severities represented", func(t *testing.T) {
		findings := []pkgsecurity.Finding{
			{ID: "f1", Severity: pkgsecurity.SeverityCritical, Mapping: &pkgsecurity.ComponentMapping{Mapped: true}},
			{ID: "f2", Severity: pkgsecurity.SeverityHigh, Mapping: &pkgsecurity.ComponentMapping{Mapped: true}},
			{ID: "f3", Severity: pkgsecurity.SeverityMedium, Mapping: &pkgsecurity.ComponentMapping{Mapped: true}},
			{ID: "f4", Severity: pkgsecurity.SeverityLow, Mapping: &pkgsecurity.ComponentMapping{Mapped: false}},
			{ID: "f5", Severity: pkgsecurity.SeverityInformational, Mapping: nil},
		}

		report := buildSecurityReport(findings, "dev", "rds", nil)

		assert.Equal(t, "dev", report.Stack)
		assert.Equal(t, "rds", report.Component)
		assert.Equal(t, 5, report.TotalFindings)
		assert.Equal(t, 3, report.MappedCount)
		assert.Equal(t, 2, report.UnmappedCount)
		assert.Equal(t, 1, report.SeverityCounts[pkgsecurity.SeverityCritical])
		assert.Equal(t, 1, report.SeverityCounts[pkgsecurity.SeverityHigh])
		assert.Equal(t, 1, report.SeverityCounts[pkgsecurity.SeverityMedium])
		assert.Equal(t, 1, report.SeverityCounts[pkgsecurity.SeverityLow])
		assert.Equal(t, 1, report.SeverityCounts[pkgsecurity.SeverityInformational])
	})

	t.Run("all unmapped with nil mappings", func(t *testing.T) {
		findings := []pkgsecurity.Finding{
			{ID: "f1", Severity: pkgsecurity.SeverityHigh, Mapping: nil},
			{ID: "f2", Severity: pkgsecurity.SeverityHigh, Mapping: nil},
			{ID: "f3", Severity: pkgsecurity.SeverityMedium, Mapping: nil},
		}

		report := buildSecurityReport(findings, "prod", "", nil)

		assert.Equal(t, 3, report.TotalFindings)
		assert.Equal(t, 0, report.MappedCount)
		assert.Equal(t, 3, report.UnmappedCount)
		assert.Equal(t, 2, report.SeverityCounts[pkgsecurity.SeverityHigh])
		assert.Equal(t, 1, report.SeverityCounts[pkgsecurity.SeverityMedium])
	})

	t.Run("single finding mapped", func(t *testing.T) {
		findings := []pkgsecurity.Finding{
			{
				ID:       "f1",
				Severity: pkgsecurity.SeverityCritical,
				Title:    "Critical vulnerability",
				Source:   pkgsecurity.SourceSecurityHub,
				Mapping:  &pkgsecurity.ComponentMapping{Mapped: true, Stack: "prod", Component: "vpc"},
			},
		}

		report := buildSecurityReport(findings, "prod", "vpc", nil)

		assert.Equal(t, 1, report.TotalFindings)
		assert.Equal(t, 1, report.MappedCount)
		assert.Equal(t, 0, report.UnmappedCount)
		assert.Equal(t, 1, report.SeverityCounts[pkgsecurity.SeverityCritical])
		// Verify findings are preserved in the report.
		require.Len(t, report.Findings, 1)
		assert.Equal(t, "f1", report.Findings[0].ID)
		assert.Equal(t, "Critical vulnerability", report.Findings[0].Title)
		assert.Equal(t, pkgsecurity.SourceSecurityHub, report.Findings[0].Source)
	})

	t.Run("empty findings slice vs nil", func(t *testing.T) {
		// Empty slice should behave the same as nil.
		report := buildSecurityReport([]pkgsecurity.Finding{}, "prod", "vpc", nil)

		assert.Equal(t, 0, report.TotalFindings)
		assert.Equal(t, 0, report.MappedCount)
		assert.Equal(t, 0, report.UnmappedCount)
		assert.NotNil(t, report.SeverityCounts)
	})

	t.Run("unmapped finding with Mapped false", func(t *testing.T) {
		// A finding with a non-nil mapping but Mapped=false should count as unmapped.
		findings := []pkgsecurity.Finding{
			{
				ID:       "f1",
				Severity: pkgsecurity.SeverityLow,
				Mapping:  &pkgsecurity.ComponentMapping{Mapped: false, Confidence: "none"},
			},
		}

		report := buildSecurityReport(findings, "", "", nil)

		assert.Equal(t, 1, report.TotalFindings)
		assert.Equal(t, 0, report.MappedCount)
		assert.Equal(t, 1, report.UnmappedCount)
	})

	t.Run("severity counts do not include missing severities", func(t *testing.T) {
		// Only the severities present in findings should appear in the counts map.
		findings := []pkgsecurity.Finding{
			{ID: "f1", Severity: pkgsecurity.SeverityCritical, Mapping: nil},
		}

		report := buildSecurityReport(findings, "", "", nil)

		assert.Equal(t, 1, report.SeverityCounts[pkgsecurity.SeverityCritical])
		assert.Equal(t, 0, report.SeverityCounts[pkgsecurity.SeverityHigh])
		assert.Equal(t, 0, report.SeverityCounts[pkgsecurity.SeverityMedium])
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
	cmd := SecurityCmd
	// Verify the analyze subcommand exists under security.
	var foundAnalyze bool
	for _, sub := range cmd.Commands() {
		if sub.Use == "analyze" {
			foundAnalyze = true
			break
		}
	}
	assert.True(t, foundAnalyze, "security command should have analyze subcommand")
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
		{"max-findings flag", "max-findings", "500", "int"},
		{"region flag", "region", "", "string"},
		{"identity flag", "identity", "", "string"},
		{"no-group flag", "no-group", "false", "bool"},
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
	assert.NotNil(t, SecurityCmd.Args, "SecurityCmd should have Args set")
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
	assert.Equal(t, 500, defaultMaxFindings, "defaultMaxFindings should be 500")
}

func TestBuildSecurityReport_TagMappingPreserved(t *testing.T) {
	// Verify that the tag mapping is included in the report when provided.
	tagMapping := &pkgsecurity.AWSSecurityTagMapping{
		StackTag:     "custom:stack",
		ComponentTag: "custom:component",
	}
	report := buildSecurityReport(nil, "prod", "vpc", tagMapping)
	require.NotNil(t, report.TagMapping)
	assert.Equal(t, "custom:stack", report.TagMapping.StackTag)
	assert.Equal(t, "custom:component", report.TagMapping.ComponentTag)
}

func TestBuildSecurityReport_TagMappingNilWhenNotProvided(t *testing.T) {
	// Verify that tag mapping is nil when not provided.
	report := buildSecurityReport(nil, "prod", "vpc", nil)
	assert.Nil(t, report.TagMapping)
}

func TestFilterByStackAndComponent(t *testing.T) {
	findings := []pkgsecurity.Finding{
		{ID: "f1", Mapping: &pkgsecurity.ComponentMapping{Stack: "plat-use2-prod", Component: "vpc", Mapped: true}},
		{ID: "f2", Mapping: &pkgsecurity.ComponentMapping{Stack: "plat-use2-prod", Component: "s3-bucket", Mapped: true}},
		{ID: "f3", Mapping: &pkgsecurity.ComponentMapping{Stack: "plat-use2-dev", Component: "vpc", Mapped: true}},
		{ID: "f4", Mapping: &pkgsecurity.ComponentMapping{Stack: "core-use2-security", Component: "account", Mapped: true}},
		{ID: "f5", Mapping: nil}, // unmapped.
		{ID: "f6", Mapping: &pkgsecurity.ComponentMapping{Stack: "", Component: "", Mapped: false}}, // unmapped.
	}

	t.Run("filter by stack only", func(t *testing.T) {
		result := filterByStackAndComponent(findings, "plat-use2-prod", "")
		require.Len(t, result, 2)
		assert.Equal(t, "f1", result[0].ID)
		assert.Equal(t, "f2", result[1].ID)
	})

	t.Run("filter by component only", func(t *testing.T) {
		result := filterByStackAndComponent(findings, "", "vpc")
		require.Len(t, result, 2)
		assert.Equal(t, "f1", result[0].ID)
		assert.Equal(t, "f3", result[1].ID)
	})

	t.Run("filter by both stack and component", func(t *testing.T) {
		result := filterByStackAndComponent(findings, "plat-use2-prod", "vpc")
		require.Len(t, result, 1)
		assert.Equal(t, "f1", result[0].ID)
	})

	t.Run("no match returns empty", func(t *testing.T) {
		result := filterByStackAndComponent(findings, "nonexistent", "")
		assert.Empty(t, result)
	})

	t.Run("unmapped findings excluded", func(t *testing.T) {
		result := filterByStackAndComponent(findings, "plat-use2-prod", "")
		for _, f := range result {
			assert.True(t, f.Mapping.Mapped)
		}
	})

	t.Run("empty filters returns all mapped", func(t *testing.T) {
		// When both filters empty, all mapped findings pass through.
		result := filterByStackAndComponent(findings, "", "")
		assert.Len(t, result, 4) // 4 mapped, 2 unmapped excluded.
	})
}

func TestAuthenticateAndResolveAWS_EmptyIdentity(t *testing.T) {
	// Empty identity should return nil without error.
	authCtx, err := authenticateAndResolveAWS(nil, "")
	require.NoError(t, err)
	assert.Nil(t, authCtx)
}

func TestExtractAWSAuthContext_NilStackInfo(t *testing.T) {
	// Mock auth manager with nil stack info.
	mockMgr := &mockStackInfoProvider{stackInfo: nil}
	_, err := extractAWSAuthContext(mockMgr, "test-identity")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no AWS credentials were produced")
}

func TestExtractAWSAuthContext_NilAuthContext(t *testing.T) {
	// Stack info exists but AuthContext is nil.
	mockMgr := &mockStackInfoProvider{
		stackInfo: &schema.ConfigAndStacksInfo{AuthContext: nil},
	}
	_, err := extractAWSAuthContext(mockMgr, "test-identity")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no AWS credentials were produced")
}

func TestExtractAWSAuthContext_NilAWS(t *testing.T) {
	// AuthContext exists but AWS is nil.
	mockMgr := &mockStackInfoProvider{
		stackInfo: &schema.ConfigAndStacksInfo{
			AuthContext: &schema.AuthContext{AWS: nil},
		},
	}
	_, err := extractAWSAuthContext(mockMgr, "test-identity")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no AWS credentials were produced")
}

func TestExtractAWSAuthContext_Success(t *testing.T) {
	// Full auth context present.
	mockMgr := &mockStackInfoProvider{
		stackInfo: &schema.ConfigAndStacksInfo{
			AuthContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile:         "test-profile",
					CredentialsFile: "/tmp/creds",
					ConfigFile:      "/tmp/config",
					Region:          "us-west-2",
				},
			},
		},
	}
	authCtx, err := extractAWSAuthContext(mockMgr, "test-identity")
	require.NoError(t, err)
	assert.Equal(t, "test-profile", authCtx.Profile)
	assert.Equal(t, "us-west-2", authCtx.Region)
}

// mockStackInfoProvider implements stackInfoProvider for testing extractAWSAuthContext.
type mockStackInfoProvider struct {
	stackInfo *schema.ConfigAndStacksInfo
}

func (m *mockStackInfoProvider) GetStackInfo() *schema.ConfigAndStacksInfo {
	return m.stackInfo
}
