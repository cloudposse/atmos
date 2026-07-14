package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
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
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions) (map[string]any, error) {
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
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions) (map[string]any, error) {
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
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, filterByStack string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions) (map[string]any, error) {
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
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions) (map[string]any, error) {
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
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions) (map[string]any, error) {
						return nil, errors.New("execution error")
					},
				}
			},
			expectedError: "execution error",
		},
		{
			name: "with invalid yq query returns error",
			args: &DescribeStacksArgs{
				Query: ".[[bad-syntax",
			},
			setupMocks: func(ctrl *gomock.Controller) *describeStacksExec {
				return &describeStacksExec{
					pageCreator:           pager.NewMockPageCreator(ctrl),
					isTTYSupportForStdout: func() bool { return false },
					executeDescribeStacks: func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager, _ bool, _ DescribeStacksErrorOptions) (map[string]any, error) {
						return map[string]any{"hello": "test"}, nil
					},
				}
			},
			expectedError: "EvaluateYqExpression",
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
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	// Define the working directory.
	workDir := "../../tests/fixtures/scenarios/packer"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
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

func TestExecuteDescribeStacks_Ansible(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	// Define the working directory.
	workDir := "../../examples/demo-ansible"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
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

	// Verify both dev and prod stacks are found.
	assert.Contains(t, stacksMap, "dev")
	assert.Contains(t, stacksMap, "prod")

	// Verify ansible component vars in dev stack.
	val, err := u.EvaluateYqExpression(&atmosConfig, stacksMap, ".dev.components.ansible.hello-world.vars.app_name")
	assert.Nil(t, err)
	assert.Equal(t, "my-app", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, stacksMap, ".dev.components.ansible.hello-world.vars.app_version")
	assert.Nil(t, err)
	assert.Equal(t, "1.0.0-dev", val)

	// Verify ansible component vars in prod stack.
	val, err = u.EvaluateYqExpression(&atmosConfig, stacksMap, ".prod.components.ansible.hello-world.vars.app_version")
	assert.Nil(t, err)
	assert.Equal(t, "2.0.0", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, stacksMap, ".prod.components.ansible.hello-world.vars.app_port")
	assert.Nil(t, err)
	assert.Equal(t, 443, val)

	// Verify component_info contains ansible type.
	val, err = u.EvaluateYqExpression(&atmosConfig, stacksMap, ".dev.components.ansible.hello-world.component_info.component_type")
	assert.Nil(t, err)
	assert.Equal(t, "ansible", val)

	// Verify settings.ansible section is preserved.
	val, err = u.EvaluateYqExpression(&atmosConfig, stacksMap, ".dev.components.ansible.hello-world.settings.ansible.playbook")
	assert.Nil(t, err)
	assert.Equal(t, "site.yml", val)
}

// ---------------------------------------------------------------------------
// ErrorOptionsFromMode / PrintErrorModeSummary
// ---------------------------------------------------------------------------

func TestResolveErrorMode(t *testing.T) {
	tests := []struct {
		name         string
		flagValue    string
		settingValue string
		expected     string
	}{
		{name: "flag wins over setting", flagValue: "strict", settingValue: "silent", expected: "strict"},
		{name: "setting used when flag unset", flagValue: "", settingValue: "silent", expected: "silent"},
		{name: "defaults to warn when both unset", flagValue: "", settingValue: "", expected: "warn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ResolveErrorMode(tt.flagValue, tt.settingValue))
		})
	}
}

// TestResolveErrorMode_ListAndDescribeAreIndependent guards against list.error_mode and
// describe.error_mode being read from a shared config field: setting one must have zero
// effect on the other's resolution.
func TestResolveErrorMode_ListAndDescribeAreIndependent(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.List.ErrorMode = "strict"

	assert.Equal(t, "strict", ResolveErrorMode("", atmosConfig.List.ErrorMode))
	assert.Equal(t, "warn", ResolveErrorMode("", atmosConfig.Describe.ErrorMode), "describe.error_mode must not inherit list.error_mode")

	atmosConfig = schema.AtmosConfiguration{}
	atmosConfig.Describe.ErrorMode = "silent"

	assert.Equal(t, "silent", ResolveErrorMode("", atmosConfig.Describe.ErrorMode))
	assert.Equal(t, "warn", ResolveErrorMode("", atmosConfig.List.ErrorMode), "list.error_mode must not inherit describe.error_mode")
}

