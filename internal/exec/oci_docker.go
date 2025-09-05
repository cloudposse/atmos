package exec

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	log "github.com/charmbracelet/log"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

// DockerConfig represents the structure of Docker's config.json file.
type DockerConfig struct {
	Auths map[string]struct {
		Auth string `json:"auth"`
	} `json:"auths"`
	CredsStore  string            `json:"credsStore"`
	CredHelpers map[string]string `json:"credHelpers"`
}

var (
	// Static errors for Docker authentication
	errFailedToGetUserHomeDir             = errors.New("failed to get user home directory")
	errDockerConfigFileNotFound           = errors.New("Docker config file not found")
	errFailedToReadDockerConfigFile       = errors.New("failed to read Docker config file")
	errFailedToParseDockerConfigJSON      = errors.New("failed to parse Docker config JSON")
	errNoCredentialHelpersConfigured      = errors.New("no credential helpers configured")
	errNoCredentialHelperFound            = errors.New("no credential helper found for registry")
	errNoGlobalCredentialStore            = errors.New("no global credential store configured")
	errFailedToDecodeAuthForRegistry      = errors.New("failed to decode auth for registry")
	errNoDirectAuthFound                  = errors.New("no direct auth found for registry")
	errInvalidRegistryName                = errors.New("invalid registry name")
	errInvalidCredentialStoreName         = errors.New("invalid credential store name")
	errCredentialHelperNotFound           = errors.New("credential helper not found")
	errFailedToGetCredentialsFromStore    = errors.New("failed to get credentials from store")
	errFailedToParseCredentialStoreOutput = errors.New("failed to parse credential store output")
	errInvalidCredentialsFromStore        = errors.New("invalid credentials from store")
	errFailedToDecodeBase64AuthString     = errors.New("failed to decode base64 auth string")
	errInvalidAuthStringFormat            = errors.New("invalid auth string format, expected username:password")
)

// resolveDockerConfigPath resolves the Docker config file path from various sources.
func resolveDockerConfigPath(atmosConfig *schema.AtmosConfiguration) (string, error) {
	// Create a Viper instance for environment variable access
	v := viper.New()
	bindEnv(v, "docker_config", "ATMOS_OCI_DOCKER_CONFIG", "DOCKER_CONFIG")

	// Resolve Docker config path (env has precedence).
	configDir := v.GetString("docker_config")
	if configDir == "" {
		configDir = atmosConfig.Settings.OCI.DockerConfig
	}
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("%w: %v", errFailedToGetUserHomeDir, err)
		}
		configDir = filepath.Join(homeDir, ".docker")
	}
	return filepath.Join(configDir, "config.json"), nil
}

// loadDockerConfig loads and parses the Docker config file.
func loadDockerConfig(configPath string) (DockerConfig, error) {
	// Check if Docker config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DockerConfig{}, fmt.Errorf("%w: %s", errDockerConfigFileNotFound, configPath)
	}

	// Read Docker config file
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return DockerConfig{}, fmt.Errorf("%w: %v", errFailedToReadDockerConfigFile, err)
	}

	// Parse Docker config JSON
	var dockerConfig DockerConfig
	if err := json.Unmarshal(configData, &dockerConfig); err != nil {
		return DockerConfig{}, fmt.Errorf("%w: %v", errFailedToParseDockerConfigJSON, err)
	}

	return dockerConfig, nil
}

// tryCredentialHelper attempts to authenticate using a specific credential helper.
func tryCredentialHelper(registry, helperKey, helper string) (authn.Authenticator, error) {
	if helper == "" {
		return nil, errNoCredentialHelperFound
	}

	// Use the exact helper key (server URL) that matched in config.
	auth, err := getCredentialStoreAuth(helperKey, helper)
	if err == nil {
		log.Debug("Using per-registry credential helper", logFieldRegistry, helperKey, "helper", helper)
		return auth, nil
	}

	log.Debug("Per-registry credential helper failed", logFieldRegistry, helperKey, "helper", helper, "error", err)
	return nil, err
}

// tryCredentialHelpers attempts to authenticate using per-registry credential helpers.
func tryCredentialHelpers(registry string, credHelpers map[string]string) (authn.Authenticator, error) {
	if credHelpers == nil {
		return nil, errNoCredentialHelpersConfigured
	}

	// Try exact registry match first
	if helper, ok := credHelpers[registry]; ok {
		if auth, err := tryCredentialHelper(registry, registry, helper); err == nil {
			return auth, nil
		}
	}

	// Try with https:// prefix
	httpsRegistry := "https://" + registry
	if helper, ok := credHelpers[httpsRegistry]; ok {
		if auth, err := tryCredentialHelper(registry, httpsRegistry, helper); err == nil {
			return auth, nil
		}
	}

	return nil, errNoCredentialHelperFound
}

