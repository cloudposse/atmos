package exec

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestDescribeStacksExec(t *testing.T) {
	testCases := []struct {
		name          string
		args          *DescribeStacksArgs
		setupMocks    func(ctrl *gomock.Controller) *describeStacksExec
		expectedError string
		expectedQuery string
	}{
		{
			name: "successful basic execution",
			args: &DescribeStacksArgs{},
			setupMocks: func(ctrl *gomock.Controller) *describeStacksExec {
				return &describeStacksExec{
					pageCreator:           pager.NewMockPageCreator(ctrl),
					isTTYSupportForStdout: func() bool { return false },
					printOrWriteToFile: func(_ *schema.AtmosConfiguration, _, _ string, _ any) error {
						return nil
					},
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
						return map[string]any{"hello": "test"}, nil
					},
				}
			},
		},
		{
			name: "with query parameter",
			args: &DescribeStacksArgs{
				Query: ".hello",
			},
			setupMocks: func(ctrl *gomock.Controller) *describeStacksExec {
				return &describeStacksExec{
					pageCreator:           pager.NewMockPageCreator(ctrl),
					isTTYSupportForStdout: func() bool { return false },
					printOrWriteToFile: func(_ *schema.AtmosConfiguration, _, _ string, data any) error {
						assert.Equal(t, "test", data)
						return nil
					},
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
						return map[string]any{"hello": "test"}, nil
					},
				}
			},
		},
		{
			name: "with filter by stack",
			args: &DescribeStacksArgs{
				FilterByStack: "test-stack",
			},
			setupMocks: func(ctrl *gomock.Controller) *describeStacksExec {
				return &describeStacksExec{
					pageCreator:           pager.NewMockPageCreator(ctrl),
					isTTYSupportForStdout: func() bool { return false },
					printOrWriteToFile: func(_ *schema.AtmosConfiguration, _, _ string, _ any) error {
						return nil
					},
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, filterByStack string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
						assert.Equal(t, "test-stack", filterByStack)
						return map[string]any{"filtered": true}, nil
					},
				}
			},
		},
		{
			name: "with file output",
			args: &DescribeStacksArgs{
				File: "output.json",
			},
			setupMocks: func(ctrl *gomock.Controller) *describeStacksExec {
				mockPageCreator := pager.NewMockPageCreator(ctrl)
				return &describeStacksExec{
					pageCreator:           mockPageCreator,
					isTTYSupportForStdout: func() bool { return true },
					printOrWriteToFile: func(_ *schema.AtmosConfiguration, format, file string, data any) error {
						assert.Equal(t, "output.json", file)
						return nil
					},
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
						return map[string]any{"output": "to file"}, nil
					},
				}
			},
		},
		{
			name: "with execute error",
			args: &DescribeStacksArgs{},
			setupMocks: func(ctrl *gomock.Controller) *describeStacksExec {
				return &describeStacksExec{
					pageCreator:           pager.NewMockPageCreator(ctrl),
					isTTYSupportForStdout: func() bool { return false },
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
						return nil, errors.New("execution error")
					},
				}
			},
			expectedError: "execution error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			exec := tc.setupMocks(ctrl)
			err := exec.Execute(&schema.AtmosConfiguration{}, tc.args)

			if tc.expectedError != "" {
				assert.ErrorContains(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecuteDescribeStacks_Packer(t *testing.T) {
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	// Define the working directory.
	workDir := "../../tests/fixtures/scenarios/packer"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	stacksMap, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",
		nil,
		nil,
		nil,
		false,
		true,
		true,
		false,
		nil,
		nil, // authManager
	)
	assert.Nil(t, err)

	val, err := u.EvaluateYqExpression(&atmosConfig, stacksMap, ".prod.components.packer.aws/bastion.vars.ami_tags.SourceAMI")
	assert.Nil(t, err)
	assert.Equal(t, "ami-0013ceeff668b979b", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, stacksMap, ".nonprod.components.packer.aws/bastion.metadata.component")
	assert.Nil(t, err)
	assert.Equal(t, "aws/bastion", val)
}

// TestExecuteDescribeStacks_LocalsComponentLevel tests locals extraction and merging in describe stacks.
func TestExecuteDescribeStacks_LocalsComponentLevel(t *testing.T) {
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	// Define the working directory.
	workDir := "../../tests/fixtures/scenarios/locals-component-level"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	stacksMap, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",
		nil,
		nil,
		nil,
		false,
		true,
		true,
		false,
		nil,
		nil, // authManager
	)
	assert.Nil(t, err)
	assert.NotNil(t, stacksMap, "stacksMap should not be nil")

	// Verify the stacks map contains the expected stack.
	assert.NotEmpty(t, stacksMap, "should have at least one stack")

	// Verify locals are present in the terraform components.
	// The fixture defines stack-level locals (namespace, environment, name_prefix)
	// and component-level locals (vpc_type, cidr_prefix, etc.).
	// After merging, component locals should be present in each component section.
	expectedComponents := map[string]bool{
		"standalone": false,
		"vpc/dev":    false,
		"vpc/custom": false,
	}
	for stackName, stackData := range stacksMap {
		stackMap, ok := stackData.(map[string]any)
		if !ok {
			continue
		}
		components, ok := stackMap["components"].(map[string]any)
		if !ok {
			continue
		}
		tfComponents, ok := components["terraform"].(map[string]any)
		if !ok {
			continue
		}
		for compName, compData := range tfComponents {
			compMap, ok := compData.(map[string]any)
			if !ok {
				continue
			}
			// Each terraform component with locals should have a "locals" section.
			if compName == "standalone" || compName == "vpc/dev" || compName == "vpc/custom" {
				expectedComponents[compName] = true
				locals, hasLocals := compMap["locals"].(map[string]any)
				assert.True(t, hasLocals, "component %s in stack %s should have locals", compName, stackName)
				assert.NotEmpty(t, locals, "component %s in stack %s should have non-empty locals", compName, stackName)
			}
		}
	}
	for compName, found := range expectedComponents {
		assert.True(t, found, "expected component %s to be present in stacks output", compName)
	}
}

