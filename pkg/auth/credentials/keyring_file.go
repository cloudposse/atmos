package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/99designs/keyring"
	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// KeyringDirPermissions is the default permission for keyring directory (read/write/execute for owner only).
	KeyringDirPermissions = 0o700
)

var (
	// ErrPasswordTooShort indicates password does not meet minimum length requirement.
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
	// ErrUnsupportedCredentialType indicates the credential type is not supported.
	ErrUnsupportedCredentialType = errors.New("unsupported credential type")
	// ErrUnknownCredentialType indicates an unknown credential type was encountered.
	ErrUnknownCredentialType = errors.New("unknown credential type")
	// ErrCredentialsNotFound indicates credentials were not found for the given key.
	ErrCredentialsNotFound = errors.New("credentials not found")
	// ErrDataNotFound indicates data was not found for the given key.
	ErrDataNotFound = errors.New("data not found")
	// ErrPasswordRequired indicates a password is required but not provided.
	ErrPasswordRequired = errors.New("keyring password required")
)

// fileKeyringStore implements the CredentialStore interface using encrypted file storage via 99designs/keyring.
type fileKeyringStore struct {
	ring keyring.Keyring
	path string
}

// parseFileKeyringConfig extracts path and password environment from auth config.
func parseFileKeyringConfig(authConfig *schema.AuthConfig) (path, passwordEnv string) {
	if authConfig == nil || authConfig.Keyring.Spec == nil {
		return "", ""
	}

	if p, ok := authConfig.Keyring.Spec["path"].(string); ok && p != "" {
		path = p
	}
	if pe, ok := authConfig.Keyring.Spec["password_env"].(string); ok && pe != "" {
		passwordEnv = pe
	}

	return path, passwordEnv
}

// getDefaultKeyringPath returns the default keyring directory path.
func getDefaultKeyringPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".atmos", "keyring"), nil
}

// newFileKeyringStore creates a new file-based keyring store with encryption.
func newFileKeyringStore(authConfig *schema.AuthConfig) (*fileKeyringStore, error) {
	defer perf.Track(nil, "credentials.newFileKeyringStore")()

	// Parse configuration.
	path, passwordEnv := parseFileKeyringConfig(authConfig)

	// Default path if not specified.
	if path == "" {
		defaultPath, err := getDefaultKeyringPath()
		if err != nil {
			return nil, errors.Join(ErrCredentialStore, err)
		}
		path = defaultPath
	}

	// Default password environment variable.
	if passwordEnv == "" {
		passwordEnv = "ATMOS_KEYRING_PASSWORD"
	}

	// Ensure the configured path (storage directory) exists with proper permissions.
	// The path is the directory where keyring files will be stored (e.g., ~/.atmos/keyring or /etc/atmos/keyring).
	if err := os.MkdirAll(path, KeyringDirPermissions); err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to create keyring directory: %w", err))
	}

	// Create password prompt function.
	passwordFunc := createPasswordPrompt(passwordEnv)

	// Configure 99designs keyring.
	cfg := keyring.Config{
		ServiceName:                    "atmos-auth",
		FileDir:                        path,
		FilePasswordFunc:               passwordFunc,
		AllowedBackends:                []keyring.BackendType{keyring.FileBackend},
		KeychainName:                   "atmos",
		KeychainPasswordFunc:           passwordFunc,
		KeychainSynchronizable:         false,
		KeychainAccessibleWhenUnlocked: true,
		KeychainTrustApplication:       false,
	}

	// Open keyring.
	ring, err := keyring.Open(cfg)
	if err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to open file keyring: %w", err))
	}

	return &fileKeyringStore{
		ring: ring,
		path: path,
	}, nil
}

// createPasswordPrompt creates a password prompt function with environment variable fallback.
func createPasswordPrompt(passwordEnv string) keyring.PromptFunc {
	return func(prompt string) (string, error) {
		// 1. Check environment variable first (for automation/CI).
		_ = viper.BindEnv(passwordEnv)
		if password := viper.GetString(passwordEnv); password != "" {
			return password, nil
		}

		// 2. Interactive prompt if TTY is available.
		if term.IsTerminal(int(os.Stdin.Fd())) {
			var password string
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title(prompt).
						Description("Enter password to encrypt/decrypt keyring file").
						EchoMode(huh.EchoModePassword).
						Value(&password).
						Validate(func(s string) error {
							if len(s) < 8 {
								return ErrPasswordTooShort
							}
							return nil
						}),
				),
			).Run()
			if err != nil {
				return "", fmt.Errorf("password prompt failed: %w", err)
			}

			return password, nil
		}

		// 3. Error if neither available.
		return "", fmt.Errorf("%w: set %s environment variable or run in interactive mode", ErrPasswordRequired, passwordEnv)
	}
}

