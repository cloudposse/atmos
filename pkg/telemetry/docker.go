package telemetry

import (
	"bufio"
	"os"
	"strings"
)

// isDocker determines if the current process is running inside a Docker container.
// It uses the most reliable method: checking the cgroup information in /proc/1/cgroup.
// In Docker containers, the cgroup will contain "docker" or "kubepods" entries,
// while on a host it will typically show "init.scope" or similar.
func isDocker(filepath ...string) bool {
	filePath := "/proc/1/cgroup"
	if len(filepath) > 0 {
		filePath = filepath[0]
	}
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Check for docker-related entries in cgroup
		if strings.Contains(line, "docker") || strings.Contains(line, "kubepods") {
			return true
		}
	}

	return false
}