// TestGetComponentBasePath tests the getComponentBasePath function.
func TestGetComponentBasePath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
			Packer: schema.Packer{
				BasePath: "components/packer",
			},
		},
	}

	tests := []struct {
		name          string
		componentKind string
		expected      string
	}{
		{
			name:          "terraform component",
			componentKind: config.TerraformSectionName,
			expected:      "components/terraform",
		},
		{
			name:          "helmfile component",
			componentKind: config.HelmfileSectionName,
			expected:      "components/helmfile",
		},
		{
			name:          "packer component",
			componentKind: config.PackerSectionName,
			expected:      "components/packer",
		},
		{
			name:          "unknown component kind",
			componentKind: "unknown",
			expected:      "",
		},
		{
			name:          "empty component kind",
			componentKind: "",
			expected:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getComponentBasePath(atmosConfig, tt.componentKind)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildComponentInfo tests the buildComponentInfo function.
func TestBuildComponentInfo(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
			Packer: schema.Packer{
				BasePath: "components/packer",
			},
		},
	}

	tests := []struct {
		name             string
		componentSection map[string]any
		componentKind    string
		expectedType     string
		expectedPath     string
		hasPath          bool
	}{
		{
			name: "terraform component with base component",
			componentSection: map[string]any{
				config.ComponentSectionName: "vpc",
			},
			componentKind: config.TerraformSectionName,
			expectedType:  config.TerraformSectionName,
			expectedPath:  "components/terraform/vpc",
			hasPath:       true,
		},
		{
			name: "helmfile component",
			componentSection: map[string]any{
				config.ComponentSectionName: "nginx-ingress",
			},
			componentKind: config.HelmfileSectionName,
			expectedType:  config.HelmfileSectionName,
			expectedPath:  "components/helmfile/nginx-ingress",
			hasPath:       true,
		},
		{
			name: "packer component",
			componentSection: map[string]any{
				config.ComponentSectionName: "aws/bastion",
			},
			componentKind: config.PackerSectionName,
			expectedType:  config.PackerSectionName,
			expectedPath:  "components/packer/aws/bastion",
			hasPath:       true,
		},
		{
			name: "component with folder prefix",
			componentSection: map[string]any{
				config.ComponentSectionName: "vpc",
				config.MetadataSectionName: map[string]any{
					"component_folder_prefix": "myprefix",
				},
			},
			componentKind: config.TerraformSectionName,
			expectedType:  config.TerraformSectionName,
			expectedPath:  "components/terraform/myprefix/vpc",
			hasPath:       true,
		},
		{
			name: "missing component name",
			componentSection: map[string]any{
				config.ComponentSectionName: "",
			},
			componentKind: config.TerraformSectionName,
			expectedType:  config.TerraformSectionName,
			hasPath:       false,
		},
		{
			name:             "no component section key",
			componentSection: map[string]any{},
			componentKind:    config.TerraformSectionName,
			expectedType:     config.TerraformSectionName,
			hasPath:          false,
		},
		{
			name: "unknown component kind - no base path",
			componentSection: map[string]any{
				config.ComponentSectionName: "test",
			},
			componentKind: "unknown",
			expectedType:  "unknown",
			hasPath:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildComponentInfo(atmosConfig, tt.componentSection, tt.componentKind)
			assert.Equal(t, tt.expectedType, result["component_type"])

			if tt.hasPath {
				path, ok := result[config.ComponentPathSectionName].(string)
				assert.True(t, ok, "component_path should be present")
				assert.Equal(t, tt.expectedPath, path)
			} else {
				_, ok := result[config.ComponentPathSectionName]
				assert.False(t, ok, "component_path should not be present")
			}
		})
	}
}

// TestGetStackManifestName tests the getStackManifestName function.
func TestGetStackManifestName(t *testing.T) {
	tests := []struct {
		name         string
		stackSection any
		expected     string
	}{
		{
			name: "stack with name field",
			stackSection: map[string]any{
				"name": "custom-stack-name",
			},
			expected: "custom-stack-name",
		},
		{
			name: "stack without name field",
			stackSection: map[string]any{
				"vars": map[string]any{"stage": "dev"},
			},
			expected: "",
		},
		{
			name: "stack with empty name",
			stackSection: map[string]any{
				"name": "",
			},
			expected: "",
		},
		{
			name: "stack with non-string name",
			stackSection: map[string]any{
				"name": 42,
			},
			expected: "",
		},
		{
			name:         "nil stack section",
			stackSection: nil,
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStackManifestName(tt.stackSection)
			assert.Equal(t, tt.expected, result)
		})
	}
}
