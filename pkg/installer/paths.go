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

func (d *detector) relative(root string) (string, bool) {
	root = d.root(root)
	if root == "" || root == "/" || root == "." {
		return "", false
	}
	prefix := strings.TrimSuffix(root, "/") + "/"
	p := d.normalize(d.executable)
	return strings.TrimPrefix(p, prefix), strings.HasPrefix(p, prefix)
}

func (d *detector) envRoot(key, fallback string) string {
	if value := d.system.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (d *detector) homePath(parts ...string) string {
	if d.home == "" {
		return ""
	}
	return joinRoot(d.normalize(d.home), parts...)
}

func joinRoot(root string, parts ...string) string {
	if root == "" {
		return ""
	}
	//nolint:forbidigo // Match normalized target-OS paths, including Windows fixtures on Unix hosts.
	return path.Join(append([]string{strings.ReplaceAll(root, `\`, "/")}, parts...)...)
}

func (d *detector) dataHome() string {
	return d.envRoot("XDG_DATA_HOME", d.homePath(".local", "share"))
}

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

func (d *detector) isBinary(name string) bool {
	return name == binaryName || (d.system.GOOS() == windowsOS && name == "atmos.exe")
}
