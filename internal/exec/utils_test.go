package exec

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func Test_processArgsAndFlags(t *testing.T) {
	inputArgsAndFlags := []string{
		"--deploy-run-init=true",
		"--init-pass-vars=true",
		"--skip-planfile=true",
		"--logs-level",
		"Debug",
	}

	info, err := processArgsAndFlags(
		"terraform",
		inputArgsAndFlags,
	)

	assert.NoError(t, err)
	assert.Equal(t, info.DeployRunInit, "true")
	assert.Equal(t, info.InitPassVars, "true")
	assert.Equal(t, info.PlanSkipPlanfile, "true")
	assert.Equal(t, info.LogsLevel, "Debug")
}

func Test_processArgsAndFlags2(t *testing.T) {
	tests := []struct {
		name              string
		componentType     string
		inputArgsAndFlags []string
		want              schema.ArgsAndFlagsInfo
		wantErr           bool
	}{
		{
			name:              "clean command",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"clean"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand: "clean",
			},
			wantErr: false,
		},
		{
			name:              "version command",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"version"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand: "version",
			},
			wantErr: false,
		},
		{
			name:              "help for single command",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand: "plan",
				NeedHelp:   true,
			},
			wantErr: false,
		},
		{
			name:              "terraform command flag",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"--terraform-command", "plan"},
			want: schema.ArgsAndFlagsInfo{
				TerraformCommand: "plan",
			},
			wantErr: false,
		},
		{
			name:              "terraform dir flag",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"--terraform-dir", "/path/to/terraform"},
			want: schema.ArgsAndFlagsInfo{
				TerraformDir: "/path/to/terraform",
			},
			wantErr: false,
		},
		{
			name:          "multiple flags",
			componentType: "terraform",
			inputArgsAndFlags: []string{
				"--terraform-command", "plan",
				"--terraform-dir", "/path/to/terraform",
				"--append-user-agent", "test-agent",
				"--skip-planfile", "true",
				"--init-pass-vars", "true",
			},
			want: schema.ArgsAndFlagsInfo{
				TerraformCommand: "plan",
				TerraformDir:     "/path/to/terraform",
				AppendUserAgent:  "test-agent",
				PlanSkipPlanfile: "true",
				InitPassVars:     "true",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags(tt.componentType, tt.inputArgsAndFlags)
			if (err != nil) != tt.wantErr {
				t.Errorf("processArgsAndFlags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processArgsAndFlags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProcessArgsAndFlagsInvalidFlag(t *testing.T) {
	inputArgsAndFlags := []string{
		"init",
		"--init-pass-vars=invalid=true",
	}

	_, err := processArgsAndFlags(
		"terraform",
		inputArgsAndFlags,
	)

	assert.Error(t, err)
	assert.ErrorContains(t, err, "invalid flag: --init-pass-vars=invalid=true")
}

func TestPostProcessTemplatesAndYamlFunctions(t *testing.T) {
	tests := []struct {
		name     string
		input    schema.ConfigAndStacksInfo
		expected schema.ConfigAndStacksInfo
	}{
		{
			name: "all-fields-present",
			input: schema.ConfigAndStacksInfo{
				ComponentSection: map[string]any{
					cfg.ProvidersSectionName:   map[string]any{"aws": map[string]any{"region": "us-west-2"}},
					cfg.VarsSectionName:        map[string]any{"environment": "dev"},
					cfg.SettingsSectionName:    map[string]any{"enabled": true},
					cfg.EnvSectionName:         map[string]any{"DB_PASSWORD": "secret"},
					cfg.OverridesSectionName:   map[string]any{"cpu": "1024"},
					cfg.MetadataSectionName:    map[string]any{"description": "test component"},
					cfg.BackendSectionName:     map[string]any{"bucket": "my-bucket"},
					cfg.BackendTypeSectionName: "s3",
					cfg.ComponentSectionName:   "vpc",
					cfg.CommandSectionName:     "apply",
					cfg.WorkspaceSectionName:   "dev",
				},
			},
			expected: schema.ConfigAndStacksInfo{
				ComponentProvidersSection: map[string]any{"aws": map[string]any{"region": "us-west-2"}},
				ComponentVarsSection:      map[string]any{"environment": "dev"},
				ComponentSettingsSection:  map[string]any{"enabled": true},
				ComponentEnvSection:       map[string]any{"DB_PASSWORD": "secret"},
				ComponentOverridesSection: map[string]any{"cpu": "1024"},
				ComponentMetadataSection:  map[string]any{"description": "test component"},
				ComponentBackendSection:   map[string]any{"bucket": "my-bucket"},
				ComponentBackendType:      "s3",
				Component:                 "vpc",
				Command:                   "apply",
				TerraformWorkspace:        "dev",
			},
		},
		{
			name: "partial-fields",
			input: schema.ConfigAndStacksInfo{
				ComponentSection: map[string]any{
					cfg.VarsSectionName:      map[string]any{"environment": "prod"},
					cfg.SettingsSectionName:  map[string]any{"enabled": false},
					cfg.ComponentSectionName: "eks",
				},
			},
			expected: schema.ConfigAndStacksInfo{
				ComponentVarsSection:     map[string]any{"environment": "prod"},
				ComponentSettingsSection: map[string]any{"enabled": false},
				Component:                "eks",
			},
		},
		{
			name: "empty-component-section",
			input: schema.ConfigAndStacksInfo{
				ComponentSection: map[string]any{},
			},
			expected: schema.ConfigAndStacksInfo{
				ComponentSection: map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of the input to avoid modifying the test case
			input := tt.input

			// Call the function being tested
			postProcessTemplatesAndYamlFunctions(&input)

			// Compare each expected field individually for better error messages
			assert.Equal(t, tt.expected.ComponentProvidersSection, input.ComponentProvidersSection, "ComponentProvidersSection mismatch")
			assert.Equal(t, tt.expected.ComponentVarsSection, input.ComponentVarsSection, "ComponentVarsSection mismatch")
			assert.Equal(t, tt.expected.ComponentSettingsSection, input.ComponentSettingsSection, "ComponentSettingsSection mismatch")
			assert.Equal(t, tt.expected.ComponentEnvSection, input.ComponentEnvSection, "ComponentEnvSection mismatch")
			assert.Equal(t, tt.expected.ComponentOverridesSection, input.ComponentOverridesSection, "ComponentOverridesSection mismatch")
			assert.Equal(t, tt.expected.ComponentMetadataSection, input.ComponentMetadataSection, "ComponentMetadataSection mismatch")
			assert.Equal(t, tt.expected.ComponentBackendSection, input.ComponentBackendSection, "ComponentBackendSection mismatch")
			assert.Equal(t, tt.expected.ComponentBackendType, input.ComponentBackendType, "ComponentBackendType mismatch")
			assert.Equal(t, tt.expected.Component, input.Component, "Component mismatch")
			assert.Equal(t, tt.expected.Command, input.Command, "Command mismatch")
			assert.Equal(t, tt.expected.TerraformWorkspace, input.TerraformWorkspace, "TerraformWorkspace mismatch")
		})
	}
}

func Test_getCliVars(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      map[string]any
		wantErr   bool
		errString string
	}{
		{
			name: "basic var flag",
			args: []string{"-var", "name=value"},
			want: map[string]any{
				"name": "value",
			},
		},
		{
			name: "multiple var flags",
			args: []string{
				"-var", "name=value",
				"-var", "region=us-west-2",
			},
			want: map[string]any{
				"name":   "value",
				"region": "us-west-2",
			},
		},
		{
			name: "var flag with equals sign in value",
			args: []string{"-var", "connection_string=host=localhost;port=5432"},
			want: map[string]any{
				"connection_string": "host=localhost;port=5432",
			},
		},
		{
			name: "var flag with spaces in value",
			args: []string{"-var", "description=This is a test"},
			want: map[string]any{
				"description": "This is a test",
			},
		},
		{
			name: "var-file without value",
			args: []string{"-var-file"},
			want: map[string]any{}, // Should ignore invalid var-file
		},
		{
			name: "ignore non-var flags",
			args: []string{
				"-var", "name=value",
				"--other-flag", "something",
				"-var", "region=us-west-2",
			},
			want: map[string]any{
				"name":   "value",
				"region": "us-west-2",
			},
		},
		{
			name: "empty args",
			args: []string{},
			want: map[string]any{},
		},
		{
			name: "only non-var flags",
			args: []string{"--flag1", "value1", "--flag2", "value2"},
			want: map[string]any{},
		},
		{
			name: "duplicate var names",
			args: []string{
				"-var", "name=value1",
				"-var", "name=value2",
			},
			want: map[string]any{
				"name": "value2", // Last value should win
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getCliVars(tt.args)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errString != "" {
					assert.Contains(t, err.Error(), tt.errString)
				}
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