func TestErrorOptionsFromMode(t *testing.T) {
	tests := []struct {
		name          string
		errorMode     string
		expectWarn    bool
		expectNilColl bool
	}{
		{name: "strict returns strict with nil collector", errorMode: "strict", expectWarn: false, expectNilColl: true},
		{name: "empty value is strict", errorMode: "", expectWarn: false, expectNilColl: true},
		{name: "warn enables OnErrorWarn with a non-nil collector", errorMode: "warn", expectWarn: true, expectNilColl: false},
		{name: "silent also enables OnErrorWarn with a non-nil collector", errorMode: "silent", expectWarn: true, expectNilColl: false},
		{name: "unrecognized value is strict", errorMode: "bogus", expectWarn: false, expectNilColl: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, collector := ErrorOptionsFromMode(tt.errorMode)

			if tt.expectWarn {
				assert.Equal(t, OnErrorWarn, opts.OnError)
				require.NotNil(t, opts.OnWarning)
			} else {
				assert.Equal(t, OnErrorMode(""), opts.OnError)
				assert.Nil(t, opts.OnWarning)
			}

			if tt.expectNilColl {
				assert.Nil(t, collector)
			} else {
				require.NotNil(t, collector)
				assert.Equal(t, 0, collector.Count())
				opts.OnWarning(DegradationWarning{Stack: "s", Component: "c", Function: "f", Reason: "r"})
				assert.Equal(t, 1, collector.Count())
			}
		})
	}
}

func TestPrintErrorModeSummary(t *testing.T) {
	t.Run("warn mode with nil collector does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			PrintErrorModeSummary("warn", nil)
		})
	})

	t.Run("strict mode is a no-op even with a populated collector", func(t *testing.T) {
		_, collector := ErrorOptionsFromMode("warn")
		require.NotNil(t, collector)
		collector.Add(DegradationWarning{Stack: "s", Component: "c", Function: "f", Reason: "r"})

		assert.NotPanics(t, func() {
			PrintErrorModeSummary("strict", collector)
		})
	})

	t.Run("silent mode is a no-op even with a populated collector", func(t *testing.T) {
		_, collector := ErrorOptionsFromMode("silent")
		require.NotNil(t, collector)
		collector.Add(DegradationWarning{Stack: "s", Component: "c", Function: "f", Reason: "r"})

		assert.NotPanics(t, func() {
			PrintErrorModeSummary("silent", collector)
		})
	})

	t.Run("warn mode with a populated collector prints the summary", func(t *testing.T) {
		_, collector := ErrorOptionsFromMode("warn")
		require.NotNil(t, collector)
		collector.Add(DegradationWarning{Stack: "s", Component: "c", Function: "f", Reason: "r"})

		assert.NotPanics(t, func() {
			PrintErrorModeSummary("warn", collector)
		})
	})
}

// ---------------------------------------------------------------------------
// NewDescribeStacksExec
// ---------------------------------------------------------------------------

func TestNewDescribeStacksExec(t *testing.T) {
	exec := NewDescribeStacksExec()
	assert.NotNil(t, exec)
}

// ---------------------------------------------------------------------------
// getComponentBasePath
// ---------------------------------------------------------------------------

func TestGetComponentBasePath_AllCases(t *testing.T) {
	ac := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
			Helmfile:  schema.Helmfile{BasePath: "components/helmfile"},
			Packer:    schema.Packer{BasePath: "components/packer"},
			Ansible:   schema.Ansible{BasePath: "components/ansible"},
		},
	}

	tests := []struct {
		kind string
		want string
	}{
		{cfg.TerraformSectionName, "components/terraform"},
		{cfg.HelmfileSectionName, "components/helmfile"},
		{cfg.PackerSectionName, "components/packer"},
		{cfg.AnsibleSectionName, "components/ansible"},
		{"unknown", ""},
	}

	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			assert.Equal(t, tc.want, getComponentBasePath(ac, tc.kind))
		})
	}
}

// ---------------------------------------------------------------------------
// buildComponentInfo
// ---------------------------------------------------------------------------

func TestBuildComponentInfo_EmptyFinalComponent(t *testing.T) {
	// When the componentSection has no "component" key, finalComponent is "" and
	// we return early without a component_path.
	ac := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
		},
	}
	cs := map[string]any{} // no "component" key

	result := buildComponentInfo(ac, cs, cfg.TerraformSectionName)

	assert.Equal(t, cfg.TerraformSectionName, result["component_type"])
	assert.NotContains(t, result, cfg.ComponentPathSectionName)
}

