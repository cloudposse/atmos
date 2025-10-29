package prompts

import (
	"embed"
	"fmt"
	"io/fs"
)

// Prompts embeds all agent prompt files.
//
//go:embed *.md
var Prompts embed.FS

// Read reads a prompt file from the embedded filesystem.
func Read(filename string) (string, error) {
	content, err := fs.ReadFile(Prompts, filename)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded prompt file %q: %w", filename, err)
	}
	return string(content), nil
}

// List returns all available prompt filenames.
func List() ([]string, error) {
	var files []string

	err := fs.WalkDir(Prompts, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-.md files.
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}

		// Only include .md files (not README.md for now).
		if len(path) > 3 && path[len(path)-3:] == ".md" && path != "README.md" {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list embedded prompts: %w", err)
	}

	return files, nil
}
