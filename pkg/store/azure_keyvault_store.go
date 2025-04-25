package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

const (
	statusCodeNotFound  = 404
	statusCodeForbidden = 403
)

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

	return &AzureKeyVaultStore{
		client:         client,
		vaultURL:       options.VaultURL,
		prefix:         prefix,
		stackDelimiter: &stackDelimiter,
	}, nil
}

func (s *AzureKeyVaultStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", ErrStackDelimiterNotSet
	}

	return getKey(s.prefix, *s.stackDelimiter, stack, component, key, "-")
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
	return *resp.Value, nil
}