// tryGlobalCredentialStore attempts to authenticate using the global credential store.
func tryGlobalCredentialStore(registry, credsStore string) (authn.Authenticator, error) {
	if credsStore == "" {
		return nil, errNoGlobalCredentialStore
	}

	if auth, err := getCredentialStoreAuth(registry, credsStore); err == nil {
		log.Debug("Using global credential store authentication", logFieldRegistry, registry, "store", credsStore)
		return auth, nil
	} else {
		log.Debug("Global credential store authentication failed", logFieldRegistry, registry, "store", credsStore, "error", err)
		return nil, err
	}
}

// tryDirectAuth attempts to authenticate using direct auth strings in the config.
func tryDirectAuth(registry string, auths map[string]struct {
	Auth string `json:"auth"`
}) (authn.Authenticator, error) {
	// Try different registry formats
	registryVariants := []string{
		registry,
		"https://" + registry,
		"http://" + registry,
	}

	for _, reg := range registryVariants {
		if authData, exists := auths[reg]; exists && authData.Auth != "" {
			username, password, err := decodeDockerAuth(authData.Auth)
			if err != nil {
				return nil, fmt.Errorf("%w %s: %w", errFailedToDecodeAuthForRegistry, reg, err)
			}
			return &authn.Basic{
				Username: username,
				Password: password,
			}, nil
		}
	}

	return nil, errNoDirectAuthFound
}

// getDockerAuth attempts to get Docker authentication for a registry
// Supports DOCKER_CONFIG environment variable, global credential stores (credsStore),
// and per-registry credential helpers (credHelpers).
func getDockerAuth(registry string, atmosConfig *schema.AtmosConfiguration) (authn.Authenticator, error) {
	// Resolve Docker config path
	configPath, err := resolveDockerConfigPath(atmosConfig)
	if err != nil {
		return nil, err
	}
	log.Debug("Using Docker config path", "path", configPath)

	// Load Docker config
	dockerConfig, err := loadDockerConfig(configPath)
	if err != nil {
		return nil, err
	}

	// Try per-registry credential helpers first
	if auth, err := tryCredentialHelpers(registry, dockerConfig.CredHelpers); err == nil {
		return auth, nil
	}

	// Try global credential store
	if auth, err := tryGlobalCredentialStore(registry, dockerConfig.CredsStore); err == nil {
		return auth, nil
	}

	// Try direct auth strings
	if auth, err := tryDirectAuth(registry, dockerConfig.Auths); err == nil {
		return auth, nil
	}

	return nil, fmt.Errorf("%w %s", errNoAuthenticationFound, registry)
}

// getCredentialStoreAuth attempts to get credentials from Docker's credential store.
func getCredentialStoreAuth(registry, credsStore string) (authn.Authenticator, error) {
	// Validate registry using an allowlist approach
	// Registry may only include letters, digits, dots, colons, slashes, and hyphens
	validRegistry := regexp.MustCompile(`^[A-Za-z0-9./:-]+$`)
	if !validRegistry.MatchString(registry) {
		return nil, fmt.Errorf("%w: %s", errInvalidRegistryName, registry)
	}

	// Validate credsStore using an allowlist (letters, digits, underscore, hyphen).
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(credsStore) {
		return nil, fmt.Errorf("%w: %s", errInvalidCredentialStoreName, credsStore)
	}

	// For Docker Desktop on macOS, the credential store is typically "desktop"
	// We need to use the docker-credential-desktop helper to get credentials

	// Try to execute the credential helper
	helperCmd := "docker-credential-" + credsStore
	if _, err := exec.LookPath(helperCmd); err != nil {
		return nil, fmt.Errorf("%w %s: %w", errCredentialHelperNotFound, helperCmd, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, helperCmd, "get")
	cmd.Stdin = strings.NewReader(registry)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", errFailedToGetCredentialsFromStore, credsStore, err)
	}

	// Parse the JSON output from the credential helper
	var creds struct {
		Username string `json:"Username"`
		Secret   string `json:"Secret"`
	}

	if err := json.Unmarshal(output, &creds); err != nil {
		return nil, fmt.Errorf("%w: %v", errFailedToParseCredentialStoreOutput, err)
	}

	if creds.Username == "" || creds.Secret == "" {
		return nil, errInvalidCredentialsFromStore
	}

	return &authn.Basic{
		Username: creds.Username,
		Password: creds.Secret,
	}, nil
}

// decodeDockerAuth decodes the base64-encoded auth string from Docker config.
func decodeDockerAuth(authString string) (string, string, error) {
	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(authString)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", errFailedToDecodeBase64AuthString, err)
	}

	// Split username:password
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", errInvalidAuthStringFormat
	}

	return parts[0], parts[1], nil
}
