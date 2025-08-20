package pro

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestUploadDeployments(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.DeploymentsUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Deployments: []schema.Deployment{
			{
				Component:     "vpc",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
				Vars: map[string]any{
					"environment": "dev",
					"tenant":      "tenant1",
					"region":      "ue2",
					"cidr_block":  "10.0.0.0/16",
				},
			},
			{
				Component:     "eks",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
				Vars: map[string]any{
					"environment":        "dev",
					"tenant":             "tenant1",
					"region":             "ue2",
					"cluster_name":       "tenant1-ue2-dev",
					"kubernetes_version": "1.27",
				},
			},
		},
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadDeployments(&dto)
	assert.NoError(t, err)

	mockRoundTripper.AssertExpectations(t)
}

func TestUploadDeployments_Error(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.DeploymentsUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Deployments: []schema.Deployment{
			{
				Component:     "vpc",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
				Vars: map[string]any{
					"environment": "dev",
					"tenant":      "tenant1",
					"region":      "ue2",
					"cidr_block":  "10.0.0.0/16",
				},
			},
			{
				Component:     "eks",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
					},
				},
				Vars: map[string]any{
					"environment":        "dev",
					"tenant":             "tenant1",
					"region":             "ue2",
					"cluster_name":       "tenant1-ue2-dev",
					"kubernetes_version": "1.27",
				},
			},
		},
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": false, "errorMessage": "Internal Server Error"}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadDeployments(&dto)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload deployment status")

	mockRoundTripper.AssertExpectations(t)
}

func TestUploadDeployments_TemplateVariablesNotRendered(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.DeploymentsUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Deployments: []schema.Deployment{
			{
				Component:     "vpc",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
						"pull_request": map[string]any{
							"merged": map[string]any{
								"workflows": map[string]any{
									"atmos-pro-terraform-apply.yaml": map[string]any{
										"inputs": map[string]any{
											"component":          "{{ .atmos_component }}",
											"github_environment": "{{ .vars.tenant }}-{{ .vars.stage }}",
											"stack":              "{{ .atmos_stack }}",
										},
									},
								},
							},
						},
					},
				},
				Vars: map[string]any{
					"tenant": "tenant1",
					"stage":  "dev",
					"region": "ue2",
				},
			},
		},
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadDeployments(&dto)
	assert.NoError(t, err)

	// Verify that the template variables are NOT rendered (this is the bug)
	// The template variables should be rendered to their actual values
	// but currently they remain as literal template strings
	deployment := dto.Deployments[0]
	proSettings := deployment.Settings["pro"].(map[string]any)
	pullRequest := proSettings["pull_request"].(map[string]any)
	merged := pullRequest["merged"].(map[string]any)
	workflows := merged["workflows"].(map[string]any)
	workflow := workflows["atmos-pro-terraform-apply.yaml"].(map[string]any)
	inputs := workflow["inputs"].(map[string]any)

	// These should be the actual values, not template strings
	assert.Equal(t, "{{ .atmos_component }}", inputs["component"])
	assert.Equal(t, "{{ .vars.tenant }}-{{ .vars.stage }}", inputs["github_environment"])
	assert.Equal(t, "{{ .atmos_stack }}", inputs["stack"])

	// The expected values should be:
	// component: "vpc"
	// github_environment: "tenant1-dev"
	// stack: "tenant1-ue2-dev"

	mockRoundTripper.AssertExpectations(t)
}

func TestUploadDeployments_TemplateVariablesRendered(t *testing.T) {
	mockRoundTripper := new(MockRoundTripper)
	httpClient := &http.Client{Transport: mockRoundTripper}
	apiClient := &AtmosProAPIClient{
		BaseURL:         "http://localhost",
		BaseAPIEndpoint: "api",
		APIToken:        "test-token",
		HTTPClient:      httpClient,
	}

	dto := dtos.DeploymentsUploadRequest{
		RepoURL:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
		Deployments: []schema.Deployment{
			{
				Component:     "vpc",
				Stack:         "tenant1-ue2-dev",
				ComponentType: "terraform",
				Settings: map[string]any{
					"pro": map[string]any{
						"drift_detection": map[string]any{
							"enabled": true,
						},
						"pull_request": map[string]any{
							"merged": map[string]any{
								"workflows": map[string]any{
									"atmos-pro-terraform-apply.yaml": map[string]any{
										"inputs": map[string]any{
											"component":          "vpc",             // This should be the rendered value
											"github_environment": "tenant1-dev",     // This should be the rendered value
											"stack":              "tenant1-ue2-dev", // This should be the rendered value
										},
									},
								},
							},
						},
					},
				},
				Vars: map[string]any{
					"tenant": "tenant1",
					"stage":  "dev",
					"region": "ue2",
				},
			},
		},
	}

	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"success": true}`)),
	}

	mockRoundTripper.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	err := apiClient.UploadDeployments(&dto)
	assert.NoError(t, err)

	// Verify that the template variables are properly rendered
	deployment := dto.Deployments[0]
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

	mockRoundTripper.AssertExpectations(t)
}
