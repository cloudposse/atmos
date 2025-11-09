package component

import (
	"path/filepath"
	"strings"
)

// IsExplicitComponentPath determines if a component argument represents an explicit filesystem path.
// Returns true if the argument should be treated as a filesystem path rather than a component name.
//
// An argument is considered an explicit path if it:
// - Is "." (current directory)
// - Starts with "./" or "../" (Unix-style relative path)
// - Starts with ".\\" or "..\\" (Windows-style relative path)
// - Is an absolute path (filepath.IsAbs handles both Unix and Windows)
//
// Otherwise, it's treated as a component name (even if it contains slashes like "vpc/security-group").
func IsExplicitComponentPath(component string) bool {
	return component == "." ||
		strings.HasPrefix(component, "./") ||
		strings.HasPrefix(component, "../") ||
		strings.HasPrefix(component, ".\\") ||
		strings.HasPrefix(component, "..\\") ||
		filepath.IsAbs(component)
}
