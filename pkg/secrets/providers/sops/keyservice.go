package sops

import (
	"context"
	"fmt"

	"github.com/getsops/sops/v3/keyservice"
	"google.golang.org/grpc"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/store/sopsauth"
)

// getsops master-key type identifiers (keys.MasterKey.TypeToIdentifier). They let Atmos infer which
// cloud a SOPS file's data key is wrapped with at runtime — credential handling is driven by the
// file's actual recipients, not by a declared provider "kind" (issue #2637).
const (
	keyTypeAWSKMS  = "kms"
	keyTypeGCPKMS  = "gcp_kms"
	keyTypeAzureKV = "azure_kv"
	keyTypeAge     = "age"
)

// cloudKeyHandler injects an Atmos identity's credentials into one cloud's SOPS master-key type and
// performs the in-process decrypt/encrypt. There is one implementation per cloud (see aws.go,
// gcp.go, azure.go); each self-registers by its getsops key-type identifier, so adding a cloud never
// touches the dispatcher. Credentials are injected via getsops' ApplyToMasterKey (no env mutation).
type cloudKeyHandler interface {
	// keyTypeID is the getsops key-type identifier this handler serves (e.g. "kms").
	keyTypeID() string
	// decrypt decrypts the request's data key using credentials resolved for identity.
	decrypt(ctx context.Context, b sopsauth.Builder, identity string, req *keyservice.DecryptRequest) ([]byte, error)
	// encrypt encrypts the request's data key using credentials resolved for identity.
	encrypt(ctx context.Context, b sopsauth.Builder, identity string, req *keyservice.EncryptRequest) ([]byte, error)
}

// cloudKeyHandlers is the registry of per-cloud handlers, keyed by getsops key-type identifier.
var cloudKeyHandlers = map[string]cloudKeyHandler{}

// registerCloudKeyHandler adds a per-cloud handler to the registry. A duplicate identifier is a
// programming error (two handlers claiming the same key type).
func registerCloudKeyHandler(h cloudKeyHandler) {
	if _, dup := cloudKeyHandlers[h.keyTypeID()]; dup {
		panic(fmt.Sprintf("duplicate SOPS cloud key handler for %q", h.keyTypeID()))
	}
	cloudKeyHandlers[h.keyTypeID()] = h
}

// keyTypeID maps a keyservice key to its getsops identifier (empty if not a cloud-KMS key type).
func keyTypeID(key *keyservice.Key) string {
	switch key.KeyType.(type) {
	case *keyservice.Key_KmsKey:
		return keyTypeAWSKMS
	case *keyservice.Key_GcpKmsKey:
		return keyTypeGCPKMS
	case *keyservice.Key_AzureKeyvaultKey:
		return keyTypeAzureKV
	default:
		return ""
	}
}

// cloudKeyTypeName maps a key-type identifier to a human-readable cloud name for error hints.
func cloudKeyTypeName(identifier string) string {
	switch identifier {
	case keyTypeAWSKMS:
		return "AWS KMS"
	case keyTypeGCPKMS:
		return "GCP KMS"
	case keyTypeAzureKV:
		return "Azure Key Vault"
	default:
		return "cloud KMS"
	}
}

// sopsKeyServiceClient routes each request's key type to its registered cloud handler — injecting the
// identity's credentials — and delegates every other key type (age, pgp, vault, …) to a fallback key
// service. The cloud is inferred from the key type at runtime; there is no per-cloud "kind".
type sopsKeyServiceClient struct {
	builder  sopsauth.Builder
	identity string
	fallback keyservice.KeyServiceClient
}

// Decrypt dispatches cloud-KMS key types to their handler, else delegates to the fallback.
func (c *sopsKeyServiceClient) Decrypt(ctx context.Context, req *keyservice.DecryptRequest, opts ...grpc.CallOption) (*keyservice.DecryptResponse, error) {
	defer perf.Track(nil, "sops.sopsKeyServiceClient.Decrypt")()

	if h, ok := cloudKeyHandlers[keyTypeID(req.Key)]; ok {
		plaintext, err := h.decrypt(ctx, c.builder, c.identity, req)
		if err != nil {
			return nil, err
		}
		return &keyservice.DecryptResponse{Plaintext: plaintext}, nil
	}
	return c.fallback.Decrypt(ctx, req, opts...)
}

// Encrypt dispatches cloud-KMS key types to their handler, else delegates to the fallback.
func (c *sopsKeyServiceClient) Encrypt(ctx context.Context, req *keyservice.EncryptRequest, opts ...grpc.CallOption) (*keyservice.EncryptResponse, error) {
	defer perf.Track(nil, "sops.sopsKeyServiceClient.Encrypt")()

	if h, ok := cloudKeyHandlers[keyTypeID(req.Key)]; ok {
		ciphertext, err := h.encrypt(ctx, c.builder, c.identity, req)
		if err != nil {
			return nil, err
		}
		return &keyservice.EncryptResponse{Ciphertext: ciphertext}, nil
	}
	return c.fallback.Encrypt(ctx, req, opts...)
}