func TestBuildComponentInfo_EmptyBasePath(t *testing.T) {
	// When the AtmosConfiguration has no basePath for the kind, we return early.
	ac := &schema.AtmosConfiguration{} // no terraform basePath
	cs := map[string]any{
		cfg.ComponentSectionName: "vpc",
	}

	result := buildComponentInfo(ac, cs, cfg.TerraformSectionName)

	assert.Equal(t, cfg.TerraformSectionName, result["component_type"])
	assert.NotContains(t, result, cfg.ComponentPathSectionName)
}

func TestBuildComponentInfo_WithFolderPrefix(t *testing.T) {
	ac := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
		},
	}
	cs := map[string]any{
		cfg.ComponentSectionName: "vpc",
		cfg.MetadataSectionName: map[string]any{
			"component_folder_prefix": "networking",
		},
	}

	result := buildComponentInfo(ac, cs, cfg.TerraformSectionName)

	assert.Equal(t, cfg.TerraformSectionName, result["component_type"])
	// buildComponentInfo normalizes paths to forward slashes via filepath.ToSlash.
	assert.Equal(t, "components/terraform/networking/vpc", result[cfg.ComponentPathSectionName])
}

func TestBuildComponentInfo_NoPrefixInMetadata(t *testing.T) {
	// Metadata exists but no component_folder_prefix.
	ac := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
		},
	}
	cs := map[string]any{
		cfg.ComponentSectionName: "vpc",
		cfg.MetadataSectionName:  map[string]any{"component": "base-vpc"},
	}

	result := buildComponentInfo(ac, cs, cfg.TerraformSectionName)

	// buildComponentInfo normalizes paths to forward slashes via filepath.ToSlash.
	assert.Equal(t, "components/terraform/vpc", result[cfg.ComponentPathSectionName])
}

// ---------------------------------------------------------------------------
// propagateAuth
// ---------------------------------------------------------------------------

func TestPropagateAuth_WithNonNilAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "my-profile",
			Region:  "us-east-1",
		},
	}

	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
	}

	mockAuthManager := types.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().GetStackInfo().Return(authStackInfo).Times(1)

	info := &schema.ConfigAndStacksInfo{}
	propagateAuth(info, mockAuthManager)

	assert.Equal(t, mockAuthManager, info.AuthManager)
	assert.Equal(t, expectedAuthContext, info.AuthContext)
}

func TestPropagateAuth_WithNilAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: nil,
	}

	mockAuthManager := types.NewMockAuthManager(ctrl)
	mockAuthManager.EXPECT().GetStackInfo().Return(authStackInfo).Times(1)

	info := &schema.ConfigAndStacksInfo{}
	propagateAuth(info, mockAuthManager)

	assert.Equal(t, mockAuthManager, info.AuthManager)
	assert.Nil(t, info.AuthContext)
}

// ---------------------------------------------------------------------------
// ExecuteDescribeStacks – error paths via integration fixture
// ---------------------------------------------------------------------------

func TestExecuteDescribeStacks_IncludeEmptyStacks(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/authmanager-propagation"
	t.Chdir(workDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Call with includeEmptyStacks=false to get the baseline.
	withoutEmpty, err := ExecuteDescribeStacks(
		&atmosConfig,
		"", nil, nil, nil, false, false, false,
		false, // includeEmptyStacks=false.
		nil, nil,
	)
	require.NoError(t, err)

	// Call with includeEmptyStacks=true — should return at least as many stacks.
	withEmpty, err := ExecuteDescribeStacks(
		&atmosConfig,
		"", nil, nil, nil, false, false, false,
		true, // includeEmptyStacks=true.
		nil, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, withEmpty)

	// The includeEmptyStacks=true result must have >= the stacks from the false result.
	// If the fixture has any import-only or component-less stacks, the count will be strictly greater.
	assert.GreaterOrEqual(t, len(withEmpty), len(withoutEmpty),
		"includeEmptyStacks=true should return at least as many stacks as false")
}

// TestExecuteDescribeStacks_FindStacksMapError exercises the FindStacksMap error branch
// in ExecuteDescribeStacks (lines 134-136) by using an atmos.yaml that points to a
// stacks directory containing a syntactically invalid YAML file.
func TestExecuteDescribeStacks_FindStacksMapError(t *testing.T) {
	tmpDir := t.TempDir()

	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "components", "terraform"), 0o755))

	// Create a YAML file with invalid syntax to force ProcessYAMLConfigFiles to error.
	badYAML := ": - badly_formatted_yaml\n  unclosed_block:\n bad_indent"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "bad.yaml"), []byte(badYAML), 0o644))

	atmosYAML := "base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// ignoreMissingFiles=false forces FindStacksMap to propagate the parse error.
	_, err = ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	require.Error(t, err)
}

