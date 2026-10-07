package installer

import (
	"path"
	"strings"
)

func detectAqua(d *detector) Installation {
	fallback := joinRoot(d.dataHome(), "aquaproj-aqua")
	if d.system.GOOS() == windowsOS {
		fallback = joinRoot(d.envRoot("XDG_DATA_HOME", d.system.Getenv("LOCALAPPDATA")), "aquaproj-aqua")
	}
	root := d.envRoot("AQUA_ROOT_DIR", fallback)
	root = joinRoot(root, "pkgs", "github_release", "github.com", "cloudposse", binaryName)
	rel, ok := d.relative(root)
	if ok && d.aquaBinary(rel) {
		return d.installation(Aqua, "aqua")
	}
	return Installation{}
}

func (d *detector) aquaBinary(relative string) bool {
	parts := strings.Split(relative, "/")
	if len(parts) < 3 {
		return false
	}
	if d.isBinary(path.Base(relative)) {
		return true
	}
	// Aqua preserves raw GitHub asset names, e.g. atmos_1.230.1_linux_amd64.
	// See aqua-registry/pkgs/cloudposse/atmos/registry.yaml (format: raw).
	name := path.Base(relative)
	if d.system.GOOS() == windowsOS {
		name = strings.TrimSuffix(name, ".exe")
	}
	asset := strings.Split(name, "_")
	if len(asset) != 4 || asset[0] != binaryName || asset[1] != strings.TrimPrefix(parts[0], "v") || asset[2] != d.system.GOOS() {
		return false
	}
	switch asset[3] {
	case "amd64", "arm64", "386", "arm":
		return true
	default:
		return false
	}
}
