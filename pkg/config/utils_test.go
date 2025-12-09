package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func Test_setFeatureFlags(t *testing.T) {
	// Test cases
	tests := []struct {
		name            string
		configAndStacks schema.ConfigAndStacksInfo
		expectedConfig  schema.AtmosConfiguration
		expectError     bool
		errorMessage    string
	}{
		{
			name: "test all feature flags",
			configAndStacks: schema.ConfigAndStacksInfo{
				DeployRunInit:           "true",
				AutoGenerateBackendFile: "true",
				WorkflowsDir:            "/custom/workflows",
				InitRunReconfigure:      "true",
				InitPassVars:            "true",
				PlanSkipPlanfile:        "true",
			},
			expectedConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						DeployRunInit:           true,
						AutoGenerateBackendFile: true,
						InitRunReconfigure:      true,
						Init: schema.TerraformInit{
							PassVars: true,
						},
						Plan: schema.TerraformPlan{
							SkipPlanfile: true,
						},
					},
				},
				Workflows: schema.Workflows{
					BasePath: "/custom/workflows",
				},
			},
			expectError: false,
		},
		{
			name: "test partial feature flags",
			configAndStacks: schema.ConfigAndStacksInfo{
				DeployRunInit:      "false",
				InitRunReconfigure: "true",
			},
			expectedConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						DeployRunInit:      false,
						InitRunReconfigure: true,
						// Default values for other fields
						AutoGenerateBackendFile: false,
						Init: schema.TerraformInit{
							PassVars: false,
						},
						Plan: schema.TerraformPlan{
							SkipPlanfile: false,
						},
					},
				},
				Workflows: schema.Workflows{
					BasePath: "",
				},
			},
			expectError: false,
		},
		{
			name: "test invalid boolean",
			configAndStacks: schema.ConfigAndStacksInfo{
				DeployRunInit: "not-a-boolean",
			},
			expectError:  true,
			errorMessage: "strconv.ParseBool: parsing \"not-a-boolean\": invalid syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a default config
			config := &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						// Initialize with default values
						DeployRunInit:           false,
						AutoGenerateBackendFile: false,
						InitRunReconfigure:      false,
						Init: schema.TerraformInit{
							PassVars: false,
						},
						Plan: schema.TerraformPlan{
							SkipPlanfile: false,
						},
					},
				},
				Workflows: schema.Workflows{
					BasePath: "",
				},
			}

			err := setFeatureFlags(config, &tt.configAndStacks)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMessage)
			} else {
				assert.NoError(t, err)
				// Compare relevant fields
				assert.Equal(t, tt.expectedConfig.Components.Terraform.DeployRunInit, config.Components.Terraform.DeployRunInit)
				assert.Equal(t, tt.expectedConfig.Components.Terraform.AutoGenerateBackendFile, config.Components.Terraform.AutoGenerateBackendFile)
				assert.Equal(t, tt.expectedConfig.Components.Terraform.InitRunReconfigure, config.Components.Terraform.InitRunReconfigure)
				assert.Equal(t, tt.expectedConfig.Components.Terraform.Init.PassVars, config.Components.Terraform.Init.PassVars)
				assert.Equal(t, tt.expectedConfig.Components.Terraform.Plan.SkipPlanfile, config.Components.Terraform.Plan.SkipPlanfile)
				assert.Equal(t, tt.expectedConfig.Workflows.BasePath, config.Workflows.BasePath)
			}
		})
	}
}

