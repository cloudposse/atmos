package rc

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// rcFilePerm is the permission for the generated CLI config file. It may carry
// credentials (e.g. a `credentials` block), so it is owner-read/write only.
const rcFilePerm = 0o600

// Setup renders `components.terraform.rc` to a temporary Terraform CLI config
// file and returns the environment variable that points Terraform at it, plus a
// closer that removes the file.
//
// It returns an empty env and a no-op closer when RC management is disabled, or
// when the user already controls TF_CLI_CONFIG_FILE (via OS env, the atmos.yaml
// `env:` section, or the component `env:` section) — Atmos defers to the user,
// mirroring how TF_PLUGIN_CACHE_DIR is handled.
//
// The closer must be invoked after the entire terraform pipeline (init,
// workspace, plan/apply) completes so the file survives every subprocess.
func Setup(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (env []string, closer func() error, err error) {
	defer perf.Track(atmosConfig, "rc.Setup")()

	rc := atmosConfig.Components.Terraform.RC
	if rc == nil || !rc.Enabled {
		return nil, noop, nil
	}

	return Generate(rc.Config, atmosConfig, info)
}

// Generate renders the given CLI-config map to a temporary file and returns the
// TF_CLI_CONFIG_FILE env entry plus a removal closer. It is the lower-level seam
// the registry cache also drives (it contributes network_mirror/host directives
// into rcMap). User precedence is honored exactly as in Setup.
func Generate(rcMap map[string]any, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (env []string, closer func() error, err error) {
	defer perf.Track(atmosConfig, "rc.Generate")()

	if userControlsCLIConfig(atmosConfig, info) {
		log.Debug("TF_CLI_CONFIG_FILE already set, deferring to the user-managed Terraform CLI config")
		return nil, noop, nil
	}

	rendered, err := Render(rcMap)
	if err != nil {
		return nil, noop, err
	}

	path, err := writeTempRC(rendered)
	if err != nil {
		return nil, noop, err
	}

	cleanup := func() error {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
		return nil
	}

	// Point both Terraform (TF_CLI_CONFIG_FILE) and OpenTofu (TOFU_CLI_CONFIG_FILE)
	// at the generated file so the config is honored regardless of which binary runs.
	env = make([]string, 0, len(cliConfigEnvKeys))
	for _, key := range cliConfigEnvKeys {
		env = append(env, fmt.Sprintf("%s=%s", key, path))
	}
	return env, cleanup, nil
}

// writeTempRC atomically writes the rendered CLI config to a unique temp file
// and returns its path.
func writeTempRC(rendered []byte) (string, error) {
	// Reserve a unique path; close immediately and rewrite atomically.
	tmp, err := os.CreateTemp("", "atmos-*.tfrc")
	if err != nil {
		return "", fmt.Errorf("%w: creating temporary CLI config: %w", errUtils.ErrInvalidConfig, err)
	}
	path := tmp.Name()
	_ = tmp.Close()

	fs := filesystem.NewOSFileSystem()
	if err := fs.WriteFileAtomic(path, rendered, rcFilePerm); err != nil {
		_ = os.Remove(path) //nolint:gosec // path is a CreateTemp result owned by this process.
		return "", fmt.Errorf("%w: writing temporary CLI config %q: %w", errUtils.ErrInvalidConfig, filepath.Base(path), err)
	}

	return path, nil
}

// userControlsCLIConfig reports whether the user already set a CLI config env var
// (TF_CLI_CONFIG_FILE, TOFU_CLI_CONFIG_FILE, or the legacy TERRAFORM_CONFIG) to a
// non-empty value via the OS environment, the atmos.yaml `env:` section, or the
// component `env:` section. In that case Atmos defers to the user.
func userControlsCLIConfig(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) bool {
	for _, key := range userCLIConfigEnvKeys {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			return true
		}
		if v, ok := atmosConfig.Env[key]; ok && v != "" {
			return true
		}
		if info != nil {
			if v, ok := info.ComponentEnvSection[key]; ok {
				if s, isStr := v.(string); !isStr || s != "" {
					return true
				}
			}
		}
	}
	return false
}
