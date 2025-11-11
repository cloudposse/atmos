package types

import "os"

// File represents a file in a configuration.
type File struct {
	Path        string      `yaml:"path"`
	Content     string      `yaml:"content"`
	IsTemplate  bool        `yaml:"is_template"`
	Permissions os.FileMode `yaml:"permissions"`
}

// Configuration represents a template configuration.
type Configuration struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	TemplateID  string `yaml:"template_id"`
	TargetDir   string `yaml:"target_dir"`
	Files       []File `yaml:"files"`
	README      string `yaml:"readme"`
}
