package version

import (
	"fmt"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const bootstrapDirectoryPermissions = 0o755

// bootstrapConfiguration preserves the project's registry and lockfile settings.
// Without a project, all automatic installation metadata belongs in XDG storage.
// Copy before overriding paths so bootstrap never changes the caller's configuration.
func bootstrapConfiguration(config *schema.AtmosConfiguration) (*schema.AtmosConfiguration, error) {
	result := &schema.AtmosConfiguration{Toolchain: schema.Toolchain{UseLockFile: true}}
	if config != nil {
		*result = *config
	}
	if result.CliConfigPath != "" {
		return result, nil
	}
	cache, err := xdg.GetXDGCacheDir("toolchain", bootstrapDirectoryPermissions)
	if err != nil {
		return nil, fmt.Errorf("resolve Atmos bootstrap cache: %w", err)
	}
	result.Toolchain.InstallPath = cache
	result.Toolchain.LockFile = filepath.Join(cache, "toolchain.lock.yaml")
	result.Toolchain.UseLockFile = true
	return result, nil
}
