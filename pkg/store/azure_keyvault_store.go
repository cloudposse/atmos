package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

const (
	statusCodeNotFound  = 404
	statusCodeForbidden = 403
	// AzureKeyVaultHyphen is the hyphen character used for Azure Key Vault secret name normalization.
	AzureKeyVaultHyphen = "-"
)

// Azure Key Vault secret names must match the pattern: ^[0-9a-zA-Z-]+$.
// They can only contain alphanumeric characters and hyphens.

// AzureKeyVaultClient interface allows us to mock the Azure Key Vault client.
type AzureKeyVaultClient interface {
	SetSecret(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error)
	GetSecret(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
}

// AzureKeyVaultStore is an implementation of the Store interface for Azure Key Vault.
type AzureKeyVaultStore struct {
	client         AzureKeyVaultClient
	vaultURL       string
	prefix         string
	stackDelimiter *string

	// Identity-based authentication fields.
	identityName string
	authResolver AuthContextResolver
	initOnce     sync.Once
	initErr      error
}

type AzureKeyVaultStoreOptions struct {
	VaultURL       string  `mapstructure:"vault_url"`
	Prefix         *string `mapstructure:"prefix"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
}

// Ensure AzureKeyVaultStore implements the store.Store and IdentityAwareStore interfaces.
var (
	_ Store              = (*AzureKeyVaultStore)(nil)
	_ IdentityAwareStore = (*AzureKeyVaultStore)(nil)
)

// NewAzureKeyVaultStore creates a new Azure Key Vault store.
// If identityName is non-empty, client initialization is deferred until first use (lazy init).
func NewAzureKeyVaultStore(options AzureKeyVaultStoreOptions, identityName string) (Store, error) {
	if options.VaultURL == "" {
		return nil, ErrVaultURLRequired
	}

	stackDelimiter := AzureKeyVaultHyphen
	if options.StackDelimiter != nil {
		stackDelimiter = *options.StackDelimiter
	}

	prefix := ""
	if options.Prefix != nil {
		prefix = *options.Prefix
	}

	store := &AzureKeyVaultStore{
		vaultURL:       options.VaultURL,
		prefix:         prefix,
		stackDelimiter: &stackDelimiter,
		identityName:   identityName,
	}

	// If no identity is configured, initialize the client eagerly (backward compatible behavior).
	if identityName == "" {
		if err := store.initDefaultClient(); err != nil {
			return nil, err
		}
	}

	return store, nil
}

// SetAuthContext implements IdentityAwareStore.
// If identityName is non-empty, it overrides the store's identity. Otherwise, the existing identity is preserved.
func (s *AzureKeyVaultStore) SetAuthContext(resolver AuthContextResolver, identityName string) {
	s.authResolver = resolver
	if identityName != "" {
		s.identityName = identityName
	}
}

// initDefaultClient initializes the Azure client using the default credential chain.
func (s *AzureKeyVaultStore) initDefaultClient() error {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrCreateClient, err)
	}

	client, err := azsecrets.NewClient(s.vaultURL, cred, nil)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrCreateClient, err)
	}

	s.client = client

	return nil
}

// initIdentityClient initializes the Azure client using identity-based credentials.
func (s *AzureKeyVaultStore) initIdentityClient() error {
	if s.authResolver == nil {
		return fmt.Errorf("%w: store requires identity %q but no auth resolver was injected", ErrIdentityNotConfigured, s.identityName)
	}

	ctx := context.TODO()
	authContext, err := s.authResolver.ResolveAzureAuthContext(ctx, s.identityName)
	if err != nil {
		return fmt.Errorf("%w: failed to resolve Azure auth context for identity %q: %w", ErrAuthContextNotAvailable, s.identityName, err)
	}

	// Create credentials from the Azure auth context with tenant hint if available.
	options := &azidentity.DefaultAzureCredentialOptions{}
	if authContext.TenantID != "" {
		options.TenantID = authContext.TenantID
	}

	cred, err := azidentity.NewDefaultAzureCredential(options)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrCreateClient, err)
	}

	client, err := azsecrets.NewClient(s.vaultURL, cred, nil)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrCreateClient, err)
	}

	s.client = client

	return nil
}

// ensureClient lazily initializes the Azure client if it hasn't been initialized yet.
func (s *AzureKeyVaultStore) ensureClient() error {
	if s.client != nil {
		return nil
	}

	s.initOnce.Do(func() {
		if s.identityName == "" {
			s.initErr = s.initDefaultClient()
		} else {
			s.initErr = s.initIdentityClient()
		}
	})

	return s.initErr
}

// normalizeSecretName converts a key path to a valid Azure Key Vault secret name.
// Azure Key Vault secret names must only contain alphanumeric characters and hyphens.
func (s *AzureKeyVaultStore) normalizeSecretName(key string) string {
	// Replace any non-alphanumeric characters with hyphens.
	normalized := regexp.MustCompile(`[^0-9a-zA-Z-]`).ReplaceAllString(key, AzureKeyVaultHyphen)
	// Replace multiple consecutive hyphens with a single hyphen.
	normalized = regexp.MustCompile(`-+`).ReplaceAllString(normalized, AzureKeyVaultHyphen)
	// Remove leading and trailing hyphens.
	normalized = strings.Trim(normalized, AzureKeyVaultHyphen)
	// Ensure the name is not empty.
	if normalized == "" {
		normalized = "default"
	}
	return normalized
}

func (s *AzureKeyVaultStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", ErrStackDelimiterNotSet
	}

	baseKey, err := getKey(s.prefix, *s.stackDelimiter, stack, component, key, AzureKeyVaultHyphen)
	if err != nil {
		return "", fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	// Normalize the key to comply with Azure Key Vault naming restrictions.
	return s.normalizeSecretName(baseKey), nil
}

func (s *AzureKeyVaultStore) Set(stack string, component string, key string, value interface{}) error {
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

	if err := s.ensureClient(); err != nil {
		return err
	}

	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	// Convert value to JSON string like other stores.
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrSerializeJSON, err)
	}
	strValue := string(jsonValue)

	params := azsecrets.SetSecretParameters{
		Value: &strValue,
	}

	_, err = s.client.SetSecret(context.Background(), secretName, params, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == statusCodeForbidden {
			return fmt.Errorf(errWrapFormatWithID, ErrPermissionDenied, fmt.Sprintf("secret %s", secretName), err)
		}
		return fmt.Errorf(errWrapFormat, ErrSetParameter, err)
	}

	return nil
}

func (s *AzureKeyVaultStore) Get(stack string, component string, key string) (interface{}, error) {
	if stack == "" {
		return nil, ErrEmptyStack
	}
	if component == "" {
		return nil, ErrEmptyComponent
	}
	if key == "" {
		return nil, ErrEmptyKey
	}

	if err := s.ensureClient(); err != nil {
		return nil, err
	}

	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	resp, err := s.client.GetSecret(context.Background(), secretName, "", nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			switch respErr.StatusCode {
			case statusCodeNotFound:
				return nil, fmt.Errorf(errWrapFormatWithID, ErrResourceNotFound, secretName, err)
			case statusCodeForbidden:
				return nil, fmt.Errorf(errWrapFormatWithID, ErrPermissionDenied, fmt.Sprintf("secret %s", secretName), err)
			}
		}
		return nil, fmt.Errorf(errWrapFormat, ErrAccessSecret, err)
	}

	if resp.Value == nil {
		return "", nil
	}

	// Try to unmarshal as JSON first, fallback to string if it fails.
	var result interface{}
	if jsonErr := json.Unmarshal([]byte(*resp.Value), &result); jsonErr != nil {
		// If JSON unmarshaling fails, return as string.
		return *resp.Value, nil
	}
	return result, nil
}

func (s *AzureKeyVaultStore) GetKey(key string) (interface{}, error) {
	if key == "" {
		return nil, ErrEmptyKey
	}

	if err := s.ensureClient(); err != nil {
		return nil, err
	}

	// Normalize the key to comply with Azure Key Vault naming restrictions.
	secretName := s.normalizeSecretName(key)

	resp, err := s.client.GetSecret(context.Background(), secretName, "", nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			switch respErr.StatusCode {
			case statusCodeNotFound:
				return nil, fmt.Errorf(errWrapFormatWithID, ErrResourceNotFound, secretName, err)
			case statusCodeForbidden:
				return nil, fmt.Errorf(errWrapFormatWithID, ErrPermissionDenied, fmt.Sprintf("secret %s", secretName), err)
			}
		}
		return nil, fmt.Errorf(errWrapFormat, ErrAccessSecret, err)
	}

	if resp.Value == nil {
		return "", nil
	}

	// Try to unmarshal as JSON first, fallback to string if it fails.
	var result interface{}
	if jsonErr := json.Unmarshal([]byte(*resp.Value), &result); jsonErr != nil {
		// If JSON unmarshaling fails, return as string.
		return *resp.Value, nil
	}
	return result, nil
}
