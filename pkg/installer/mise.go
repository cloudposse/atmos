package installer

func detectMise(d *detector) Installation {
	fallback := joinRoot(d.dataHome(), "mise")
	if d.system.GOOS() == windowsOS {
		fallback = joinRoot(d.envRoot("XDG_DATA_HOME", d.system.Getenv("LOCALAPPDATA")), "mise")
	}
	root := d.envRoot("MISE_DATA_DIR", fallback)
	root = d.envRoot("MISE_INSTALLS_DIR", joinRoot(root, "installs"))
	if d.versionedBinary(joinRoot(root, binaryName)) {
		return d.installation(Mise, "mise")
	}
	return Installation{}
}
