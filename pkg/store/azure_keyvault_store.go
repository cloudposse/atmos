package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

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
}

type AzureKeyVaultStoreOptions struct {
	VaultURL       string  `mapstructure:"vault_url"`
	Prefix         *string `mapstructure:"prefix"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
}

// Ensure AzureKeyVaultStore implements the store.Store interface.
var _ Store = (*AzureKeyVaultStore)(nil)

func NewAzureKeyVaultStore(options AzureKeyVaultStoreOptions) (Store, error) {
	if options.VaultURL == "" {
		return nil, ErrVaultURLRequired
	}

	// Create a credential using the default Azure credential chain.
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrCreateClient, err)
	}

	// Create the Key Vault client.
	client, err := azsecrets.NewClient(options.VaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrCreateClient, err)
	}

	stackDelimiter := AzureKeyVaultHyphen
	if options.StackDelimiter != nil {
		stackDelimiter = *options.StackDelimiter
	}

	prefix := ""
	if options.Prefix != nil {
		prefix = *options.Prefix
	}

	return &AzureKeyVaultStore{
		client:         client,
		vaultURL:       options.VaultURL,
		prefix:         prefix,
		stackDelimiter: &stackDelimiter,
	}, nil
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
		//nolint:nilerr // Intentionally ignoring JSON error - fallback to string value.
		return *resp.Value, nil
	}
	return result, nil
}

func (s *AzureKeyVaultStore) GetKey(key string) (interface{}, error) {
	if key == "" {
		return nil, ErrEmptyKey
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
		//nolint:nilerr // Intentionally ignoring JSON error - fallback to string value.
		return *resp.Value, nil
	}
	return result, nil
}
