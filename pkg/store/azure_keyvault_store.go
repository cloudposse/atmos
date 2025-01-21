package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

var invalidCharsRegex = regexp.MustCompile(`[^a-zA-Z0-9-]`)

// KeyVaultClient interface allows us to mock the Azure Key Vault client
type KeyVaultClient interface {
	SetSecret(ctx context.Context, name string, parameters azsecrets.SetSecretParameters, options *azsecrets.SetSecretOptions) (azsecrets.SetSecretResponse, error)
	GetSecret(ctx context.Context, name string, version string, options *azsecrets.GetSecretOptions) (azsecrets.GetSecretResponse, error)
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
		return nil, fmt.Errorf("failed to create Azure credential: %w", err)
	}

	// Create the Key Vault client
	client, err := azsecrets.NewClient(options.VaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Key Vault client: %w", err)
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
	if stack == "" || component == "" || key == "" {
		return "", fmt.Errorf("stack, component, and key cannot be empty")
	}

	stackParts := strings.Split(stack, *s.stackDelimiter)
	componentParts := strings.Split(component, "/")

	parts := append([]string{s.prefix}, stackParts...)
	parts = append(parts, componentParts...)
	parts = append(parts, key)

	// Azure Key Vault secret names can only contain alphanumeric characters and dashes
	secretName := strings.ToLower(strings.Join(parts, "-"))
	secretName = strings.Trim(secretName, "-")

	if len(secretName) > 127 {
		return "", fmt.Errorf("generated secret name exceeds Azure Key Vault's 127-character limit: %s (%d characters)", secretName, len(secretName))
	}

	return secretName, nil
}

func (s *KeyVaultStore) Set(stack string, component string, key string, value interface{}) error {
	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return err
	}

	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	params := azsecrets.SetSecretParameters{
		Value: &strValue,
	}

	_, err = s.client.SetSecret(context.Background(), secretName, params, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == 403 {
			return fmt.Errorf("failed to set secret '%s': access denied. Please check your Azure credentials and permissions: %w", secretName, err)
		}
		return fmt.Errorf("failed to set secret '%s': %w", secretName, err)
	}

	return nil
}

func (s *KeyVaultStore) Get(stack string, component string, key string) (interface{}, error) {
	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.GetSecret(context.Background(), secretName, "", nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			switch respErr.StatusCode {
			case 404:
				return nil, fmt.Errorf("secret '%s' not found: %w", secretName, err)
			case 403:
				return nil, fmt.Errorf("failed to get secret '%s': access denied. Please check your Azure credentials and permissions: %w", secretName, err)
			}
		}
		return nil, fmt.Errorf("failed to get secret '%s': %w", secretName, err)
	}

	return resp.Value, nil
}