// Store stores credentials for the given alias.
func (s *fileKeyringStore) Store(alias string, creds types.ICredentials) error {
	defer perf.Track(nil, "credentials.fileKeyringStore.Store")()

	var (
		typ string
		raw []byte
		err error
	)

	switch c := creds.(type) {
	case *types.AWSCredentials:
		typ = "aws"
		raw, err = json.Marshal(c)
	case *types.OIDCCredentials:
		typ = "oidc"
		raw, err = json.Marshal(c)
	default:
		return fmt.Errorf("%w: %T", errors.Join(ErrCredentialStore, ErrUnsupportedCredentialType), creds)
	}
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal credentials: %w", err))
	}

	env := credentialEnvelope{Type: typ, Data: raw}
	data, err := json.Marshal(&env)
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal credentials: %w", err))
	}

	// Store in keyring.
	if err := s.ring.Set(keyring.Item{
		Key:  alias,
		Data: data,
	}); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to store credentials in file keyring: %w", err))
	}

	return nil
}

// Retrieve retrieves credentials for the given alias.
func (s *fileKeyringStore) Retrieve(alias string) (types.ICredentials, error) {
	defer perf.Track(nil, "credentials.fileKeyringStore.Retrieve")()

	item, err := s.ring.Get(alias)
	if err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to retrieve credentials from file keyring: %w", err))
	}

	var env credentialEnvelope
	if err := json.Unmarshal(item.Data, &env); err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal credential envelope: %w", err))
	}

	switch env.Type {
	case "aws":
		var c types.AWSCredentials
		if err := json.Unmarshal(env.Data, &c); err != nil {
			return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal AWS credentials: %w", err))
		}
		return &c, nil
	case "oidc":
		var c types.OIDCCredentials
		if err := json.Unmarshal(env.Data, &c); err != nil {
			return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal OIDC credentials: %w", err))
		}
		return &c, nil
	default:
		return nil, fmt.Errorf("%w: %q", errors.Join(ErrCredentialStore, ErrUnknownCredentialType), env.Type)
	}
}

// Delete deletes credentials for the given alias.
func (s *fileKeyringStore) Delete(alias string) error {
	defer perf.Track(nil, "credentials.fileKeyringStore.Delete")()

	if err := s.ring.Remove(alias); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to delete credentials from file keyring: %w", err))
	}

	return nil
}

// List returns all stored credential aliases.
func (s *fileKeyringStore) List() ([]string, error) {
	defer perf.Track(nil, "credentials.fileKeyringStore.List")()

	keys, err := s.ring.Keys()
	if err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to list credentials from file keyring: %w", err))
	}

	return keys, nil
}

// IsExpired checks if credentials for the given alias are expired.
func (s *fileKeyringStore) IsExpired(alias string) (bool, error) {
	defer perf.Track(nil, "credentials.fileKeyringStore.IsExpired")()

	creds, err := s.Retrieve(alias)
	if err != nil {
		return true, err
	}
	// Delegate to the credential's IsExpired implementation.
	return creds.IsExpired(), nil
}

// GetAny retrieves and unmarshals any type from the file keyring.
func (s *fileKeyringStore) GetAny(key string, dest interface{}) error {
	defer perf.Track(nil, "credentials.fileKeyringStore.GetAny")()

	item, err := s.ring.Get(key)
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to retrieve data from file keyring: %w", err))
	}

	if err := json.Unmarshal(item.Data, dest); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal data: %w", err))
	}

	return nil
}

// SetAny marshals and stores any type in the file keyring.
func (s *fileKeyringStore) SetAny(key string, value interface{}) error {
	defer perf.Track(nil, "credentials.fileKeyringStore.SetAny")()

	data, err := json.Marshal(value)
	if err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to marshal data: %w", err))
	}

	if err := s.ring.Set(keyring.Item{
		Key:  key,
		Data: data,
	}); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to store data in file keyring: %w", err))
	}

	return nil
}
