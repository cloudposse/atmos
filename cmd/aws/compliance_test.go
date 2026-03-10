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

func TestComplianceSubcommandRegistered(t *testing.T) {
	cmd := awsCmd
	var found bool
	for _, sub := range cmd.Commands() {
		if sub.Use == "compliance" {
			found = true
			break
		}
	}
	assert.True(t, found, "aws command should have compliance subcommand")
}
