package list

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Mock interfaces for testing
type MockAtmosProAPIClientInterface struct {
	mock.Mock
}

func (m *MockAtmosProAPIClientInterface) UploadDeployments(req *dtos.DeploymentsUploadRequest) error {
	args := m.Called(req)
	return args.Error(0)
}

func (m *MockAtmosProAPIClientInterface) UploadDeploymentStatus(req *dtos.DeploymentStatusUploadRequest) error {
	args := m.Called(req)
	return args.Error(0)
}

// Test formatDeployments function
func TestFormatDeployments(t *testing.T) {
	deployments := []schema.Deployment{
		{Component: "vpc", Stack: "stack1"},
		{Component: "app", Stack: "stack2"},
		{Component: "db", Stack: "stack1"},
	}

	t.Run("TTY mode", func(t *testing.T) {
		// Mock TTY environment
		originalStdout := os.Stdout
		defer func() { os.Stdout = originalStdout }()

		// Create a pipe to simulate TTY
		_, w, _ := os.Pipe()
		os.Stdout = w

		output := formatDeployments(deployments)

		w.Close()
		os.Stdout = originalStdout

		// Should return styled table format
		assert.Contains(t, output, "Component")
		assert.Contains(t, output, "Stack")
		assert.Contains(t, output, "vpc")
		assert.Contains(t, output, "app")
		assert.Contains(t, output, "db")
	})

	t.Run("non-TTY mode", func(t *testing.T) {
		// Mock non-TTY environment by redirecting stdout
		originalStdout := os.Stdout
		defer func() { os.Stdout = originalStdout }()

		r, w, _ := os.Pipe()
		os.Stdout = w

		output := formatDeployments(deployments)

		w.Close()
		os.Stdout = originalStdout

		// Read the output
		buf := make([]byte, 1024)
		n, _ := r.Read(buf)
		csvOutput := string(buf[:n])

		// Should return CSV format
		expectedCSV := "Component,Stack\nvpc,stack1\napp,stack2\ndb,stack1\n"
		assert.Equal(t, expectedCSV, csvOutput)
		assert.Equal(t, expectedCSV, output)
	})

	t.Run("empty deployments", func(t *testing.T) {
		originalStdout := os.Stdout
		defer func() { os.Stdout = originalStdout }()

		r, w, _ := os.Pipe()
		os.Stdout = w

		output := formatDeployments([]schema.Deployment{})

		w.Close()
		os.Stdout = originalStdout

		buf := make([]byte, 1024)
		n, _ := r.Read(buf)
		csvOutput := string(buf[:n])

		expectedCSV := "Component,Stack\n"
		assert.Equal(t, expectedCSV, csvOutput)
		assert.Equal(t, expectedCSV, output)
	})
}

// Test processComponentConfig edge cases
func TestProcessComponentConfig(t *testing.T) {
	t.Run("invalid component config type", func(t *testing.T) {
		result := processComponentConfig("stack1", "comp1", "terraform", "invalid")
		assert.Nil(t, result)
	})

	t.Run("valid component config", func(t *testing.T) {
		config := map[string]any{
			"settings": map[string]any{"pro": map[string]any{"enabled": true}},
			"vars":     map[string]any{"key": "value"},
		}
		result := processComponentConfig("stack1", "comp1", "terraform", config)
		assert.NotNil(t, result)
		assert.Equal(t, "comp1", result.Component)
		assert.Equal(t, "stack1", result.Stack)
		assert.Equal(t, "terraform", result.ComponentType)
	})
}

// Test processComponentType edge cases
func TestProcessComponentType(t *testing.T) {
	t.Run("invalid type components", func(t *testing.T) {
		result := processComponentType("stack1", "terraform", "invalid")
		assert.Nil(t, result)
	})

	t.Run("empty type components", func(t *testing.T) {
		result := processComponentType("stack1", "terraform", map[string]any{})
		assert.Empty(t, result)
	})
}

// Test processStackComponents edge cases
func TestProcessStackComponents(t *testing.T) {
	t.Run("invalid stack config", func(t *testing.T) {
		result := processStackComponents("stack1", "invalid")
		assert.Nil(t, result)
	})

	t.Run("stack config without components", func(t *testing.T) {
		config := map[string]any{"other": "value"}
		result := processStackComponents("stack1", config)
		assert.Nil(t, result)
	})

	t.Run("invalid components type", func(t *testing.T) {
		config := map[string]any{"components": "invalid"}
		result := processStackComponents("stack1", config)
		assert.Nil(t, result)
	})
}

