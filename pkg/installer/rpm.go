package installer

import "strings"

func detectRPM(d *detector) Installation {
	if d.system.GOOS() != "linux" {
		return Installation{}
	}
	output, ok := d.probe("rpm", "-qf", "--queryformat", "%{NAME}", d.executable)
	if !ok || strings.TrimSpace(output) != binaryName {
		return Installation{}
	}
	result := d.installation(RPM, "dnf")
	if result.Manager == "" {
		result = d.installation(RPM, "yum")
	}
	result.PackageName = binaryName
	return result
}
