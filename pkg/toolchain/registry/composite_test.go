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

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "test", Registry: &mockRegistry{tools: map[string]*Tool{}}, Priority: 100},
	})

	// LoadLocalConfig is a no-op, should return nil.
	err := composite.LoadLocalConfig("/some/path")
	if err != nil {
		t.Errorf("LoadLocalConfig should return nil, got: %v", err)
	}
}

// searchableMockRegistry is a mock that returns search and list results.
type searchableMockRegistry struct {
	mockRegistry
	searchResults []*Tool
	listResults   []*Tool
	searchErr     error
	listErr       error
	metadataErr   error
}

func (m *searchableMockRegistry) Search(_ context.Context, _ string, _ ...SearchOption) ([]*Tool, error) {
	defer perf.Track(nil, "searchableMockRegistry.Search")()

	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.searchResults, nil
}

func (m *searchableMockRegistry) ListAll(_ context.Context, _ ...ListOption) ([]*Tool, error) {
	defer perf.Track(nil, "searchableMockRegistry.ListAll")()

	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listResults, nil
}

func (m *searchableMockRegistry) GetMetadata(_ context.Context) (*RegistryMetadata, error) {
	defer perf.Track(nil, "searchableMockRegistry.GetMetadata")()

	if m.metadataErr != nil {
		return nil, m.metadataErr
	}
	return &RegistryMetadata{
		Name:      "searchable-mock",
		Type:      "mock",
		Source:    "mock://test",
		ToolCount: len(m.tools),
	}, nil
}

func TestCompositeRegistry_Search(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_Search")()

	reg1 := &searchableMockRegistry{
		mockRegistry: mockRegistry{tools: map[string]*Tool{}},
		searchResults: []*Tool{
			{Name: "terraform", RepoOwner: "hashicorp", RepoName: "terraform"},
		},
	}
	reg2 := &searchableMockRegistry{
		mockRegistry: mockRegistry{tools: map[string]*Tool{}},
		searchResults: []*Tool{
			{Name: "terraform-dup", RepoOwner: "hashicorp", RepoName: "terraform"}, // same owner/repo.
			{Name: "packer", RepoOwner: "hashicorp", RepoName: "packer"},
		},
	}

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "high", Registry: reg1, Priority: 100},
		{Name: "low", Registry: reg2, Priority: 10},
	})

	results, err := composite.Search(context.Background(), "hashicorp")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should have 2 results: terraform (from high-priority) and packer (from low).
	// hashicorp/terraform deduplicates to the first one found (high priority).
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestCompositeRegistry_Search_ErrorContinues(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_Search_ErrorContinues")()

	reg1 := &searchableMockRegistry{
		mockRegistry: mockRegistry{tools: map[string]*Tool{}},
		searchErr:    errors.New("search failed"),
	}
	reg2 := &searchableMockRegistry{
		mockRegistry: mockRegistry{tools: map[string]*Tool{}},
		searchResults: []*Tool{
			{Name: "terraform", RepoOwner: "hashicorp", RepoName: "terraform"},
		},
	}

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "broken", Registry: reg1, Priority: 100},
		{Name: "working", Registry: reg2, Priority: 10},
	})

	results, err := composite.Search(context.Background(), "terraform")
	if err != nil {
		t.Fatalf("Search should not fail: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result from working registry, got %d", len(results))
	}
}

func TestCompositeRegistry_ListAll(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_ListAll")()

	reg := &searchableMockRegistry{
		mockRegistry: mockRegistry{tools: map[string]*Tool{}},
		listResults: []*Tool{
			{Name: "tool1", RepoOwner: "owner1", RepoName: "repo1"},
			{Name: "tool2", RepoOwner: "owner2", RepoName: "repo2"},
		},
	}

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "test", Registry: reg, Priority: 100},
	})

	results, err := composite.ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestCompositeRegistry_ListAll_ErrorContinues(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_ListAll_ErrorContinues")()

	reg1 := &searchableMockRegistry{
		mockRegistry: mockRegistry{tools: map[string]*Tool{}},
		listErr:      errors.New("list failed"),
	}
	reg2 := &searchableMockRegistry{
		mockRegistry: mockRegistry{tools: map[string]*Tool{}},
		listResults: []*Tool{
			{Name: "tool", RepoOwner: "owner", RepoName: "repo"},
		},
	}

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "broken", Registry: reg1, Priority: 100},
		{Name: "working", Registry: reg2, Priority: 10},
	})

	results, err := composite.ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll should not fail: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestCompositeRegistry_GetMetadata(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_GetMetadata")()

	reg1 := &searchableMockRegistry{
		mockRegistry: mockRegistry{tools: map[string]*Tool{
			"a/b": {Name: "tool1"},
			"c/d": {Name: "tool2"},
		}},
	}
	reg2 := &searchableMockRegistry{
		mockRegistry: mockRegistry{tools: map[string]*Tool{
			"e/f": {Name: "tool3"},
		}},
	}

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "reg1", Registry: reg1, Priority: 100},
		{Name: "reg2", Registry: reg2, Priority: 50},
	})

	meta, err := composite.GetMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if meta.Name != "composite" {
		t.Errorf("Expected name 'composite', got %s", meta.Name)
	}
	if meta.ToolCount != 3 {
		t.Errorf("Expected 3 tools, got %d", meta.ToolCount)
	}
}

func TestCompositeRegistry_GetMetadata_ErrorContinues(t *testing.T) {
	defer perf.Track(nil, "registry.TestCompositeRegistry_GetMetadata_ErrorContinues")()

	reg1 := &searchableMockRegistry{
		mockRegistry: mockRegistry{tools: map[string]*Tool{}},
		metadataErr:  errors.New("metadata failed"),
	}
	reg2 := &searchableMockRegistry{
		mockRegistry: mockRegistry{tools: map[string]*Tool{
			"a/b": {Name: "tool"},
		}},
	}

	composite := NewCompositeRegistry([]PrioritizedRegistry{
		{Name: "broken", Registry: reg1, Priority: 100},
		{Name: "working", Registry: reg2, Priority: 50},
	})

	meta, err := composite.GetMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetMetadata should not fail: %v", err)
	}
	// Only counts from the working registry.
	if meta.ToolCount != 1 {
		t.Errorf("Expected 1 tool from working registry, got %d", meta.ToolCount)
	}
}
