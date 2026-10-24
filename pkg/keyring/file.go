package keyring

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	keyringlib "github.com/99designs/keyring"
	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// Owner-only permission bits for the keyring directory.
	dirPermissions = 0o700
	// Minimum accepted file-backend password length.
	minPasswordLength = 8
)

// fileKeyring stores values in an encrypted file via 99designs/keyring (FileBackend). The file
// layout is addressed solely by FileDir + key, so it is stable across ServiceName values.
type fileKeyring struct {
	ring keyringlib.Keyring
	dir  string
}

// newFileKeyring opens (creating if needed) an encrypted file keyring under cfg.FileDir.
func newFileKeyring(cfg Config) (*fileKeyring, error) {
	dir := cfg.FileDir
	if dir == "" {
		defaultDir, err := xdg.GetXDGDataDir("keyring", dirPermissions)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		dir = defaultDir
	}

	// keyring.Open with FileBackend expects a directory. If the caller passed a file path
	// (something with an extension), use its parent directory.
	if filepath.Ext(dir) != "" {
		dir = filepath.Dir(dir)
	}

	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return nil, fmt.Errorf("%w: failed to create keyring directory: %w", ErrUnavailable, err)
	}

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	passwordEnv := cfg.PasswordEnv
	if passwordEnv == "" {
		passwordEnv = defaultPasswordEnv
	}
	passwordFunc := newPasswordPrompt(passwordEnv)

	ring, err := keyringlib.Open(keyringlib.Config{
		ServiceName:                    serviceName,
		FileDir:                        dir,
		FilePasswordFunc:               passwordFunc,
		AllowedBackends:                []keyringlib.BackendType{keyringlib.FileBackend},
		KeychainName:                   "atmos",
		KeychainPasswordFunc:           passwordFunc,
		KeychainSynchronizable:         false,
		KeychainAccessibleWhenUnlocked: true,
		KeychainTrustApplication:       false,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to open file keyring: %w", ErrUnavailable, err)
	}

	return &fileKeyring{ring: ring, dir: dir}, nil
}

func (s *fileKeyring) Get(key string) (string, error) {
	item, err := s.ring.Get(key)
	if err != nil {
		if errors.Is(err, keyringlib.ErrKeyNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return string(item.Data), nil
}

func (s *fileKeyring) Set(key string, value string) error {
	return s.ring.Set(keyringlib.Item{Key: key, Data: []byte(value)})
}

func (s *fileKeyring) Delete(key string) error {
	if err := s.ring.Remove(key); err != nil {
		// Treat "not found" as success (idempotent), covering both the library sentinel and a
		// missing-file error from the filesystem.
		if errors.Is(err, keyringlib.ErrKeyNotFound) || os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

func (s *fileKeyring) Has(key string) (bool, error) {
	_, err := s.Get(key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *fileKeyring) List() ([]string, error) {
	keys, err := s.ring.Keys()
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *fileKeyring) Type() string {
	return TypeFile
}

// Seams for testing the interactive password path without a live TTY. Production wiring reads the
// real stdin terminal state and runs the form; tests override these to drive the form in
// accessible mode with scripted IO. Restore overrides via t.Cleanup.
var (
	// Reports whether stdin is an interactive terminal.
	stdinIsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

	// Executes the built password form.
	runPasswordForm = func(f *huh.Form) error { return f.Run() }
)

// newPasswordPrompt returns a password function that reads the named environment variable, then
// falls back to an interactive TTY prompt, then errors. Passwords must meet minPasswordLength.
func newPasswordPrompt(passwordEnv string) keyringlib.PromptFunc {
	return func(prompt string) (string, error) {
		// 1. Environment variable (automation/CI). os.Getenv avoids viper singleton caching in tests.
		//nolint:forbidigo // os.Getenv required here to avoid viper singleton caching in tests.
		if password := os.Getenv(passwordEnv); password != "" {
			if len(password) < minPasswordLength {
				return "", ErrPasswordTooShort
			}
			return password, nil
		}

		// 2. Interactive prompt if a TTY is available.
		if stdinIsTerminal() {
			var password string
			form := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title(prompt).
						Description("Enter password to encrypt/decrypt keyring file").
						EchoMode(huh.EchoModePassword).
						Value(&password).
						Validate(func(s string) error {
							if len(s) < minPasswordLength {
								return ErrPasswordTooShort
							}
							return nil
						}),
				),
			)
			if err := runPasswordForm(form); err != nil {
				return "", fmt.Errorf("password prompt failed: %w", err)
			}
			return password, nil
		}

		// 3. Neither available.
		return "", fmt.Errorf("%w: set %s environment variable or run in interactive mode", ErrPasswordRequired, passwordEnv)
	}
}
