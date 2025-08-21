package toolchain

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// MockToolRunner is a mock implementation of ToolRunner for testing
type MockToolRunner struct {
	FindBinaryPathFunc   func(owner, repo, version string) (string, error)
	GetResolverFunc      func() ToolResolver
	CreateLatestFileFunc func(owner, repo, version string) error
	ReadLatestFileFunc   func(owner, repo string) (string, error)
}

func (m *MockToolRunner) FindBinaryPath(owner, repo, version string) (string, error) {
	return m.FindBinaryPathFunc(owner, repo, version)
}

func (m *MockToolRunner) GetResolver() ToolResolver {
	return m.GetResolverFunc()
}

func (m *MockToolRunner) CreateLatestFile(owner, repo, version string) error {
	return m.CreateLatestFileFunc(owner, repo, version)
}

func (m *MockToolRunner) ReadLatestFile(owner, repo string) (string, error) {
	return m.ReadLatestFileFunc(owner, repo)
}

// MockToolResolver is a mock implementation of ToolResolver for testing
type MockToolResolver struct {
	ResolveFunc func(tool string) (string, string, error)
}

func (m *MockToolResolver) Resolve(tool string) (string, string, error) {
	return m.ResolveFunc(tool)
}

func TestRunToolWithInstaller(t *testing.T) {
	tests := []struct {
		name             string
		tool             string
		version          string
		args             []string
		resolveResult    [3]interface{} // owner, repo, error
		binaryPathResult [2]interface{} // path, error
		cmdRunError      error
		expectedError    bool
	}{
		{
			name:             "Successful execution with valid binary path",
			tool:             "hashicorp/terraform",
			version:          "v1.0.0",
			args:             []string{"--flag"},
			resolveResult:    [3]interface{}{"hashicorp", "terraform", nil},
			binaryPathResult: [2]interface{}{"true", nil}, // Use "true" command for testing
			expectedError:    false,
		},
		{
			name:             "Error on failed resolution",
			tool:             "invalid/tool",
			version:          "v1.0.0",
			args:             []string{},
			resolveResult:    [3]interface{}{"", "", errors.New("invalid tool name")},
			binaryPathResult: [2]interface{}{"", nil},
			expectedError:    true,
		},
		{
			name:             "Error on binary path not found",
			tool:             "hashicorp/terraform",
			version:          "v1.0.0",
			args:             []string{},
			resolveResult:    [3]interface{}{"hashicorp", "terraform", nil},
			binaryPathResult: [2]interface{}{"", errors.New("binary not found")},
			expectedError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockRunner := &MockToolRunner{
				GetResolverFunc: func() ToolResolver {
					return &MockToolResolver{
						ResolveFunc: func(tool string) (string, string, error) {
							// Type-assert the error value
							var err error
							if tt.resolveResult[2] != nil {
								err = tt.resolveResult[2].(error)
							}
							return tt.resolveResult[0].(string), tt.resolveResult[1].(string), err
						},
					}
				},
				FindBinaryPathFunc: func(owner, repo, version string) (string, error) {
					// Type-assert the error value
					var err error
					if tt.binaryPathResult[1] != nil {
						err = tt.binaryPathResult[1].(error)
					}
					return tt.binaryPathResult[0].(string), err
				},
				CreateLatestFileFunc: func(owner, repo, version string) error {
					return nil
				},
				ReadLatestFileFunc: func(owner, repo string) (string, error) {
					return "", nil
				},
			}

			// Mock exec.Command
			originalExecCommand := execCommand
			execCommand = func(name string, arg ...string) *exec.Cmd {
				if tt.cmdRunError == nil && name == "true" {
					return exec.Command("true") // Mock successful command
				}
				return exec.Command("false") // Mock failed command
			}
			defer func() { execCommand = originalExecCommand }()
			SetAtmosConfig(&schema.AtmosConfiguration{
				Toolchain: schema.Toolchain{},
			})
			// Run the function
			err := RunToolWithInstaller(mockRunner, tt.tool, tt.version, tt.args)

			// Check results
			if (err != nil) != tt.expectedError {
				t.Errorf("RunToolWithInstaller() error = %v, expectedError %v", err, tt.expectedError)
			}
		})
	}
}
