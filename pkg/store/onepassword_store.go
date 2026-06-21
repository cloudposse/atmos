package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

// opIntegrationName/opIntegrationVersion identify Atmos to the 1Password SDK telemetry.
const (
	opIntegrationName    = "Atmos"
	opIntegrationVersion = "1.0.0"
	opReferenceScheme    = "op://"
)

// 1Password auth-mode selectors.
const (
	opModeAuto           = "auto"
	opModeConnect        = "connect"
	opModeServiceAccount = "service-account"
)

// OnePasswordStoreOptions configures a 1Password store. Addressing is reference-based: each
// declared secret carries an `op://...` reference (optionally Go-templated), so there is no
// prefix/stack-delimiter key composition like the other stores.
type OnePasswordStoreOptions struct {
	// Mode selects the integration backend: "auto" (default), "connect", or "service-account".
	Mode string `mapstructure:"mode"`
	// Token is the service-account token; falls back to OP_SERVICE_ACCOUNT_TOKEN.
	Token string `mapstructure:"token"`
	// ConnectHost is the 1Password Connect server URL; falls back to OP_CONNECT_HOST.
	ConnectHost string `mapstructure:"connect_host"`
	// ConnectToken is the 1Password Connect API token; falls back to OP_CONNECT_TOKEN.
	ConnectToken string `mapstructure:"connect_token"`
	// Vault optionally supplies a default vault, letting references omit the scheme and vault
	// (e.g. `Datadog/api_key` becomes `op://<vault>/Datadog/api_key`).
	Vault string `mapstructure:"vault"`
}

// OnePasswordStore implements the Store interface backed by 1Password. It resolves templated
// `op://` references via either the native SDK (service account) or Connect (REST). Writes
// (Set/Delete) create/update/remove the field the reference points to (creating the item if
// needed); created items use the API Credential category with a Concealed value field.
type OnePasswordStore struct {
	options OnePasswordStoreOptions
	vault   string

	// The client is built lazily on first use so that merely declaring a 1Password store does
	// not require credentials at config-load time (only resolving a secret does). Tests inject a
	// client directly, which the lazy initializer preserves.
	once    sync.Once
	client  onePasswordClient
	initErr error
}

// Ensure OnePasswordStore implements the expected interfaces.
var (
	_ Store          = (*OnePasswordStore)(nil)
	_ StatusStore    = (*OnePasswordStore)(nil)
	_ DeletableStore = (*OnePasswordStore)(nil)
)

// NewOnePasswordStore initializes a 1Password store. Credential selection is deferred until the
// first secret resolution (see getClient), so this never fails for missing credentials.
func NewOnePasswordStore(options *OnePasswordStoreOptions) (Store, error) {
	return &OnePasswordStore{options: *options, vault: options.Vault}, nil
}

// getClient lazily builds the backend client from options/environment, preserving an
// already-injected client (tests).
func (s *OnePasswordStore) getClient() (onePasswordClient, error) {
	s.once.Do(func() {
		if s.client == nil {
			s.client, s.initErr = newOnePasswordClient(&s.options)
		}
	})
	return s.client, s.initErr
}

// opCredentials holds the resolved 1Password credentials (options override environment).
type opCredentials struct {
	connectHost  string
	connectToken string
	saToken      string
}

// resolveOPCredentials reads credentials from options, falling back to the canonical OP_* vars.
// Precedence for the service-account token: explicit options.token > OP_SERVICE_ACCOUNT_TOKEN.
// To source the token from another store (e.g. the keychain) without an env var, set
// `options.token: !store.get <store> <KEY>` — resolved lazily via the store-reference resolver.
func resolveOPCredentials(options *OnePasswordStoreOptions) opCredentials {
	return opCredentials{
		connectHost:  firstNonEmpty(options.ConnectHost, opEnv("OP_CONNECT_HOST")),
		connectToken: firstNonEmpty(options.ConnectToken, opEnv("OP_CONNECT_TOKEN")),
		saToken:      firstNonEmpty(options.Token, opEnv("OP_SERVICE_ACCOUNT_TOKEN")),
	}
}

