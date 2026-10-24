// Package sopsauth bridges the Atmos auth/identity system to the getsops SDK for cloud-KMS SOPS
// backends. Given a store.AuthContextResolver and an identity, it returns getsops master-key
// "appliers" that inject the identity's cloud credentials into the in-process SOPS encrypt/decrypt —
// so KMS operations no longer require ambient cloud credentials in the process environment (#2637).
//
// This package legitimately imports cloud SDKs; it lives under pkg/store/ (a depguard-exempt path,
// like pkg/store/authbridge) so the provider-agnostic SOPS provider never imports a cloud SDK
// directly. There is one implementation per cloud (aws.go, gcp.go, azure.go).
package sopsauth

import (
	"context"

	"github.com/getsops/sops/v3/azkv"
	"github.com/getsops/sops/v3/gcpkms"
	"github.com/getsops/sops/v3/kms"

	"github.com/cloudposse/atmos/pkg/store"
)

// KMSApplier injects credentials into an AWS KMS SOPS master key.
type KMSApplier interface{ ApplyToMasterKey(*kms.MasterKey) }

// GCPApplier injects credentials into a GCP KMS SOPS master key.
type GCPApplier interface{ ApplyToMasterKey(*gcpkms.MasterKey) }

// AzureApplier injects credentials into an Azure Key Vault SOPS master key.
type AzureApplier interface{ ApplyToMasterKey(*azkv.MasterKey) }

// Builder resolves getsops master-key credential appliers for cloud-KMS SOPS kinds. Each method
// authenticates the named identity and returns an applier carrying that identity's credentials.
type Builder interface {
	// AWSKMS returns an applier that injects the identity's AWS credentials into a KMS master key.
	AWSKMS(ctx context.Context, identity string) (KMSApplier, error)
	// GCPKMS returns an applier that injects the identity's GCP credentials into a GCP KMS master key.
	GCPKMS(ctx context.Context, identity string) (GCPApplier, error)
	// AzureKV returns an applier that injects the identity's Azure credentials into a Key Vault master key.
	AzureKV(ctx context.Context, identity string) (AzureApplier, error)
}

// resolverBuilder is the default Builder backed by a store.AuthContextResolver.
type resolverBuilder struct {
	resolver store.AuthContextResolver
}

// NewBuilder returns a Builder that resolves credentials for an identity via the given resolver.
func NewBuilder(resolver store.AuthContextResolver) Builder {
	return &resolverBuilder{resolver: resolver}
}
