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

	// GitHub provider kinds.
	ProviderKindGitHubOIDC = "github/oidc"
)
