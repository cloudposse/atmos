package installer

// detectNative matches Atmos-managed versions under default and caller-supplied roots.
func detectNative(d *detector) Installation {
	cache := d.homePath(".cache")
	if d.system.GOOS() == windowsOS {
		cache = joinRoot(d.system.Getenv("LOCALAPPDATA"), "cache")
	}
	cache = d.envRoot("ATMOS_XDG_CACHE_HOME", d.envRoot("XDG_CACHE_HOME", cache))
	roots := []string{d.homePath(".atmos"), joinRoot(cache, binaryName, "toolchain"), ".tools"}
	for _, root := range append(roots, d.nativeRoots...) {
		if d.versionedBinary(joinRoot(root, "bin", "cloudposse", binaryName)) {
			return d.installation(Native, binaryName)
		}
	}
	return Installation{}
}
