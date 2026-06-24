package emulator

import (
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// defaultMountType is the mount type assumed when a `container.mounts` entry omits
// `type` (a host-path bind, the common case).
const defaultMountType = "bind"

// convertEmulatorMounts converts the component's `container.mounts` into runtime
// mounts, defaulting the type to bind and expanding a leading `~` in bind sources
// (mirrors the workflow container's mount handling).
func convertEmulatorMounts(mounts []schema.ContainerMount) []container.Mount {
	defer perf.Track(nil, "emulator.convertEmulatorMounts")()

	result := make([]container.Mount, 0, len(mounts))
	for _, mount := range mounts {
		mountType := mount.Type
		if mountType == "" {
			mountType = defaultMountType
		}
		result = append(result, container.Mount{
			Type:     mountType,
			Source:   expandHome(mount.Source),
			Target:   mount.Target,
			ReadOnly: mount.ReadOnly,
		})
	}
	return result
}

// mountsTargetPath reports whether any mount already targets the given container
// path, so an explicit user mount onto the data dir wins over the auto-injected
// persistence mount.
func mountsTargetPath(mounts []container.Mount, target string) bool {
	for i := range mounts {
		if mounts[i].Target == target {
			return true
		}
	}
	return false
}

// resolveMounts assembles the runtime mounts for an emulator instance: the user's
// explicit `container.mounts`, plus an auto-injected persistence bind mount of a
// host XDG cache directory onto the driver's data dir when persistence is enabled
// and the user has not already mounted that path.
func resolveMounts(spec *Spec, stack, name string) ([]container.Mount, error) {
	defer perf.Track(nil, "emulator.resolveMounts")()

	var mounts []container.Mount
	if spec.Container != nil {
		mounts = convertEmulatorMounts(spec.Container.Mounts)
	}
	if !spec.PersistEnabled() {
		return mounts, nil
	}
	dataDir, err := spec.DataDir()
	if err != nil {
		return nil, err
	}
	if dataDir == "" || mountsTargetPath(mounts, dataDir) {
		return mounts, nil
	}
	hostDir, err := InstanceDataDir(stack, name)
	if err != nil {
		return nil, err
	}
	return append(mounts, container.Mount{Type: defaultMountType, Source: hostDir, Target: dataDir}), nil
}

// expandHome expands a leading `~` (or `~/`) in a path to the user's home
// directory, returning the path unchanged on any error or when it has no `~`.
func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := homedir.Dir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
