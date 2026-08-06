package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/cloudposse/atmos/pkg/store"
)

const (
	statusCodeNotFound  = 404
	statusCodeForbidden = 403
	// AzureKeyVaultHyphen is the hyphen character used for Azure Key Vault secret name normalization.
	AzureKeyVaultHyphen = "-"
	// Format string that turns a secret name into the identifier used in wrapped permission errors.
	secretIDFormat = "secret %s"
)

// Azure Key Vault secret names must match the pattern: ^[0-9a-zA-Z-]+$.
// They can only contain alphanumeric characters and hyphens.

// AzureKeyVaultClient interface allows us to mock the Azure Key Vault client.
type AzureKeyVaultClient interface {
	SetSecret(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error)
	GetSecret(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
	DeleteSecret(ctx context.Context, name string, options *azsecrets.DeleteSecretOptions) (azsecrets.DeleteSecretResponse, error)
	// NewListSecretPropertiesVersionsPager lists the versions/properties of a single named secret
	// without ever returning the secret value, so existence can be confirmed via the "list"
	// permission instead of "get". Has() uses this to avoid retrieving the secret value.
	NewListSecretPropertiesVersionsPager(name string, options *azsecrets.ListSecretPropertiesVersionsOptions) *runtime.Pager[azsecrets.ListSecretPropertiesVersionsResponse]
	// NewListSecretPropertiesPager lists every secret's properties in the vault (metadata only,
	// no values). Azure Key Vault has no server-side name-prefix filter, so Keys uses this to
	// list everything and filters client-side.
	NewListSecretPropertiesPager(options *azsecrets.ListSecretPropertiesOptions) *runtime.Pager[azsecrets.ListSecretPropertiesResponse]
}

// AzureKeyVaultStore is an implementation of the store.Store interface for Azure Key Vault.
type AzureKeyVaultStore struct {
	client         AzureKeyVaultClient
	vaultURL       string
	prefix         string
	stackDelimiter *string
	clientOptions  *azsecrets.ClientOptions
	withoutAuth    bool
	secret         bool

	// Identity-based authentication fields.
	identityName string
	authResolver store.AuthContextResolver
	initOnce     sync.Once
	initErr      error
}

type AzureKeyVaultStoreOptions struct {
	VaultURL                             string  `mapstructure:"vault_url"`
	Endpoint                             *string `mapstructure:"endpoint"`
	Prefix                               *string `mapstructure:"prefix"`
	StackDelimiter                       *string `mapstructure:"stack_delimiter"`
	DisableChallengeResourceVerification bool    `mapstructure:"disable_challenge_resource_verification"`
	WithoutAuthentication                bool    `mapstructure:"without_authentication"`
	InsecureAllowCredentialWithHTTP      bool    `mapstructure:"insecure_allow_credential_with_http"`
	EndpointInsecure                     bool    `mapstructure:"endpoint_insecure"`
}

// Ensure AzureKeyVaultStore implements the store.Store, store.IdentityAwareStore,
// store.DeletableStore, and store.StatusStore interfaces.
var (
	_ store.Store              = (*AzureKeyVaultStore)(nil)
	_ store.IdentityAwareStore = (*AzureKeyVaultStore)(nil)
	_ store.DeletableStore     = (*AzureKeyVaultStore)(nil)
	_ store.StatusStore        = (*AzureKeyVaultStore)(nil)
	_ store.ListableStore      = (*AzureKeyVaultStore)(nil)
	_ store.SecretAwareStore   = (*AzureKeyVaultStore)(nil)
)

// NewAzureKeyVaultStore creates a new Azure Key Vault store.
// If identityName is non-empty, client initialization is deferred until first use (lazy init).
func NewAzureKeyVaultStore(options AzureKeyVaultStoreOptions, identityName string) (store.Store, error) {
	vaultURL := options.VaultURL
	if vaultURL == "" {
		vaultURL = firstNonEmptyStringPtr(options.Endpoint)
	}
	if vaultURL == "" {
		return nil, store.ErrVaultURLRequired
	}
	clientOptions := &azsecrets.ClientOptions{
		DisableChallengeResourceVerification: options.DisableChallengeResourceVerification,
		ClientOptions: azcore.ClientOptions{
			InsecureAllowCredentialWithHTTP: options.InsecureAllowCredentialWithHTTP || options.WithoutAuthentication,
		},
	}
	if options.EndpointInsecure && strings.HasPrefix(vaultURL, "http://") {
		vaultURL = "https://" + strings.TrimPrefix(vaultURL, "http://")
		clientOptions.Transport = azureInsecureEndpointTransport{base: http.DefaultClient}
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
		vaultURL:       vaultURL,
		prefix:         prefix,
		stackDelimiter: &stackDelimiter,
		clientOptions:  clientOptions,
		withoutAuth:    options.WithoutAuthentication,
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

// SetAuthContext implements store.IdentityAwareStore.
// If identityName is non-empty, it overrides the store's identity. Otherwise, the existing identity is preserved.
func (s *AzureKeyVaultStore) SetAuthContext(resolver store.AuthContextResolver, identityName string) {
	s.authResolver = resolver
	if identityName != "" && s.identityName != identityName {
		s.identityName = identityName
		s.client = nil
		s.initOnce = sync.Once{}
		s.initErr = nil
	}
}

// IdentityName returns the configured identity name, if any.
func (s *AzureKeyVaultStore) IdentityName() string {
	return s.identityName
}

// initDefaultClient initializes the Azure client using the default credential chain.
func (s *AzureKeyVaultStore) initDefaultClient() error {
	cred, err := s.defaultCredential(nil)
	if err != nil {
		return fmt.Errorf(errWrapFormat, store.ErrCreateClient, err)
	}

	client, err := azsecrets.NewClient(s.vaultURL, cred, s.clientOptions)
	if err != nil {
		return fmt.Errorf(errWrapFormat, store.ErrCreateClient, err)
	}

	s.client = client

	return nil
}

// initIdentityClient initializes the Azure client using identity-based credentials.
func (s *AzureKeyVaultStore) initIdentityClient() error {
	if s.authResolver == nil {
		return fmt.Errorf("%w: store requires identity %q but no auth resolver was injected", store.ErrIdentityNotConfigured, s.identityName)
	}

	ctx := context.TODO()
	authContext, err := s.authResolver.ResolveAzureAuthContext(ctx, s.identityName)
	if err != nil {
		return fmt.Errorf("%w: failed to resolve Azure auth context for identity %q: %w", store.ErrAuthContextNotAvailable, s.identityName, err)
	}

	// Create credentials from the Azure auth context with tenant hint if available.
	options := &azidentity.DefaultAzureCredentialOptions{}
	if authContext.TenantID != "" {
		options.TenantID = authContext.TenantID
	}

	cred, err := s.defaultCredential(options)
	if err != nil {
		return fmt.Errorf(errWrapFormat, store.ErrCreateClient, err)
	}

	client, err := azsecrets.NewClient(s.vaultURL, cred, s.clientOptions)
	if err != nil {
		return fmt.Errorf(errWrapFormat, store.ErrCreateClient, err)
	}

	s.client = client

	return nil
}

// defaultCredential returns the token credential used to authenticate the Key Vault
// client: a local stub when the store is configured withoutAuth (for Floci/local
// testing), otherwise Azure's default credential chain.
func (s *AzureKeyVaultStore) defaultCredential(options *azidentity.DefaultAzureCredentialOptions) (azcore.TokenCredential, error) {
	if s.withoutAuth {
		return localAzureTokenCredential{}, nil
	}
	return azidentity.NewDefaultAzureCredential(options)
}

// localAzureTokenCredential is a stub azcore.TokenCredential that returns a fixed,
// non-functional token for local/no-auth Key Vault testing against an emulator.
type localAzureTokenCredential struct{}

func (localAzureTokenCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{
		Token:     "floci-local-token",
		ExpiresOn: time.Now().Add(time.Hour),
	}, nil
}

// azureInsecureEndpointTransport wraps an HTTP transport and rewrites request URLs
// from https to http so the Key Vault client can talk to an insecure local endpoint.
type azureInsecureEndpointTransport struct {
	base policy.Transporter
}

// Do rewrites the request scheme to http and forwards it to the wrapped transport,
// falling back to http.DefaultClient when no base transport is set.
func (t azureInsecureEndpointTransport) Do(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	base := t.base
	if base == nil {
		base = http.DefaultClient
	}
	return base.Do(clone)
}

// ensureClient lazily initializes the Azure client if it hasn't been initialized yet.
// The client-nil check is inside initOnce.Do to avoid a data race between the
// unsynchronized read and the write that happens inside Do on another goroutine.
func (s *AzureKeyVaultStore) ensureClient() error {
	s.initOnce.Do(func() {
		if s.client != nil {
			return // Already initialized (eager init path).
		}
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
		return "", store.ErrStackDelimiterNotSet
	}

	baseKey, err := getKey(s.prefix, *s.stackDelimiter, stack, component, key, AzureKeyVaultHyphen)
	if err != nil {
		return "", fmt.Errorf(errWrapFormat, store.ErrGetKey, err)
	}

	// Normalize the key to comply with Azure Key Vault naming restrictions.
	return s.normalizeSecretName(baseKey), nil
}

// Set writes a value for an Atmos secret coordinate. Empty stack and component segments are
// valid for stack-scoped and global secrets and are omitted by getKey.
func (s *AzureKeyVaultStore) Set(stack string, component string, key string, value interface{}) error {
	if key == "" {
		return store.ErrEmptyKey
	}
	if value == nil {
		return fmt.Errorf("%w for key %s in stack %s component %s", store.ErrNilValue, key, stack, component)
	}

	if err := s.ensureClient(); err != nil {
		return err
	}

	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, store.ErrGetKey, err)
	}

	strValue, err := marshalAzureSecretValue(value, s.secret)
	if err != nil {
		return fmt.Errorf(errWrapFormat, store.ErrSerializeJSON, err)
	}

	params := azsecrets.SetSecretParameters{
		Value: &strValue,
	}

	_, err = s.client.SetSecret(context.Background(), secretName, params, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == statusCodeForbidden {
			return fmt.Errorf(errWrapFormatWithID, store.ErrPermissionDenied, fmt.Sprintf(secretIDFormat, secretName), err)
		}
		return fmt.Errorf(errWrapFormat, store.ErrSetParameter, err)
	}

	return nil
}

// SetSecret implements SecretAwareStore. Secret string values are written verbatim so
// `atmos secret set` round-trips through `!secret ... | raw`; structured values remain JSON.
func (s *AzureKeyVaultStore) SetSecret(secret bool) {
	s.secret = secret
}

func marshalAzureSecretValue(value any, rawStrings bool) (string, error) {
	if rawStrings {
		if text, ok := value.(string); ok {
			return text, nil
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (s *AzureKeyVaultStore) Get(stack string, component string, key string) (interface{}, error) {
	raw, err := s.GetRaw(stack, component, key)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if jsonErr := json.Unmarshal([]byte(raw), &result); jsonErr != nil {
		return raw, nil
	}
	return result, nil
}

// GetRaw retrieves the original Key Vault secret string without JSON decoding. Empty stack and
// component segments are valid for stack-scoped and global secrets and are omitted by getKey.
func (s *AzureKeyVaultStore) GetRaw(stack string, component string, key string) (string, error) {
	if key == "" {
		return "", store.ErrEmptyKey
	}

	if err := s.ensureClient(); err != nil {
		return "", err
	}

	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return "", fmt.Errorf(errWrapFormat, store.ErrGetKey, err)
	}
	return s.getRawByName(secretName)
}

func (s *AzureKeyVaultStore) getRawByName(secretName string) (string, error) {
	resp, err := s.client.GetSecret(context.Background(), secretName, "", nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			switch respErr.StatusCode {
			case statusCodeNotFound:
				return "", fmt.Errorf(errWrapFormatWithID, store.ErrResourceNotFound, secretName, err)
			case statusCodeForbidden:
				return "", fmt.Errorf(errWrapFormatWithID, store.ErrPermissionDenied, fmt.Sprintf(secretIDFormat, secretName), err)
			}
		}
		return "", fmt.Errorf(errWrapFormat, store.ErrAccessSecret, err)
	}

	if resp.Value == nil {
		return "", nil
	}

	return *resp.Value, nil
}

// Delete removes a secret from Azure Key Vault for the given stack, component, and key. Empty
// stack and component segments are valid for stack-scoped and global secrets.
func (s *AzureKeyVaultStore) Delete(stack string, component string, key string) error {
	if key == "" {
		return store.ErrEmptyKey
	}

	if err := s.ensureClient(); err != nil {
		return err
	}

	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, store.ErrGetKey, err)
	}

	_, err = s.client.DeleteSecret(context.Background(), secretName, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			switch respErr.StatusCode {
			case statusCodeNotFound:
				return fmt.Errorf(errWrapFormatWithID, store.ErrResourceNotFound, secretName, err)
			case statusCodeForbidden:
				return fmt.Errorf(errWrapFormatWithID, store.ErrPermissionDenied, fmt.Sprintf(secretIDFormat, secretName), err)
			}
		}
		return fmt.Errorf(errWrapFormatWithID, store.ErrDeleteSecret, secretName, err)
	}

	return nil
}

