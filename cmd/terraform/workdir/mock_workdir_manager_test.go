package workdir

import (
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
)

// MockWorkdirManager is a mock implementation of WorkdirManager for testing.
type MockWorkdirManager struct {
	// ListWorkdirsFunc is called when ListWorkdirs is invoked.
	ListWorkdirsFunc func(atmosConfig *schema.AtmosConfiguration) ([]WorkdirInfo, error)

	// GetWorkdirInfoFunc is called when GetWorkdirInfo is invoked.
	GetWorkdirInfoFunc func(atmosConfig *schema.AtmosConfiguration, component, stack string) (*WorkdirInfo, error)

	// DescribeWorkdirFunc is called when DescribeWorkdir is invoked.
	DescribeWorkdirFunc func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error)

	// CleanWorkdirFunc is called when CleanWorkdir is invoked.
	CleanWorkdirFunc func(atmosConfig *schema.AtmosConfiguration, component, stack string) error

	// CleanAllWorkdirsFunc is called when CleanAllWorkdirs is invoked.
	CleanAllWorkdirsFunc func(atmosConfig *schema.AtmosConfiguration) error

	// Invocation tracking.
	ListWorkdirsCalls     int
	GetWorkdirInfoCalls   int
	DescribeWorkdirCalls  int
	CleanWorkdirCalls     int
	CleanAllWorkdirsCalls int
}

// ListWorkdirs implements WorkdirManager.
func (m *MockWorkdirManager) ListWorkdirs(atmosConfig *schema.AtmosConfiguration) ([]WorkdirInfo, error) {
	m.ListWorkdirsCalls++
	if m.ListWorkdirsFunc != nil {
		return m.ListWorkdirsFunc(atmosConfig)
	}
	return []WorkdirInfo{}, nil
}

// GetWorkdirInfo implements WorkdirManager.
func (m *MockWorkdirManager) GetWorkdirInfo(atmosConfig *schema.AtmosConfiguration, component, stack string) (*WorkdirInfo, error) {
	m.GetWorkdirInfoCalls++
	if m.GetWorkdirInfoFunc != nil {
		return m.GetWorkdirInfoFunc(atmosConfig, component, stack)
	}
	return nil, nil
}

// DescribeWorkdir implements WorkdirManager.
func (m *MockWorkdirManager) DescribeWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
	m.DescribeWorkdirCalls++
	if m.DescribeWorkdirFunc != nil {
		return m.DescribeWorkdirFunc(atmosConfig, component, stack)
	}
	return "", nil
}

// CleanWorkdir implements WorkdirManager.
func (m *MockWorkdirManager) CleanWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string) error {
	m.CleanWorkdirCalls++
	if m.CleanWorkdirFunc != nil {
		return m.CleanWorkdirFunc(atmosConfig, component, stack)
	}
	return nil
}

// CleanAllWorkdirs implements WorkdirManager.
func (m *MockWorkdirManager) CleanAllWorkdirs(atmosConfig *schema.AtmosConfiguration) error {
	m.CleanAllWorkdirsCalls++
	if m.CleanAllWorkdirsFunc != nil {
		return m.CleanAllWorkdirsFunc(atmosConfig)
	}
	return nil
}

// NewMockWorkdirManager creates a new mock WorkdirManager.
func NewMockWorkdirManager() *MockWorkdirManager {
	return &MockWorkdirManager{}
}

// Reset resets all call counters.
func (m *MockWorkdirManager) Reset() {
	m.ListWorkdirsCalls = 0
	m.GetWorkdirInfoCalls = 0
	m.DescribeWorkdirCalls = 0
	m.CleanWorkdirCalls = 0
	m.CleanAllWorkdirsCalls = 0
}

// CreateSampleWorkdirInfo creates a sample WorkdirInfo for testing.
func CreateSampleWorkdirInfo(component, stack string) *WorkdirInfo {
	return &WorkdirInfo{
		Name:        stack + "-" + component,
		Component:   component,
		Stack:       stack,
		Source:      "components/terraform/" + component,
		Path:        ".workdir/terraform/" + stack + "-" + component,
		ContentHash: "abc123",
		CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// CreateSampleWorkdirList creates a sample list of workdirs for testing.
func CreateSampleWorkdirList() []WorkdirInfo {
	return []WorkdirInfo{
		*CreateSampleWorkdirInfo("vpc", "dev"),
		*CreateSampleWorkdirInfo("vpc", "prod"),
		*CreateSampleWorkdirInfo("s3", "dev"),
	}
}