// TestExecuteDescribeStacks_SkipEmptyStacks exercises the skip-empty-stacks branch
// in ExecuteDescribeStacks (lines 156-157): stacks with no components and no imports
// are skipped when includeEmptyStacks=false (the default).
func TestExecuteDescribeStacks_SkipEmptyStacks(t *testing.T) {
	tmpDir := t.TempDir()

	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "components", "terraform"), 0o755))

	// Stack file with only vars — no components and no imports.
	emptyStack := "vars:\n  region: us-east-1\n  environment: dev\n"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "empty.yaml"), []byte(emptyStack), 0o644))

	atmosYAML := "base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// includeEmptyStacks=false: the stack with no components/imports is skipped.
	result, err := ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, result, "stack with no components/imports should be skipped")
}

// TestExecuteDescribeStacks_ProcessStackFileError exercises the processStackFile error branch
// in ExecuteDescribeStacks (lines 166-168) by configuring an invalid stacks.name_template
// that causes resolveStackName to fail when a component is processed.
func TestExecuteDescribeStacks_ProcessStackFileError(t *testing.T) {
	tmpDir := t.TempDir()

	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))
	vpcDir := filepath.Join(tmpDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	// Minimal main.tf so the component directory exists.
	require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "main.tf"), []byte(""), 0o644))

	// Stack file with a component so processComponentEntry (and resolveStackName) is reached.
	stackContent := "components:\n  terraform:\n    vpc:\n      vars:\n        region: us-east-1\n"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "stack.yaml"), []byte(stackContent), 0o644))

	// atmos.yaml with an invalid Go template for name_template — causes resolveStackName to fail.
	atmosYAML := "base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\n  name_template: \"{{.unclosed_template\"\ncomponents:\n  terraform:\n    base_path: components/terraform\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	_, err = ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	require.Error(t, err)
}

// buildDescribeStacksDegradationFixture creates a temp atmos project with one terraform
// component (vpc) in stack file "dev.yaml" whose "bucket" var uses a `!terraform.state`
// YAML function, changes the process working directory to the fixture root, and returns the
// initialized AtmosConfiguration. Used by
// TestExecuteDescribeStacks_OnErrorWarn_DegradesRecoverableError and its _Strict companion
// to exercise the OnErrorWarn dispatch in executeDescribeStacks (describe_stacks.go).
func buildDescribeStacksDegradationFixture(t *testing.T) schema.AtmosConfiguration {
	t.Helper()

	tmpDir := t.TempDir()

	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))
	vpcDir := filepath.Join(tmpDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	// Minimal main.tf so the component directory exists.
	require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "main.tf"), []byte(""), 0o644))

	stackContent := "components:\n  terraform:\n    vpc:\n      vars:\n        bucket: \"!terraform.state vpc dev bucket_name\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(stacksDir, "dev.yaml"), []byte(stackContent), 0o644))

	atmosYAML := "base_path: \".\"\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\ncomponents:\n  terraform:\n    base_path: components/terraform\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return atmosConfig
}

// TestExecuteDescribeStacks_OnErrorWarn_DegradesRecoverableError exercises the
// `errOptions.OnError == OnErrorWarn` dispatch in executeDescribeStacks (describe_stacks.go)
// end-to-end through the exported ExecuteDescribeStacksWithOptions: a recoverable
// `!terraform.state` error is degraded to degradation.AtmosComputedValue{} instead of
// failing the whole describe-stacks call, and reported exactly once via OnWarning.
func TestExecuteDescribeStacks_OnErrorWarn_DegradesRecoverableError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := buildDescribeStacksDegradationFixture(t)

	recoverableErr := fmt.Errorf("%w for component `vpc` in stack `dev`", errUtils.ErrTerraformStateNotProvisioned)
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "dev", "vpc", "bucket_name", false, gomock.Any(), gomock.Any()).
		Return(nil, recoverableErr).
		Times(1)

	var warnings []DegradationWarning
	result, err := ExecuteDescribeStacksWithOptions(
		&atmosConfig, "", nil, nil, nil, false,
		false, // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil, nil, false,
		DescribeStacksErrorOptions{
			OnError: OnErrorWarn,
			OnWarning: func(w DegradationWarning) {
				warnings = append(warnings, w)
			},
		},
	)

	require.NoError(t, err)
	assert.Len(t, warnings, 1)
	assert.NotEmpty(t, result)
}

