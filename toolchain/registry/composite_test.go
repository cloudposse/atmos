package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudposse/atmos/pkg/perf"
)

// mockRegistry is a simple mock implementation for testing.
type mockRegistry struct {
	tools map[string]*Tool
	err   error
}

func (m *mockRegistry) GetTool(owner, repo string) (*Tool, error) {
	defer perf.Track(nil, "mockRegistry.GetTool")()

	if m.err != nil {
		return nil, m.err
	}

	key := owner + "/" + repo
	tool, exists := m.tools[key]
	if !exists {
		return nil, ErrToolNotFound
	}

	return tool, nil
}

func (m *mockRegistry) GetToolWithVersion(owner, repo, version string) (*Tool, error) {
	defer perf.Track(nil, "mockRegistry.GetToolWithVersion")()

	tool, err := m.GetTool(owner, repo)
	if err != nil {
		return nil, err
	}

	tool.Version = version
	return tool, nil
}

func (m *mockRegistry) GetLatestVersion(owner, repo string) (string, error) {
	defer perf.Track(nil, "mockRegistry.GetLatestVersion")()

	if m.err != nil {
		return "", m.err
	}

	return "v1.0.0", nil
}

func (m *mockRegistry) LoadLocalConfig(configPath string) error {
	defer perf.Track(nil, "mockRegistry.LoadLocalConfig")()

	return nil
}

func (m *mockRegistry) Search(ctx context.Context, query string, opts ...SearchOption) ([]*Tool, error) {
	defer perf.Track(nil, "mockRegistry.Search")()

	return []*Tool{}, nil
}

func (m *mockRegistry) ListAll(ctx context.Context, opts ...ListOption) ([]*Tool, error) {
	defer perf.Track(nil, "mockRegistry.ListAll")()

	return []*Tool{}, nil
}

func (m *mockRegistry) GetMetadata(ctx context.Context) (*RegistryMetadata, error) {
	defer perf.Track(nil, "mockRegistry.GetMetadata")()

	return &RegistryMetadata{
		Name:      "mock",
		Type:      "mock",
		Source:    "mock://test",
		Priority:  0,
		ToolCount: len(m.tools),
	}, nil
}

func TestNewCompositeRegistry(t *testing.T) {
	defer perf.Track(nil, "registry.TestNewCompositeRegistry")()

	registries := []PrioritizedRegistry{
		{Name: "high", Priority: 100},
		{Name: "low", Priority: 10},
		{Name: "medium", Priority: 50},
	}

	composite := NewCompositeRegistry(registries)

	// Check that registries are sorted by priority (descending)
	if composite.registries[0].Name != "high" {
		t.Errorf("Expected first registry to be 'high', got %s", composite.registries[0].Name)
	}
	if composite.registries[1].Name != "medium" {
		t.Errorf("Expected second registry to be 'medium', got %s", composite.registries[1].Name)
	}
	if composite.registries[2].Name != "low" {
		t.Errorf("Expected third registry to be 'low', got %s", composite.registries[2].Name)
	}
}

func TestCompositeRegistry_GetTool_Priority(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_GetTool_Priority")()

	// Create two mock registries with different tools.
	highPriority := &mockRegistry{
		tools: map[string]*Tool{
			"owner/repo": {Name: "high-tool", RepoOwner: "owner", RepoName: "repo"},
		},
	}

	lowPriority := &mockRegistry{
		tools: map[string]*Tool{
			"owner/repo": {Name: "low-tool", RepoOwner: "owner", RepoName: "repo"},
		},
	}

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "high", Registry: highPriority, Priority: 100},
		{Name: "low", Registry: lowPriority, Priority: 10},
	})

	tool, err := composite.GetTool("owner", "repo")
	if err != nil {
		t.Fatalf("GetTool failed: %v", err)
	}

	// Should return the tool from high-priority registry.
	if tool.Name != "high-tool" {
		t.Errorf("Expected tool from high-priority registry, got: %s", tool.Name)
	}
}

func TestCompositeRegistry_GetTool_Fallback(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_GetTool_Fallback")()

	// High-priority registry doesn't have the tool.
	highPriority := &mockRegistry{
		tools: map[string]*Tool{},
	}

	// Low-priority registry has the tool.
	lowPriority := &mockRegistry{
		tools: map[string]*Tool{
			"owner/repo": {Name: "fallback-tool", RepoOwner: "owner", RepoName: "repo"},
		},
	}

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "high", Registry: highPriority, Priority: 100},
		{Name: "low", Registry: lowPriority, Priority: 10},
	})

	tool, err := composite.GetTool("owner", "repo")
	if err != nil {
		t.Fatalf("GetTool failed: %v", err)
	}

	// Should fallback to low-priority registry.
	if tool.Name != "fallback-tool" {
		t.Errorf("Expected tool from fallback registry, got: %s", tool.Name)
	}
}

func TestCompositeRegistry_GetTool_LocalConfig(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_GetTool_LocalConfig")()

	t.Skip("Local config support was removed in refactoring")
}

func TestCompositeRegistry_GetTool_NotFound(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_GetTool_NotFound")()

	// Empty registry.
	registry := &mockRegistry{
		tools: map[string]*Tool{},
	}

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "test", Registry: registry, Priority: 100},
	})

	_, err := composite.GetTool("owner", "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent tool")
	}

	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("Expected ErrToolNotFound, got: %v", err)
	}
}

func TestCompositeRegistry_GetToolWithVersion(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_GetToolWithVersion")()

	registry := &mockRegistry{
		tools: map[string]*Tool{
			"owner/repo": {Name: "test-tool", RepoOwner: "owner", RepoName: "repo"},
		},
	}

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "test", Registry: registry, Priority: 100},
	})

	tool, err := composite.GetToolWithVersion("owner", "repo", "v1.2.3")
	if err != nil {
		t.Fatalf("GetToolWithVersion failed: %v", err)
	}

	if tool.Version != "v1.2.3" {
		t.Errorf("Expected version v1.2.3, got: %s", tool.Version)
	}
}

func TestCompositeRegistry_GetLatestVersion(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_GetLatestVersion")()

	registry := &mockRegistry{
		tools: map[string]*Tool{
			"owner/repo": {Name: "test-tool", RepoOwner: "owner", RepoName: "repo"},
		},
	}

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "test", Registry: registry, Priority: 100},
	})

	version, err := composite.GetLatestVersion("owner", "repo")
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}

	if version != "v1.0.0" {
		t.Errorf("Expected version v1.0.0, got: %s", version)
	}
}

func TestCompositeRegistry_LoadLocalConfig(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_LoadLocalConfig")()

	t.Skip("Local config support was removed in refactoring")
}
