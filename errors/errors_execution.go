package errors

import (
	"errors"
	"fmt"

	"github.com/cloudposse/atmos/pkg/duration"
)

var (
	// List package errors.
	ErrExecuteDescribeStacks     = errors.New("failed to execute describe stacks")
	ErrProcessInstances          = errors.New("failed to process instances")
	ErrParseFlag                 = errors.New("failed to parse flag value")
	ErrFailedToFinalizeCSVOutput = errors.New("failed to finalize CSV output")
	ErrParseStacks               = errors.New("could not parse stacks")
	ErrParseComponents           = errors.New("could not parse components")
	ErrNoComponentsFound         = errors.New("no components found")
	ErrNoStacksFound             = errors.New("no stacks found")
	ErrStackNotFound             = errors.New("stack not found")
	ErrProcessStack              = errors.New("error processing stack")

	// Dependency errors.
	ErrUnsupportedDependencyType   = errors.New("unsupported dependency type")
	ErrMissingDependencyField      = errors.New("dependency missing required field")
	ErrDependencyTargetNotFound    = errors.New("dependency target not found")
	ErrDependencyTargetUnavailable = errors.New("dependency target unavailable")
	// ErrSurvivingConsumers is returned when a destroy selection would remove a component that
	// components outside the selection still require.
	ErrSurvivingConsumers = errors.New("destroy would leave components that depend on it")
	// ErrCustomCommandDependencyNotRegistered is returned when a dependencies.commands entry
	// names a command that isn't registered under the custom-command cobra tree.
	ErrCustomCommandDependencyNotRegistered = errors.New("dependency command is not registered")
	// ErrAmbiguousCommandName is returned when a dependencies.commands entry names a command
	// that more than one declared Command shares (e.g. two unrelated parents each with a nested
	// child of the same name) -- there is no well-defined "first" match to silently pick.
	ErrAmbiguousCommandName = errors.New("multiple commands share this name; use a unique name or restructure your commands")

	// Terraform --all flag errors.
	ErrComponentWithAllFlagConflict = errors.New("component argument can't be used with --all flag")

	// ErrCacheCertUntrusted is returned when the OS trust store does not trust the
	// registry cache proxy's certificate (macOS/Windows require a one-time trust step).
	ErrCacheCertUntrusted = errors.New("registry cache certificate is not trusted")

	// ErrTrustStore is returned when installing or removing the registry cache proxy's
	// certificate in the OS trust store fails.
	ErrTrustStore = errors.New("registry cache trust store operation failed")

	// Terraform execution errors.
	ErrTerraformExecFailed          = errors.New("terraform execution failed")
	ErrDescribeAffected             = errors.New("describe affected failed")
	ErrUploadRequiresSupportedEvent = errors.New("upload requires a supported CI event")
	ErrDescribeStacks               = errors.New("describe stacks failed")
	ErrBuildDepGraph                = errors.New("build dependency graph failed")
	ErrTopologicalOrder             = errors.New("topological sort failed")
	ErrGraphExecutionCanceled       = errors.New("graph execution canceled")
	ErrGraphExecutionOptions        = errors.New("graph execution options are invalid")
	ErrFormatForLogging             = errors.New("format affected for logging failed")
	ErrQueryEvaluation              = errors.New("query evaluation failed")
	ErrNilResult                    = errors.New("nil result")

	// Cache-related errors.
	ErrCacheLocked    = errors.New("cache file is locked")
	ErrCacheRead      = errors.New("cache read failed")
	ErrCacheWrite     = errors.New("cache write failed")
	ErrCacheFetch     = errors.New("failed to fetch content for cache")
	ErrCacheUnmarshal = errors.New("cache unmarshal failed")
	ErrCacheMarshal   = errors.New("cache marshal failed")
	ErrCacheDir       = errors.New("cache directory creation failed")

	// Logger errors.
	ErrInvalidLogLevel  = errors.New("invalid log level")
	ErrInvalidLogsColor = errors.New("invalid logs color")

	// File operation errors.
	ErrCopyFile            = errors.New("failed to copy file")
	ErrCreateDirectory     = errors.New("failed to create directory")
	ErrCreateFile          = errors.New("failed to create file")
	ErrOpenFile            = errors.New("failed to open file")
	ErrWriteFile           = errors.New("failed to write to file")
	ErrCloseFile           = errors.New("failed to close file")
	ErrStatFile            = errors.New("failed to stat file")
	ErrRemoveDirectory     = errors.New("failed to remove directory")
	ErrSetPermissions      = errors.New("failed to set permissions")
	ErrReadDirectory       = errors.New("failed to read directory")
	ErrComputeRelativePath = errors.New("failed to compute relative path")
	ErrFileOperation       = errors.New("file operation failed")

	// Archive extraction safety errors (pkg/filesystem).
	ErrArchiveEntryEscapesDest = errors.New("archive entry escapes destination directory")

	// OCI/Container image errors.
	ErrCreateTempDirectory   = ErrCreateTempDir // Alias to avoid duplicate sentinels
	ErrInvalidImageReference = errors.New("invalid image reference")
	ErrPullImage             = errors.New("failed to pull image")
	ErrOCIGetFileUnsupported = errors.New("oci:// sources must be fetched in directory mode, not as a single file")
	ErrGetImageDescriptor    = errors.New("cannot get a descriptor for the OCI image")
	ErrGetImageLayers        = errors.New("failed to get image layers")
	ErrProcessLayer          = errors.New("failed to process layer")
	ErrLayerDecompression    = errors.New("layer decompression error")
	ErrLayerExtraction       = errors.New("layer extraction error")
	ErrArchiveTooLarge       = errors.New("archive exceeds maximum size")
	ErrArchiveEntryTooLarge  = errors.New("archive entry exceeds maximum extracted size")

	// Initialization and configuration errors.
	ErrInitializeCLIConfig    = errors.New("error initializing CLI config")
	ErrGetHooks               = errors.New("error getting hooks")
	ErrPerComponentHookFailed = errors.New("per-component hook failed")
	ErrSetFlag                = errors.New("failed to set flag")
	ErrVersionMismatch        = errors.New("version mismatch")

	// Download and client errors.
	ErrMergeConfiguration = errors.New("failed to merge configuration")

	// Template and documentation errors.
	ErrGenerateTerraformDocs = errors.New("failed to generate terraform docs")
	ErrMergeInputYAMLs       = errors.New("failed to merge input YAMLs")
	ErrRenderTemplate        = errors.New("failed to render template with datasources")
	// ErrGomplateDatasourceUnavailable is returned when a gomplate datasource is read outside a
	// template render, or while gomplate functions are disabled, so no datasource registry exists.
	ErrGomplateDatasourceUnavailable = errors.New("gomplate datasource functions are unavailable: gomplate is disabled or no template render is in progress")
	ErrInvalidDatasourceURL          = errors.New("invalid gomplate datasource URL")
	ErrResolveOutputPath             = errors.New("failed to resolve output path")
	ErrWriteOutput                   = errors.New("failed to write output")
	ErrCollectKeysNotMap             = errors.New("collectKeys: argument is not a map")

	// Import-related errors.
	ErrBasePath             = errors.New("base path required to process imports")
	ErrTempDir              = errors.New("temporary directory required to process imports")
	ErrResolveLocal         = errors.New("failed to resolve local import path")
	ErrSourceDestination    = errors.New("source and destination cannot be nil")
	ErrImportPathRequired   = errors.New("import path required to process imports")
	ErrNoFileMatchPattern   = errors.New("no files matching patterns found")
	ErrMaxImportDepth       = errors.New("maximum import depth reached")
	ErrNoValidAbsolutePaths = errors.New("no valid absolute paths found")
	ErrDownloadRemoteConfig = errors.New("failed to download remote config")
	ErrMockImportFailure    = errors.New("mock error: simulated import failure")
	ErrProcessNestedImports = errors.New("failed to process nested imports")
	// ErrRequiredImportFailed is returned when a required (non-optional) CLI-config import
	// cannot be resolved or merged. Object-form imports are required by default (fail closed);
	// set `skip_if_missing: true` to opt back into warn-and-skip.
	ErrRequiredImportFailed = errors.New("required config import failed to load")
	// ErrInvalidConfigImport is returned when an object-form CLI-config import entry cannot be
	// decoded (e.g. a malformed `skip_if_missing` or `when` value). A malformed entry must fail
	// loudly rather than being silently dropped and treated as an optional omission.
	ErrInvalidConfigImport = errors.New("invalid config import entry")

	// Profiler-related errors.
	ErrProfilerStart           = errors.New("profiler start failed")
	ErrProfilerUnsupportedType = errors.New("profiler: unsupported profile type")
	ErrProfilerStartCPU        = errors.New("profiler: failed to start CPU profile")
	ErrProfilerStartTrace      = errors.New("profiler: failed to start trace profile")
	ErrProfilerCreateFile      = errors.New("profiler: failed to create profile file")

	// Auth package errors.
	ErrAuthNotConfigured            = errors.New("authentication not configured in atmos.yaml")
	ErrInvalidAuthConfig            = errors.New("invalid auth config")
	ErrInvalidIdentityKind          = errors.New("invalid identity kind")
	ErrInvalidIdentityConfig        = errors.New("invalid identity config")
	ErrInvalidProviderKind          = errors.New("invalid provider kind")
	ErrInvalidProviderConfig        = errors.New("invalid provider config")
	ErrInvalidBrowserExecutable     = errors.New("invalid browser executable")
	ErrAuthenticationFailed         = errors.New("authentication failed")
	ErrAuthenticationUnavailable    = errors.New("authentication unavailable")
	ErrPrepareShellEnvironment      = errors.New("failed to prepare authenticated shell environment")
	ErrInvalidADCContent            = errors.New("invalid ADC content")
	ErrWriteADCFile                 = errors.New("failed to write ADC file")
	ErrWritePropertiesFile          = errors.New("failed to write properties file")
	ErrWriteAccessTokenFile         = errors.New("failed to write access token file")
	ErrPostAuthenticationHookFailed = errors.New("post authentication hook failed")
	ErrAuthManager                  = errors.New("auth manager error")
	ErrDefaultIdentity              = errors.New("default identity error")
	ErrAwsAuth                      = errors.New("aws auth error")
	ErrAwsUserNotConfigured         = errors.New("aws user not configured")
	ErrAwsUserKeyringReadFailed     = errors.New("failed to read AWS user credentials from keyring")
	ErrAwsSAMLDecodeFailed          = errors.New("aws saml decode failed")
	ErrPlaywrightDriverSeed         = errors.New("failed to pre-seed the Playwright driver")
	ErrAwsMissingEnvVars            = errors.New("missing required AWS environment variables")
	ErrIdentityNotAWS               = errors.New("identity does not produce AWS credentials")
	ErrUnsupportedPlatform          = errors.New("unsupported platform")
	ErrChromeNotFound               = errors.New("chrome/chromium not found for isolated browser sessions")
	ErrSayNotFound                  = errors.New("text-to-speech command not found")
	ErrVoiceListUnsupported         = errors.New("voice enumeration not supported for backend")
	ErrUserAborted                  = errors.New("user aborted")

	// GitHub OIDC provider errors.
	// ErrGitHubOIDCEndpointUnavailable marks a token request that failed in a way worth retrying:
	// a timeout or other transport error, HTTP 429, or HTTP 5xx.
	ErrGitHubOIDCEndpointUnavailable = errors.New("GitHub OIDC token endpoint unavailable")

	// AWS credential_process errors (consuming and producing the AWS process-credential format).
	ErrCredentialProcessFailed        = errors.New("credential_process command failed")
	ErrCredentialProcessInvalidOutput = errors.New("credential_process returned invalid output")
	ErrCredentialProcessRecursion     = errors.New("credential_process recursion detected")
	// ErrCredentialProcessIdentityRequired is returned when the credential-process producer is not
	// given a concrete identity (it never opens the interactive selector, which the AWS CLI hides).
	ErrCredentialProcessIdentityRequired = errors.New("credential_process requires a specific identity")
	// ErrAWSCredentialsIncomplete is returned when AWS credentials lack an access key ID or secret access key.
	ErrAWSCredentialsIncomplete = errors.New("AWS credentials are missing an access key ID or secret access key")

	// AWS SSO specific errors.
	ErrSSOSessionExpired      = errors.New("aws sso session expired")
	ErrSSODeviceAuthFailed    = errors.New("aws sso device authorization failed")
	ErrSSOTokenCreationFailed = errors.New("aws sso token creation failed")
	ErrSSOAccountListFailed   = errors.New("failed to list aws sso accounts")
	ErrSSORoleListFailed      = errors.New("failed to list aws sso roles")
	ErrSSOProvisioningFailed  = errors.New("aws sso identity provisioning failed")
	ErrSSOInvalidToken        = errors.New("invalid aws sso token")

	// AWS browser webflow errors.
	ErrWebflowAuthFailed     = errors.New("browser authentication failed")
	ErrWebflowDisabled       = errors.New("browser authentication is disabled")
	ErrWebflowTokenExchange  = errors.New("failed to exchange authorization code for credentials")
	ErrWebflowCallbackServer = errors.New("failed to start local callback server")
	ErrWebflowTimeout        = errors.New("browser authentication timed out")
	ErrWebflowRefreshFailed  = errors.New("failed to refresh browser credentials")
	// ErrWebflowRefreshTokenRevoked indicates the refresh token has been definitively
	// rejected by the AWS signin service (e.g. HTTP 400 invalid_grant/invalid_token).
	// This is the only condition under which the cached refresh token should be deleted;
	// transient failures (HTTP 5xx, 429, network errors) must preserve the cache.
	ErrWebflowRefreshTokenRevoked = errors.New("browser refresh token has been revoked")
	ErrWebflowCodeRequired        = errors.New("authorization code is required")
	ErrWebflowReadAuthCodeFailed  = errors.New("failed to read authorization code")
	ErrWebflowAuthorizationError  = errors.New("authorization error")
	ErrWebflowMissingCallbackCode = errors.New("missing authorization code in callback")
	ErrWebflowStateMismatch       = errors.New("state mismatch: possible CSRF attack")
	ErrWebflowEmptyCachedToken    = errors.New("cached refresh token is empty")
	// ErrWebflowDPoP indicates a failure generating or serializing the RFC 9449
	// DPoP proof required on AWS signin token requests (issue #2542).
	ErrWebflowDPoP = errors.New("failed to build DPoP proof")

	// Credential errors.
	ErrCredentialsInvalid = errors.New("credentials are invalid or have been revoked")
	ErrInvalidDuration    = duration.ErrInvalid // Alias so errors.Is matches pkg/duration errors.

	// Auth manager and identity/provider resolution errors (centralized sentinels).
	ErrFailedToInitializeAuthManager = errors.New("failed to initialize auth manager")
	ErrNoCredentialsFound            = errors.New("no credentials found for identity")
	ErrExpiredCredentials            = errors.New("credentials for identity are expired or invalid")
	ErrNilParam                      = errors.New("parameter cannot be nil")
	ErrInitializingProviders         = errors.New("failed to initialize providers")
	ErrInitializingIdentities        = errors.New("failed to initialize identities")
	ErrInitializingCredentialStore   = errors.New("failed to initialize credential store")
	ErrCircularDependency            = errors.New("circular dependency detected in identity chain")
	ErrIdentityNotFound              = errors.New("identity not found")
	ErrProviderNotFound              = errors.New("provider not found")
	ErrMutuallyExclusiveFlags        = errors.New("mutually exclusive flags provided")
	ErrNoDefaultIdentity             = errors.New("no default identity configured for authentication")
	ErrMultipleDefaultIdentities     = errors.New("multiple default identities found")
	ErrNoIdentitiesAvailable         = errors.New("no identities available")
	ErrNoProvidersAvailable          = errors.New("no providers available")
	ErrNoIdentitiesMatchTags         = errors.New("no identities match the specified tags")
	ErrNoProvidersMatchTags          = errors.New("no providers match the specified tags")
	ErrNoDefaultProvider             = errors.New("no default provider configured and multiple providers exist")
	ErrIdentitySelectionRequiresTTY  = fmt.Errorf("interactive identity selection: %w", ErrTTYRequired)
	ErrProviderSelectionRequiresTTY  = fmt.Errorf("interactive provider selection: %w", ErrTTYRequired)
	ErrAuthPromptUnavailable         = fmt.Errorf("interactive authentication prompt unavailable: %w", ErrTTYRequired)
	ErrAuthenticationChainNotBuilt   = errors.New("authentication chain not built")
	ErrInvalidStackConfig            = errors.New("invalid stack config")
	ErrNoCommandSpecified            = errors.New("no command specified")
	ErrCommandNotFound               = errors.New("command not found")
	ErrCommandFailed                 = errors.New("command execution failed")

	// Auth validation errors - specific sentinel errors for configuration validation.
	ErrMissingPrincipal     = errors.New("principal is required")
	ErrMissingAssumeRole    = errors.New("assume_role is required in principal")
	ErrMissingPermissionSet = errors.New("permission set name is required in principal")
	ErrMissingAccountSpec   = errors.New("account specification is required in principal")

	ErrInvalidSubcommand = errors.New("invalid subcommand")
	ErrSubcommandFailed  = errors.New("subcommand failed")

	ErrInvalidArgumentError = errors.New("invalid argument error")
	ErrMissingInput         = errors.New("missing input")

	ErrAuthAwsFileManagerFailed = errors.New("failed to create AWS file manager")

	ErrAuthOidcDecodeFailed    = errors.New("failed to decode OIDC token")
	ErrAuthOidcUnmarshalFailed = errors.New("failed to unmarshal oidc claims")

	// Realm errors.
	ErrFailedToComputeRealm = errors.New("failed to compute realm")
	ErrInvalidRealm         = errors.New("invalid realm value")
	ErrEmptyRealm           = errors.New("realm is required for credential isolation but was not set")

	// Logout errors.
	ErrFailedGetConfigDir   = errors.New("failed to get config directory")
	ErrFailedDiscoverRealms = errors.New("failed to discover realms")
	ErrFailedRemoveRealm    = errors.New("failed to remove realm")

	// Store and hook errors.
	ErrNilTerraformOutput = errors.New("terraform output returned nil")
	ErrNilStoreValue      = errors.New("cannot store nil value")

	// Devcontainer errors.
	ErrDevcontainerNotFound      = errors.New("devcontainer not found")
	ErrContainerRuntimeOperation = errors.New("container runtime operation failed")
	ErrContainerNotFound         = errors.New("container not found")
	ErrContainerAlreadyExists    = errors.New("container already exists")
	ErrContainerNotRunning       = errors.New("container is not running")
	ErrContainerRunning          = errors.New("container is running")
	ErrContainerNotHealthy       = errors.New("container did not become healthy")
	ErrInvalidDevcontainerConfig = errors.New("invalid devcontainer configuration")
	ErrRuntimeNotAvailable       = errors.New("container runtime not available")
	ErrDevcontainerNameEmpty     = errors.New("devcontainer name cannot be empty")
	ErrDevcontainerNameInvalid   = errors.New("devcontainer name contains invalid characters")
	ErrDevcontainerNameTooLong   = errors.New("devcontainer name is too long")
	ErrPTYNotSupported           = errors.New("PTY not supported on this platform")

	// Container bulk-operation errors.
	ErrNoContainerComponentSelected = errors.New("no container component selected")
	ErrContainerComponentWithAll    = errors.New("cannot combine a component argument with --all")

	// Container run configuration errors.
	ErrInvalidContainerRestartPolicy = errors.New("invalid container restart policy")
	ErrInvalidContainerHealthCheck   = errors.New("invalid container healthcheck")

	// Logout errors.
	ErrLogoutFailed                         = errors.New("logout failed")
	ErrPartialLogout                        = errors.New("partial logout")
	ErrLogoutNotSupported                   = errors.New("logout not supported for this provider")
	ErrLogoutNotImplemented                 = errors.New("logout not implemented for this provider")
	ErrKeyringDeletion                      = errors.New("keyring deletion failed")
	ErrKeychainDeletionRequiresConfirmation = errors.New("keychain deletion requires interactive confirmation or --force flag")
	ErrProviderLogout                       = errors.New("provider logout failed")
	ErrIdentityLogout                       = errors.New("identity logout failed")
	ErrIdentityNotInConfig                  = errors.New("identity not found in configuration")
	ErrProviderNotInConfig                  = errors.New("provider not found in configuration")
	ErrInvalidLogoutOption                  = errors.New("invalid logout option")

	// Provision target (delivery destination) errors.
	ErrProvisionTargetNotFound    = errors.New("provision target not found")
	ErrProvisionTargetKindUnknown = errors.New("provision target kind is not registered")
	ErrProvisionTargetKindMissing = errors.New("provision target is missing a kind")
	ErrProvisionTargetNoFetch     = errors.New("provision target kind does not support reading current state")

	// Git target pull-request publishing errors.
	ErrGitTargetPullRequestConfig = errors.New("invalid git target pull_request configuration")

	// Backend provisioning errors.
	ErrBucketRequired             = errors.New("backend.bucket is required")
	ErrRegionRequired             = errors.New("backend.region is required")
	ErrBackendNotFound            = errors.New("backend configuration not found")
	ErrProvisioningNotConfigured  = errors.New("provisioning not configured")
	ErrCreateNotImplemented       = errors.New("create not implemented for backend type")
	ErrInvalidBucketNamespace     = errors.New("provision.backend.bucket_namespace must be a string")
	ErrUnsupportedBucketNamespace = errors.New("unsupported provision.backend.bucket_namespace value")
	ErrDeleteNotImplemented       = errors.New("delete not implemented for backend type")
	ErrProvisionerFailed          = errors.New("provisioner failed")
	ErrLoadAWSConfig              = errors.New("failed to load AWS config")
	ErrCheckBucketExist           = errors.New("failed to check bucket existence")
	ErrCreateBucket               = errors.New("failed to create bucket")
	ErrApplyBucketDefaults        = errors.New("failed to apply bucket defaults")
	ErrEnableVersioning           = errors.New("failed to enable versioning")
	ErrEnableEncryption           = errors.New("failed to enable encryption")
	ErrBlockPublicAccess          = errors.New("failed to block public access")
	ErrApplyTags                  = errors.New("failed to apply tags")
	ErrForceRequired              = errors.New("--force flag required for backend deletion")
	ErrBucketNotEmpty             = errors.New("bucket contains objects and cannot be deleted")
	ErrStateFilesExist            = errors.New("bucket contains terraform state files")
	ErrDeleteObjects              = errors.New("failed to delete objects from bucket")
	ErrDeleteBucket               = errors.New("failed to delete bucket")
	ErrListObjects                = errors.New("failed to list bucket objects")

	// Azure (azurerm) backend provisioning errors.
	ErrResourceGroupRequired     = errors.New("backend.resource_group_name is required for azurerm backend provisioning")
	ErrAzureSubscriptionRequired = errors.New("an Azure subscription id is required for azurerm backend provisioning (set backend.subscription_id or a subscription on the active Azure identity)")
	ErrAzureLocationRequired     = errors.New("an Azure location is required to create the resource group for azurerm backend provisioning")
	ErrLoadAzureConfig           = errors.New("failed to build Azure client for backend provisioning")
	ErrCheckResourceGroupExist   = errors.New("failed to check Azure resource group existence")
	ErrCreateResourceGroup       = errors.New("failed to create Azure resource group")
	ErrCheckStorageAccountExist  = errors.New("failed to check Azure storage account existence")
	ErrCreateStorageAccount      = errors.New("failed to create Azure storage account")
	ErrApplyBlobDataProtection   = errors.New("failed to apply Azure blob data-protection defaults")
	ErrCheckContainerExist       = errors.New("failed to check Azure storage container existence")
	ErrCreateStorageContainer    = errors.New("failed to create Azure storage container")
	ErrDeleteStorageAccount      = errors.New("failed to delete Azure storage account")

	// Component path resolution errors.
	ErrPathNotInComponentDir  = errors.New("path is not within Atmos component directories")
	ErrComponentTypeMismatch  = errors.New("path component type does not match command")
	ErrComponentNotInStack    = errors.New("component not found in stack configuration")
	ErrPathResolutionFailed   = errors.New("failed to resolve component from path")
	ErrPathIsComponentBase    = errors.New("must specify a component directory, not the base directory")
	ErrAmbiguousComponentPath = errors.New("ambiguous component path")
	ErrComponentPathNotFound  = errors.New("component path does not exist")

	// Interactive prompt errors.
	ErrInteractiveModeNotAvailable = errors.New("interactive mode not available")
	ErrNoOptionsAvailable          = errors.New("no options available")

	// Locals-related errors.
	ErrLocalsInvalidType        = errors.New("locals must be a map")
	ErrLocalsCircularDep        = errors.New("circular dependency in locals")
	ErrLocalsDependencyExtract  = errors.New("failed to extract dependencies for local")
	ErrLocalsResolution         = errors.New("failed to resolve local")
	ErrLocalsYamlFunctionFailed = errors.New("failed to process YAML function in local")

	// Init and scaffold errors.
	ErrScaffoldNotFound        = errors.New("scaffold template not found")
	ErrScaffoldValidation      = errors.New("scaffold validation failed")
	ErrScaffoldGeneration      = errors.New("scaffold generation failed")
	ErrInitialization          = errors.New("initialization failed")
	ErrTemplateProcessing      = errors.New("template processing failed")
	ErrInvalidScaffoldConfig   = errors.New("invalid scaffold configuration")
	ErrTargetDirectoryExists   = errors.New("target directory already exists")
	ErrTargetDirectoryNotEmpty = errors.New("target directory is not empty")
	ErrScaffoldParseYAML       = errors.New("failed to parse scaffold YAML")
	ErrPromptFailed            = errors.New("interactive prompt failed")

	// Manifest registry errors (pkg/manifest).
	ErrManifestKindUnknown    = errors.New("unknown manifest kind")
	ErrManifestKindEmpty      = errors.New("manifest kind is empty")
	ErrManifestKindMismatch   = errors.New("manifest kind mismatch")
	ErrManifestAPIVersion     = errors.New("unsupported manifest apiVersion")
	ErrManifestParse          = errors.New("failed to parse manifest")
	ErrManifestPrototypeNil   = errors.New("manifest prototype is nil")
	ErrManifestSchemaGenerate = errors.New("failed to generate manifest JSON schema")
	ErrManifestValidation     = errors.New("manifest validation failed")
)
