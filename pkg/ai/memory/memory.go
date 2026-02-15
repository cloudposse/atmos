package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	// FilePermissions defines the file permissions for ATMOS.md files (read/write for owner only).
	filePermissions = 0o600

	// DirPermissions defines the directory permissions for creating parent directories.
	dirPermissions = 0o755

	// CodeBlockMarker is the markdown code block delimiter.
	codeBlockMarker = "```"

	// InlineCodeMarker is the markdown inline code delimiter.
	inlineCodeMarker = "`"
)

// defaultTemplate is the default ATMOS.md template content.
const defaultTemplate = `# Atmos Project Memory

> This file is automatically maintained by Atmos AI to remember project-specific
> context, patterns, and conventions. Edit freely - AI will preserve manual changes.

## Project Context

**Organization:** your-org
**Atmos Version:** 1.89.0
**Primary Regions:** us-east-1, us-west-2
**Environments:** dev, staging, prod

**Stack Naming Pattern:**
` + codeBlockMarker + `
{org}-{tenant}-{environment}-{region}-{stage}
Example: acme-core-prod-use1-network
` + codeBlockMarker + `

## Common Commands

### Describe and Validate
` + codeBlockMarker + `bash
# Describe a component
atmos describe component <component> -s <stack>

# List all stacks
atmos list stacks

# Validate stacks
atmos validate stacks
` + codeBlockMarker + `

### Terraform Operations
` + codeBlockMarker + `bash
# Plan a component
atmos terraform plan <component> -s <stack>

# Apply a component
atmos terraform apply <component> -s <stack>
` + codeBlockMarker + `

## Stack Patterns

### Stack Structure
` + codeBlockMarker + `yaml
# Common import pattern
import:
  - catalog/stacks/baseline
  - catalog/stacks/network
` + codeBlockMarker + `

### CIDR Blocks
- dev: 10.0.0.0/16
- staging: 10.1.0.0/16
- prod: 10.2.0.0/16

## Frequent Issues & Solutions

### Stack not found error
**Problem:** ` + inlineCodeMarker + `Error: stack 'my-stack' not found` + inlineCodeMarker + `
**Solution:** Check stack naming and verify config exists in stacks/ directory

### YAML validation fails
**Problem:** ` + inlineCodeMarker + `invalid YAML: mapping values are not allowed` + inlineCodeMarker + `
**Solution:** Check indentation - YAML requires consistent 2-space indents

## Infrastructure Patterns

### Multi-Region Setup
- Primary region for production workloads
- DR region with replication
- Cross-region networking configured

### Component Dependencies
` + codeBlockMarker + `
vpc → subnets → security-groups → rds → eks
` + codeBlockMarker + `

## Component Catalog Structure

` + codeBlockMarker + `
components/
  terraform/
    vpc/           # VPC and networking
    rds/           # RDS databases
    eks/           # EKS clusters
    s3/            # S3 buckets
` + codeBlockMarker + `

## Team Conventions

- All infrastructure changes require PR review
- Use ` + inlineCodeMarker + `atmos validate` + inlineCodeMarker + ` before committing
- Tag all resources appropriately
- Document component changes

## Recent Learnings

*AI will add notes here as it learns about the project*

`

// Manager manages project memory (ATMOS.md) lifecycle.
type Manager struct {
	config   *Config
	basePath string
	memory   *ProjectMemory
}

// NewManager creates a new project memory manager.
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
func (m *Manager) Load(ctx context.Context) (*ProjectMemory, error) {
	if !m.config.Enabled {
		return nil, nil
	}

	filePath := m.getFilePath()

	// Check if file exists and create if needed.
	info, err := m.ensureFileExists(ctx, filePath)
	if err != nil {
		return nil, err
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

	m.memory = &ProjectMemory{
		FilePath:     filePath,
		Content:      string(content),
		Sections:     sections,
		LastModified: info.ModTime(),
		Enabled:      true,
	}

	log.Debug(fmt.Sprintf("Loaded project memory from %s (%d sections)", filePath, len(sections)))

	return m.memory, nil
}

// ensureFileExists checks if the file exists and creates it if needed.
func (m *Manager) ensureFileExists(ctx context.Context, filePath string) (os.FileInfo, error) {
	info, err := os.Stat(filePath)
	if err == nil {
		return info, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat ATMOS.md: %w", err)
	}

	// File doesn't exist - create default if configured.
	if !m.config.CreateIfMiss {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrAIProjectMemoryNotFound, filePath)
	}

	log.Debug(fmt.Sprintf("ATMOS.md not found, creating template at %s", filePath))
	if err := m.CreateDefault(ctx); err != nil {
		return nil, fmt.Errorf("failed to create default ATMOS.md: %w", err)
	}

	// Reload after creation.
	info, err = os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat ATMOS.md after creation: %w", err)
	}

	return info, nil
}

