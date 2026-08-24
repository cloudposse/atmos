package instructions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// Manager manages project instructions (ATMOS.md) lifecycle.
type Manager struct {
	config   *Config
	basePath string
	memory   *ProjectInstructions
}

// NewManager creates a new project instructions manager.
func NewManager(basePath string, config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	return &Manager{
		config:   config,
		basePath: basePath,
	}
}

// Load loads the ATMOS.md file from disk and parses it.
// If the file does not exist, it returns nil without error.
func (m *Manager) Load(ctx context.Context) (*ProjectInstructions, error) {
	if !m.config.Enabled {
		return nil, nil
	}

	filePath := m.getFilePath()

	// Check if file exists.
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debugf("ATMOS.md not found at %s, skipping project instructions", filePath)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to stat ATMOS.md: %w", err)
	}

	// Read file contents.
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read ATMOS.md: %w", err)
	}

	// Parse sections.
	sections, err := ParseSections(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ATMOS.md: %w", err)
	}

	m.memory = &ProjectInstructions{
		FilePath:     filePath,
		Content:      string(content),
		Sections:     sections,
		LastModified: info.ModTime(),
		Enabled:      true,
	}

	log.Debugf("Loaded project instructions from %s (%d sections)", filePath, len(sections))

	return m.memory, nil
}

// GetContext returns the full ATMOS.md content formatted for AI context.
func (m *Manager) GetContext() string {
	if m.memory == nil || !m.config.Enabled {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("# Project Instructions (ATMOS.md)\n\n")
	sb.WriteString(m.memory.Content)

	return sb.String()
}

// Reload reloads the instructions from disk if the file has been modified.
func (m *Manager) Reload(ctx context.Context) error {
	if m.memory == nil {
		_, err := m.Load(ctx)
		return err
	}

	// Check if file has been modified.
	info, err := os.Stat(m.memory.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File was deleted, clear memory.
			m.memory = nil
			return nil
		}
		return fmt.Errorf("failed to stat ATMOS.md: %w", err)
	}

	if info.ModTime().After(m.memory.LastModified) {
		log.Debug("ATMOS.md has been modified, reloading")
		_, err := m.Load(ctx)
		return err
	}

	return nil
}

// getFilePath returns the absolute path to the ATMOS.md file.
func (m *Manager) getFilePath() string {
	if filepath.IsAbs(m.config.FilePath) {
		return m.config.FilePath
	}
	return filepath.Join(m.basePath, m.config.FilePath)
}
