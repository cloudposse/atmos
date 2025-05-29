package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockProAPIClient is a mock implementation of the pro API client
type MockProAPIClient struct {
	mock.Mock
}

func (m *MockProAPIClient) UploadDriftResultStatus(dto pro.DriftStatusUploadRequest) error {
	args := m.Called(dto)
	return args.Error(0)
}

func TestShouldUploadDriftResult(t *testing.T) {
	testCases := []struct {
		name     string
		info     *schema.ConfigAndStacksInfo
		expected bool
	}{
		{
			name: "should return true for plan command with pro enabled",
			info: &schema.ConfigAndStacksInfo{
				SubCommand: "plan",
				ComponentSettingsSection: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "should return false for plan command with pro disabled",
			info: &schema.ConfigAndStacksInfo{
				SubCommand: "plan",
				ComponentSettingsSection: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": false,
					},
				},
			},
			expected: false,
		},
		{
			name: "should return false for plan command with no pro settings",
			info: &schema.ConfigAndStacksInfo{
				SubCommand:               "plan",
				ComponentSettingsSection: map[string]interface{}{},
			},
			expected: false,
		},
		{
			name: "should return false for non-plan command",
			info: &schema.ConfigAndStacksInfo{
				SubCommand: "apply",
				ComponentSettingsSection: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := shouldUploadDriftResult(tc.info)
			assert.Equal(t, tc.expected, result)
		})
	}
}
