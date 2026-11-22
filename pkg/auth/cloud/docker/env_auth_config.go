package docker

import (
	"encoding/json"
	"fmt"
	"os"

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
// when it is unset or empty (docker/cli treats an empty value as unset).
func dockerAuthConfigWarning(registry string) string {
	//nolint:forbidigo // DOCKER_AUTH_CONFIG is a standard Docker env var, not Atmos config.
	value := os.Getenv(dockerAuthConfigEnv)
	if value == "" {
		return ""
	}

	if dockerAuthConfigLists(value, registry) {
		return fmt.Sprintf("%[1]s is set and has credentials for %[2]s, which Docker uses instead of config.json: "+
			"the login stored for %[2]s will not be used while %[1]s is set. Unset %[1]s to use it.",
			dockerAuthConfigEnv, registry)
	}
	return fmt.Sprintf("%[1]s is set and takes precedence over config.json: Docker uses its credentials for every registry "+
		"it lists, so the login stored for %[2]s will not be used if %[1]s lists that registry. Unset %[1]s to use it.",
		dockerAuthConfigEnv, registry)
}

// dockerAuthConfigLists reports whether the DOCKER_AUTH_CONFIG value has an
// auths entry for registry. docker/cli's in-memory store looks keys up exactly
// (cli/config/memorystore/store.go, Config.Get), so the match is exact.
func dockerAuthConfigLists(value, registry string) bool {
	var env struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}
	if err := json.Unmarshal([]byte(value), &env); err != nil {
		return false
	}
	_, ok := env.Auths[registry]
	return ok
}