// Test createDeployment edge cases
func TestCreateDeployment(t *testing.T) {
	t.Run("abstract component should be filtered", func(t *testing.T) {
		config := map[string]any{
			"metadata": map[string]any{"type": "abstract"},
		}
		result := createDeployment("stack1", "comp1", "terraform", config)
		assert.Nil(t, result)
	})

	t.Run("component with all sections", func(t *testing.T) {
		config := map[string]any{
			"settings": map[string]any{"key": "value"},
			"vars":     map[string]any{"var": "value"},
			"env":      map[string]any{"env": "value"},
			"backend":  map[string]any{"backend": "value"},
			"metadata": map[string]any{"meta": "value"},
		}
		result := createDeployment("stack1", "comp1", "terraform", config)
		assert.NotNil(t, result)
		assert.Equal(t, "comp1", result.Component)
		assert.Equal(t, "stack1", result.Stack)
		assert.Equal(t, "terraform", result.ComponentType)
		assert.Equal(t, map[string]any{"key": "value"}, result.Settings)
		assert.Equal(t, map[string]any{"var": "value"}, result.Vars)
		assert.Equal(t, map[string]any{"env": "value"}, result.Env)
		assert.Equal(t, map[string]any{"backend": "value"}, result.Backend)
		assert.Equal(t, map[string]any{"meta": "value"}, result.Metadata)
	})

	t.Run("component with invalid section types", func(t *testing.T) {
		config := map[string]any{
			"settings": "invalid",
			"vars":     "invalid",
			"env":      "invalid",
			"backend":  "invalid",
			"metadata": "invalid",
		}
		result := createDeployment("stack1", "comp1", "terraform", config)
		assert.NotNil(t, result)
		// Should have empty maps for invalid sections
		assert.Empty(t, result.Settings)
		assert.Empty(t, result.Vars)
		assert.Empty(t, result.Env)
		assert.Empty(t, result.Backend)
		assert.Empty(t, result.Metadata)
	})
}

// Test sortDeployments
func TestSortDeployments(t *testing.T) {
	t.Run("empty deployments", func(t *testing.T) {
		result := sortDeployments([]schema.Deployment{})
		assert.Empty(t, result)
	})

	t.Run("single deployment", func(t *testing.T) {
		deployments := []schema.Deployment{
			{Component: "vpc", Stack: "stack1"},
		}
		result := sortDeployments(deployments)
		assert.Len(t, result, 1)
		assert.Equal(t, "vpc", result[0].Component)
	})

	t.Run("multiple deployments with same stack", func(t *testing.T) {
		deployments := []schema.Deployment{
			{Component: "db", Stack: "stack1"},
			{Component: "vpc", Stack: "stack1"},
			{Component: "app", Stack: "stack1"},
		}
		result := sortDeployments(deployments)
		assert.Len(t, result, 3)
		// Should be sorted by component name
		assert.Equal(t, "app", result[0].Component)
		assert.Equal(t, "db", result[1].Component)
		assert.Equal(t, "vpc", result[2].Component)
	})

	t.Run("multiple deployments with different stacks", func(t *testing.T) {
		deployments := []schema.Deployment{
			{Component: "vpc", Stack: "stack2"},
			{Component: "app", Stack: "stack1"},
			{Component: "db", Stack: "stack1"},
		}
		result := sortDeployments(deployments)
		assert.Len(t, result, 3)
		// Should be sorted by stack first, then component
		assert.Equal(t, "app", result[0].Component)
		assert.Equal(t, "stack1", result[0].Stack)
		assert.Equal(t, "db", result[1].Component)
		assert.Equal(t, "stack1", result[1].Stack)
		assert.Equal(t, "vpc", result[2].Component)
		assert.Equal(t, "stack2", result[2].Stack)
	})
}

// Test filterProEnabledDeployments edge cases
func TestFilterProEnabledDeploymentsEdgeCases(t *testing.T) {
	t.Run("deployments with invalid pro settings", func(t *testing.T) {
		deployments := []schema.Deployment{
			{
				Component: "vpc",
				Stack:     "stack1",
				Settings: map[string]interface{}{
					"pro": "invalid", // Not a map
				},
			},
			{
				Component: "app",
				Stack:     "stack1",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"enabled": "invalid", // Not a bool
					},
				},
			},
		}

		filtered := filterProEnabledDeployments(deployments)
		assert.Empty(t, filtered)
	})

	t.Run("deployments with missing pro settings", func(t *testing.T) {
		deployments := []schema.Deployment{
			{
				Component: "vpc",
				Stack:     "stack1",
				Settings:  map[string]interface{}{},
			},
			{
				Component: "app",
				Stack:     "stack1",
				Settings: map[string]interface{}{
					"other": "value",
				},
			},
		}

		filtered := filterProEnabledDeployments(deployments)
		assert.Empty(t, filtered)
	})
}

// Test collectDeployments edge cases
func TestCollectDeployments(t *testing.T) {
	t.Run("empty stacks map", func(t *testing.T) {
		result := collectDeployments(map[string]interface{}{})
		assert.Empty(t, result)
	})

	t.Run("stacks with invalid stack configs", func(t *testing.T) {
		stacks := map[string]interface{}{
			"stack1": "invalid",
			"stack2": map[string]interface{}{
				"components": "invalid",
			},
		}
		result := collectDeployments(stacks)
		assert.Empty(t, result)
	})
}
