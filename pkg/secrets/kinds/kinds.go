// Package kinds defines the shared provider-kind vocabulary for the Atmos secrets subsystem.
// Store-backed kinds correspond to store types in the store registry (track 1); the sops/*
// kinds are secrets-native provider kinds (track 2).
package kinds

import "github.com/cloudposse/atmos/pkg/perf"

const (
	// AWSSSM is the AWS Systems Manager Parameter Store backend.
	AWSSSM = "aws/ssm"
	// AWSASM is the AWS Secrets Manager backend.
	AWSASM = "aws/asm"

	// AzureKeyVault is the Azure Key Vault backend.
	AzureKeyVault = "azure/keyvault"

	// GCPSecretManager is the GCP Secret Manager backend.
	GCPSecretManager = "gcp/secretmanager"

	// HashicorpVault is the HashiCorp Vault backend.
	HashicorpVault = "hashicorp/vault"

	// SOPSAge is the SOPS backend using age encryption.
	SOPSAge = "sops/age"
	// SOPSAwsKms is the SOPS backend using AWS KMS encryption.
	SOPSAwsKms = "sops/aws-kms"
	// SOPSGcpKms is the SOPS backend using GCP KMS encryption.
	SOPSGcpKms = "sops/gcp-kms"
	// SOPSGPG is the SOPS backend using GPG encryption.
	SOPSGPG = "sops/gpg"
)

// IsSOPS reports whether a kind is a SOPS (track 2) provider kind.
func IsSOPS(kind string) bool {
	defer perf.Track(nil, "kinds.IsSOPS")()

	switch kind {
	case SOPSAge, SOPSAwsKms, SOPSGcpKms, SOPSGPG:
		return true
	default:
		return false
	}
}