// GetContext returns the project memory content formatted for AI context.
func (m *Manager) GetContext() string {
	if m.memory == nil || !m.config.Enabled {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("# Project Memory (ATMOS.md)\n\n")
	sb.WriteString("The following information has been stored about this project:\n\n")

	// Include configured sections in order.
	for _, sectionKey := range m.config.Sections {
		if section, ok := m.memory.Sections[sectionKey]; ok && section.Content != "" {
			title := SectionTitles[sectionKey]
			if title == "" {
				title = section.Name
			}
			sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", title, section.Content))
		}
	}

	return sb.String()
}

// UpdateSection updates a specific section in memory and optionally writes to disk.
func (m *Manager) UpdateSection(ctx context.Context, sectionKey, content string, writeToDisk bool) error {
	if !m.config.Enabled || !m.config.AutoUpdate {
		return nil
	}

	if m.memory == nil {
		return errUtils.ErrAIProjectMemoryNotLoaded
	}

	// Update in-memory section.
	if _, ok := m.memory.Sections[sectionKey]; !ok {
		// Create new section.
		m.memory.Sections[sectionKey] = &Section{
			Name:    sectionKey,
			Content: content,
			Order:   SectionOrder[sectionKey],
		}
	} else {
		// Update existing section.
		m.memory.Sections[sectionKey].Content = content
	}

	// Write to disk if requested.
	if writeToDisk {
		if err := m.Save(ctx); err != nil {
			return fmt.Errorf("failed to save ATMOS.md: %w", err)
		}
		log.Debug(fmt.Sprintf("Updated section '%s' in ATMOS.md", sectionKey))
	}

	return nil
}

// Save writes the current memory state to disk.
func (m *Manager) Save(ctx context.Context) error {
	if m.memory == nil {
		return errUtils.ErrAIProjectMemoryNotLoaded
	}

	// Reconstruct markdown content from sections.
	content := m.reconstructMarkdown()

	// Write to file.
	if err := os.WriteFile(m.memory.FilePath, []byte(content), filePermissions); err != nil {
		return fmt.Errorf("failed to write ATMOS.md: %w", err)
	}

	// Update state.
	m.memory.Content = content
	m.memory.LastModified = time.Now()

	return nil
}

// CreateDefault creates a default ATMOS.md template file.
func (m *Manager) CreateDefault(ctx context.Context) error {
	filePath := m.getFilePath()

	// Ensure directory exists.
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create directory for ATMOS.md: %w", err)
	}

	// Get default template.
	template := GetDefaultTemplate()

	// Write template to file.
	if err := os.WriteFile(filePath, []byte(template), filePermissions); err != nil {
		return fmt.Errorf("failed to write default ATMOS.md: %w", err)
	}

	log.Info(fmt.Sprintf("Created default ATMOS.md at %s", filePath))

	return nil
}

// Reload reloads the memory from disk if it has been modified.
func (m *Manager) Reload(ctx context.Context) error {
	if m.memory == nil {
		_, err := m.Load(ctx)
		return err
	}

	// Check if file has been modified.
	info, err := os.Stat(m.memory.FilePath)
	if err != nil {
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

// reconstructMarkdown reconstructs the markdown content from parsed sections.
func (m *Manager) reconstructMarkdown() string {
	var sb strings.Builder

	sb.WriteString("# Atmos Project Memory\n\n")
	sb.WriteString("> This file is automatically maintained by Atmos AI to remember project-specific\n")
	sb.WriteString("> context, patterns, and conventions. Edit freely - AI will preserve manual changes.\n\n")

	// Write sections in canonical order.
	for _, sectionKey := range m.config.Sections {
		section, ok := m.memory.Sections[sectionKey]
		if !ok {
			continue
		}

		title := SectionTitles[sectionKey]
		if title == "" {
			title = section.Name
		}
		sb.WriteString(fmt.Sprintf("## %s\n\n", title))
		sb.WriteString(section.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// GetDefaultTemplate returns the default ATMOS.md template content.
func GetDefaultTemplate() string {
	return defaultTemplate
}
