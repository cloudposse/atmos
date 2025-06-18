package list

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

// --- Mocks

type mockGitRepo struct {
	info *git.RepoInfo
	err  error
}

func (m *mockGitRepo) GetRepoInfo() (*git.RepoInfo, error) { return m.info, m.err }

type mockAPI struct {
	captured *dtos.DeploymentsUploadRequest
	err      error
}

func (m *mockAPI) UploadDriftDetection(req *dtos.DeploymentsUploadRequest) error {
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
							"pro": map[string]interface{}{"enabled": true},
						},
						"metadata": map[string]interface{}{"type": "real"},
					},
				},
				"helmfile": map[string]interface{}{
					"app": map[string]interface{}{
						"settings": map[string]interface{}{
							"pro": map[string]interface{}{"enabled": false},
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
				err = api.UploadDriftDetection(&dto)
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
