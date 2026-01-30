package ansible

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAnsibleComponentProvider_GetType(t *testing.T) {
	provider := &AnsibleComponentProvider{}
	assert.Equal(t, "ansible", provider.GetType())
}

func TestAnsibleComponentProvider_GetGroup(t *testing.T) {
	provider := &AnsibleComponentProvider{}
	assert.Equal(t, "Configuration Management", provider.GetGroup())
}

func TestAnsibleComponentProvider_GetBasePath(t *testing.T) {
	provider := &AnsibleComponentProvider{}

	tests := []struct {
		name         string
		atmosConfig  *schema.AtmosConfiguration
		expectedPath string
	}{
		{
			name:         "nil config returns default",
			atmosConfig:  nil,
			expectedPath: "components/ansible",
		},
		{
			name: "with configured base_path",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Ansible: schema.Ansible{
						BasePath: "custom/ansible/path",
					},
				},
			},
			expectedPath: "custom/ansible/path",
		},
		{
			name: "with empty base_path returns default",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Ansible: schema.Ansible{
						BasePath: "",
					},
				},
			},
			expectedPath: "components/ansible",
		},
		{
			name: "with command but no base_path",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Ansible: schema.Ansible{
						Command: "/usr/local/bin/ansible",
					},
				},
			},
			expectedPath: "components/ansible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := provider.GetBasePath(tt.atmosConfig)
			assert.Equal(t, tt.expectedPath, path)
		})
	}
}

func TestAnsibleComponentProvider_ListComponents(t *testing.T) {
	provider := &AnsibleComponentProvider{}

	tests := []struct {
		name          string
		stackConfig   map[string]any
		expectedComps []string
		expectedErr   bool
	}{
		{
			name: "with ansible components",
			stackConfig: map[string]any{
				"components": map[string]any{
					"ansible": map[string]any{
						"webserver": map[string]any{
							"vars": map[string]any{
								"nginx_version": "1.24",
							},
						},
						"database": map[string]any{
							"vars": map[string]any{
								"postgres_version": "15",
							},
						},
					},
				},
			},
			expectedComps: []string{"database", "webserver"},
			expectedErr:   false,
		},
		{
			name: "without ansible section",
			stackConfig: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{},
				},
			},
			expectedComps: []string{},
			expectedErr:   false,
		},
		{
			name:          "without components section",
			stackConfig:   map[string]any{},
			expectedComps: []string{},
			expectedErr:   false,
		},
		{
			name:          "with nil stack config",
			stackConfig:   nil,
			expectedComps: []string{},
			expectedErr:   false,
		},
		{
			name: "with mixed component types",
			stackConfig: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{},
					},
					"ansible": map[string]any{
						"playbook-a": map[string]any{},
					},
					"helmfile": map[string]any{
						"app": map[string]any{},
					},
				},
			},
			expectedComps: []string{"playbook-a"},
			expectedErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			components, err := provider.ListComponents(context.Background(), "test-stack", tt.stackConfig)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedComps, components)
			}
		})
	}
}

