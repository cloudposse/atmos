package types

// Provider kind constants for identifying provider types.
const (
	// AWS provider kinds.
	ProviderKindAWSIAMIdentityCenter = "aws/iam-identity-center"
	ProviderKindAWSSAML              = "aws/saml"
	ProviderKindAWSUser              = "aws/user"
	ProviderKindAWSAssumeRole        = "aws/assume-role"
	ProviderKindAWSPermissionSet     = "aws/permission-set"
	ProviderKindAWSAssumeRoot        = "aws/assume-root"

	// Azure provider kinds.
	ProviderKindAzureOIDC       = "azure/oidc"
	ProviderKindAzureCLI        = "azure/cli"
	ProviderKindAzureDeviceCode = "azure/device-code"

	// GCP provider kinds.
	ProviderKindGCPADC                        = "gcp/adc"
	ProviderKindGCPOIDC                       = "gcp/oidc"
	ProviderKindGCPWorkloadIdentityFederation = "gcp/workload-identity-federation"

	// GCP identity kinds.
	IdentityKindGCPServiceAccount = "gcp/service-account"
	IdentityKindGCPProject        = "gcp/project"

	// Emulator identity kinds. These are root identities not minted by a cloud:
	// their connection profile (SDK env vars or a kubeconfig) is harvested from a
	// running emulator component (kind: <target>/emulator, emulator: <name>).
	IdentityKindAWSEmulator        = "aws/emulator"
	IdentityKindGCPEmulator        = "gcp/emulator"
	IdentityKindAzureEmulator      = "azure/emulator"
	IdentityKindKubernetesEmulator = "kubernetes/emulator"

	// GitHub provider kinds.
	ProviderKindGitHubOIDC = "github/oidc"
)

// EmulatorIdentityKinds lists the identity kinds that bind to a running emulator.
// They are standalone (no via) root identities: the emulator container is the
// credential source, so they authenticate to nothing and inject their profile
// (env vars / kubeconfig) at environment-preparation time.
var EmulatorIdentityKinds = []string{
	IdentityKindAWSEmulator,
	IdentityKindGCPEmulator,
	IdentityKindAzureEmulator,
	IdentityKindKubernetesEmulator,
}

// IsEmulatorIdentityKind reports whether the given identity kind binds to an emulator.
func IsEmulatorIdentityKind(kind string) bool {
	for _, k := range EmulatorIdentityKinds {
		if k == kind {
			return true
		}
	}
	return false
}