// TestExecuteDescribeStacks_OnErrorWarn_DegradesRecoverableError_Strict is the contrasting
// strict-mode companion to TestExecuteDescribeStacks_OnErrorWarn_DegradesRecoverableError:
// with a zero-value DescribeStacksErrorOptions{} (OnErrorStrict), the same recoverable error
// now fails the whole call instead of being degraded.
func TestExecuteDescribeStacks_OnErrorWarn_DegradesRecoverableError_Strict(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := buildDescribeStacksDegradationFixture(t)

	recoverableErr := fmt.Errorf("%w for component `vpc` in stack `dev`", errUtils.ErrTerraformStateNotProvisioned)
	mockStateGetter.EXPECT().
		GetState(gomock.Any(), gomock.Any(), "dev", "vpc", "bucket_name", false, gomock.Any(), gomock.Any()).
		Return(nil, recoverableErr).
		Times(1)

	_, err := ExecuteDescribeStacksWithOptions(
		&atmosConfig, "", nil, nil, nil, false,
		false, // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil, nil, false,
		DescribeStacksErrorOptions{},
	)

	require.Error(t, err)
}

// TestExecuteDescribeStacks_NonMapStackEntry exercises the type-guard at lines 150-153 in
// ExecuteDescribeStacks: "stackMap, ok := stackSection.(map[string]any); if !ok { continue }".
// Since FindStacksMap always returns map[string]any values in normal operation, we inject a
// non-map value directly into the FindStacksMap cache to trigger the defensive skip.
func TestExecuteDescribeStacks_NonMapStackEntry(t *testing.T) {
	ac := &schema.AtmosConfiguration{}

	// Compute the exact cache key that FindStacksMap will look up for this atmosConfig.
	cacheKey := getFindStacksMapCacheKey(ac, false)

	// Pre-populate the cache with a stacksMap containing a non-map value ("string" instead of map).
	// This simulates the defensive scenario the type-guard is protecting against.
	findStacksMapCacheMu.Lock()
	findStacksMapCache[cacheKey] = &findStacksMapCacheEntry{
		stacksMap: map[string]any{
			"non-map-stack": "this is a string, not a map[string]any",
		},
	}
	findStacksMapCacheMu.Unlock()

	t.Cleanup(func() {
		findStacksMapCacheMu.Lock()
		delete(findStacksMapCache, cacheKey)
		findStacksMapCacheMu.Unlock()
	})

	// ExecuteDescribeStacks will read the cached stacksMap, hit the type-guard at line 150,
	// skip the non-map entry, and return an empty result with no error.
	result, err := ExecuteDescribeStacks(ac, "", nil, nil, nil, false, false, false, false, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, result, "non-map stack entries must be skipped silently")
}

// TestEnsureComponentEntryInMap_InvalidStackEntryType verifies that ensureComponentEntryInMap
// handles a non-map stack entry gracefully (ok guard) instead of panicking.
func TestEnsureComponentEntryInMap_InvalidStackEntryType(t *testing.T) {
	finalStacksMap := map[string]any{
		"my-stack": "not-a-map", // invalid type.
	}
	// Must not panic — the ok guard should return early.
	assert.NotPanics(t, func() {
		ensureComponentEntryInMap(finalStacksMap, "my-stack", "terraform", "vpc")
	})
}

// TestEnsureComponentEntryInMap_InvalidComponentsType verifies that ensureComponentEntryInMap
// handles a non-map components section gracefully.
func TestEnsureComponentEntryInMap_InvalidComponentsType(t *testing.T) {
	finalStacksMap := map[string]any{
		"my-stack": map[string]any{
			"components": "not-a-map", // invalid type.
		},
	}
	assert.NotPanics(t, func() {
		ensureComponentEntryInMap(finalStacksMap, "my-stack", "terraform", "vpc")
	})
}

// ---------------------------------------------------------------------------
// getComponentDestMap
// ---------------------------------------------------------------------------

