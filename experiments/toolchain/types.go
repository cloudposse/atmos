package main

// ToolRegistry represents the structure of a tool registry YAML file
type ToolRegistry struct {
	Tools []Tool `yaml:"tools"`
}

// Tool represents a single tool in the registry
type Tool struct {
	Name         string            `yaml:"name"`
	Registry     string            `yaml:"registry"`
	Version      string            `yaml:"version"`
	Type         string            `yaml:"type"`
	RepoOwner    string            `yaml:"repo_owner"`
	RepoName     string            `yaml:"repo_name"`
	Asset        string            `yaml:"asset"`
	Format       string            `yaml:"format"`
	Files        []File            `yaml:"files"`
	Overrides    []Override        `yaml:"overrides"`
	SupportedIf  *SupportedIf      `yaml:"supported_if"`
	Replacements map[string]string `yaml:"replacements"`
}

// File represents a file to be extracted from the archive
type File struct {
	Name string `yaml:"name"`
	Src  string `yaml:"src"`
}

// Override represents platform-specific overrides
type Override struct {
	GOOS   string `yaml:"goos"`
	GOARCH string `yaml:"goarch"`
	Asset  string `yaml:"asset"`
	Files  []File `yaml:"files"`
}

// SupportedIf represents conditions for when a tool is supported
type SupportedIf struct {
	GOOS   string `yaml:"goos"`
	GOARCH string `yaml:"goarch"`
}
