//go:build darwin

package aws

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// darwinSysctl reads a string sysctl value. It is a variable so tests can simulate a failed lookup.
var darwinSysctl = unix.Sysctl

// darwinKernelInfo returns the Darwin kernel release (what Node.js os.release() returns) and the
// CPU brand string (what Node.js os.cpus() reports as the model).
func darwinKernelInfo() (string, string, error) {
	release, err := darwinSysctl("kern.osrelease")
	if err != nil {
		return "", "", err
	}
	// The CPU brand decides the -arm64 suffix, so a failed lookup must not silently select an Intel
	// host: the caller would then look for the wrong browser directory.
	cpuBrand, err := darwinSysctl("machdep.cpu.brand_string")
	if err != nil {
		return "", "", fmt.Errorf("could not read the CPU brand: %w", err)
	}
	return release, cpuBrand, nil
}