func (c opCredentials) hasConnect() bool { return c.connectHost != "" && c.connectToken != "" }

// autoClient picks Connect when its credentials are present (the CI/cloud convention), otherwise
// a service account, erroring when neither is configured.
func (c opCredentials) autoClient() (onePasswordClient, error) {
	if c.hasConnect() {
		return newConnectClient(c.connectHost, c.connectToken), nil
	}
	if c.saToken != "" {
		return newSDKClient(c.saToken), nil
	}
	return nil, ErrOnePasswordNoAuth
}

// newOnePasswordClient resolves the auth mode and returns the matching client. An explicit mode
// forces one backend and errors if its credentials are missing; "auto" prefers Connect.
func newOnePasswordClient(options *OnePasswordStoreOptions) (onePasswordClient, error) {
	creds := resolveOPCredentials(options)

	mode := options.Mode
	if mode == "" {
		mode = opModeAuto
	}

	switch mode {
	case opModeConnect:
		if !creds.hasConnect() {
			return nil, ErrOnePasswordNoAuth
		}
		return newConnectClient(creds.connectHost, creds.connectToken), nil
	case opModeServiceAccount:
		if creds.saToken == "" {
			return nil, ErrOnePasswordNoAuth
		}
		return newSDKClient(creds.saToken), nil
	case opModeAuto:
		return creds.autoClient()
	default:
		return nil, fmt.Errorf("%w: %q", ErrOnePasswordUnknownMode, mode)
	}
}

// opEnv reads a 1Password environment variable (the canonical OP_* names).
func opEnv(name string) string {
	//nolint:forbidigo // OP_* are the standard 1Password integration environment variables.
	return os.Getenv(name)
}

// Set creates or updates the field the templated reference (carried as `key`) points to,
// creating the 1Password item if it does not yet exist.
func (s *OnePasswordStore) Set(stack string, component string, key string, value any) error {
	if value == nil {
		return ErrNilValue
	}
	reference, err := s.referenceFor(key, stack, component)
	if err != nil {
		return err
	}
	strValue, err := opStringValue(value)
	if err != nil {
		return err
	}
	client, err := s.getClient()
	if err != nil {
		return err
	}
	if err := client.Set(context.TODO(), reference, strValue); err != nil {
		return fmt.Errorf(errWrapFormatWithID, ErrOnePasswordWrite, reference, err)
	}
	return nil
}

// Get resolves the templated reference (carried as `key`) for the given stack/component.
func (s *OnePasswordStore) Get(stack string, component string, key string) (any, error) {
	reference, err := s.referenceFor(key, stack, component)
	if err != nil {
		return nil, err
	}
	return s.resolve(reference)
}

// GetKey resolves a raw reference without stack/component context (templated vars render empty).
func (s *OnePasswordStore) GetKey(key string) (any, error) {
	reference, err := s.referenceFor(key, "", "")
	if err != nil {
		return nil, err
	}
	return s.resolve(reference)
}

// Delete removes the field the templated reference points to (deleting the item if it becomes
// empty). It is idempotent: a missing vault/item/field is not an error.
func (s *OnePasswordStore) Delete(stack string, component string, key string) error {
	reference, err := s.referenceFor(key, stack, component)
	if err != nil {
		return err
	}
	client, err := s.getClient()
	if err != nil {
		return err
	}
	if err := client.Delete(context.TODO(), reference); err != nil {
		return fmt.Errorf(errWrapFormatWithID, ErrOnePasswordDelete, reference, err)
	}
	return nil
}

