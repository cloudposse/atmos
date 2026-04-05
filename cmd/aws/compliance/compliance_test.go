package compliance

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/aws/security"
)

func TestValidateFramework(t *testing.T) {
	tests := []struct {
		name      string
		framework string
		wantErr   bool
	}{
		{"cis-aws valid", "cis-aws", false},
		{"pci-dss valid", "pci-dss", false},
		{"soc2 valid", "soc2", false},
		{"hipaa valid", "hipaa", false},
		{"nist valid", "nist", false},
		{"invalid framework", "iso-27001", true},
		{"empty string", "", true},
		{"random string", "foo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFramework(tt.framework)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateFramework_ErrorType(t *testing.T) {
	// Verify that invalid framework errors are the correct sentinel.
	tests := []struct {
		name      string
		framework string
	}{
		{"iso-27001", "iso-27001"},
		{"empty", ""},
		{"random", "foo"},
		{"fedramp", "fedramp"},
		{"gdpr", "gdpr"},
		{"case sensitive cis", "CIS-AWS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFramework(tt.framework)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrAISecurityInvalidFramework),
				"expected ErrAISecurityInvalidFramework, got: %v", err)
		})
	}
}

func TestValidateFramework_AllValidFrameworks(t *testing.T) {
	// Exhaustively test all valid frameworks.
	validFrameworks := []string{"cis-aws", "pci-dss", "soc2", "hipaa", "nist"}
	for _, fw := range validFrameworks {
		t.Run(fw, func(t *testing.T) {
			err := validateFramework(fw)
			require.NoError(t, err, "framework %q should be valid", fw)
		})
	}
}

func TestValidateFramework_CaseSensitive(t *testing.T) {
	// Framework validation should be case-sensitive (all lowercase).
	tests := []struct {
		name      string
		framework string
	}{
		{"uppercase CIS-AWS", "CIS-AWS"},
		{"mixed case Hipaa", "Hipaa"},
		{"uppercase NIST", "NIST"},
		{"uppercase PCI-DSS", "PCI-DSS"},
		{"uppercase SOC2", "SOC2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFramework(tt.framework)
			require.Error(t, err, "framework %q (wrong case) should be invalid", tt.framework)
		})
	}
}

func TestComplianceReportFileFlag(t *testing.T) {
	// Verify the --file flag is registered on the report subcommand.
	flag := complianceReportCmd.Flags().Lookup("file")
	require.NotNil(t, flag, "complianceReportCmd should have --file flag")
	assert.Equal(t, "", flag.DefValue, "--file default should be empty")
	assert.Equal(t, "string", flag.Value.Type(), "--file should be a string flag")
}

func TestComplianceSubcommandRegistered(t *testing.T) {
	cmd := ComplianceCmd
	// Verify the report subcommand exists under compliance.
	var foundReport bool
	for _, sub := range cmd.Commands() {
		if sub.Use == "report" {
			foundReport = true
			break
		}
	}
	assert.True(t, foundReport, "compliance command should have report subcommand")
}

func TestComplianceReportAllFlagsRegistered(t *testing.T) {
	// Verify all expected flags are registered on complianceReportCmd.
	tests := []struct {
		name     string
		flagName string
		defValue string
		flagType string
	}{
		{"stack flag", "stack", "", "string"},
		{"framework flag", "framework", "", "string"},
		{"format flag", "format", "markdown", "string"},
		{"file flag", "file", "", "string"},
		{"controls flag", "controls", "", "string"},
		{"identity flag", "identity", "", "string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := complianceReportCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, f, "flag %q should be registered", tt.flagName)
			assert.Equal(t, tt.defValue, f.DefValue, "flag %q default", tt.flagName)
			assert.Equal(t, tt.flagType, f.Value.Type(), "flag %q type", tt.flagName)
		})
	}
}

func TestComplianceReportFlagShorthand(t *testing.T) {
	// Verify shorthand aliases for key flags.
	tests := []struct {
		name      string
		flagName  string
		shorthand string
	}{
		{"stack shorthand", "stack", "s"},
		{"format shorthand", "format", "f"},
		{"identity shorthand", "identity", "i"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := complianceReportCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, f, "flag %q should be registered", tt.flagName)
			assert.Equal(t, tt.shorthand, f.Shorthand, "flag %q shorthand", tt.flagName)
		})
	}
}

