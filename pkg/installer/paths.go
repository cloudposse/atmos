package installer

import (
	"path"
	"strings"
)

// Normalize according to the target OS so Windows detection can also be tested on Unix.
func (d *detector) normalize(p string) string {
	if d.system.GOOS() == windowsOS {
		p = strings.ToLower(strings.ReplaceAll(p, `\`, "/"))
	}
	return path.Clean(p)
}

// root resolves an installation root to an absolute, normalized path when possible.
func (d *detector) root(p string) string {
	if p == "" {
		return ""
	}
	abs, err := d.system.Abs(p)
	if err != nil {
		return ""
	}
	if resolved, resolveErr := d.system.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	}
	return d.normalize(abs)
}

// relative checks containment using a directory boundary after resolving the root.
func (d *detector) relative(root string) (string, bool) {
	root = d.root(root)
	if root == "" || root == "/" || root == "." {
		return "", false
	}
	prefix := strings.TrimSuffix(root, "/") + "/"
	p := d.normalize(d.executable)
	return strings.TrimPrefix(p, prefix), strings.HasPrefix(p, prefix)
}

// envRoot selects an explicit manager root before its platform fallback.
func (d *detector) envRoot(key, fallback string) string {
	if value := d.system.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// homePath builds a user-relative root, leaving it unset when no home directory is known.
func (d *detector) homePath(parts ...string) string {
	if d.home == "" {
		return ""
	}
	return joinRoot(d.normalize(d.home), parts...)
}

// joinRoot joins target-platform path components without inventing a missing root.
func joinRoot(root string, parts ...string) string {
	if root == "" {
		return ""
	}
	//nolint:forbidigo // Match normalized target-OS paths, including Windows fixtures on Unix hosts.
	return path.Join(append([]string{strings.ReplaceAll(root, `\`, "/")}, parts...)...)
}

// dataHome selects the XDG data directory or its conventional user-local default.
func (d *detector) dataHome() string {
	return d.envRoot("XDG_DATA_HOME", d.homePath(".local", "share"))
}

// versionedBinary accepts a binary directly under a version or its bin directory.
func (d *detector) versionedBinary(root string) bool {
	rel, ok := d.relative(root)
	if !ok {
		return false
	}
	parts := strings.Split(rel, "/")
	// Managers may place the executable directly in the version directory or in bin/.
	if len(parts) == 3 && parts[1] == "bin" {
		parts = []string{parts[0], parts[2]}
	}
	return len(parts) == 2 && parts[0] != "" && d.isBinary(parts[1])
}

// isBinary checks the Atmos executable name for the detected platform.
func (d *detector) isBinary(name string) bool {
	return name == binaryName || (d.system.GOOS() == windowsOS && name == "atmos.exe")
}
