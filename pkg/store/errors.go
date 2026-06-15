package store

import "errors"

// Common errors shared across store implementations.
var (
	// Common validation errors.
	ErrEmptyStack           = errors.New("stack cannot be empty")
	ErrEmptyComponent       = errors.New("component cannot be empty")
	ErrEmptyKey             = errors.New("key cannot be empty")
	ErrStackDelimiterNotSet = errors.New("stack delimiter is not set")
	ErrGetKey               = errors.New("failed to get key")

	// AWS SSM specific errors.
	ErrRegionRequired  = errors.New("region is required in ssm store configuration")
	ErrLoadAWSConfig   = errors.New("failed to load AWS config")
	ErrSetParameter    = errors.New("failed to set parameter")
	ErrGetParameter    = errors.New("failed to get parameter")
	ErrDeleteParameter = errors.New("failed to delete parameter")

	// ErrDeleteNotSupported is returned by stores that do not support deletion.
	ErrDeleteNotSupported = errors.New("delete is not supported by this store")

	// Azure Key Vault specific errors.
	ErrVaultURLRequired = errors.New("vault_url is required in azure key vault store configuration")
	ErrCreateClient     = errors.New("failed to create client")
	ErrAccessSecret     = errors.New("failed to access secret")
	ErrResourceNotFound = errors.New("resource not found")
	ErrPermissionDenied = errors.New("permission denied")

	// Redis specific errors.
	ErrParseRedisURL   = errors.New("failed to parse redis url")
	ErrMissingRedisURL = errors.New("either url must be set in options or ATMOS_REDIS_URL environment variable must be set")
	ErrGetRedisKey     = errors.New("failed to get key from redis")

	// Artifactory specific errors.
	ErrMissingArtifactoryToken = errors.New("either access_token must be set in options or one of JFROG_ACCESS_TOKEN or ARTIFACTORY_ACCESS_TOKEN environment variables must be set")
	ErrCreateTempDir           = errors.New("failed to create temp dir")
	ErrCreateTempFile          = errors.New("failed to create temp file")
	ErrDownloadFile            = errors.New("failed to download file")
	ErrNoFilesDownloaded       = errors.New("no files downloaded")
	ErrReadFile                = errors.New("failed to read file")
	ErrUnmarshalFile           = errors.New("failed to unmarshal file")
	ErrWriteTempFile           = errors.New("failed to write to temp file")
	ErrUploadFile              = errors.New("failed to upload file")

	// Google Secret Manager specific errors.
	ErrProjectIDRequired = errors.New("project_id is required in Google Secret Manager store configuration")
	ErrValueMustBeString = errors.New("value must be a string")
	ErrCreateSecret      = errors.New("failed to create secret")
	ErrAddSecretVersion  = errors.New("failed to add secret version")

	// Registry specific errors.
	ErrParseArtifactoryOptions    = errors.New("failed to parse Artifactory store options")
	ErrParseAzureKeyVaultOptions  = errors.New("failed to parse Azure Key Vault store options")
	ErrParseSSMOptions            = errors.New("failed to parse SSM store options")
	ErrParseSecretsManagerOptions = errors.New("failed to parse AWS Secrets Manager store options")
	ErrParseGSMOptions            = errors.New("failed to parse Google Secret Manager store options")
	ErrParseVaultOptions          = errors.New("failed to parse HashiCorp Vault store options")
	ErrParseRedisOptions          = errors.New("failed to parse Redis store options")
	ErrStoreTypeNotFound          = errors.New("store type not found")
	ErrSecretBackendNotEncrypted  = errors.New("store cannot be marked secret: backend does not encrypt values at rest")

	// AWS Secrets Manager specific errors.
	ErrSetSecret    = errors.New("failed to set secret")
	ErrGetSecret    = errors.New("failed to get secret")
	ErrDeleteSecret = errors.New("failed to delete secret")

	// HashiCorp Vault specific errors.
	ErrVaultAddressRequired = errors.New("address is required in hashicorp vault store configuration")
	ErrVaultMountRequired   = errors.New("mount is required in hashicorp vault store configuration")
	ErrVaultWrite           = errors.New("failed to write secret to vault")
	ErrVaultRead            = errors.New("failed to read secret from vault")
	ErrVaultDelete          = errors.New("failed to delete secret from vault")
	ErrVaultEmptyData       = errors.New("vault returned empty data for secret")

	// 1Password specific errors.
	ErrOnePasswordNoAuth            = errors.New("no 1Password credentials found: set OP_SERVICE_ACCOUNT_TOKEN (or options.token), or OP_CONNECT_HOST + OP_CONNECT_TOKEN (or options.connect_host/connect_token)")
	ErrOnePasswordUnknownMode       = errors.New("unknown 1Password mode (expected auto, connect, or service-account)")
	ErrOnePasswordClientInit        = errors.New("failed to initialize 1Password client")
	ErrOnePasswordResolve           = errors.New("failed to resolve 1Password reference")
	ErrOnePasswordWrite             = errors.New("failed to write 1Password secret")
	ErrOnePasswordDelete            = errors.New("failed to delete 1Password secret")
	ErrOnePasswordReferenceTemplate = errors.New("failed to render 1Password reference template")
	ErrOnePasswordInvalidReference  = errors.New("invalid 1Password secret reference")
	ErrOnePasswordNotFound          = errors.New("1Password reference not found")
	ErrParseOnePasswordOptions      = errors.New("failed to parse 1Password store options")

	// GitHub Actions specific errors.
	ErrParseGitHubActionsOptions = errors.New("failed to parse GitHub Actions store options")
	ErrGitHubOwnerRepoRequired   = errors.New("owner and repo are required in GitHub Actions store configuration")
	ErrGitHubInvalidSecretName   = errors.New("invalid GitHub Actions secret name")
	ErrGitHubSecretValueCIOnly   = errors.New("GitHub Actions secret value is not readable outside a GitHub Actions runner")
	ErrGitHubSecretNotInEnv      = errors.New("GitHub Actions secret is not present in the environment")
	ErrGitHubSealSecret          = errors.New("failed to encrypt GitHub Actions secret")
	ErrGitHubPublicKeySize       = errors.New("GitHub Actions public key has unexpected size")
	ErrGitHubGetPublicKey        = errors.New("failed to get GitHub Actions public key")
	ErrGitHubPutSecret           = errors.New("failed to write GitHub Actions secret")
	ErrGitHubGetSecret           = errors.New("failed to get GitHub Actions secret")
	ErrGitHubDeleteSecret        = errors.New("failed to delete GitHub Actions secret")
	ErrGitHubResolveRepoID       = errors.New("failed to resolve GitHub repository ID")

	// Keychain specific errors.
	ErrParseKeychainOptions = errors.New("failed to parse keychain store options")
	ErrKeychainInit         = errors.New("failed to initialize keychain store")
	ErrKeychainWrite        = errors.New("failed to write keychain secret")
	ErrKeychainRead         = errors.New("failed to read keychain secret")
	ErrKeychainDelete       = errors.New("failed to delete keychain secret")
	ErrKeychainNotFound     = errors.New("keychain secret not found")

	// Identity errors.
	ErrIdentityNotConfigured   = errors.New("store identity is configured but auth resolver is not set")
	ErrAuthContextNotAvailable = errors.New("auth context not available for identity")

	// Shared errors.
	ErrSerializeJSON = errors.New("failed to serialize value to JSON")
	ErrMarshalValue  = errors.New("failed to marshal value")
	ErrNilValue      = errors.New("cannot store nil value")
)
