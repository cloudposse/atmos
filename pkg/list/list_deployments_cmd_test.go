package list

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// --- Mocks

type mockAPI struct {
	captured *dtos.DeploymentsUploadRequest
	err      error
}

func (m *mockAPI) UploadDeployments(req *dtos.DeploymentsUploadRequest) error {
	m.captured = req
	return m.err
}

type mockDescribe struct {
	stacks map[string]interface{}
	err    error
}

func (m *mockDescribe) Execute() (map[string]interface{}, error) { return m.stacks, m.err }

// --- Tests

func TestListDeploymentsCommandLogic(t *testing.T) {
	mockStacks := map[string]interface{}{
		"stack1": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"settings": map[string]interface{}{
							"pro": map[string]interface{}{
								"drift_detection": map[string]interface{}{"enabled": true},
							},
						},
						"metadata": map[string]interface{}{"type": "real"},
					},
				},
				"helmfile": map[string]interface{}{
					"app": map[string]interface{}{
						"settings": map[string]interface{}{
							"pro": map[string]interface{}{
								"drift_detection": map[string]interface{}{"enabled": false},
							},
						},
						"metadata": map[string]interface{}{"type": "real"},
					},
				},
			},
		},
	}

	tests := []struct {
		name              string
		upload            bool
		stacks            map[string]interface{}
		uploadErr         error
		expectedListSize  int
		expectedUploadNum int
		expectError       bool
	}{
		{
			name:             "list only",
			upload:           false,
			stacks:           mockStacks,
			expectedListSize: 2, // vpc and app (abstract filtered)
		},
		{
			name:              "list with upload",
			upload:            true,
			stacks:            mockStacks,
			expectedListSize:  2,
			expectedUploadNum: 1, // only vpc pro-enabled
		},
		{
			name:        "describe error",
			upload:      false,
			stacks:      nil,
			expectError: true,
		},
		{
			name:              "upload error is surfaced",
			upload:            true,
			stacks:            mockStacks,
			uploadErr:         errors.New("upload failed"),
			expectedListSize:  2,
			expectedUploadNum: 1,
			expectError:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// describe mock
			descr := &mockDescribe{stacks: tc.stacks, err: nil}
			if tc.stacks == nil {
				descr.err = errors.New("describe error")
			}

			// execute logic similar to command
			stacks, err := descr.Execute()
			if tc.expectError && err != nil {
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			list := collectDeployments(stacks)
			list = sortDeployments(list)
			assert.Equal(t, tc.expectedListSize, len(list))

			if tc.upload {
				api := &mockAPI{err: tc.uploadErr}
				proDeps := filterProEnabledDeployments(list)
				dto := dtos.DeploymentsUploadRequest{Deployments: proDeps}
				err = api.UploadDeployments(&dto)
				if tc.expectError {
					assert.Error(t, err)
					return
				}
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedUploadNum, len(proDeps))

				// Verify API received correct payload
				assert.NotNil(t, api.captured)
				assert.Equal(t, len(proDeps), len(api.captured.Deployments))
			}
		})
	}
}

func TestCreateDeploymentWithTemplateRendering(t *testing.T) {
	// Test that createDeployment properly handles pre-rendered template data
	componentConfigMap := map[string]any{
		"settings": map[string]any{
			"pro": map[string]any{
				"drift_detection": map[string]any{
					"enabled": true,
				},
				"pull_request": map[string]any{
					"merged": map[string]any{
						"workflows": map[string]any{
							"atmos-pro-terraform-apply.yaml": map[string]any{
								"inputs": map[string]any{
									"component":          "vpc",             // Rendered value
									"github_environment": "tenant1-dev",     // Rendered value
									"stack":              "tenant1-ue2-dev", // Rendered value
								},
							},
						},
					},
				},
			},
		},
		"vars": map[string]any{
			"tenant": "tenant1",
			"stage":  "dev",
			"region": "ue2",
		},
		"metadata": map[string]any{
			"type": "real",
		},
	}

	deployment := createDeployment("tenant1-ue2-dev", "vpc", "terraform", componentConfigMap)
	assert.NotNil(t, deployment)
	assert.Equal(t, "vpc", deployment.Component)
	assert.Equal(t, "tenant1-ue2-dev", deployment.Stack)
	assert.Equal(t, "terraform", deployment.ComponentType)

	// Verify that the template variables in settings are properly rendered
	proSettings := deployment.Settings["pro"].(map[string]any)
	pullRequest := proSettings["pull_request"].(map[string]any)
	merged := pullRequest["merged"].(map[string]any)
	workflows := merged["workflows"].(map[string]any)
	workflow := workflows["atmos-pro-terraform-apply.yaml"].(map[string]any)
	inputs := workflow["inputs"].(map[string]any)

	// These should be the actual rendered values, not template strings
	assert.Equal(t, "vpc", inputs["component"])
	assert.Equal(t, "tenant1-dev", inputs["github_environment"])
	assert.Equal(t, "tenant1-ue2-dev", inputs["stack"])

	// Verify that the deployment would be included in pro-enabled deployments
	proDeployments := filterProEnabledDeployments([]schema.Deployment{*deployment})
	assert.Len(t, proDeployments, 1)
	assert.Equal(t, "vpc", proDeployments[0].Component)
}
