package exec

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	log "github.com/charmbracelet/log"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

// getDockerAuth attempts to get Docker authentication for a registry
// Supports DOCKER_CONFIG environment variable, global credential stores (credsStore),
// and per-registry credential helpers (credHelpers)
func getDockerAuth(registry string, atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, error) {
	// Create a Viper instance for environment variable access
	v := viper.New()
	bindEnv(v, "docker_config", "ATMOS_DOCKER_CONFIG", "DOCKER_CONFIG")

	// Resolve Docker config path
	configDir := atmosConfig.Settings.OCI.DockerConfig
	if configDir == "" {
		configDir = v.GetString("docker_config") // Use Viper instead of os.Getenv
	}
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		configDir = filepath.Join(homeDir, ".docker")
	}
	dockerConfigPath := filepath.Join(configDir, "config.json")
	log.Debug("Using Docker config path", "path", dockerConfigPath)

	// Check if Docker config file exists
	if _, err := os.Stat(dockerConfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("Docker config file not found: %s", dockerConfigPath)
	}

	// Read Docker config file
	configData, err := os.ReadFile(dockerConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Docker config file: %w", err)
	}

	// Parse Docker config JSON
	var dockerConfig struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
		CredsStore  string            `json:"credsStore"`
		CredHelpers map[string]string `json:"credHelpers"`
	}

	if err := json.Unmarshal(configData, &dockerConfig); err != nil {
		return nil, fmt.Errorf("failed to parse Docker config JSON: %w", err)
	}

	// Next, try per-registry credHelpers
	if dockerConfig.CredHelpers != nil {
		// Try exact registry match first
		if helper, ok := dockerConfig.CredHelpers[registry]; ok && helper != "" {
			if auth, err := getCredentialStoreAuth(registry, helper); err == nil {
				log.Debug("Using per-registry credential helper", "registry", registry, "helper", helper)
				return auth, nil
			} else {
				log.Debug("Per-registry credential helper failed", "registry", registry, "helper", helper, "error", err)
			}
		}

		// Try with https:// prefix
		httpsRegistry := "https://" + registry
		if helper, ok := dockerConfig.CredHelpers[httpsRegistry]; ok && helper != "" {
			if auth, err := getCredentialStoreAuth(registry, helper); err == nil {
				log.Debug("Using per-registry credential helper", "registry", httpsRegistry, "helper", helper)
				return auth, nil
			} else {
				log.Debug("Per-registry credential helper failed", "registry", httpsRegistry, "helper", helper, "error", err)
			}
		}
	}

	// Fallback to global credential store (credsStore) if it exists
	if dockerConfig.CredsStore != "" {
		if auth, err := getCredentialStoreAuth(registry, dockerConfig.CredsStore); err == nil {
			log.Debug("Using global credential store authentication", "registry", registry, "store", dockerConfig.CredsStore)
			return auth, nil
		} else {
			log.Debug("Global credential store authentication failed", "registry", registry, "store", dockerConfig.CredsStore, "error", err)
		}
	}

	// Fallback to direct auth strings in the config file
	// Look for exact registry match first
	if authData, exists := dockerConfig.Auths[registry]; exists && authData.Auth != "" {
		username, password, err := decodeDockerAuth(authData.Auth)
		if err != nil {
			return nil, fmt.Errorf("failed to decode auth for registry %s: %w", registry, err)
		}
		return &authn.Basic{
			Username: username,
			Password: password,
		}, nil
	}

	// Look for registry with https:// prefix
	httpsRegistry := "https://" + registry
	if authData, exists := dockerConfig.Auths[httpsRegistry]; exists && authData.Auth != "" {
		username, password, err := decodeDockerAuth(authData.Auth)
		if err != nil {
			return nil, fmt.Errorf("failed to decode auth for registry %s: %w", httpsRegistry, err)
		}
		return &authn.Basic{
			Username: username,
			Password: password,
		}, nil
	}

	// Look for registry with http:// prefix
	httpRegistry := "http://" + registry
	if authData, exists := dockerConfig.Auths[httpRegistry]; exists && authData.Auth != "" {
		username, password, err := decodeDockerAuth(authData.Auth)
		if err != nil {
			return nil, fmt.Errorf("failed to decode auth for registry %s: %w", httpRegistry, err)
		}
		return &authn.Basic{
			Username: username,
			Password: password,
		}, nil
	}

	return nil, fmt.Errorf("no authentication found in Docker config for registry %s", registry)
}

// getCredentialStoreAuth attempts to get credentials from Docker's credential store
func getCredentialStoreAuth(registry, credsStore string) (authn.Authenticator, error) {
	// Validate registry to prevent command injection
	if strings.ContainsAny(registry, ";&|`$(){}[]<>'\"\n\r") {
		return nil, fmt.Errorf("invalid registry name: %s", registry)
	}

	// Validate credsStore to prevent command injection
	if strings.ContainsAny(credsStore, ";&|`$(){}[]<>/\\") {
		return nil, fmt.Errorf("invalid credential store name: %s", credsStore)
	}

	// For Docker Desktop on macOS, the credential store is typically "desktop"
	// We need to use the docker-credential-desktop helper to get credentials

	// Try to execute the credential helper
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker-credential-"+credsStore, "get")
	cmd.Stdin = strings.NewReader(registry)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials from store %s: %w", credsStore, err)
	}

	// Parse the JSON output from the credential helper
	var creds struct {
		Username string `json:"Username"`
		Secret   string `json:"Secret"`
	}

	if err := json.Unmarshal(output, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credential store output: %w", err)
	}

	if creds.Username == "" || creds.Secret == "" {
		return nil, fmt.Errorf("invalid credentials from store")
	}

	return &authn.Basic{
		Username: creds.Username,
		Password: creds.Secret,
	}, nil
}

// decodeDockerAuth decodes the base64-encoded auth string from Docker config
func decodeDockerAuth(authString string) (string, string, error) {
	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(authString)
	if err != nil {
		return "", "", fmt.Errorf("failed to decode base64 auth string: %w", err)
	}

	// Split username:password
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid auth string format, expected username:password")
	}

	return parts[0], parts[1], nil
}
