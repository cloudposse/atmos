package filetype

import (
	"path/filepath"
	"strings"
)

// ParseFileByExtension parses a file based on its file extension.
// It determines the format from the extension, not the content.
// Supported extensions:
// - .json → JSON parsing
// - .yaml, .yml → YAML parsing
// - .hcl, .tf, .tfvars → HCL parsing
// - All others (including .txt or no extension) → raw string.
func ParseFileByExtension(readFileFunc func(string) ([]byte, error), filename string) (any, error) {
	// Extract clean filename from potential URL
	cleanFilename := ExtractFilenameFromPath(filename)
	ext := GetFileExtension(cleanFilename)

	// Read the file content
	data, err := readFileFunc(filename)
	if err != nil {
		return nil, err
	}

	// Parse based on extension
	return ParseByExtension(data, ext, filename)
}

// ParseFileRaw always returns the file content as a raw string,
// regardless of the file extension or content.
func ParseFileRaw(readFileFunc func(string) ([]byte, error), filename string) (any, error) {
	data, err := readFileFunc(filename)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// ParseByExtension parses data based on the provided extension.
func ParseByExtension(data []byte, ext string, filename string) (any, error) {
	switch ext {
	case ".json":
		return parseJSON(data)
	case ".yaml", ".yml":
		return parseYAML(data)
	case ".hcl", ".tf", ".tfvars":
		return parseHCL(data, filename)
	default:
		// Return as raw string for unknown extensions
		return string(data), nil
	}
}

// ExtractFilenameFromPath extracts the actual filename from a path or URL.
// It removes query strings and fragments from URLs.
// Examples:
//   - "https://example.com/file.json?v=1#section" → "file.json"
//   - "/path/to/file.yaml" → "file.yaml"
//   - "file.txt" → "file.txt"
func ExtractFilenameFromPath(path string) string {
	// Remove fragment (everything after #)
	if idx := strings.Index(path, "#"); idx != -1 {
		path = path[:idx]
	}

	// Remove query string (everything after ?)
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	// Extract the base filename
	return filepath.Base(path)
}

// GetFileExtension returns the lowercase file extension including the dot.
// Examples:
//   - "file.json" → ".json"
//   - "FILE.JSON" → ".json"
//   - "file.backup.json" → ".json"
//   - "file" → ""
//   - ".hidden" → ""
func GetFileExtension(filename string) string {
	// Handle special cases
	if filename == "" || filename == "." {
		return ""
	}

	ext := filepath.Ext(filename)

	// If the extension is the whole filename (e.g., ".env"), check if it looks like a known extension
	if ext == filename {
		// Check if it's actually an extension (has letters after the dot)
		if len(ext) > 1 && strings.Contains(ext[1:], ".") == false {
			// It looks like an extension file (e.g., ".json", ".yaml")
			// Check if it's a known extension
			lowerExt := strings.ToLower(ext)
			knownExts := []string{".json", ".yaml", ".yml", ".hcl", ".tf", ".tfvars", ".txt", ".md"}
			for _, known := range knownExts {
				if lowerExt == known {
					return lowerExt
				}
			}
		}
		// Otherwise it's a hidden file without an extension (e.g., ".env", ".gitignore")
		return ""
	}

	// If filename ends with a dot, there's no extension
	if ext == "." {
		return ""
	}

	return strings.ToLower(ext)
}
