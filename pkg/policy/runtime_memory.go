package policy

import (
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	cgroupRoot       = "/sys/fs/cgroup"
	cgroupMemoryRoot = "/sys/fs/cgroup/memory"
	uint64Bits       = 64
)

// constrainContainerMemory handles cgroup v2 and the conventional v1 memory mount.
// The scope remains host when no readable finite limit is discovered.
func constrainContainerMemory(facts *MemoryFacts) {
	if runtime.GOOS != "linux" {
		return
	}
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) != 3 {
			continue
		}
		if fields[0] == "0" && fields[1] == "" {
			applyMemoryHierarchy(facts, cgroupRoot, fields[2], "memory.max", "memory.current")
		}
		for _, controller := range strings.Split(fields[1], ",") {
			if controller == "memory" {
				applyMemoryHierarchy(facts, cgroupMemoryRoot, fields[2], "memory.limit_in_bytes", "memory.usage_in_bytes")
			}
		}
	}
}

func applyMemoryLimit(facts *MemoryFacts, path, limitName, usageName string) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return
	}
	defer root.Close()
	limitData, err := root.ReadFile(limitName)
	if err != nil {
		return
	}
	limit, err := strconv.ParseUint(strings.TrimSpace(string(limitData)), decimalBase, uint64Bits)
	if err != nil || facts.TotalBytes == nil || limit > *facts.TotalBytes {
		return
	}
	usageData, err := root.ReadFile(usageName)
	if err != nil {
		return
	}
	usage, err := strconv.ParseUint(strings.TrimSpace(string(usageData)), decimalBase, uint64Bits)
	if err != nil {
		return
	}
	available := uint64(0)
	if limit > usage {
		available = limit - usage
	}
	if facts.AvailableBytes != nil {
		available = min(available, *facts.AvailableBytes)
	}
	facts.TotalBytes, facts.AvailableBytes, facts.Scope = &limit, &available, "container"
}

func cgroupPath(root, group string) string {
	return filepath.Join(root, strings.TrimPrefix(path.Clean("/"+group), "/"))
}

func applyMemoryHierarchy(facts *MemoryFacts, root, group, limitName, usageName string) {
	path := cgroupPath(root, group)
	for {
		applyMemoryLimit(facts, path, limitName, usageName)
		// Also covers a namespaced cgroup exposed directly at the mount root.
		if path == root {
			return
		}
		path = filepath.Dir(path)
	}
}
