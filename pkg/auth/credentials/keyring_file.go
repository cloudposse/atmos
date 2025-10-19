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
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fileKeyringStore implements the CredentialStore interface using encrypted file storage via 99designs/keyring.
type fileKeyringStore struct {
	ring keyring.Keyring
	path string
}

// newFileKeyringStore creates a new file-based keyring store with encryption.
func newFileKeyringStore(authConfig *schema.AuthConfig) (*fileKeyringStore, error) {
	defer perf.Track(nil, "credentials.newFileKeyringStore")()

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
			return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to get user home directory: %w", err))
		}
		path = filepath.Join(homeDir, ".atmos", "keyring")
	}

	// Default password environment variable.
	if passwordEnv == "" {
		passwordEnv = "ATMOS_KEYRING_PASSWORD"
	}

	// Ensure directory exists with proper permissions.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to create keyring directory: %w", err))
	}

	// Create password prompt function.
	passwordFunc := createPasswordPrompt(passwordEnv)

	// Configure 99designs keyring.
	cfg := keyring.Config{
		ServiceName:                    "atmos-auth",
		FileDir:                        dir,
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
		return errors.Join(ErrCredentialStore, fmt.Errorf("unsupported credential type %T", creds))
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
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("unknown credential type %q", env.Type))
	}
}

// Delete deletes credentials for the given alias.
func (s *fileKeyringStore) Delete(alias string) error {
	if err := s.ring.Remove(alias); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to delete credentials from file keyring: %w", err))
	}

	return nil
}

// List returns all stored credential aliases.
func (s *fileKeyringStore) List() ([]string, error) {
	keys, err := s.ring.Keys()
	if err != nil {
		return nil, errors.Join(ErrCredentialStore, fmt.Errorf("failed to list credentials from file keyring: %w", err))
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
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to retrieve data from file keyring: %w", err))
	}

	if err := json.Unmarshal(item.Data, dest); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal data: %w", err))
	}

	return nil
}

// SetAny marshals and stores any type in the file keyring.
func (s *fileKeyringStore) SetAny(key string, value interface{}) error {
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
