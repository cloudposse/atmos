package manage

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
)

const maxRegistryPassword = 64 << 10

// RegistryCredentials stores explicit registry credentials in the shared Atmos keyring. The
// seams default to the process terminal, standard input, and configured keyring backend.
type RegistryCredentials struct {
	Auth *schema.AuthConfig
	// Input supplies the password or token. Nil uses standard input.
	Input io.Reader
	// StdinIsTTY reports whether standard input is an interactive terminal. Nil probes the real one.
	StdinIsTTY func() bool
	// Open opens the keyring backend. Nil uses the configured Atmos keyring.
	Open func(*schema.AuthConfig) (keyring.Keyring, error)
}

// Store reads a password from the input and saves it for the registry host. It refuses
// configurations that cannot persist the secret, rather than reporting a login that was lost.
func (c *RegistryCredentials) Store(host, username string) error {
	defer perf.Track(nil, "pluginmanage.RegistryCredentials.Store")()

	if err := c.checkPersistent(); err != nil {
		return err
	}
	if c.stdinIsTTY() {
		return errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanation("`--password-stdin` reads the password from standard input, but standard input is an interactive terminal.").
			WithHint("Pipe the secret in, for example `printenv REGISTRY_TOKEN | atmos plugin registry login <registry> --username=<name> --password-stdin`.").Err()
	}
	secret, err := io.ReadAll(io.LimitReader(c.input(), maxRegistryPassword+1))
	if err != nil {
		return err
	}
	if len(secret) > maxRegistryPassword {
		return errUtils.ErrPluginPayloadTooLarge
	}
	password := strings.TrimRight(string(secret), "\r\n")
	if password == "" {
		return errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanation("Standard input did not contain a password or token.").
			WithHint("Pipe the secret into the command, for example `printenv REGISTRY_TOKEN | atmos plugin registry login ... --password-stdin`.").Err()
	}
	store, err := c.open()
	if err != nil {
		return c.storeFailure(err)
	}
	if err := (oci.CredentialKeychain{Store: store}).Login(host, &authn.AuthConfig{Username: username, Password: password}); err != nil {
		return c.storeFailure(err)
	}
	return nil
}

// Remove deletes the stored credentials for a registry host. It is idempotent.
func (c *RegistryCredentials) Remove(host string) error {
	defer perf.Track(nil, "pluginmanage.RegistryCredentials.Remove")()

	store, err := c.open()
	if err != nil {
		return c.storeFailure(err)
	}
	if err := (oci.CredentialKeychain{Store: store}).Logout(host); err != nil {
		return c.storeFailure(err)
	}
	return nil
}

// checkPersistent rejects keyring backends that keep nothing beyond this process.
func (c *RegistryCredentials) checkPersistent() error {
	kind := credentials.KeyringType(c.Auth)
	if kind != keyring.TypeMemory && kind != keyring.TypeNoop {
		return nil
	}
	return errUtils.Build(errUtils.ErrPluginCredentialStore).
		WithExplanationf("`auth.keyring.type: %s` keeps nothing after this command exits, so a stored login would be lost immediately.", kind).
		WithHint("Set `auth.keyring.type` to `file` or `system`, or define an `oci/user` identity with `password_env` and select it with `plugins.registries.<name>.identity`.").Err()
}

func (c *RegistryCredentials) storeFailure(err error) error {
	kind := credentials.KeyringType(c.Auth)
	return errUtils.Build(fmt.Errorf("%w: the %s keyring failed: %w", errUtils.ErrPluginCredentialStore, kind, withoutSentinel(err, errUtils.ErrOCIRegistry))).
		WithHint("Check that the keyring is available and unlocked, or set `auth.keyring.type: file` and `auth.keyring.spec.path` in atmos.yaml.").
		WithHint("Alternatively, use an `oci/user` identity with `password_env` so no credential is stored.").Err()
}

func (c *RegistryCredentials) stdinIsTTY() bool {
	if c.StdinIsTTY != nil {
		return c.StdinIsTTY()
	}
	return terminal.HasRealTTYInput()
}

func (c *RegistryCredentials) input() io.Reader {
	if c.Input != nil {
		return c.Input
	}
	return iolib.GetContext().Input()
}

func (c *RegistryCredentials) open() (keyring.Keyring, error) {
	if c.Open != nil {
		return c.Open(c.Auth)
	}
	return credentials.OpenKeyring(c.Auth)
}

// withoutSentinel drops a generic sentinel from a joined error so the backend's own message
// is what the user reads. An error that is only the sentinel is returned unchanged.
func withoutSentinel(err, sentinel error) error {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return err
	}
	kept := []error{}
	for _, cause := range joined.Unwrap() {
		if !errors.Is(cause, sentinel) {
			kept = append(kept, cause)
		}
	}
	if len(kept) == 0 {
		return err
	}
	return errors.Join(kept...)
}