// Has reports whether a secret exists for the given stack, component, and key.
//
// It uses the secret-versions listing API (NewListSecretPropertiesVersionsPager), which returns
// only secret metadata/properties and never the secret value. Existence is therefore confirmed
// without retrieving the value: a non-existent secret yields a 404 mapped to (false, nil), while
// any other error (e.g. permission denied) is wrapped and returned. Note that Azure Key Vault
// secrets have no separate "decrypt" permission distinct from "get"; the versions listing relies
// on the "list" permission and is the lightest existence check that avoids reading the value.
func (s *AzureKeyVaultStore) Has(stack string, component string, key string) (bool, error) {
	if key == "" {
		return false, store.ErrEmptyKey
	}

	if err := s.ensureClient(); err != nil {
		return false, err
	}

	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return false, fmt.Errorf(errWrapFormat, store.ErrGetKey, err)
	}

	// Fetch only the first page of secret versions. We never read page contents (the secret
	// value is not part of this response); a successful fetch is sufficient to prove existence.
	pager := s.client.NewListSecretPropertiesVersionsPager(secretName, nil)
	if _, err := pager.NextPage(context.Background()); err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			switch respErr.StatusCode {
			case statusCodeNotFound:
				return false, nil
			case statusCodeForbidden:
				return false, fmt.Errorf(errWrapFormatWithID, store.ErrPermissionDenied, fmt.Sprintf(secretIDFormat, secretName), err)
			}
		}
		return false, fmt.Errorf(errWrapFormat, store.ErrAccessSecret, err)
	}

	return true, nil
}

