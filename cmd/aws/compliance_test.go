package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestComplianceReportFileFlag(t *testing.T) {
	// Verify the --file flag is registered on the report subcommand.
	flag := complianceReportCmd.Flags().Lookup("file")
	require.NotNil(t, flag, "complianceReportCmd should have --file flag")
	assert.Equal(t, "", flag.DefValue, "--file default should be empty")
	assert.Equal(t, "string", flag.Value.Type(), "--file should be a string flag")
}

func TestComplianceSubcommandRegistered(t *testing.T) {
	cmd := awsCmd
	var foundCompliance bool
	for _, sub := range cmd.Commands() {
		if sub.Use != "compliance" {
			continue
		}

		foundCompliance = true
		// Verify the report subcommand exists under compliance.
		var foundReport bool
		for _, subSub := range sub.Commands() {
			if subSub.Use == "report" {
				foundReport = true
				break
			}
		}
		assert.True(t, foundReport, "compliance command should have report subcommand")
		break
	}
	assert.True(t, foundCompliance, "aws command should have compliance subcommand")
}
