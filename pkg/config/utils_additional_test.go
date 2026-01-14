package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// TestCheckConfig tests the checkConfig function for various validation scenarios.
func TestCheckConfig(t *testing.T) {
	tests := []struct {
		name           string
		config         schema.AtmosConfiguration
		isProcessStack bool
		expectError    bool
		errorContains  string
	}{
		{
			name: "valid config for stack processing",
			config: schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					BasePath:      "/stacks",
					IncludedPaths: []string{"deploy/**/*"},
				},
				Logs: schema.Logs{
					Level: "Info",
				},
			},
			isProcessStack: true,
			expectError:    false,
		},
		{
			name: "missing stacks base path",
			config: schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					BasePath:      "",
					IncludedPaths: []string{"deploy/**/*"},
				},
			},
			isProcessStack: true,
			expectError:    true,
			errorContains:  "stack base path must be provided",
		},
		{
			name: "missing stacks included paths",
			config: schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					BasePath:      "/stacks",
					IncludedPaths: []string{},
				},
			},
			isProcessStack: true,
			expectError:    true,
			errorContains:  "at least one path must be provided",
		},
		{
			name: "invalid log level",
			config: schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					BasePath:      "/stacks",
					IncludedPaths: []string{"deploy/**/*"},
				},
				Logs: schema.Logs{
					Level: "InvalidLevel",
				},
			},
			isProcessStack: true,
			expectError:    true,
			errorContains:  "invalid log level",
		},
		{
			name: "valid log levels",
			config: schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					BasePath:      "/stacks",
					IncludedPaths: []string{"deploy/**/*"},
				},
				Logs: schema.Logs{
					Level: "Debug",
				},
			},
			isProcessStack: true,
			expectError:    false,
		},
		{
			name: "non-stack processing ignores stack validation",
			config: schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					BasePath:      "",
					IncludedPaths: []string{},
				},
				Logs: schema.Logs{
					Level: "Info",
				},
			},
			isProcessStack: false,
			expectError:    false,
		},
		{
			name: "empty log level uses default",
			config: schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					BasePath:      "/stacks",
					IncludedPaths: []string{"deploy/**/*"},
				},
				Logs: schema.Logs{
					Level: "",
				},
			},
			isProcessStack: true,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkConfig(tt.config, tt.isProcessStack)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestProcessCommandLineArgs tests command line argument processing.
