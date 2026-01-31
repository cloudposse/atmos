package memory

import (
	"time"
)

// ProjectMemory represents the in-memory state of an ATMOS.md file.
type ProjectMemory struct {
	FilePath     string
	Content      string
	Sections     map[string]*Section
	LastModified time.Time
	Enabled      bool
}

// Section represents a parsed section from ATMOS.md.
type Section struct {
	Name    string
	Content string
	Order   int
}

// Config holds configuration for project memory.
type Config struct {
	Enabled      bool
	FilePath     string   // Path to ATMOS.md (relative to base path)
	AutoUpdate   bool     // Auto-update memory based on AI learnings
	Sections     []string // Sections to include in AI context
	CreateIfMiss bool     // Create ATMOS.md if it doesn't exist
}

// DefaultConfig returns the default memory configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:      false,
		FilePath:     "ATMOS.md",
		AutoUpdate:   false,
		CreateIfMiss: true,
		Sections: []string{
			"project_context",
			"common_commands",
			"stack_patterns",
			"frequent_issues",
			"infrastructure_patterns",
			"recent_learnings",
		},
	}
}

// SectionOrder defines the canonical order for sections in ATMOS.md.
var SectionOrder = map[string]int{
	"project_context":         1,
	"common_commands":         2,
	"stack_patterns":          3,
	"component_dependencies":  4,
	"naming_conventions":      5,
	"frequent_issues":         6,
	"infrastructure_patterns": 7,
	"component_catalog":       8,
	"team_conventions":        9,
	"recent_learnings":        10,
}

// SectionTitles maps section keys to display titles.
var SectionTitles = map[string]string{
	"project_context":         "Project Context",
	"common_commands":         "Common Commands",
	"stack_patterns":          "Stack Patterns",
	"component_dependencies":  "Component Dependencies",
	"naming_conventions":      "Naming Conventions",
	"frequent_issues":         "Frequent Issues & Solutions",
	"infrastructure_patterns": "Infrastructure Patterns",
	"component_catalog":       "Component Catalog Structure",
	"team_conventions":        "Team Conventions",
	"recent_learnings":        "Recent Learnings",
}
