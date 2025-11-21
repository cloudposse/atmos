package component

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// MockStackLoader is a mock implementation of StackLoader for testing.
type MockStackLoader struct {
	StacksMap       map[string]any
	RawStackConfigs map[string]map[string]any
	Err             error
}

// FindStacksMap returns the mock stacks map and error.
func (m *MockStackLoader) FindStacksMap(_ *schema.AtmosConfiguration, _ bool) (
	map[string]any,
	map[string]map[string]any,
	error,
) {
	return m.StacksMap, m.RawStackConfigs, m.Err
}

// NewMockStackLoader creates a new mock stack loader with the given stacks.
func NewMockStackLoader(stacksMap map[string]any) *MockStackLoader {
	return &MockStackLoader{
		StacksMap:       stacksMap,
		RawStackConfigs: make(map[string]map[string]any),
		Err:             nil,
	}
}

// NewMockStackLoaderWithError creates a mock that returns an error.
func NewMockStackLoaderWithError(err error) *MockStackLoader {
	return &MockStackLoader{
		StacksMap:       nil,
		RawStackConfigs: nil,
		Err:             err,
	}
}
