//go:build darwin

package aws

import (
	"golang.org/x/sys/unix"
)

// darwinKernelInfo returns the Darwin kernel release (what Node.js os.release() returns) and the
// CPU brand string (what Node.js os.cpus() reports as the model).
func darwinKernelInfo() (string, string, error) {
	release, err := unix.Sysctl("kern.osrelease")
	if err != nil {
		return "", "", err
	}
	// The CPU brand only decides the -arm64 suffix; a missing value means an Intel host.
	cpuBrand, err := unix.Sysctl("machdep.cpu.brand_string")
	if err != nil {
		cpuBrand = ""
	}
	return release, cpuBrand, nil
}
