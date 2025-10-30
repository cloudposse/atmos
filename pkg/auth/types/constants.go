package types

// Provider kind constants for identifying provider types.
const (
	// AWS provider kinds.
	ProviderKindAWSIAMIdentityCenter = "aws/iam-identity-center"
	ProviderKindAWSSAML              = "aws/saml"
	ProviderKindAWSUser              = "aws/user"
	ProviderKindAWSAssumeRole        = "aws/assume-role"
	ProviderKindAWSPermissionSet     = "aws/permission-set"

	// Azure provider kinds.
	ProviderKindAzureOIDC = "azure/oidc"

	// GCP provider kinds.
	ProviderKindGCPOIDC = "gcp/oidc"

	// GitHub provider kinds.
	ProviderKindGitHubOIDC = "github/oidc"
)