// matchSecretName reports the listed key name for a secret's properties, trimming prefix and
// reporting false when the item has no usable ID or its name does not match prefix. Factored out
// of Keys to keep its cyclomatic complexity within the linter's limit.
func matchSecretName(item *azsecrets.SecretProperties, prefix string) (string, bool) {
	if item == nil || item.ID == nil {
		return "", false
	}
	name := item.ID.Name()
	if prefix == "" {
		return name, true
	}
	return strings.CutPrefix(name, prefix)
}

// Keys lists the secret names under a stack/component scope (or globally when both are empty).
// Azure Key Vault has no server-side name-prefix filter parameter, so this lists every secret in
// the vault and filters client-side by the normalized prefix.
func (s *AzureKeyVaultStore) Keys(stack string, component string) ([]string, error) {
	if s.stackDelimiter == nil {
		return nil, store.ErrStackDelimiterNotSet
	}

	if err := s.ensureClient(); err != nil {
		return nil, err
	}

	rawPrefix := getKeyPrefix(s.prefix, *s.stackDelimiter, stack, component, AzureKeyVaultHyphen)
	prefix := ""
	if rawPrefix != "" {
		prefix = s.normalizeSecretName(rawPrefix) + AzureKeyVaultHyphen
	}

	ctx := context.Background()
	var names []string
	pager := s.client.NewListSecretPropertiesPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf(errWrapFormat, store.ErrListSecretProperties, err)
		}
		for _, item := range page.Value {
			if name, ok := matchSecretName(item, prefix); ok {
				names = append(names, name)
			}
		}
	}

	return names, nil
}

func (s *AzureKeyVaultStore) GetKey(key string) (interface{}, error) {
	if key == "" {
		return nil, store.ErrEmptyKey
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
				return nil, fmt.Errorf(errWrapFormatWithID, store.ErrResourceNotFound, secretName, err)
			case statusCodeForbidden:
				return nil, fmt.Errorf(errWrapFormatWithID, store.ErrPermissionDenied, fmt.Sprintf(secretIDFormat, secretName), err)
			}
		}
		return nil, fmt.Errorf(errWrapFormat, store.ErrAccessSecret, err)
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

func init() {
	store.Register(store.KindAzureKeyVault, buildAzureKeyVaultStore)
}

// buildAzureKeyVaultStore is the store.StoreFactory for Azure Key Vault stores.
func buildAzureKeyVaultStore(_ string, config store.StoreConfig) (store.Store, error) {
	var opts AzureKeyVaultStoreOptions
	if err := parseOptions(config.Options, &opts); err != nil {
		return nil, fmt.Errorf(errFormat, store.ErrParseAzureKeyVaultOptions, err)
	}

	return NewAzureKeyVaultStore(opts, config.Identity)
}