func TestProcessCommandLineArgs(t *testing.T) {
	tests := []struct {
		name                string
		configAndStacksInfo schema.ConfigAndStacksInfo
		initialConfig       schema.AtmosConfiguration
		expectedUpdates     func(*testing.T, *schema.AtmosConfiguration)
		expectError         bool
	}{
		{
			name: "set base path from args",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				BasePath: "/custom/base",
			},
			initialConfig: schema.AtmosConfiguration{},
			expectedUpdates: func(t *testing.T, cfg *schema.AtmosConfiguration) {
				assert.Equal(t, "/custom/base", cfg.BasePath)
			},
		},
		{
			name: "set terraform config from args",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				TerraformCommand: "/usr/local/bin/terraform",
				TerraformDir:     "/custom/terraform",
			},
			initialConfig: schema.AtmosConfiguration{},
			expectedUpdates: func(t *testing.T, cfg *schema.AtmosConfiguration) {
				assert.Equal(t, "/usr/local/bin/terraform", cfg.Components.Terraform.Command)
				assert.Equal(t, "/custom/terraform", cfg.Components.Terraform.BasePath)
			},
		},
		{
			name: "set helmfile config from args",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				HelmfileCommand: "/usr/local/bin/helmfile",
				HelmfileDir:     "/custom/helmfile",
			},
			initialConfig: schema.AtmosConfiguration{},
			expectedUpdates: func(t *testing.T, cfg *schema.AtmosConfiguration) {
				assert.Equal(t, "/usr/local/bin/helmfile", cfg.Components.Helmfile.Command)
				assert.Equal(t, "/custom/helmfile", cfg.Components.Helmfile.BasePath)
			},
		},
		{
			name: "set packer config from args",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				PackerCommand: "/usr/local/bin/packer",
				PackerDir:     "/custom/packer",
			},
			initialConfig: schema.AtmosConfiguration{},
			expectedUpdates: func(t *testing.T, cfg *schema.AtmosConfiguration) {
				assert.Equal(t, "/usr/local/bin/packer", cfg.Components.Packer.Command)
				assert.Equal(t, "/custom/packer", cfg.Components.Packer.BasePath)
			},
		},
		{
			name: "set stacks config from args",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				StacksDir: "/custom/stacks",
			},
			initialConfig: schema.AtmosConfiguration{},
			expectedUpdates: func(t *testing.T, cfg *schema.AtmosConfiguration) {
				assert.Equal(t, "/custom/stacks", cfg.Stacks.BasePath)
			},
		},
		{
			name: "set feature flags from args",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				DeployRunInit:           "true",
				AutoGenerateBackendFile: "true",
				InitRunReconfigure:      "true",
				InitPassVars:            "true",
				PlanSkipPlanfile:        "true",
			},
			initialConfig: schema.AtmosConfiguration{},
			expectedUpdates: func(t *testing.T, cfg *schema.AtmosConfiguration) {
				assert.True(t, cfg.Components.Terraform.DeployRunInit)
				assert.True(t, cfg.Components.Terraform.AutoGenerateBackendFile)
				assert.True(t, cfg.Components.Terraform.InitRunReconfigure)
				assert.True(t, cfg.Components.Terraform.Init.PassVars)
				assert.True(t, cfg.Components.Terraform.Plan.SkipPlanfile)
			},
		},
		{
			name: "set schema dirs from args",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				JsonSchemaDir:           "/schemas/jsonschema",
				OpaDir:                  "/schemas/opa",
				CueDir:                  "/schemas/cue",
				AtmosManifestJsonSchema: "/schemas/atmos-manifest.json",
			},
			initialConfig: schema.AtmosConfiguration{
				Schemas: make(map[string]interface{}),
			},
			expectedUpdates: func(t *testing.T, cfg *schema.AtmosConfiguration) {
				jsonSchema, ok := cfg.Schemas["jsonschema"].(schema.ResourcePath)
				assert.True(t, ok)
				assert.Equal(t, "/schemas/jsonschema", jsonSchema.BasePath)

				opaSchema, ok := cfg.Schemas["opa"].(schema.ResourcePath)
				assert.True(t, ok)
				assert.Equal(t, "/schemas/opa", opaSchema.BasePath)

				cueSchema, ok := cfg.Schemas["cue"].(schema.ResourcePath)
				assert.True(t, ok)
				assert.Equal(t, "/schemas/cue", cueSchema.BasePath)

				atmosSchema, ok := cfg.Schemas["atmos"].(schema.SchemaRegistry)
				assert.True(t, ok)
				assert.Equal(t, "/schemas/atmos-manifest.json", atmosSchema.Manifest)
			},
		},
		{
			name: "set logging config from args",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				LogsLevel: "Debug",
				LogsFile:  "/var/log/atmos.log",
			},
			initialConfig: schema.AtmosConfiguration{},
			expectedUpdates: func(t *testing.T, cfg *schema.AtmosConfiguration) {
				assert.Equal(t, "Debug", cfg.Logs.Level)
				assert.Equal(t, "/var/log/atmos.log", cfg.Logs.File)
			},
		},
		{
			name: "set settings config from args",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				SettingsListMergeStrategy: "append",
			},
			initialConfig: schema.AtmosConfiguration{},
			expectedUpdates: func(t *testing.T, cfg *schema.AtmosConfiguration) {
				assert.Equal(t, "append", cfg.Settings.ListMergeStrategy)
			},
		},
		{
			name: "invalid boolean for feature flag",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				DeployRunInit: "invalid-bool",
			},
			initialConfig: schema.AtmosConfiguration{},
			expectError:   true,
		},
		{
			name: "invalid log level",
			configAndStacksInfo: schema.ConfigAndStacksInfo{
				LogsLevel: "InvalidLevel",
			},
			initialConfig: schema.AtmosConfiguration{},
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.initialConfig
			err := processCommandLineArgs(&config, &tt.configAndStacksInfo)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectedUpdates != nil {
					tt.expectedUpdates(t, &config)
				}
			}
		})
	}
}

