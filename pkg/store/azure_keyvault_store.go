package store

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

var invalidCharsRegex = regexp.MustCompile(`[^a-zA-Z0-9-]`)

// KeyVaultStore is an implementation of the Store interface for Azure Key Vault.
type KeyVaultStore struct {
	client         *azsecrets.Client
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
	stackParts := strings.Split(stack, *s.stackDelimiter)
	componentParts := strings.Split(component, "/")

	parts := append([]string{s.prefix}, stackParts...)
	parts = append(parts, componentParts...)
	parts = append(parts, key)

	// Azure Key Vault secret names can only contain alphanumeric characters and dashes
	// Replace all invalid characters with dashes and convert to lowercase
	secretName := strings.ToLower(strings.Join(parts, "-"))

	// Replace invalid characters with dashes
	secretName = invalidCharsRegex.ReplaceAllString(secretName, "-")

	// Replace multiple consecutive dashes with a single dash
	secretName = strings.ReplaceAll(secretName, "--", "-")
	secretName = strings.Trim(secretName, "-")

	if len(secretName) > 127 {
		return "", fmt.Errorf("generated secret name exceeds Azure Key Vault's 127-character limit: %s (%d characters)", secretName, len(secretName))
	}

	return secretName, nil
}

func (s *KeyVaultStore) Set(stack string, component string, key string, value interface{}) error {
	if stack == "" {
		return fmt.Errorf("stack cannot be empty")
	}
	if component == "" {
		return fmt.Errorf("component cannot be empty")
	}
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	// Convert value to string
	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return err
	}

	log.Printf("Setting secret '%s' in Azure Key Vault %s", secretName, s.vaultURL)

	params := azsecrets.SetSecretParameters{
		Value: &strValue,
	}

	_, err = s.client.SetSecret(context.Background(), secretName, params, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		switch {
		case errors.As(err, &respErr) && respErr.StatusCode == 403:
			return fmt.Errorf("failed to set secret '%s': access denied. Please check your Azure credentials and permissions: %w", secretName, err)
		case errors.As(err, &respErr) && respErr.StatusCode == 401:
			return fmt.Errorf("failed to set secret '%s': unauthorized. Please check your Azure credentials: %w", secretName, err)
		case errors.As(err, &respErr) && respErr.StatusCode == 429:
			return fmt.Errorf("failed to set secret '%s': rate limit exceeded: %w", secretName, err)
		default:
			return fmt.Errorf("failed to set secret '%s': %w", secretName, err)
		}
	}

	return nil
}

func (s *KeyVaultStore) Get(stack string, component string, key string) (interface{}, error) {
	if stack == "" {
		return nil, fmt.Errorf("stack cannot be empty")
	}
	if component == "" {
		return nil, fmt.Errorf("component cannot be empty")
	}
	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	secretName, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, err
	}

	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		resp, err := s.client.GetSecret(ctx, secretName, "", nil)
		cancel()

		if err == nil {
			return *resp.Value, nil
		}

		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			// Don't retry on permanent errors
			if respErr.StatusCode == 403 || respErr.StatusCode == 404 {
				return nil, fmt.Errorf("failed to get secret '%s': %w", secretName, err)
			}
			// Add exponential backoff for retries
			time.Sleep(time.Duration(i+1) * time.Second)
			lastErr = err
			continue
		}

		return nil, fmt.Errorf("failed to get secret '%s': %w", secretName, err)
	}

	return nil, fmt.Errorf("failed to get secret '%s' after %d retries: %w", secretName, maxRetries, lastErr)
}
