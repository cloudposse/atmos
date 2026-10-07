package installer

func detectScoop(d *detector) Installation {
	if d.system.GOOS() != windowsOS {
		return Installation{}
	}
	roots := []string{
		d.envRoot("SCOOP", d.homePath("scoop")),
		d.envRoot("SCOOP_GLOBAL", joinRoot(d.envRoot("ProgramData", "C:/ProgramData"), "scoop")),
	}
	for index, root := range roots {
		if d.versionedBinary(joinRoot(root, "apps", binaryName)) {
			result := d.installation(Scoop, "scoop")
			result.Global = index == 1
			return result
		}
	}
	return Installation{}
}