// onePasswordExistenceChecker is an optional capability a onePasswordClient may implement to
// answer existence queries without revealing the secret value (e.g. checking item/field metadata
// rather than resolving the plaintext). Has prefers it when available and otherwise falls back to
// a value-resolving probe.
//
// It lives in the store (not the client interface) so that adding it does not force every client
// implementation to change at once; clients opt in by implementing Exists.
type onePasswordExistenceChecker interface {
	// Exists reports whether the referenced secret exists without returning/revealing its value.
	// A missing vault/item/field returns (false, nil); auth/transport failures return an error.
	Exists(ctx context.Context, reference string) (bool, error)
}

// Has reports whether the referenced secret exists, treating a not-found reference as absence
// while propagating auth/transport errors. When the underlying client supports a metadata-only
// existence check (onePasswordExistenceChecker), Has uses it so that probing for existence does
// not reveal/retrieve the secret value. Otherwise it falls back to a value-resolving probe.
//
// Limitation: the production SDK and Connect clients do not yet implement Exists, so for those
// backends Has currently still resolves the value. The no-reveal path is exercised by clients
// that opt into onePasswordExistenceChecker.
func (s *OnePasswordStore) Has(stack string, component string, key string) (bool, error) {
	reference, err := s.referenceFor(key, stack, component)
	if err != nil {
		return false, err
	}
	client, err := s.getClient()
	if err != nil {
		return false, err
	}

	// Prefer a metadata-only existence check that does not reveal the secret value.
	if checker, ok := client.(onePasswordExistenceChecker); ok {
		exists, existsErr := checker.Exists(context.TODO(), reference)
		if existsErr != nil {
			if errors.Is(existsErr, ErrOnePasswordNotFound) {
				return false, nil
			}
			return false, fmt.Errorf(errWrapFormatWithID, ErrOnePasswordResolve, reference, existsErr)
		}
		return exists, nil
	}

	// Fallback: resolve the value, mapping a not-found reference to absence.
	if _, err := s.resolve(reference); err != nil {
		if errors.Is(err, ErrOnePasswordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// referenceFor renders the templated reference for a stack/component scope into a full `op://`
// reference.
func (s *OnePasswordStore) referenceFor(key, stack, component string) (string, error) {
	if key == "" {
		return "", ErrEmptyKey
	}
	return s.renderReference(key, map[string]any{
		"atmos_stack":     stack,
		"atmos_component": component,
	})
}

// resolve resolves a rendered reference through the selected client.
func (s *OnePasswordStore) resolve(reference string) (any, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	value, err := client.Resolve(context.TODO(), reference)
	if err != nil {
		if errors.Is(err, ErrOnePasswordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf(errWrapFormatWithID, ErrOnePasswordResolve, reference, err)
	}
	return value, nil
}

// opStringValue converts a secret value to the string stored in a Concealed field. Strings and
// byte slices pass through; other types are JSON-encoded.
func opStringValue(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		b, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("%w: %w", ErrOnePasswordWrite, err)
		}
		return string(b), nil
	}
}

// renderReference renders the reference as a Go template (with sprig funcs and strict missing-key
// handling, mirroring the SOPS provider) and normalizes it to a full `op://` reference.
func (s *OnePasswordStore) renderReference(raw string, data map[string]any) (string, error) {
	tmpl, err := template.New("op-reference").Funcs(sprig.FuncMap()).Option("missingkey=error").Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w: invalid `reference` template %q: %w", ErrOnePasswordReferenceTemplate, raw, err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("%w: rendering `reference` %q: %w", ErrOnePasswordReferenceTemplate, raw, err)
	}

	reference := strings.TrimSpace(buf.String())
	if reference == "" {
		return "", fmt.Errorf("%w: reference rendered empty", ErrOnePasswordInvalidReference)
	}
	if strings.HasPrefix(reference, opReferenceScheme) {
		return reference, nil
	}
	// Allow vault-relative references when a default vault is configured.
	if s.vault != "" {
		return opReferenceScheme + s.vault + "/" + strings.TrimPrefix(reference, "/"), nil
	}
	return "", fmt.Errorf("%w: %q is not an `op://vault/item/field` reference and no default `vault` is set",
		ErrOnePasswordInvalidReference, reference)
}
