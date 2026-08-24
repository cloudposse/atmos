package sops

import (
	"context"

	"github.com/getsops/sops/v3/azkv"
	"github.com/getsops/sops/v3/keyservice"

	"github.com/cloudposse/atmos/pkg/store/sopsauth"
)

// init registers the Azure Key Vault handler so the key service dispatches "azure_kv" master keys to it.
func init() {
	registerCloudKeyHandler(azureKeyHandler{})
}

// azureKeyHandler injects an Atmos identity's Azure credentials into Azure Key Vault SOPS master keys.
type azureKeyHandler struct{}

func (azureKeyHandler) keyTypeID() string { return keyTypeAzureKV }

func (azureKeyHandler) decrypt(ctx context.Context, b sopsauth.Builder, identity string, req *keyservice.DecryptRequest) ([]byte, error) {
	applier, err := b.AzureKV(ctx, identity)
	if err != nil {
		return nil, err
	}
	mk := azureMasterKey(req.Key.GetAzureKeyvaultKey())
	applier.ApplyToMasterKey(&mk)
	mk.EncryptedKey = string(req.Ciphertext)
	return mk.Decrypt()
}

func (azureKeyHandler) encrypt(ctx context.Context, b sopsauth.Builder, identity string, req *keyservice.EncryptRequest) ([]byte, error) {
	applier, err := b.AzureKV(ctx, identity)
	if err != nil {
		return nil, err
	}
	mk := azureMasterKey(req.Key.GetAzureKeyvaultKey())
	applier.ApplyToMasterKey(&mk)
	if err := mk.Encrypt(req.Plaintext); err != nil {
		return nil, err
	}
	return []byte(mk.EncryptedKey), nil
}

// azureMasterKey reconstructs an Azure Key Vault master key from a keyservice request.
func azureMasterKey(key *keyservice.AzureKeyVaultKey) azkv.MasterKey {
	return azkv.MasterKey{VaultURL: key.VaultUrl, Name: key.Name, Version: key.Version}
}