func Test_processEnvVars(t *testing.T) {
	// Test cases
	tests := []struct {
		name           string
		envVars        map[string]string
		expectedConfig schema.AtmosConfiguration
		expectError    bool
		errorMessage   string
	}{
		{
			name: "test string env vars",
			envVars: map[string]string{
				"ATMOS_BASE_PATH":        "/test/base/path",
				"ATMOS_VENDOR_BASE_PATH": "/test/vendor/path",
				"ATMOS_STACKS_BASE_PATH": "/test/stacks/path",
			},
			expectedConfig: schema.AtmosConfiguration{
				BasePath: "/test/base/path",
				Vendor:   schema.Vendor{BasePath: "/test/vendor/path"},
				Stacks:   schema.Stacks{BasePath: "/test/stacks/path"},
			},
			expectError: false,
		},
		{
			name: "test list env vars",
			envVars: map[string]string{
				"ATMOS_STACKS_INCLUDED_PATHS": "path1,path2,path3",
				"ATMOS_STACKS_EXCLUDED_PATHS": "path4,path5",
			},
			expectedConfig: schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					IncludedPaths: []string{"path1", "path2", "path3"},
					ExcludedPaths: []string{"path4", "path5"},
				},
			},
			expectError: false,
		},
		{
			name: "test stack name and template env vars",
			envVars: map[string]string{
				"ATMOS_STACKS_NAME_PATTERN":  "pattern-{tenant}-{environment}-{stage}",
				"ATMOS_STACKS_NAME_TEMPLATE": "template-{tenant}-{environment}-{stage}",
			},
			expectedConfig: schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					NamePattern:  "pattern-{tenant}-{environment}-{stage}",
					NameTemplate: "template-{tenant}-{environment}-{stage}",
				},
			},
			expectError: false,
		},
		{
			name: "test terraform config env vars",
			envVars: map[string]string{
				"ATMOS_COMPONENTS_TERRAFORM_COMMAND":                    "/path/to/terraform",
				"ATMOS_COMPONENTS_TERRAFORM_BASE_PATH":                  "/terraform/base/path",
				"ATMOS_COMPONENTS_TERRAFORM_APPEND_USER_AGENT":          "true",
				"ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE": "true",
			},
			expectedConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						Command:                 "/path/to/terraform",
						BasePath:                "/terraform/base/path",
						AppendUserAgent:         "true",
						AutoGenerateBackendFile: true,
					},
				},
			},
			expectError: false,
		},
		{
			name: "test helmfile config env vars",
			envVars: map[string]string{
				"ATMOS_COMPONENTS_HELMFILE_COMMAND":                  "/path/to/helmfile",
				"ATMOS_COMPONENTS_HELMFILE_BASE_PATH":                "/helmfile/base/path",
				"ATMOS_COMPONENTS_HELMFILE_USE_EKS":                  "true",
				"ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH":          "/path/to/kubeconfig",
				"ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN": "{namespace}-{tenant}-{environment}-{stage}-helm-aws-profile",
				"ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN":     "{namespace}-{tenant}-{environment}-{stage}-cluster",
			},
			expectedConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						Command:               "/path/to/helmfile",
						BasePath:              "/helmfile/base/path",
						UseEKS:                true,
						KubeconfigPath:        "/path/to/kubeconfig",
						HelmAwsProfilePattern: "{namespace}-{tenant}-{environment}-{stage}-helm-aws-profile",
						ClusterNamePattern:    "{namespace}-{tenant}-{environment}-{stage}-cluster",
					},
				},
			},
			expectError: false,
		},
		{
			name: "test packer config env vars",
			envVars: map[string]string{
				"ATMOS_COMPONENTS_PACKER_COMMAND":   "/path/to/packer",
				"ATMOS_COMPONENTS_PACKER_BASE_PATH": "/packer/base/path",
			},
			expectedConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Packer: schema.Packer{
						Command:  "/path/to/packer",
						BasePath: "/packer/base/path",
					},
				},
			},
			expectError: false,
		},
		{
			name: "test workflows config env var",
			envVars: map[string]string{
				"ATMOS_WORKFLOWS_BASE_PATH": "/workflows/base/path",
			},
			expectedConfig: schema.AtmosConfiguration{
				Workflows: schema.Workflows{
					BasePath: "/workflows/base/path",
				},
			},
			expectError: false,
		},
		{
			name: "test schema config env vars",
			envVars: map[string]string{
				"ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH": "/schemas/jsonschema",
				"ATMOS_SCHEMAS_OPA_BASE_PATH":        "/schemas/opa",
				"ATMOS_SCHEMAS_CUE_BASE_PATH":        "/schemas/cue",
				"ATMOS_SCHEMAS_ATMOS_MANIFEST":       "/schemas/atmos-manifest.json",
			},
			expectedConfig: schema.AtmosConfiguration{
				Schemas: map[string]interface{}{
					"jsonschema": schema.ResourcePath{BasePath: "/schemas/jsonschema"},
					"opa":        schema.ResourcePath{BasePath: "/schemas/opa"},
					"cue":        schema.ResourcePath{BasePath: "/schemas/cue"},
					"atmos":      schema.SchemaRegistry{Manifest: "/schemas/atmos-manifest.json"},
				},
			},
			expectError: false,
		},
		{
			name: "test boolean env vars",
			envVars: map[string]string{
				"ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE":   "true",
				"ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT":      "false",
				"ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE": "true",
				"ATMOS_COMPONENTS_TERRAFORM_INIT_PASS_VARS":       "true",
				"ATMOS_COMPONENTS_TERRAFORM_PLAN_SKIP_PLANFILE":   "true",
			},
			expectedConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						ApplyAutoApprove:   true,
						DeployRunInit:      false,
						InitRunReconfigure: true,
						Init: schema.TerraformInit{
							PassVars: true,
						},
						Plan: schema.TerraformPlan{
							SkipPlanfile: true,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "test invalid boolean env var",
			envVars: map[string]string{
				"ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE": "not-a-boolean",
			},
			expectError:  true,
			errorMessage: "strconv.ParseBool: parsing \"not-a-boolean\": invalid syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables for the test case
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			// Clean up environment variables after the test
			defer func() {
				for k := range tt.envVars {
					os.Unsetenv(k)
				}
			}()

			// Create a default config and apply env vars
			config := &schema.AtmosConfiguration{
				Schemas: make(map[string]interface{}),
			}

			err := processEnvVars(config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMessage)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFindAllStackConfigsInPathsForStack(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/stack-templates-2"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)

	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Unset env values after testing
	defer func() {
		err := os.Unsetenv("ATMOS_BASE_PATH")
		assert.NoError(t, err)
		err = os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		assert.NoError(t, err)
	}()

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := InitCliConfig(configAndStacksInfo, true)
	assert.NoError(t, err)

	_, relativePaths, _, err := FindAllStackConfigsInPathsForStack(
		atmosConfig,
		"nonprod",
		atmosConfig.IncludeStackAbsolutePaths,
		atmosConfig.ExcludeStackAbsolutePaths,
	)
	assert.NoError(t, err)
	assert.Equal(t, "deploy/nonprod.yaml", relativePaths[0])
}

func TestFindAllStackConfigsInPaths(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/atmos-overrides-section"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)

	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Unset env values after testing
	defer func() {
		err := os.Unsetenv("ATMOS_BASE_PATH")
		assert.NoError(t, err)
		err = os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		assert.NoError(t, err)
	}()

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := InitCliConfig(configAndStacksInfo, true)
	assert.NoError(t, err)

	_, relativePaths, err := FindAllStackConfigsInPaths(
		&atmosConfig,
		atmosConfig.IncludeStackAbsolutePaths,
		atmosConfig.ExcludeStackAbsolutePaths,
	)
	assert.NoError(t, err)
	assert.Equal(t, "deploy/dev.yaml", relativePaths[0])
	assert.Equal(t, "deploy/prod.yaml", relativePaths[1])
	assert.Equal(t, "deploy/sandbox.yaml", relativePaths[2])
	assert.Equal(t, "deploy/staging.yaml", relativePaths[3])
}
