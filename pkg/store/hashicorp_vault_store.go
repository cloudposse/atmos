package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

// vaultValueKey is the field used to store an Atmos value within a Vault KV v2 secret. Each
// Atmos (stack, component, key) maps to a distinct KV path holding a single "value" field.
const vaultValueKey = "value"

// vaultHTTPNotFound is the HTTP status Vault returns for a missing secret.
const vaultHTTPNotFound = 404

// VaultStore is an implementation of the Store interface for HashiCorp Vault (KV v2).
type VaultStore struct {
	client         VaultKVClient
	mount          string
	prefix         string
	stackDelimiter *string

	// Identity-based authentication fields (reserved for KMS/cloud auth methods).
	identityName string
	authResolver AuthContextResolver
}

// VaultStoreOptions configures a HashiCorp Vault store.
type VaultStoreOptions struct {
	URL            string  `mapstructure:"url"`
	Address        string  `mapstructure:"address"`
	Token          string  `mapstructure:"token"`
	Mount          string  `mapstructure:"mount"`
	Path           string  `mapstructure:"path"`
	Prefix         *string `mapstructure:"prefix"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
}

// VaultKVClient abstracts the Vault KV v2 operations the store needs (for testability).
type VaultKVClient interface {
	Put(ctx context.Context, path string, data map[string]any) error
	Get(ctx context.Context, path string) (map[string]any, error)
	Delete(ctx context.Context, path string) error
}

// Ensure VaultStore implements the expected interfaces.
var (
	_ Store              = (*VaultStore)(nil)
	_ IdentityAwareStore = (*VaultStore)(nil)
	_ DeletableStore     = (*VaultStore)(nil)
	_ StatusStore        = (*VaultStore)(nil)
)

// vaultKVv2Client adapts the official Vault KV v2 helper to VaultKVClient.
type vaultKVv2Client struct {
	kv *vault.KVv2
}

func (c *vaultKVv2Client) Put(ctx context.Context, path string, data map[string]any) error {
	_, err := c.kv.Put(ctx, path, data)
	return err
}

func (c *vaultKVv2Client) Get(ctx context.Context, path string) (map[string]any, error) {
	secret, err := c.kv.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	if secret == nil {
		return nil, nil
	}
	return secret.Data, nil
}

func (c *vaultKVv2Client) Delete(ctx context.Context, path string) error {
	return c.kv.Delete(ctx, path)
}

// NewVaultStore initializes a new VaultStore using token authentication. The address may be
// supplied via options or the standard VAULT_ADDR environment variable (read by the Vault SDK);
// the token may be supplied via options or the standard VAULT_TOKEN environment variable.
func NewVaultStore(options *VaultStoreOptions, identityName string) (Store, error) {
	mount := options.Mount
	if mount == "" {
		mount = options.Path // back-compat: allow `path` as the KV mount.
	}
	if mount == "" {
		return nil, ErrVaultMountRequired
	}

	// vault.DefaultConfig() reads VAULT_ADDR (and other VAULT_* settings) from the environment.
	cfg := vault.DefaultConfig()
	if addr := firstNonEmpty(options.Address, options.URL); addr != "" {
		cfg.Address = addr
	}
	if cfg.Address == "" {
		return nil, ErrVaultAddressRequired
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrCreateClient, err)
	}

	token := options.Token
	if token == "" {
		//nolint:forbidigo // VAULT_TOKEN is the standard HashiCorp Vault token environment variable.
		token = os.Getenv("VAULT_TOKEN")
	}
	if token != "" {
		client.SetToken(token)
	}

	s := &VaultStore{
		client:       &vaultKVv2Client{kv: client.KVv2(mount)},
		mount:        mount,
		identityName: identityName,
	}
	if options.Prefix != nil {
		s.prefix = *options.Prefix
	}
	if options.StackDelimiter != nil {
		s.stackDelimiter = options.StackDelimiter
	} else {
		delim := "/"
		s.stackDelimiter = &delim
	}

	return s, nil
}

// SetAuthContext implements IdentityAwareStore. Vault token auth needs no resolver, but the
// hook is kept for future cloud auth methods.
func (s *VaultStore) SetAuthContext(resolver AuthContextResolver, identityName string) {
	s.authResolver = resolver
	if identityName != "" {
		s.identityName = identityName
	}
}

func (s *VaultStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", ErrStackDelimiterNotSet
	}
	return getKey(s.prefix, *s.stackDelimiter, stack, component, key, "/")
}

// Set writes the value to a KV v2 path under a single "value" field.
func (s *VaultStore) Set(stack string, component string, key string, value any) error {
	if stack == "" {
		return ErrEmptyStack
	}
	if component == "" {
		return ErrEmptyComponent
	}
	if key == "" {
		return ErrEmptyKey
	}
	if value == nil {
		return fmt.Errorf("%w for key %s in stack %s component %s", ErrNilValue, key, stack, component)
	}

	path, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	if err := s.client.Put(context.TODO(), path, map[string]any{vaultValueKey: value}); err != nil {
		return fmt.Errorf(errWrapFormatWithID, ErrVaultWrite, path, err)
	}
	return nil
}

// Get reads the "value" field from a KV v2 path.
func (s *VaultStore) Get(stack string, component string, key string) (any, error) {
	if stack == "" {
		return nil, ErrEmptyStack
	}
	if component == "" {
		return nil, ErrEmptyComponent
	}
	if key == "" {
		return nil, ErrEmptyKey
	}

	path, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}
	return s.getByPath(path)
}

// GetKey reads the "value" field by a raw KV path (optionally prefixed).
func (s *VaultStore) GetKey(key string) (any, error) {
	if key == "" {
		return nil, ErrEmptyKey
	}
	path := key
	if s.prefix != "" {
		path = strings.TrimSuffix(s.prefix, "/") + "/" + strings.TrimPrefix(key, "/")
	}
	return s.getByPath(path)
}

func (s *VaultStore) getByPath(path string) (any, error) {
	data, err := s.client.Get(context.TODO(), path)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormatWithID, ErrVaultRead, path, err)
	}
	if data == nil {
		return nil, fmt.Errorf("%w '%s'", ErrVaultEmptyData, path)
	}
	if v, ok := data[vaultValueKey]; ok {
		return v, nil
	}
	// Fall back to returning the whole data map for secrets not written by Atmos.
	return data, nil
}

// Delete removes a KV v2 secret at the computed path.
func (s *VaultStore) Delete(stack string, component string, key string) error {
	if stack == "" {
		return ErrEmptyStack
	}
	if component == "" {
		return ErrEmptyComponent
	}
	if key == "" {
		return ErrEmptyKey
	}

	path, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}
	if err := s.client.Delete(context.TODO(), path); err != nil {
		return fmt.Errorf(errWrapFormatWithID, ErrVaultDelete, path, err)
	}
	return nil
}

// Has reports whether a secret exists at the computed path.
func (s *VaultStore) Has(stack string, component string, key string) (bool, error) {
	_, err := s.Get(stack, component, key)
	if err != nil {
		if errors.Is(err, ErrVaultEmptyData) || isVaultNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// isVaultNotFound reports whether the error chain indicates a missing Vault secret (404).
func isVaultNotFound(err error) bool {
	var respErr *vault.ResponseError
	if errors.As(err, &respErr) {
		return respErr.StatusCode == vaultHTTPNotFound
	}
	return false
}

// firstNonEmpty returns the first non-empty string from the arguments.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