// TestGetComponentDestMap_ValidPath verifies the happy path traversal.
func TestGetComponentDestMap_ValidPath(t *testing.T) {
	compMap := map[string]any{"region": "us-east-1"}
	finalMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": compMap,
				},
			},
		},
	}
	dest, ok := getComponentDestMap(finalMap, "dev", "terraform", "vpc")
	require.True(t, ok)
	assert.Equal(t, "us-east-1", dest["region"])
}

// TestGetComponentDestMap_MissingStack returns false when stack is absent.
func TestGetComponentDestMap_MissingStack(t *testing.T) {
	finalMap := map[string]any{}
	_, ok := getComponentDestMap(finalMap, "dev", "terraform", "vpc")
	assert.False(t, ok)
}

// TestGetComponentDestMap_InvalidStackType returns false when stack entry is not a map.
func TestGetComponentDestMap_InvalidStackType(t *testing.T) {
	finalMap := map[string]any{"dev": "not-a-map"}
	_, ok := getComponentDestMap(finalMap, "dev", "terraform", "vpc")
	assert.False(t, ok)
}

// TestGetComponentDestMap_MissingComponentsSection returns false.
func TestGetComponentDestMap_MissingComponentsSection(t *testing.T) {
	finalMap := map[string]any{
		"dev": map[string]any{},
	}
	_, ok := getComponentDestMap(finalMap, "dev", "terraform", "vpc")
	assert.False(t, ok)
}

// TestGetComponentDestMap_MissingComponentName returns false.
func TestGetComponentDestMap_MissingComponentName(t *testing.T) {
	finalMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{},
			},
		},
	}
	_, ok := getComponentDestMap(finalMap, "dev", "terraform", "vpc")
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// processStackFile — name_template ghost entry prevention
// ---------------------------------------------------------------------------

// TestProcessStackFile_NoGhostEntry_NameTemplate verifies that when NameTemplate is set
// and no manifest name is defined, processStackFile does NOT pre-create an entry under
// the raw file name. This prevents ghost entries when includeEmptyStacks=true.
func TestProcessStackFile_NoGhostEntry_NameTemplate(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Stacks.NameTemplate = "{{ .vars.tenant }}-{{ .vars.stage }}"

	p := newDescribeStacksProcessor(
		atmosConfig,
		"", nil, nil, nil,
		false, false,
		true, // includeEmptyStacks.
		nil, nil,
	)

	stackMap := map[string]any{}
	err := p.processStackFile("stacks/prod.yaml", stackMap)
	require.NoError(t, err)

	// No entry should exist under "stacks/prod.yaml" because NameTemplate is set and
	// the real stack name can only be resolved per-component (which there are none of).
	_, exists := p.finalStacksMap["stacks/prod.yaml"]
	assert.False(t, exists, "ghost entry under stackFileName must not exist when NameTemplate is set")
}

// TestProcessStackFile_NoGhostEntry_NamePattern verifies that when NamePattern is set,
// processStackFile does NOT pre-create an entry under the raw file name.
func TestProcessStackFile_NoGhostEntry_NamePattern(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Stacks.NamePattern = "{tenant}-{environment}-{stage}"

	p := newDescribeStacksProcessor(
		atmosConfig,
		"", nil, nil, nil,
		false, false,
		true, // includeEmptyStacks.
		nil, nil,
	)

	stackMap := map[string]any{}
	err := p.processStackFile("stacks/prod.yaml", stackMap)
	require.NoError(t, err)

	_, exists := p.finalStacksMap["stacks/prod.yaml"]
	assert.False(t, exists, "ghost entry under stackFileName must not exist when NamePattern is set")
}

// TestProcessStackFile_NoGhostEntry_FilterByStack verifies that when filterByStack
// is active and the stack doesn't match, processStackFile returns early without
// creating an entry.
func TestProcessStackFile_NoGhostEntry_FilterByStack(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	p := newDescribeStacksProcessor(
		atmosConfig,
		"other-stack", nil, nil, nil,
		false, false,
		true, // includeEmptyStacks.
		nil, nil,
	)

	stackMap := map[string]any{}
	err := p.processStackFile("stacks/prod.yaml", stackMap)
	require.NoError(t, err)

	_, exists := p.finalStacksMap["stacks/prod.yaml"]
	assert.False(t, exists, "non-matching stack should not create an entry when filterByStack is active")
}
