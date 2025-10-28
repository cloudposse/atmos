package filemanager

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/toolchain"
)

// ToolVersionsFileManager manages .tool-versions file.
type ToolVersionsFileManager struct {
	config   *schema.AtmosConfiguration
	filePath string
}

// NewToolVersionsFileManager creates a new .tool-versions manager.
func NewToolVersionsFileManager(config *schema.AtmosConfiguration) *ToolVersionsFileManager {
	defer perf.Track(nil, "filemanager.NewToolVersionsFileManager")()

	// Prefer VersionsFile (new), fall back to FilePath (legacy compatibility)
	filePath := config.Toolchain.VersionsFile
	if filePath == "" {
		filePath = config.Toolchain.FilePath
	}
	if filePath == "" {
		filePath = ".tool-versions"
	}

	return &ToolVersionsFileManager{
		config:   config,
		filePath: filePath,
	}
}

// Enabled returns true if this file manager is enabled by configuration.
func (m *ToolVersionsFileManager) Enabled() bool {
	defer perf.Track(nil, "filemanager.ToolVersionsFileManager.Enabled")()

	// Check configuration flag
	return m.config.Toolchain.UseToolVersions
}

// AddTool adds or updates a tool version.
func (m *ToolVersionsFileManager) AddTool(ctx context.Context, tool, version string, opts ...AddOption) error {
	defer perf.Track(nil, "filemanager.ToolVersionsFileManager.AddTool")()

	if !m.Enabled() {
		return nil // Skip if disabled
	}

	cfg := &AddConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.AsDefault {
		return toolchain.AddToolToVersionsAsDefault(m.filePath, tool, version)
	}
	return toolchain.AddToolToVersions(m.filePath, tool, version)
}

// RemoveTool removes a tool version.
func (m *ToolVersionsFileManager) RemoveTool(ctx context.Context, tool, version string) error {
	defer perf.Track(nil, "filemanager.ToolVersionsFileManager.RemoveTool")()

	if !m.Enabled() {
		return nil
	}

	return toolchain.RemoveToolFromVersions(m.filePath, tool, version)
}

// SetDefault sets a tool version as default.
func (m *ToolVersionsFileManager) SetDefault(ctx context.Context, tool, version string) error {
	defer perf.Track(nil, "filemanager.ToolVersionsFileManager.SetDefault")()

	if !m.Enabled() {
		return nil
	}

	return toolchain.AddToolToVersionsAsDefault(m.filePath, tool, version)
}

// GetTools returns all tools managed by this file.
func (m *ToolVersionsFileManager) GetTools(ctx context.Context) (map[string][]string, error) {
	defer perf.Track(nil, "filemanager.ToolVersionsFileManager.GetTools")()

	if !m.Enabled() {
		return nil, nil
	}

	tv, err := toolchain.LoadToolVersions(m.filePath)
	if err != nil {
		return nil, err
	}
	return tv.Tools, nil
}

// Verify verifies the integrity of the managed file.
func (m *ToolVersionsFileManager) Verify(ctx context.Context) error {
	defer perf.Track(nil, "filemanager.ToolVersionsFileManager.Verify")()

	if !m.Enabled() {
		return nil
	}

	// .tool-versions doesn't have verification (no checksums)
	return nil
}

// Name returns the manager name for logging.
func (m *ToolVersionsFileManager) Name() string {
	defer perf.Track(nil, "filemanager.ToolVersionsFileManager.Name")()

	return "tool-versions"
}
