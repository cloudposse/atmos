package installer

import "strings"

const nixHashLength = 32

// detectNix identifies Atmos store paths without inferring their controlling Nix configuration.
func detectNix(d *detector) Installation {
	if d.system.GOOS() == windowsOS {
		return Installation{}
	}
	rel, ok := d.relative(d.envRoot("NIX_STORE_DIR", "/nix/store"))
	parts := strings.Split(rel, "/")
	if !ok || len(parts) != 3 || parts[1] != "bin" || parts[2] != binaryName {
		return Installation{}
	}
	hash, name, found := strings.Cut(parts[0], "-")
	if found && len(hash) == nixHashLength && strings.HasPrefix(name, "atmos-") {
		return d.installation(Nix, "nix")
	}
	return Installation{}
}
