package installer

func detectASDF(d *detector) Installation {
	if d.system.GOOS() == windowsOS {
		return Installation{}
	}
	root := d.envRoot("ASDF_DATA_DIR", d.homePath(".asdf"))
	if d.versionedBinary(joinRoot(root, "installs", binaryName)) {
		return d.installation(ASDF, "asdf")
	}
	return Installation{}
}
