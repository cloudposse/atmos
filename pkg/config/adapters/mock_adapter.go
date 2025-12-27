package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// mockConfigFilePerms is the file permission for mock config files.
const mockConfigFilePerms = 0o600

// MockAdapter generates synthetic YAML for testing.
// This enables unit tests without external dependencies.
//
// Usage:
//   - mock://empty        → Empty YAML config
//   - mock://error        → Returns error (test error handling)
//   - mock://nested       → Config with nested mock:// imports
//   - mock://custom/path  → Config with mock_path set to "custom/path"
//
// Tests can inject custom data via MockData field.
type MockAdapter struct {
	// MockData allows tests to inject custom YAML content.
	// Key is the path after "mock://", value is YAML content.
	MockData map[string]string
}

// globalMockAdapter is the singleton instance used for registration.
// Tests can replace MockData via SetMockData().
var globalMockAdapter = &MockAdapter{
	MockData: make(map[string]string),
}

// GetGlobalMockAdapter returns the global mock adapter instance.
// This is used by the init.go to register the singleton.
func GetGlobalMockAdapter() *MockAdapter {
	return globalMockAdapter
}

// SetMockData sets the mock data for the global mock adapter.
// This is intended for testing.
func SetMockData(data map[string]string) {
	globalMockAdapter.MockData = data
}

// ClearMockData clears all mock data.
// This is intended for testing cleanup.
func ClearMockData() {
	globalMockAdapter.MockData = make(map[string]string)
}

// Schemes returns the mock:// scheme.
func (m *MockAdapter) Schemes() []string {
	return []string{"mock://"}
}

// Resolve generates synthetic YAML based on the mock path.
//
//nolint:revive // argument-limit: matches ImportAdapter interface signature.
func (m *MockAdapter) Resolve(
	ctx context.Context,
	importPath string,
	basePath string,
	tempDir string,
	currentDepth int,
	maxDepth int,
) ([]config.ResolvedPaths, error) {
	defer perf.Track(nil, "adapters.MockAdapter.Resolve")()

	mockPath := strings.TrimPrefix(importPath, "mock://")

	switch mockPath {
	case "error":
		return nil, errUtils.ErrMockImportFailure

	case "empty":
		return m.writeConfig(importPath, tempDir, "# Empty mock configuration\n")

	case "nested":
		content := `# Nested mock configuration
import:
  - mock://component/base

vars:
  nested: true
  level: 1
`
		paths, err := m.writeConfig(importPath, tempDir, content)
		if err != nil {
			return nil, err
		}

		// Process nested imports.
		nestedPaths, err := config.ProcessImportsFromAdapter(basePath, []string{"mock://component/base"}, tempDir, currentDepth+1, maxDepth)
		if err != nil {
			return paths, nil // Return what we have, log error.
		}
		return append(paths, nestedPaths...), nil

	default:
		// Check for injected mock data.
		if m.MockData != nil {
			if content, ok := m.MockData[mockPath]; ok {
				return m.writeConfig(importPath, tempDir, content)
			}
		}

		// Generate default mock config.
		content := fmt.Sprintf(`# Mock configuration for %s
vars:
  mock_path: "%s"
  mock_source: "mock_adapter"
`, mockPath, mockPath)
		return m.writeConfig(importPath, tempDir, content)
	}
}

func (m *MockAdapter) writeConfig(importPath, tempDir, content string) ([]config.ResolvedPaths, error) {
	fileName := fmt.Sprintf("mock-import-%d.yaml", time.Now().UnixNano())
	filePath := filepath.Join(tempDir, fileName)

	if err := os.WriteFile(filePath, []byte(content), mockConfigFilePerms); err != nil {
		return nil, fmt.Errorf("failed to write mock config: %w", err)
	}

	return []config.ResolvedPaths{
		{
			FilePath:    filePath,
			ImportPaths: importPath,
			ImportType:  config.ADAPTER,
		},
	}, nil
}
