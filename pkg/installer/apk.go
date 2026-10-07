package installer

import "strings"

func detectAPK(d *detector) Installation {
	if d.system.GOOS() != "linux" {
		return Installation{}
	}
	output, ok := d.probe("apk", "info", "--who-owns", d.executable)
	if !ok {
		return Installation{}
	}
	file, owner, found := strings.Cut(strings.TrimSpace(output), " is owned by ")
	if !found || d.normalize(file) != d.normalize(d.executable) || !strings.HasPrefix(owner, "atmos-") {
		return Installation{}
	}
	// Reject similarly named packages and malformed/multiline ownership output.
	version := strings.TrimPrefix(owner, "atmos-")
	if version == "" || version[0] < '0' || version[0] > '9' || strings.ContainsAny(version, " \t\r\n") {
		return Installation{}
	}
	result := d.installation(APK, "apk")
	result.PackageName = binaryName
	return result
}
