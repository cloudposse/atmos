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
// - Starts with "./" or "../" (relative path)
// - Is an absolute path (starts with "/")
//
// Otherwise, it's treated as a component name (even if it contains slashes like "vpc/security-group").
func IsExplicitComponentPath(component string) bool {
	return component == "." ||
		strings.HasPrefix(component, "./") ||
		strings.HasPrefix(component, "../") ||
		filepath.IsAbs(component)
}