func TestAnsibleComponentProvider_ValidateComponent(t *testing.T) {
	provider := &AnsibleComponentProvider{}

	tests := []struct {
		name    string
		config  map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "empty config",
			config:  map[string]any{},
			wantErr: false,
		},
		{
			name: "valid config with vars",
			config: map[string]any{
				"vars": map[string]any{
					"nginx_version": "1.24",
					"port":          80,
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with settings.ansible.playbook",
			config: map[string]any{
				"settings": map[string]any{
					"ansible": map[string]any{
						"playbook":  "site.yml",
						"inventory": "inventory/production",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid playbook type (not string)",
			config: map[string]any{
				"settings": map[string]any{
					"ansible": map[string]any{
						"playbook": 123,
					},
				},
			},
			wantErr: true,
			errMsg:  "playbook must be a string",
		},
		{
			name: "invalid inventory type (not string)",
			config: map[string]any{
				"settings": map[string]any{
					"ansible": map[string]any{
						"inventory": []string{"host1", "host2"},
					},
				},
			},
			wantErr: true,
			errMsg:  "inventory must be a string",
		},
		{
			name: "abstract component is valid",
			config: map[string]any{
				"metadata": map[string]any{
					"type": "abstract",
				},
			},
			wantErr: false,
		},
		{
			name: "complex valid config",
			config: map[string]any{
				"vars": map[string]any{
					"package_name": "nginx",
					"package_state": "present",
				},
				"settings": map[string]any{
					"ansible": map[string]any{
						"playbook":  "playbooks/webserver.yml",
						"inventory": "inventory/staging",
					},
					"validation": map[string]any{
						"enabled": true,
					},
				},
				"metadata": map[string]any{
					"component": "webserver",
				},
				"env": map[string]any{
					"ANSIBLE_CONFIG": "/etc/ansible/ansible.cfg",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.ValidateComponent(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAnsibleComponentProvider_Execute(t *testing.T) {
	provider := &AnsibleComponentProvider{}

	// Execute delegates to ExecutePlaybook or ExecuteVersion based on subcommand.
	// Without proper configuration, it will fail during config initialization.
	ctx := component.ExecutionContext{
		ComponentType: "ansible",
		Component:     "webserver",
		Stack:         "dev/us-east-1",
		Command:       "ansible",
		SubCommand:    "playbook",
	}

	// Execute attempts to run but fails due to missing configuration.
	// This validates that the Execute method routes to the correct executor.
	err := provider.Execute(&ctx)
	assert.Error(t, err)
	// The error will be from config initialization or missing atmos.yaml.
}

func TestAnsibleComponentProvider_GenerateArtifacts(t *testing.T) {
	provider := &AnsibleComponentProvider{}

	ctx := component.ExecutionContext{
		Component: "webserver",
		Stack:     "dev/us-east-1",
	}

	// GenerateArtifacts is a no-op for ansible since artifact generation
	// is handled within ExecuteAnsible.
	err := provider.GenerateArtifacts(&ctx)
	assert.NoError(t, err)
}

func TestAnsibleComponentProvider_GetAvailableCommands(t *testing.T) {
	provider := &AnsibleComponentProvider{}

	commands := provider.GetAvailableCommands()

	assert.NotEmpty(t, commands)
	assert.Contains(t, commands, "playbook")
	assert.Contains(t, commands, "version")
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "components/ansible", config.BasePath)
	assert.Equal(t, "ansible", config.Command)
	assert.False(t, config.AutoGenerateFiles)
}

// TestAnsibleComponentProvider_Integration tests the provider with realistic scenarios.
func TestAnsibleComponentProvider_Integration(t *testing.T) {
	provider := &AnsibleComponentProvider{}

	// Simulate a complete workflow.
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Ansible: schema.Ansible{
				BasePath:          "test/ansible",
				Command:           "/usr/local/bin/ansible-playbook",
				AutoGenerateFiles: true,
			},
		},
	}

	// Get base path.
	basePath := provider.GetBasePath(atmosConfig)
	assert.Equal(t, "test/ansible", basePath)

	// List components.
	stackConfig := map[string]any{
		"components": map[string]any{
			"ansible": map[string]any{
				"webserver": map[string]any{
					"vars": map[string]any{
						"nginx_version": "1.24",
					},
					"settings": map[string]any{
						"ansible": map[string]any{
							"playbook":  "playbooks/webserver.yml",
							"inventory": "inventory/staging",
						},
					},
				},
				"database": map[string]any{
					"vars": map[string]any{
						"postgres_version": "15",
					},
				},
			},
		},
	}

	components, err := provider.ListComponents(context.Background(), "staging/us-west-2", stackConfig)
	require.NoError(t, err)
	assert.Len(t, components, 2)
	assert.ElementsMatch(t, []string{"webserver", "database"}, components)

	// Validate component.
	componentConfig := map[string]any{
		"vars": map[string]any{
			"nginx_version": "1.24",
		},
		"settings": map[string]any{
			"ansible": map[string]any{
				"playbook":  "playbooks/webserver.yml",
				"inventory": "inventory/staging",
			},
		},
	}

	err = provider.ValidateComponent(componentConfig)
	assert.NoError(t, err)

	// Generate artifacts (no-op for ansible).
	ctx := component.ExecutionContext{
		AtmosConfig:   atmosConfig,
		ComponentType: "ansible",
		Component:     "webserver",
		Stack:         "staging/us-west-2",
		Command:       "playbook",
	}

	err = provider.GenerateArtifacts(&ctx)
	assert.NoError(t, err)
}

// TestAnsibleComponentProvider_ImplementsInterface verifies the provider implements ComponentProvider.
func TestAnsibleComponentProvider_ImplementsInterface(t *testing.T) {
	var _ component.ComponentProvider = (*AnsibleComponentProvider)(nil)
}
