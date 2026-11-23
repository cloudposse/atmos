package docker

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ui"
)

// dockerAuthConfigEnv is the environment variable docker/cli reads registry
// credentials from before config.json (cli/config/configfile/file.go,
// DockerEnvConfigKey and ConfigFile.GetCredentialsStore). Its in-memory store
// wins for every registry it lists; config.json is only the fallback.
const dockerAuthConfigEnv = "DOCKER_AUTH_CONFIG"

// warner returns the sink for user-facing warnings (stderr via pkg/ui by default).
func (m *ConfigManager) warner() func(string) {
	if m.warn == nil {
		return ui.Warning
	}
	return m.warn
}

// dockerAuthConfigWarning returns a warning when DOCKER_AUTH_CONFIG is set, since
// Docker then may not use the credentials Atmos just stored for registry, or ""
// when it is unset or empty (docker/cli treats an empty value as unset). A value
// Docker itself rejects is reported as invalid, without claiming it shadows the
// login: Docker prints an error and falls back to config.json for it.
func dockerAuthConfigWarning(registry string) string {
	//nolint:forbidigo // DOCKER_AUTH_CONFIG is a standard Docker env var, not Atmos config.
	value := os.Getenv(dockerAuthConfigEnv)
	if value == "" {
		return ""
	}

	listed, err := dockerAuthConfigLists(value, registry)
	if err != nil {
		return fmt.Sprintf("%[1]s is set but Docker cannot use it (%[3]v), so Docker ignores it and reads %[2]s from config.json. "+
			"Fix or unset %[1]s.", dockerAuthConfigEnv, registry, err)
	}
	if listed {
		return fmt.Sprintf("%[1]s is set and has credentials for %[2]s, which Docker uses instead of config.json: "+
			"the login stored for %[2]s will not be used while %[1]s is set. Unset %[1]s to use it.",
			dockerAuthConfigEnv, registry)
	}
	return fmt.Sprintf("%[1]s is set and takes precedence over config.json: Docker uses its credentials for every registry "+
		"it lists, so the login stored for %[2]s will not be used if %[1]s lists that registry. Unset %[1]s to use it.",
		dockerAuthConfigEnv, registry)
}

// dockerAuthConfigLists reports whether the DOCKER_AUTH_CONFIG value has an
// auths entry for registry. The value is validated the way docker/cli does
// (cli/config/configfile/file.go, parseEnvConfig): unknown fields, a missing
// auth field, or an auth value that is not base64("user:password") make Docker
// reject the whole value, and that is returned as an error. The in-memory
// store looks keys up exactly (cli/config/memorystore/store.go, Config.Get),
// so the match is exact.
func dockerAuthConfigLists(value, registry string) (bool, error) {
	var env struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&env); err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if decoder.More() {
		return false, errUtils.ErrDockerAuthConfigMultipleObjects
	}
	for addr, entry := range env.Auths {
		if entry.Auth == "" {
			return false, fmt.Errorf("%w for %s", errUtils.ErrDockerAuthConfigMissingAuth, addr)
		}
		if err := validateDockerAuthValue(entry.Auth); err != nil {
			return false, fmt.Errorf("invalid auth for %s: %w", addr, err)
		}
	}
	_, ok := env.Auths[registry]
	return ok, nil
}

// validateDockerAuthValue mirrors docker/cli's decodeAuth: the value must be
// base64("username:password") with a non-empty username.
func validateDockerAuthValue(auth string) error {
	decoded, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return err
	}
	username, _, ok := strings.Cut(string(decoded), ":")
	if !ok || username == "" {
		return errUtils.ErrDockerAuthConfigBadAuth
	}
	return nil
}
