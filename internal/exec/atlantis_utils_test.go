package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBuildAtlantisProjectNameFromComponentConfig(t *testing.T) {
	tests := []struct {
		name            string
		atmosConfig     schema.AtmosConfiguration
		configAndStacks schema.ConfigAndStacksInfo
		expectedName    string
		expectedErr     bool
	}{
		{
			name:        "project_template provided",
			atmosConfig: schema.AtmosConfiguration{},
			configAndStacks: schema.ConfigAndStacksInfo{
				ComponentSettingsSection: map[string]interface{}{
					"atlantis": map[string]interface{}{
						"project_template": map[string]interface{}{
							"Name": "test-project-{component}",
						},
					},
				},
				ComponentFromArg: "test/component",
				ComponentVarsSection: map[string]interface{}{
					"environment": "dev",
				},
			},
			expectedName: "test-project-test-component",
			expectedErr:  false,
		},
		{
			name: "project_template_name provided",
			atmosConfig: schema.AtmosConfiguration{
				Integrations: schema.Integrations{
					Atlantis: schema.Atlantis{
						ProjectTemplates: map[string]schema.AtlantisProjectConfig{
							"template1": {
								Name: "template-project-{component}",
							},
						},
					},
				},
			},
			configAndStacks: schema.ConfigAndStacksInfo{
				ComponentSettingsSection: map[string]interface{}{
					"atlantis": map[string]interface{}{
						"project_template_name": "template1",
					},
				},
				ComponentFromArg: "test/component",
				ComponentVarsSection: map[string]interface{}{
					"environment": "dev",
				},
			},
			expectedName: "template-project-test-component",
			expectedErr:  false,
		},
		{
			name:        "no atlantis settings",
			atmosConfig: schema.AtmosConfiguration{},
			configAndStacks: schema.ConfigAndStacksInfo{
				ComponentSettingsSection: map[string]interface{}{},
				ComponentFromArg:         "test/component",
			},
			expectedName: "",
			expectedErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildAtlantisProjectNameFromComponentConfig(&tt.atmosConfig, tt.configAndStacks)

			if (err != nil) != tt.expectedErr {
				t.Errorf("BuildAtlantisProjectNameFromComponentConfig() error = %v, expectedErr %v", err, tt.expectedErr)
				return
			}

			if got != tt.expectedName {
				t.Errorf("BuildAtlantisProjectNameFromComponentConfig() got = %v, want %v", got, tt.expectedName)
			}
		})
	}
}