func TestComplianceCmdUsesNoArgs(t *testing.T) {
	// Both compliance and compliance report commands should accept no positional args.
	assert.NotNil(t, ComplianceCmd.Args, "ComplianceCmd should have Args set")
	assert.NotNil(t, complianceReportCmd.Args, "complianceReportCmd should have Args set")
}

func TestComplianceCmdAttributes(t *testing.T) {
	// Verify compliance command metadata.
	assert.Equal(t, "compliance", ComplianceCmd.Use)
	assert.Contains(t, ComplianceCmd.Short, "compliance")
	assert.Contains(t, ComplianceCmd.Long, "compliance")
}

func TestComplianceReportCmdAttributes(t *testing.T) {
	// Verify the report subcommand metadata.
	assert.Equal(t, "report", complianceReportCmd.Use)
	assert.Contains(t, complianceReportCmd.Short, "compliance")
	assert.NotNil(t, complianceReportCmd.RunE, "complianceReportCmd should have RunE set")
}

func TestValidateFramework_MultipleInSequence(t *testing.T) {
	// Simulate validating multiple frameworks in sequence, like the compliance
	// command does when processing framework lists.
	frameworks := []string{"cis-aws", "pci-dss", "soc2", "hipaa", "nist"}
	for _, fw := range frameworks {
		err := validateFramework(fw)
		require.NoError(t, err, "framework %q should validate successfully in sequence", fw)
	}
}

func TestValidateFramework_InvalidInSequence(t *testing.T) {
	// When validating a list of frameworks, an invalid one should be caught.
	frameworks := []string{"cis-aws", "pci-dss", "invalid-framework", "hipaa"}
	for _, fw := range frameworks {
		err := validateFramework(fw)
		if fw == "invalid-framework" {
			require.Error(t, err, "framework %q should fail validation", fw)
			assert.True(t, errors.Is(err, errUtils.ErrAISecurityInvalidFramework))
		} else {
			require.NoError(t, err, "framework %q should pass validation", fw)
		}
	}
}

