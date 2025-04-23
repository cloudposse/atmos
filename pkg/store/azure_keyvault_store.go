package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

var invalidCharsRegex = regexp.MustCompile(`[^a-zA-Z0-9-]`)

// KeyVaultClient interface allows us to mock the Azure Key Vault client
type KeyVaultClient interface {
	SetSecret(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error)
	GetSecret(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
	DeleteSecret(ctx context.Context, name string, options *azsecrets.DeleteSecretOptions) (azsecrets.DeleteSecretResponse, error)
	NewListSecretPropertiesPager(options *azsecrets.ListSecretPropertiesOptions) *runtime.Pager[azsecrets.ListSecretPropertiesResponse]
}

// KeyVaultStore is an implementation of the Store interface for Azure Key Vault.
type KeyVaultStore struct {
	client         KeyVaultClient
	vaultURL       string
	prefix         string
	stackDelimiter *string
}

type KeyVaultStoreOptions struct {
	VaultURL       string  `mapstructure:"vault_url"`
	Prefix         *string `mapstructure:"prefix"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
}

// Ensure KeyVaultStore implements the store.Store interface.
var _ Store = (*KeyVaultStore)(nil)

func NewKeyVaultStore(options KeyVaultStoreOptions) (Store, error) {
	if options.VaultURL == "" {
		return nil, fmt.Errorf("vault_url is required in key vault store configuration")
	}

	// Create a credential using the default Azure credential chain
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrCreateClient, err)
	}

	// Create the Key Vault client
	client, err := azsecrets.NewClient(options.VaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrCreateClient, err)
	}

	stackDelimiter := "-"
	if options.StackDelimiter != nil {
		stackDelimiter = *options.StackDelimiter
	}

	prefix := ""
	if options.Prefix != nil {
		prefix = *options.Prefix
	}

	return &KeyVaultStore{
		client:         client,
		vaultURL:       options.VaultURL,
		prefix:         prefix,
		stackDelimiter: &stackDelimiter,
	}, nil
}

func (s *KeyVaultStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", ErrStackDelimiterNotSet
	}

	return getKey(s.prefix, *s.stackDelimiter, stack, component, key, "-")
}

func (s *KeyVaultStore) Set(stack string, component string, key string, value interface{}) error {
	if stack == "" {
		return ErrEmptyStack
	}
	if component == "" {
		return ErrEmptyComponent
	}
	if key == "" {
		return ErrEmptyKey
	}

	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	strValue, ok := value.(string)
	if !ok {
		return ErrValueMustBeString
	}

	params := azsecrets.SetSecretParameters{
		Value: &strValue,
	}

	_, err = s.client.SetSecret(context.Background(), secretName, params, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == 403 {
			return fmt.Errorf(errWrapFormatWithID, ErrPermissionDenied, fmt.Sprintf("secret %s", secretName), err)
		}
		return fmt.Errorf(errWrapFormat, ErrSetParameter, err)
	}

	return nil
}

func (s *KeyVaultStore) Get(stack string, component string, key string) (interface{}, error) {
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
			case 404:
				return nil, fmt.Errorf(errWrapFormatWithID, ErrResourceNotFound, secretName, err)
			case 403:
				return nil, fmt.Errorf(errWrapFormatWithID, ErrPermissionDenied, fmt.Sprintf("secret %s", secretName), err)
			}
		}
		return nil, fmt.Errorf(errWrapFormat, ErrAccessSecret, err)
	}

	return resp.Value, nil
}

func (s *KeyVaultStore) Delete(stack string, component string, key string) error {
	if stack == "" {
		return ErrEmptyStack
	}
	if component == "" {
		return ErrEmptyComponent
	}
	if key == "" {
		return ErrEmptyKey
	}

	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	_, err = s.client.DeleteSecret(context.Background(), secretName, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			switch respErr.StatusCode {
			case 404:
				return fmt.Errorf(errWrapFormatWithID, ErrResourceNotFound, secretName, err)
			case 403:
				return fmt.Errorf(errWrapFormatWithID, ErrPermissionDenied, fmt.Sprintf("secret %s", secretName), err)
			}
		}
		return fmt.Errorf(errWrapFormat, ErrDeleteParameter, err)
	}

	return nil
}

func (s *KeyVaultStore) List(stack string, component string) ([]string, error) {
	if stack == "" {
		return nil, ErrEmptyStack
	}
	if component == "" {
		return nil, ErrEmptyComponent
	}

	prefix, err := s.getKey(stack, component, "")
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}
	prefix = strings.TrimSuffix(prefix, "-")

	var keys []string
	pager := s.client.NewListSecretPropertiesPager(nil)
	for pager.More() {
		page, err := pager.NextPage(context.Background())
		if err != nil {
			return nil, fmt.Errorf(errWrapFormat, ErrListParameters, err)
		}

		for _, secret := range page.Value {
			if secret.ID != nil {
				name := secret.ID.Name()
				if strings.HasPrefix(name, prefix) {
					// Extract just the key name from the full secret path
					key := strings.TrimPrefix(name, prefix+"-")
					if key != "" {
						keys = append(keys, key)
					}
				}
			}
		}
	}

	return keys, nil
}