// TestProcessStoreConfig tests store configuration processing.
func TestProcessStoreConfig(t *testing.T) {
	tests := []struct {
		name         string
		config       schema.AtmosConfiguration
		expectError  bool
		expectStores bool
	}{
		{
			name: "empty stores config",
			config: schema.AtmosConfiguration{
				StoresConfig: store.StoresConfig{},
			},
			expectError:  false,
			expectStores: true,
		},
		{
			name: "nil stores config",
			config: schema.AtmosConfiguration{
				StoresConfig: store.StoresConfig{},
			},
			expectError:  false,
			expectStores: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			err := processStoreConfig(&config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectStores {
					assert.NotNil(t, config.Stores)
				}
			}
		})
	}
}

// TestGetContextFromVars tests extracting context from variables map.
func TestGetContextFromVars(t *testing.T) {
	tests := []struct {
		name     string
		vars     map[string]any
		expected schema.Context
	}{
		{
			name: "full context",
			vars: map[string]any{
				"namespace":   "cp",
				"tenant":      "platform",
				"environment": "ue2",
				"stage":       "prod",
				"region":      "us-east-2",
				"attributes":  []any{"blue", "green"},
			},
			expected: schema.Context{
				Namespace:   "cp",
				Tenant:      "platform",
				Environment: "ue2",
				Stage:       "prod",
				Region:      "us-east-2",
				Attributes:  []any{"blue", "green"},
			},
		},
		{
			name: "partial context",
			vars: map[string]any{
				"namespace": "cp",
				"stage":     "dev",
			},
			expected: schema.Context{
				Namespace: "cp",
				Stage:     "dev",
			},
		},
		{
			name:     "empty vars",
			vars:     map[string]any{},
			expected: schema.Context{},
		},
		{
			name: "wrong types ignored",
			vars: map[string]any{
				"namespace": 123, // Wrong type, should be ignored
				"stage":     "dev",
			},
			expected: schema.Context{
				Stage: "dev",
			},
		},
		{
			name: "attributes as empty slice",
			vars: map[string]any{
				"attributes": []any{},
			},
			expected: schema.Context{
				Attributes: []any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetContextFromVars(tt.vars)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetContextPrefix tests context prefix calculation.
func TestGetContextPrefix(t *testing.T) {
	tests := []struct {
		name             string
		stack            string
		context          schema.Context
		stackNamePattern string
		stackFile        string
		expected         string
		expectError      bool
		errorContains    string
	}{
		{
			name:  "full context with all tokens",
			stack: "test-stack",
			context: schema.Context{
				Namespace:   "cp",
				Tenant:      "platform",
				Environment: "ue2",
				Stage:       "prod",
			},
			stackNamePattern: "{namespace}-{tenant}-{environment}-{stage}",
			stackFile:        "stacks/test.yaml",
			expected:         "cp-platform-ue2-prod",
		},
		{
			name:  "partial context",
			stack: "test-stack",
			context: schema.Context{
				Tenant: "platform",
				Stage:  "dev",
			},
			stackNamePattern: "{tenant}-{stage}",
			stackFile:        "stacks/test.yaml",
			expected:         "platform-dev",
		},
		{
			name:             "empty pattern",
			stack:            "test-stack",
			context:          schema.Context{},
			stackNamePattern: "",
			stackFile:        "stacks/test.yaml",
			expectError:      true,
			errorContains:    "stack name pattern must be provided",
		},
		{
			name:             "missing namespace",
			stack:            "test-stack",
			context:          schema.Context{},
			stackNamePattern: "{namespace}-{stage}",
			stackFile:        "stacks/test.yaml",
			expectError:      true,
			errorContains:    "does not have a namespace defined",
		},
		{
			name:  "missing tenant",
			stack: "test-stack",
			context: schema.Context{
				Namespace: "cp",
			},
			stackNamePattern: "{namespace}-{tenant}",
			stackFile:        "stacks/test.yaml",
			expectError:      true,
			errorContains:    "does not have a tenant defined",
		},
		{
			name:  "missing environment",
			stack: "test-stack",
			context: schema.Context{
				Namespace: "cp",
			},
			stackNamePattern: "{namespace}-{environment}",
			stackFile:        "stacks/test.yaml",
			expectError:      true,
			errorContains:    "does not have an environment defined",
		},
		{
			name:  "missing stage",
			stack: "test-stack",
			context: schema.Context{
				Namespace: "cp",
			},
			stackNamePattern: "{namespace}-{stage}",
			stackFile:        "stacks/test.yaml",
			expectError:      true,
			errorContains:    "does not have a stage defined",
		},
		{
			name:  "single token pattern",
			stack: "test-stack",
			context: schema.Context{
				Stage: "prod",
			},
			stackNamePattern: "{stage}",
			stackFile:        "stacks/test.yaml",
			expected:         "prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetContextPrefix(tt.stack, tt.context, tt.stackNamePattern, tt.stackFile)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestReplaceContextTokens tests token replacement in patterns.
func TestReplaceContextTokens(t *testing.T) {
	tests := []struct {
		name     string
		context  schema.Context
		pattern  string
		expected string
	}{
		{
			name: "all tokens",
			context: schema.Context{
				BaseComponent:      "vpc",
				Component:          "vpc/defaults",
				ComponentPath:      "components/terraform/vpc",
				Namespace:          "cp",
				Environment:        "ue2",
				Region:             "us-east-2",
				Tenant:             "platform",
				Stage:              "prod",
				Workspace:          "default",
				TerraformWorkspace: "prod-vpc",
				Attributes:         []any{"blue", "green"},
			},
			pattern:  "{namespace}-{tenant}-{environment}-{stage}-{region}-{component}-{workspace}",
			expected: "cp-platform-ue2-prod-us-east-2-vpc/defaults-default",
		},
		{
			name: "attributes token",
			context: schema.Context{
				Namespace:  "cp",
				Attributes: []any{"blue", "green"},
			},
			pattern:  "{namespace}-{attributes}",
			expected: "cp-blue-green",
		},
		{
			name: "terraform workspace token",
			context: schema.Context{
				Stage:              "prod",
				TerraformWorkspace: "prod-vpc",
			},
			pattern:  "{stage}/{terraform_workspace}",
			expected: "prod/prod-vpc",
		},
		{
			name: "base component and component path",
			context: schema.Context{
				BaseComponent: "vpc",
				ComponentPath: "components/terraform/vpc",
			},
			pattern:  "{base-component} at {component-path}",
			expected: "vpc at components/terraform/vpc",
		},
		{
			name:     "no tokens",
			context:  schema.Context{},
			pattern:  "static-string",
			expected: "static-string",
		},
		{
			name: "empty attributes",
			context: schema.Context{
				Namespace:  "cp",
				Attributes: []any{},
			},
			pattern:  "{namespace}-{attributes}",
			expected: "cp-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplaceContextTokens(tt.context, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetStackNameFromContextAndStackNamePattern tests stack name generation from pattern.
func TestGetStackNameFromContextAndStackNamePattern(t *testing.T) {
	tests := []struct {
		name             string
		namespace        string
		tenant           string
		environment      string
		stage            string
		stackNamePattern string
		expected         string
		expectError      bool
		errorContains    string
	}{
		{
			name:             "full pattern",
			namespace:        "cp",
			tenant:           "platform",
			environment:      "ue2",
			stage:            "prod",
			stackNamePattern: "{namespace}-{tenant}-{environment}-{stage}",
			expected:         "cp-platform-ue2-prod",
		},
		{
			name:             "partial pattern",
			namespace:        "",
			tenant:           "platform",
			environment:      "",
			stage:            "dev",
			stackNamePattern: "{tenant}-{stage}",
			expected:         "platform-dev",
		},
		{
			name:             "single token",
			namespace:        "",
			tenant:           "",
			environment:      "",
			stage:            "prod",
			stackNamePattern: "{stage}",
			expected:         "prod",
		},
		{
			name:             "empty pattern",
			namespace:        "cp",
			tenant:           "platform",
			environment:      "ue2",
			stage:            "prod",
			stackNamePattern: "",
			expectError:      true,
			errorContains:    "stack name pattern must be provided",
		},
		{
			name:             "missing namespace",
			namespace:        "",
			tenant:           "platform",
			environment:      "ue2",
			stage:            "prod",
			stackNamePattern: "{namespace}-{stage}",
			expectError:      true,
			errorContains:    "namespace is not provided",
		},
		{
			name:             "missing tenant",
			namespace:        "cp",
			tenant:           "",
			environment:      "ue2",
			stage:            "prod",
			stackNamePattern: "{namespace}-{tenant}",
			expectError:      true,
			errorContains:    "tenant is not provided",
		},
		{
			name:             "missing environment",
			namespace:        "cp",
			tenant:           "platform",
			environment:      "",
			stage:            "prod",
			stackNamePattern: "{namespace}-{environment}",
			expectError:      true,
			errorContains:    "environment is not provided",
		},
		{
			name:             "missing stage",
			namespace:        "cp",
			tenant:           "platform",
			environment:      "ue2",
			stage:            "",
			stackNamePattern: "{namespace}-{stage}",
			expectError:      true,
			errorContains:    "stage is not provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetStackNameFromContextAndStackNamePattern(
				tt.namespace,
				tt.tenant,
				tt.environment,
				tt.stage,
				tt.stackNamePattern,
			)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestSearchConfigFile tests config file search with various extensions.
func TestSearchConfigFile(t *testing.T) {
	// Create temporary directory for test files.
	tmpDir := t.TempDir()

	// Create test files.
	testFiles := []string{
		"test1.yaml",
		"test2.yml",
		"test3.yaml.tmpl",
		"test4.yml.tmpl",
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tmpDir, file)
		err := os.WriteFile(fullPath, []byte("test"), 0o644)
		assert.NoError(t, err)
	}

	atmosConfig := schema.AtmosConfiguration{}

	tests := []struct {
		name          string
		configPath    string
		expected      string
		expectError   bool
		errorContains string
	}{
		{
			name:       "file with yaml extension exists",
			configPath: filepath.Join(tmpDir, "test1.yaml"),
			expected:   filepath.Join(tmpDir, "test1.yaml"),
		},
		{
			name:       "file with yml extension exists",
			configPath: filepath.Join(tmpDir, "test2.yml"),
			expected:   filepath.Join(tmpDir, "test2.yml"),
		},
		{
			name:       "file without extension - finds yaml",
			configPath: filepath.Join(tmpDir, "test1"),
			expected:   filepath.Join(tmpDir, "test1.yaml"),
		},
		{
			name:       "file without extension - finds yml",
			configPath: filepath.Join(tmpDir, "test2"),
			expected:   filepath.Join(tmpDir, "test2.yml"),
		},
		{
			name:          "specified file does not exist",
			configPath:    filepath.Join(tmpDir, "nonexistent.yaml"),
			expectError:   true,
			errorContains: "specified config file not found",
		},
		{
			name:          "file without extension not found",
			configPath:    filepath.Join(tmpDir, "nonexistent"),
			expectError:   true,
			errorContains: "failed to find a match",
		},
		{
			name:          "directory does not exist",
			configPath:    filepath.Join(tmpDir, "nonexistent-dir", "test.yaml"),
			expectError:   true,
			errorContains: "specified config file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SearchConfigFile(tt.configPath, atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestGetConfigFilePatterns tests config file pattern generation.
func TestGetConfigFilePatterns(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		forGlobMatch  bool
		expectedCount int
		contains      []string
	}{
		{
			name:         "path with extension",
			path:         "config.yaml",
			forGlobMatch: false,
			contains:     []string{"config.yaml"},
		},
		{
			name:          "path without extension for glob",
			path:          "config",
			forGlobMatch:  true,
			expectedCount: 4,
			contains:      []string{"config.yaml", "config.yml", "config.yaml.tmpl", "config.yml.tmpl"},
		},
		{
			name:          "path without extension for direct search",
			path:          "config",
			forGlobMatch:  false,
			expectedCount: 5,
			contains:      []string{"config", "config.yaml", "config.yml", "config.yaml.tmpl", "config.yml.tmpl"},
		},
		{
			name:          "empty path",
			path:          "",
			forGlobMatch:  false,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getConfigFilePatterns(tt.path, tt.forGlobMatch)

			if tt.expectedCount > 0 {
				assert.Equal(t, tt.expectedCount, len(result))
			}

			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

// TestMatchesStackFilePattern tests stack file pattern matching.
func TestMatchesStackFilePattern(t *testing.T) {
	tests := []struct {
		name      string
		filePath  string
		stackName string
		expected  bool
	}{
		{
			name:      "matches yaml extension",
			filePath:  "/stacks/deploy/prod.yaml",
			stackName: "prod",
			expected:  true,
		},
		{
			name:      "matches yml extension",
			filePath:  "/stacks/deploy/prod.yml",
			stackName: "prod",
			expected:  true,
		},
		{
			name:      "matches yaml template",
			filePath:  "/stacks/deploy/prod.yaml.tmpl",
			stackName: "prod",
			expected:  true,
		},
		{
			name:      "matches yml template",
			filePath:  "/stacks/deploy/prod.yml.tmpl",
			stackName: "prod",
			expected:  true,
		},
		{
			name:      "no match",
			filePath:  "/stacks/deploy/dev.yaml",
			stackName: "prod",
			expected:  false,
		},
		{
			name:      "matches with directory separator",
			filePath:  "/stacks/deploy/prod/main.yaml",
			stackName: "deploy/prod/main",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesStackFilePattern(tt.filePath, tt.stackName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetStackFilePatterns tests stack file pattern generation.
func TestGetStackFilePatterns(t *testing.T) {
	tests := []struct {
		name             string
		basePath         string
		includeTemplates bool
		expected         []string
	}{
		{
			name:             "with templates",
			basePath:         "stacks/prod",
			includeTemplates: true,
			expected: []string{
				"stacks/prod.yaml",
				"stacks/prod.yml",
				"stacks/prod.yaml.tmpl",
				"stacks/prod.yml.tmpl",
			},
		},
		{
			name:             "without templates",
			basePath:         "stacks/prod",
			includeTemplates: false,
			expected: []string{
				"stacks/prod.yaml",
				"stacks/prod.yml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStackFilePatterns(tt.basePath, tt.includeTemplates)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

// TestSetBasePaths tests base path configuration.
func TestSetBasePaths(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	configAndStacks := &schema.ConfigAndStacksInfo{
		BasePath: "/custom/base",
	}

	err := setBasePaths(config, configAndStacks)
	assert.NoError(t, err)
	assert.Equal(t, "/custom/base", config.BasePath)
}

// TestSetTerraformConfig tests terraform configuration.
func TestSetTerraformConfig(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	configAndStacks := &schema.ConfigAndStacksInfo{
		TerraformCommand: "/usr/bin/terraform",
		TerraformDir:     "/terraform",
	}

	err := setTerraformConfig(config, configAndStacks)
	assert.NoError(t, err)
	assert.Equal(t, "/usr/bin/terraform", config.Components.Terraform.Command)
	assert.Equal(t, "/terraform", config.Components.Terraform.BasePath)
}

// TestSetHelmfileConfig tests helmfile configuration.
func TestSetHelmfileConfig(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	configAndStacks := &schema.ConfigAndStacksInfo{
		HelmfileCommand: "/usr/bin/helmfile",
		HelmfileDir:     "/helmfile",
	}

	err := setHelmfileConfig(config, configAndStacks)
	assert.NoError(t, err)
	assert.Equal(t, "/usr/bin/helmfile", config.Components.Helmfile.Command)
	assert.Equal(t, "/helmfile", config.Components.Helmfile.BasePath)
}

// TestSetPackerConfig tests packer configuration.
func TestSetPackerConfig(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	configAndStacks := &schema.ConfigAndStacksInfo{
		PackerCommand: "/usr/bin/packer",
		PackerDir:     "/packer",
	}

	err := setPackerConfig(config, configAndStacks)
	assert.NoError(t, err)
	assert.Equal(t, "/usr/bin/packer", config.Components.Packer.Command)
	assert.Equal(t, "/packer", config.Components.Packer.BasePath)
}

// TestSetStacksConfig tests stacks configuration.
func TestSetStacksConfig(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	configAndStacks := &schema.ConfigAndStacksInfo{
		StacksDir: "/stacks",
	}

	err := setStacksConfig(config, configAndStacks)
	assert.NoError(t, err)
	assert.Equal(t, "/stacks", config.Stacks.BasePath)
}

// TestSetSchemaDirs tests schema directory configuration.
func TestSetSchemaDirs(t *testing.T) {
	config := &schema.AtmosConfiguration{
		Schemas: make(map[string]interface{}),
	}
	configAndStacks := &schema.ConfigAndStacksInfo{
		JsonSchemaDir:           "/schemas/json",
		OpaDir:                  "/schemas/opa",
		CueDir:                  "/schemas/cue",
		AtmosManifestJsonSchema: "/schemas/atmos.json",
	}

	err := setSchemaDirs(config, configAndStacks)
	assert.NoError(t, err)

	jsonSchema := config.Schemas["jsonschema"].(schema.ResourcePath)
	assert.Equal(t, "/schemas/json", jsonSchema.BasePath)

	opaSchema := config.Schemas["opa"].(schema.ResourcePath)
	assert.Equal(t, "/schemas/opa", opaSchema.BasePath)

	cueSchema := config.Schemas["cue"].(schema.ResourcePath)
	assert.Equal(t, "/schemas/cue", cueSchema.BasePath)

	atmosSchema := config.Schemas["atmos"].(schema.SchemaRegistry)
	assert.Equal(t, "/schemas/atmos.json", atmosSchema.Manifest)
}

// TestSetLoggingConfig tests logging configuration.
func TestSetLoggingConfig(t *testing.T) {
	tests := []struct {
		name        string
		logsLevel   string
		logsFile    string
		expectError bool
	}{
		{
			name:      "valid log level",
			logsLevel: "Debug",
			logsFile:  "/var/log/atmos.log",
		},
		{
			name:        "invalid log level",
			logsLevel:   "InvalidLevel",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			configAndStacks := &schema.ConfigAndStacksInfo{
				LogsLevel: tt.logsLevel,
				LogsFile:  tt.logsFile,
			}

			err := setLoggingConfig(config, configAndStacks)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.logsLevel, config.Logs.Level)
				assert.Equal(t, tt.logsFile, config.Logs.File)
			}
		})
	}
}

// TestSetSettingsConfig tests settings configuration.
func TestSetSettingsConfig(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	configAndStacks := &schema.ConfigAndStacksInfo{
		SettingsListMergeStrategy: "append",
	}

	err := setSettingsConfig(config, configAndStacks)
	assert.NoError(t, err)
	assert.Equal(t, "append", config.Settings.ListMergeStrategy)
}