func TestParseControlFilter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]bool
	}{
		{
			"empty string returns nil",
			"",
			nil,
		},
		{
			"whitespace only returns nil",
			"   ",
			nil,
		},
		{
			"single control",
			"CIS.1.1",
			map[string]bool{"CIS.1.1": true},
		},
		{
			"multiple controls",
			"CIS.1.1,CIS.2.3,CIS.3.1",
			map[string]bool{"CIS.1.1": true, "CIS.2.3": true, "CIS.3.1": true},
		},
		{
			"controls with whitespace",
			" CIS.1.1 , CIS.2.3 ",
			map[string]bool{"CIS.1.1": true, "CIS.2.3": true},
		},
		{
			"trailing comma produces no empty entry",
			"CIS.1.1,CIS.2.3,",
			map[string]bool{"CIS.1.1": true, "CIS.2.3": true},
		},
		{
			"leading comma produces no empty entry",
			",CIS.1.1",
			map[string]bool{"CIS.1.1": true},
		},
		{
			"duplicate controls collapsed",
			"CIS.1.1,CIS.1.1",
			map[string]bool{"CIS.1.1": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseControlFilter(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestFilterComplianceReport(t *testing.T) {
	baseReport := &security.ComplianceReport{
		GeneratedAt:     time.Now().UTC(),
		Stack:           "prod",
		Framework:       "cis-aws",
		FrameworkTitle:  "CIS AWS Foundations Benchmark",
		TotalControls:   10,
		PassingControls: 7,
		FailingControls: 3,
		ScorePercent:    70.0,
		FailingDetails: []security.ComplianceControl{
			{ControlID: "CIS.1.1", Title: "Root account MFA", Severity: security.SeverityCritical},
			{ControlID: "CIS.2.3", Title: "CloudTrail encryption", Severity: security.SeverityHigh},
			{ControlID: "CIS.3.1", Title: "S3 bucket logging", Severity: security.SeverityMedium},
		},
	}

	t.Run("filter matches subset of controls", func(t *testing.T) {
		filter := map[string]bool{"CIS.1.1": true, "CIS.3.1": true}
		result := filterComplianceReport(baseReport, filter)

		require.Len(t, result.FailingDetails, 2)
		assert.Equal(t, "CIS.1.1", result.FailingDetails[0].ControlID)
		assert.Equal(t, "CIS.3.1", result.FailingDetails[1].ControlID)
		assert.Equal(t, 2, result.FailingControls)
		// TotalControls = filtered failing + original passing.
		assert.Equal(t, 9, result.TotalControls)
	})

	t.Run("filter matches no controls", func(t *testing.T) {
		filter := map[string]bool{"CIS.99.99": true}
		result := filterComplianceReport(baseReport, filter)

		assert.Empty(t, result.FailingDetails)
		assert.Equal(t, 0, result.FailingControls)
		assert.Equal(t, 7, result.TotalControls) // only passing remain.
		assert.Equal(t, 100.0, result.ScorePercent)
	})

	t.Run("filter matches all failing controls", func(t *testing.T) {
		filter := map[string]bool{"CIS.1.1": true, "CIS.2.3": true, "CIS.3.1": true}
		result := filterComplianceReport(baseReport, filter)

		require.Len(t, result.FailingDetails, 3)
		assert.Equal(t, 3, result.FailingControls)
		assert.Equal(t, 10, result.TotalControls)
		assert.InDelta(t, 70.0, result.ScorePercent, 0.1)
	})

	t.Run("filter single control", func(t *testing.T) {
		filter := map[string]bool{"CIS.2.3": true}
		result := filterComplianceReport(baseReport, filter)

		require.Len(t, result.FailingDetails, 1)
		assert.Equal(t, "CIS.2.3", result.FailingDetails[0].ControlID)
		assert.Equal(t, 1, result.FailingControls)
		assert.Equal(t, 8, result.TotalControls) // 7 passing + 1 failing.
		assert.InDelta(t, 87.5, result.ScorePercent, 0.1)
	})

	t.Run("original report is not mutated", func(t *testing.T) {
		filter := map[string]bool{"CIS.1.1": true}
		_ = filterComplianceReport(baseReport, filter)

		// Original report should be unchanged.
		assert.Equal(t, 3, baseReport.FailingControls)
		assert.Equal(t, 10, baseReport.TotalControls)
		assert.Equal(t, 70.0, baseReport.ScorePercent)
		require.Len(t, baseReport.FailingDetails, 3)
	})

	t.Run("preserves metadata fields", func(t *testing.T) {
		filter := map[string]bool{"CIS.1.1": true}
		result := filterComplianceReport(baseReport, filter)

		assert.Equal(t, "prod", result.Stack)
		assert.Equal(t, "cis-aws", result.Framework)
		assert.Equal(t, "CIS AWS Foundations Benchmark", result.FrameworkTitle)
		assert.Equal(t, baseReport.GeneratedAt, result.GeneratedAt)
	})

	t.Run("zero passing controls with empty filter", func(t *testing.T) {
		noPassingReport := &security.ComplianceReport{
			TotalControls:   2,
			PassingControls: 0,
			FailingControls: 2,
			ScorePercent:    0.0,
			FailingDetails: []security.ComplianceControl{
				{ControlID: "CIS.1.1", Title: "Root account MFA", Severity: security.SeverityCritical},
				{ControlID: "CIS.2.3", Title: "CloudTrail encryption", Severity: security.SeverityHigh},
			},
		}
		filter := map[string]bool{"NONEXISTENT": true}
		result := filterComplianceReport(noPassingReport, filter)

		assert.Equal(t, 0, result.FailingControls)
		assert.Equal(t, 0, result.TotalControls)
		assert.Equal(t, 0.0, result.ScorePercent)
	})
}

func TestAuthenticateAndResolveAWS_EmptyIdentity(t *testing.T) {
	// Empty identity should return nil without error.
	authCtx, err := authenticateAndResolveAWS(nil, "")
	require.NoError(t, err)
	assert.Nil(t, authCtx)
}
