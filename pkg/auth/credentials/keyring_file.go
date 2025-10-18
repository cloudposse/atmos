package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/99designs/keyring"
	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fileKeyringStore implements the CredentialStore interface using encrypted file storage via 99designs/keyring.
type fileKeyringStore struct {
	ring keyring.Keyring
	path string
}

// newFileKeyringStore creates a new file-based keyring store with encryption.
func newFileKeyringStore(authConfig *schema.AuthConfig) (*fileKeyringStore, error) {

	var path string
	var passwordEnv string

	// Parse spec for configuration.
	if authConfig != nil && authConfig.Keyring.Spec != nil {
		if p, ok := authConfig.Keyring.Spec["path"].(string); ok && p != "" {
			path = p
		}
		if pe, ok := authConfig.Keyring.Spec["password_env"].(string); ok && pe != "" {
			passwordEnv = pe
		}
	}

	// Default path if not specified.
	if path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("%w: failed to get user home directory: %v", ErrCredentialStore, err)
		}
		path = filepath.Join(homeDir, ".atmos", "keyring")
	}

	// Default password environment variable.
	if passwordEnv == "" {
		passwordEnv = "ATMOS_KEYRING_PASSWORD"
	}

	// Ensure directory exists with proper permissions.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("%w: failed to create keyring directory: %v", ErrCredentialStore, err)
	}

	// Create password prompt function.
	passwordFunc := createPasswordPrompt(passwordEnv)

	// Configure 99designs keyring.
	cfg := keyring.Config{
		ServiceName:                     "atmos-auth",
		FileDir:                         dir,
		FilePasswordFunc:                passwordFunc,
		AllowedBackends:                 []keyring.BackendType{keyring.FileBackend},
		KeychainName:                    "atmos",
		KeychainPasswordFunc:            passwordFunc,
		KeychainSynchronizable:          false,
		KeychainAccessibleWhenUnlocked:  true,
		KeychainTrustApplication:        false,
	}

	// Open keyring.
	ring, err := keyring.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to open file keyring: %v", ErrCredentialStore, err)
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
		if password := os.Getenv(passwordEnv); password != "" {
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
								return errors.New("password must be at least 8 characters")
							}
							return nil
						}),
				),
			).Run()

			if err != nil {
				return "", fmt.Errorf("password prompt failed: %v", err)
			}

			return password, nil
		}

		// 3. Error if neither available.
		return "", fmt.Errorf("keyring password required: set %s environment variable or run in interactive mode", passwordEnv)
	}
}

// Store stores credentials for the given alias.
func (s *fileKeyringStore) Store(alias string, creds types.ICredentials) error {

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
		return fmt.Errorf("%w: unsupported credential type %T", ErrCredentialStore, creds)
	}
	if err != nil {
		return errors.Join(ErrCredentialStore, err)
	}

	env := credentialEnvelope{Type: typ, Data: raw}
	data, err := json.Marshal(&env)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal credentials: %v", ErrCredentialStore, err)
	}

	// Store in keyring.
	if err := s.ring.Set(keyring.Item{
		Key:  alias,
		Data: data,
	}); err != nil {
		return fmt.Errorf("%w: failed to store credentials in file keyring: %v", ErrCredentialStore, err)
	}

	return nil
}

// Retrieve retrieves credentials for the given alias.
func (s *fileKeyringStore) Retrieve(alias string) (types.ICredentials, error) {

	item, err := s.ring.Get(alias)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to retrieve credentials from file keyring: %v", ErrCredentialStore, err)
	}

	var env credentialEnvelope
	if err := json.Unmarshal(item.Data, &env); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal credential envelope: %v", ErrCredentialStore, err)
	}

	switch env.Type {
	case "aws":
		var c types.AWSCredentials
		if err := json.Unmarshal(env.Data, &c); err != nil {
			return nil, fmt.Errorf("%w: failed to unmarshal AWS credentials: %v", ErrCredentialStore, err)
		}
		return &c, nil
	case "oidc":
		var c types.OIDCCredentials
		if err := json.Unmarshal(env.Data, &c); err != nil {
			return nil, fmt.Errorf("%w: failed to unmarshal OIDC credentials: %v", ErrCredentialStore, err)
		}
		return &c, nil
	default:
		return nil, fmt.Errorf("%w: unknown credential type %q", ErrCredentialStore, env.Type)
	}
}

// Delete deletes credentials for the given alias.
func (s *fileKeyringStore) Delete(alias string) error {

	if err := s.ring.Remove(alias); err != nil {
		return fmt.Errorf("%w: failed to delete credentials from file keyring: %v", ErrCredentialStore, err)
	}

	return nil
}

// List returns all stored credential aliases.
func (s *fileKeyringStore) List() ([]string, error) {

	keys, err := s.ring.Keys()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to list credentials from file keyring: %v", ErrCredentialStore, err)
	}

	return keys, nil
}

// IsExpired checks if credentials for the given alias are expired.
func (s *fileKeyringStore) IsExpired(alias string) (bool, error) {

	creds, err := s.Retrieve(alias)
	if err != nil {
		return true, err
	}
	// Delegate to the credential's IsExpired implementation.
	return creds.IsExpired(), nil
}

// GetAny retrieves and unmarshals any type from the file keyring.
func (s *fileKeyringStore) GetAny(key string, dest interface{}) error {

	item, err := s.ring.Get(key)
	if err != nil {
		return fmt.Errorf("%w: failed to retrieve data from file keyring: %v", ErrCredentialStore, err)
	}

	if err := json.Unmarshal(item.Data, dest); err != nil {
		return fmt.Errorf("%w: failed to unmarshal data: %v", ErrCredentialStore, err)
	}

	return nil
}

// SetAny marshals and stores any type in the file keyring.
func (s *fileKeyringStore) SetAny(key string, value interface{}) error {

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal data: %v", ErrCredentialStore, err)
	}

	if err := s.ring.Set(keyring.Item{
		Key:  key,
		Data: data,
	}); err != nil {
		return fmt.Errorf("%w: failed to store data in file keyring: %v", ErrCredentialStore, err)
	}

	return nil
}
