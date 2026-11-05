package installer

import "strings"

// detectDEB verifies unambiguous dpkg ownership without assuming an APT repository.
func detectDEB(d *detector) Installation {
	if d.system.GOOS() != "linux" {
		return Installation{}
	}
	output, ok := d.probe("dpkg-query", "--search", d.executable)
	if !ok {
		return Installation{}
	}
	// dpkg ownership is evidence of a DEB installation, not of an APT repository.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		return Installation{}
	}
	owner, file, found := strings.Cut(lines[0], ": ")
	packageName, _, _ := strings.Cut(owner, ":")
	if !found || packageName != binaryName || d.normalize(file) != d.normalize(d.executable) {
		return Installation{}
	}
	result := d.installation(DEB, "apt-get")
	result.PackageName = packageName
	return result
}
