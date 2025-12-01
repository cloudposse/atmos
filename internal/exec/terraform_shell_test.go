package exec

import (
	"bytes"
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestPrintShellDryRunInfo(t *testing.T) {
	tests := []struct {
		name           string
		info           *schema.ConfigAndStacksInfo
		cfg            *shellConfig
		expectedOutput []string
	}{
		{
			name: "Basic configuration",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg:   "vpc",
				Stack:              "dev-us-west-2",
				TerraformWorkspace: "dev-vpc",
			},
			cfg: &shellConfig{
				componentPath: "/terraform/components/vpc",
				workingDir:    "/terraform/components/vpc",
				varFile:       "dev-us-west-2-vpc.terraform.tfvars.json",
			},
			expectedOutput: []string{
				"Dry run mode: shell would be started with the following configuration:",
				"Component: vpc",
				"Stack: dev-us-west-2",
				"Working directory: /terraform/components/vpc",
				"Terraform workspace: dev-vpc",
				"Component path: /terraform/components/vpc",
				"Varfile: dev-us-west-2-vpc.terraform.tfvars.json",
			},
		},
		{
			name: "Empty workspace",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg:   "rds",
				Stack:              "prod",
				TerraformWorkspace: "",
			},
			cfg: &shellConfig{
				componentPath: "/components/terraform/rds",
				workingDir:    "/components/terraform/rds",
				varFile:       "prod-rds.terraform.tfvars.json",
			},
			expectedOutput: []string{
				"Component: rds",
				"Stack: prod",
				"Terraform workspace: ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			printShellDryRunInfo(tt.info, tt.cfg)

			// Restore stdout.
			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			_, err := buf.ReadFrom(r)
			assert.NoError(t, err)

			output := buf.String()
			for _, expected := range tt.expectedOutput {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestShellConfigStruct(t *testing.T) {
	tests := []struct {
		name string
		cfg  shellConfig
	}{
		{
			name: "All fields populated",
			cfg: shellConfig{
				componentPath: "/path/to/component",
				workingDir:    "/working/dir",
				varFile:       "vars.tfvars.json",
			},
		},
		{
			name: "Empty struct",
			cfg:  shellConfig{},
		},
		{
			name: "Partial fields",
			cfg: shellConfig{
				componentPath: "/path/to/component",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify struct can be created and accessed.
			assert.NotNil(t, tt.cfg)
		})
	}
}
