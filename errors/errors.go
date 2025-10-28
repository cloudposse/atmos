package errors

import (
	"errors"
	"fmt"
)

var (
	ErrNoGitRepo                             = errors.New("not in a git repository")
	ErrDownloadPackage                       = errors.New("failed to download package")
	ErrDownloadFile                          = errors.New("failed to download file")
	ErrParseFile                             = errors.New("failed to parse file")
	ErrParseURL                              = errors.New("failed to parse URL")
	ErrInvalidURL                            = errors.New("invalid URL")
	ErrCreateDownloadClient                  = errors.New("failed to create download client")
	ErrProcessOCIImage                       = errors.New("failed to process OCI image")
	ErrCopyPackage                           = errors.New("failed to copy package")
	ErrCreateTempDir                         = errors.New("failed to create temp directory")
	ErrUnknownPackageType                    = errors.New("unknown package type")
	ErrLocalMixinURICannotBeEmpty            = errors.New("local mixin URI cannot be empty")
	ErrLocalMixinInstallationNotImplemented  = errors.New("local mixin installation not implemented")
	ErrNotImplemented                        = errors.New("not implemented")
	ErrFailedToInitializeTUIModel            = errors.New("failed to initialize TUI model: verify terminal capabilities and permissions")
	ErrSetTempDirPermissions                 = errors.New("failed to set temp directory permissions")
	ErrCopyPackageToTarget                   = errors.New("failed to copy package to target")
	ErrNoValidInstallerPackage               = errors.New("no valid installer package provided")
	ErrFailedToInitializeTUIModelWithDetails = errors.New("failed to initialize TUI model: verify terminal capabilities and permissions")
	ErrValidPackage                          = errors.New("no valid installer package provided for")
	ErrTUIModel                              = errors.New("failed to initialize TUI model")
	ErrTUIRun                                = errors.New("failed to run TUI")
	ErrNoFilesFound                          = errors.New("no files found in directory")
	ErrMultipleFilesFound                    = errors.New("multiple files found in directory")
	ErrSourceDirNotExist                     = errors.New("source directory does not exist")
	ErrEmptyFilePath                         = errors.New("file path is empty")
	ErrEmptyWorkdir                          = errors.New("workdir cannot be empty")
	ErrWorkdirNotExist                       = errors.New("workdir does not exist")
	ErrPathResolution                        = errors.New("failed to resolve absolute path")
	ErrInvalidTemplateFunc                   = errors.New("invalid template function")
	ErrInvalidTemplateSettings               = errors.New("invalid template settings")
	ErrRefuseDeleteSymbolicLink              = errors.New("refusing to delete symbolic link")
	ErrNoDocsGenerateEntry                   = errors.New("no docs.generate entry found")
	ErrMissingDocType                        = errors.New("doc-type argument missing")
	ErrUnsupportedInputType                  = errors.New("unsupported input type")
	ErrMissingStackNameTemplateAndPattern    = errors.New("stack naming configuration is missing")
	ErrFailedMarshalConfigToYaml             = errors.New("failed to marshal config to YAML")
	ErrStacksDirectoryDoesNotExist           = errors.New("directory for Atmos stacks does not exist")
	ErrCommandNil                            = errors.New("command cannot be nil")
	ErrGitHubRateLimitExceeded               = errors.New("GitHub API rate limit exceeded")
	ErrInvalidLimit                          = errors.New("invalid limit value")
	ErrInvalidOffset                         = errors.New("invalid offset value")
	ErrInvalidSinceDate                      = errors.New("invalid date format")
	ErrUnsupportedOutputFormat               = errors.New("unsupported output format")
	ErrTerminalTooNarrow                     = errors.New("terminal too narrow")
	ErrSpinnerReturnedNilModel               = errors.New("spinner returned nil model")
	ErrSpinnerUnexpectedModelType            = errors.New("spinner returned unexpected model type")

	// ErrAuthConsole is returned when auth console command operations fail.
	ErrAuthConsole          = errors.New("auth console operation failed")
	ErrProviderNotSupported = errors.New("provider does not support this operation")
	ErrUnknownServiceAlias  = errors.New("unknown service alias")

	// ErrPlanHasDiff is returned when there are differences between two Terraform plan files.
	ErrPlanHasDiff = errors.New("plan files have differences")

	ErrInvalidTerraformFlagsWithAffectedFlag                 = errors.New("incompatible flags")
	ErrInvalidTerraformComponentWithMultiComponentFlags      = errors.New("incompatible arguments")
	ErrInvalidTerraformSingleComponentAndMultiComponentFlags = errors.New("incompatible flags")

	ErrYamlFuncInvalidArguments         = errors.New("invalid number of arguments in the Atmos YAML function")
	ErrDescribeComponent                = errors.New("failed to describe component")
	ErrReadTerraformState               = errors.New("failed to read Terraform state")
	ErrEvaluateTerraformBackendVariable = errors.New("failed to evaluate terraform backend variable")
	ErrUnsupportedBackendType           = errors.New("unsupported backend type")
	ErrProcessTerraformStateFile        = errors.New("error processing terraform state file")

	ErrLoadAwsConfig    = errors.New("failed to load AWS config")
	ErrGetObjectFromS3  = errors.New("failed to get object from S3")
	ErrReadS3ObjectBody = errors.New("failed to read S3 object body")

	// Azure Blob Storage specific errors.
	ErrGetBlobFromAzure       = errors.New("failed to get blob from Azure Blob Storage")
	ErrReadAzureBlobBody      = errors.New("failed to read Azure blob body")
	ErrCreateAzureCredential  = errors.New("failed to create Azure credential")
	ErrCreateAzureClient      = errors.New("failed to create Azure Blob Storage client")
	ErrAzureContainerRequired = errors.New("container_name is required for azurerm backend")
	ErrStorageAccountRequired = errors.New("storage_account_name is required for azurerm backend")
	ErrAzurePermissionDenied  = errors.New("permission denied accessing Azure blob")
	ErrBackendConfigRequired  = errors.New("backend configuration is required")

	// Git-related errors.
	ErrGitNotAvailable      = errors.New("git is not available")
	ErrInvalidGitPort       = errors.New("invalid port number")
	ErrSSHKeyUsage          = errors.New("error using SSH key")
	ErrGitCommandExited     = errors.New("git command exited with non-zero status")
	ErrGitCommandFailed     = errors.New("failed to execute git command")
	ErrReadDestDir          = errors.New("failed to read the destination directory during git update")
	ErrRemoveGitDir         = errors.New("failed to remove the .git directory in the destination directory during git update")
	ErrUnexpectedGitOutput  = errors.New("unexpected 'git version' output")
	ErrGitVersionMismatch   = errors.New("git version requirement not met")
	ErrRemoteRepoNotGitRepo = errors.New("target remote repository is not a git repository")
	ErrFailedToGetLocalRepo = errors.New("failed to get local repository")
	ErrFailedToGetRepoInfo  = errors.New("failed to get repository info")
	ErrLocalRepoFetch       = errors.New("local repo unavailable")
	ErrHeadLookup           = errors.New("HEAD not found")

	// Slice utility errors.
	ErrNilInput         = errors.New("input must not be nil")
	ErrNonStringElement = errors.New("element is not a string")

	ErrReadFile    = errors.New("error reading file")
	ErrInvalidFlag = errors.New("invalid flag")

	// File and URL handling errors.
	ErrInvalidPagerCommand = errors.New("invalid pager command")
	ErrEmptyURL            = errors.New("empty URL provided")
	ErrFailedToFindImport  = errors.New("failed to find import")
	ErrInvalidFilePath     = errors.New("invalid file path")
	ErrRelPath             = errors.New("error determining relative path")
	ErrHTTPRequestFailed   = errors.New("HTTP request failed")

	// Config loading errors.
	ErrAtmosDirConfigNotFound      = errors.New("atmos config directory not found")
	ErrReadConfig                  = errors.New("failed to read config")
	ErrMergeTempConfig             = errors.New("failed to merge temp config")
	ErrPreprocessYAMLFunctions     = errors.New("failed to preprocess YAML functions")
	ErrMergeEmbeddedConfig         = errors.New("failed to merge embedded config")
	ErrExpectedDirOrPattern        = errors.New("expected directory or pattern")
	ErrFileNotFound                = errors.New("file not found")
	ErrFileAccessDenied            = errors.New("file access denied")
	ErrExpectedFile                = errors.New("expected file")
	ErrAtmosArgConfigNotFound      = errors.New("atmos configuration not found")
	ErrEmptyConfigPath             = errors.New("config path is empty")
	ErrEmptyConfigFile             = errors.New("config file path is empty")
	ErrAtmosFilesDirConfigNotFound = errors.New("atmos configuration file not found in directory")
	ErrAtmosConfigNotFound         = errors.New("atmos configuration file not found")

	ErrMissingStack                               = errors.New("stack is required")
	ErrMissingComponent                           = errors.New("component is required")
	ErrMissingComponentType                       = errors.New("component type is required")
	ErrInvalidArguments                           = errors.New("invalid arguments")
	ErrInvalidComponent                           = errors.New("invalid component")
	ErrInvalidComponentMapType                    = errors.New("invalid component map type")
	ErrAbstractComponentCantBeProvisioned         = errors.New("abstract component cannot be provisioned")
	ErrLockedComponentCantBeProvisioned           = errors.New("locked component cannot be provisioned")
	ErrSpaceliftAdminStackWorkspaceNotEnabled     = errors.New("spacelift admin stack does not have workspace enabled")
	ErrSpaceliftAdminStackComponentNotProvisioned = errors.New("spacelift admin stack component cannot be provisioned")

	// Terraform-specific errors.
	ErrHTTPBackendWorkspaces            = errors.New("workspaces are not supported for the HTTP backend")
	ErrInvalidTerraformComponent        = errors.New("invalid Terraform component")
	ErrNoTty                            = errors.New("no TTY attached")
	ErrNoSuitableShell                  = errors.New("no suitable shell found")
	ErrFailedToLoadTerraformModule      = errors.New("failed to load terraform module")
	ErrNoJSONOutput                     = errors.New("no JSON output found in terraform show output")
	ErrOriginalPlanFileRequired         = errors.New("original plan file (--orig) is required")
	ErrOriginalPlanFileNotExist         = errors.New("original plan file does not exist")
	ErrNewPlanFileNotExist              = errors.New("new plan file does not exist")
	ErrTerraformGenerateBackendArgument = errors.New("invalid arguments")

	ErrMissingPackerTemplate = errors.New("packer template is required")
	ErrMissingPackerManifest = errors.New("packer manifest is missing")

	ErrAtmosConfigIsNil              = errors.New("atmos config is nil")
	ErrFailedToInitializeAtmosConfig = errors.New("failed to initialize atmos config")
	ErrInvalidListMergeStrategy      = errors.New("invalid list merge strategy")
	ErrMerge                         = errors.New("merge error")

	// Stack processing errors.
	ErrStackManifestFileNotFound              = errors.New("stack manifest file not found")
	ErrInvalidStackManifest                   = errors.New("invalid stack manifest")
	ErrStackManifestSchemaValidation          = errors.New("stack manifest schema validation failed")
	ErrStackImportSelf                        = errors.New("stack manifest imports itself")
	ErrStackImportNotFound                    = errors.New("stack import not found")
	ErrStackCircularInheritance               = errors.New("circular component inheritance detected")
	ErrInvalidHooksSection                    = errors.New("invalid 'hooks' section in the file")
	ErrInvalidTerraformHooksSection           = errors.New("invalid 'terraform.hooks' section in the file")
	ErrInvalidComponentVars                   = errors.New("invalid component vars section")
	ErrInvalidComponentSettings               = errors.New("invalid component settings section")
	ErrInvalidComponentEnv                    = errors.New("invalid component env section")
	ErrInvalidComponentProviders              = errors.New("invalid component providers section")
	ErrInvalidComponentHooks                  = errors.New("invalid component hooks section")
	ErrInvalidComponentAuth                   = errors.New("invalid component auth section")
	ErrInvalidComponentMetadata               = errors.New("invalid component metadata section")
	ErrInvalidComponentBackendType            = errors.New("invalid component backend_type attribute")
	ErrInvalidComponentBackend                = errors.New("invalid component backend section")
	ErrInvalidComponentRemoteStateBackendType = errors.New("invalid component remote_state_backend_type attribute")
	ErrInvalidComponentRemoteStateBackend     = errors.New("invalid component remote_state_backend section")
	ErrInvalidComponentCommand                = errors.New("invalid component command attribute")
	ErrInvalidComponentOverrides              = errors.New("invalid component overrides section")
	ErrInvalidComponentOverridesVars          = errors.New("invalid component overrides vars section")
	ErrInvalidComponentOverridesSettings      = errors.New("invalid component overrides settings section")
	ErrInvalidComponentOverridesEnv           = errors.New("invalid component overrides env section")
	ErrInvalidComponentOverridesAuth          = errors.New("invalid component overrides auth section")
	ErrInvalidComponentOverridesCommand       = errors.New("invalid component overrides command attribute")
	ErrInvalidComponentOverridesProviders     = errors.New("invalid component overrides providers section")
	ErrInvalidComponentOverridesHooks         = errors.New("invalid component overrides hooks section")
	ErrInvalidComponentAttribute              = errors.New("invalid component attribute")
	ErrInvalidComponentMetadataComponent      = errors.New("invalid component metadata.component attribute")
	ErrInvalidSpaceLiftSettings               = errors.New("invalid spacelift settings section")
	ErrInvalidComponentMetadataInherits       = errors.New("invalid component metadata.inherits section")
	ErrComponentNotDefined                    = errors.New("component not defined in any config files")

	// Component registry errors.
	ErrComponentProviderNotFound          = errors.New("component provider not found")
	ErrComponentProviderNil               = errors.New("component provider cannot be nil")
	ErrComponentTypeEmpty                 = errors.New("component type is empty")
	ErrComponentEmpty                     = errors.New("component is empty")
	ErrStackEmpty                         = errors.New("stack is empty")
	ErrComponentConfigInvalid             = errors.New("component configuration invalid")
	ErrComponentListFailed                = errors.New("failed to list components")
	ErrComponentValidationFailed          = errors.New("component validation failed")
	ErrComponentExecutionFailed           = errors.New("component execution failed")
	ErrComponentArtifactGeneration        = errors.New("component artifact generation failed")
	ErrComponentProviderRegistration      = errors.New("failed to register component provider")
	ErrInvalidTerraformBackend            = errors.New("invalid terraform.backend section")
	ErrInvalidTerraformRemoteStateBackend = errors.New("invalid terraform.remote_state_backend section")
	ErrUnsupportedComponentType           = errors.New("unsupported component type")

	// List command errors.
	ErrInvalidStackPattern         = errors.New("invalid stack pattern")
	ErrEmptyTargetComponentName    = errors.New("target component name cannot be empty")
	ErrComponentsSectionNotFound   = errors.New("components section not found in stack")
	ErrComponentNotFoundInSections = errors.New("component not found in terraform or helmfile sections")
	ErrQueryFailed                 = errors.New("query execution failed")
	ErrTableTooWide                = errors.New("the table is too wide to display properly")
	ErrGettingCommonFlags          = errors.New("error getting common flags")
	ErrGettingAbstractFlag         = errors.New("error getting abstract flag")
	ErrGettingVarsFlag             = errors.New("error getting vars flag")
	ErrInitializingCLIConfig       = errors.New("error initializing CLI config")
	ErrDescribingStacks            = errors.New("error describing stacks")
	ErrComponentNameRequired       = errors.New("component name is required")

	// Atlantis errors.
	ErrAtlantisInvalidFlags          = errors.New("incompatible atlantis flags")
	ErrAtlantisProjectTemplateNotDef = errors.New("atlantis project template is not defined")
	ErrAtlantisConfigTemplateNotDef  = errors.New("atlantis config template is not defined")
	ErrAtlantisConfigTemplateNotSpec = errors.New("atlantis config template is not specified")

	// Validation errors.
	ErrValidationFailed = errors.New("validation failed")

	// Global/Stack-level section errors.
	ErrInvalidVarsSection               = errors.New("invalid vars section")
	ErrInvalidSettingsSection           = errors.New("invalid settings section")
	ErrInvalidEnvSection                = errors.New("invalid env section")
	ErrInvalidTerraformSection          = errors.New("invalid terraform section")
	ErrInvalidHelmfileSection           = errors.New("invalid helmfile section")
	ErrInvalidPackerSection             = errors.New("invalid packer section")
	ErrInvalidComponentsSection         = errors.New("invalid components section")
	ErrInvalidAuthSection               = errors.New("invalid auth section")
	ErrInvalidImportSection             = errors.New("invalid import section")
	ErrInvalidImport                    = errors.New("invalid import")
	ErrInvalidOverridesSection          = errors.New("invalid overrides section")
	ErrInvalidTerraformOverridesSection = errors.New("invalid terraform overrides section")
	ErrInvalidHelmfileOverridesSection  = errors.New("invalid helmfile overrides section")
	ErrInvalidBaseComponentConfig       = errors.New("invalid base component config")

	// Terraform-specific subsection errors.
	ErrInvalidTerraformCommand            = errors.New("invalid terraform command")
	ErrInvalidTerraformVars               = errors.New("invalid terraform vars section")
	ErrInvalidTerraformSettings           = errors.New("invalid terraform settings section")
	ErrInvalidTerraformEnv                = errors.New("invalid terraform env section")
	ErrInvalidTerraformProviders          = errors.New("invalid terraform providers section")
	ErrInvalidTerraformBackendType        = errors.New("invalid terraform backend_type")
	ErrInvalidTerraformRemoteStateType    = errors.New("invalid terraform remote_state_backend_type")
	ErrInvalidTerraformRemoteStateSection = errors.New("invalid terraform remote_state_backend section")
	ErrInvalidTerraformAuth               = errors.New("invalid terraform auth section")

	// Helmfile-specific subsection errors.
	ErrInvalidHelmfileCommand  = errors.New("invalid helmfile command")
	ErrInvalidHelmfileVars     = errors.New("invalid helmfile vars section")
	ErrInvalidHelmfileSettings = errors.New("invalid helmfile settings section")
	ErrInvalidHelmfileEnv      = errors.New("invalid helmfile env section")
	ErrInvalidHelmfileAuth     = errors.New("invalid helmfile auth section")

	// Helmfile configuration errors.
	ErrMissingHelmfileBasePath           = errors.New("helmfile base path is required")
	ErrMissingHelmfileKubeconfigPath     = errors.New("helmfile kubeconfig path is required")
	ErrMissingHelmfileAwsProfilePattern  = errors.New("helmfile AWS profile pattern is required")
	ErrMissingHelmfileClusterNamePattern = errors.New("helmfile cluster name pattern is required")

	// Packer-specific subsection errors.
	ErrInvalidPackerCommand  = errors.New("invalid packer command")
	ErrInvalidPackerVars     = errors.New("invalid packer vars section")
	ErrInvalidPackerSettings = errors.New("invalid packer settings section")
	ErrInvalidPackerEnv      = errors.New("invalid packer env section")
	ErrInvalidPackerAuth     = errors.New("invalid packer auth section")

	// Component type-specific section errors.
	ErrInvalidComponentsTerraform = errors.New("invalid components.terraform section")
	ErrInvalidComponentsHelmfile  = errors.New("invalid components.helmfile section")
	ErrInvalidComponentsPacker    = errors.New("invalid components.packer section")

	// Specific component configuration errors.
	ErrInvalidSpecificTerraformComponent = errors.New("invalid terraform component configuration")
	ErrInvalidSpecificHelmfileComponent  = errors.New("invalid helmfile component configuration")
	ErrInvalidSpecificPackerComponent    = errors.New("invalid packer component configuration")

	// Pro API client errors.
	ErrFailedToCreateRequest        = errors.New("failed to create request")
	ErrFailedToMarshalPayload       = errors.New("failed to marshal request body")
	ErrFailedToCreateAuthRequest    = errors.New("failed to create authenticated request")
	ErrFailedToMakeRequest          = errors.New("failed to make request")
	ErrFailedToUploadStacks         = errors.New("failed to upload stacks")
	ErrFailedToReadResponseBody     = errors.New("failed to read response body")
	ErrFailedToLockStack            = errors.New("failed to lock stack")
	ErrFailedToUnlockStack          = errors.New("failed to unlock stack")
	ErrOIDCWorkspaceIDRequired      = errors.New("workspace ID environment variable is required for OIDC authentication")
	ErrOIDCTokenExchangeFailed      = errors.New("failed to exchange OIDC token for Atmos token")
	ErrOIDCAuthFailedNoToken        = errors.New("OIDC authentication failed: no token")
	ErrNotInGitHubActions           = errors.New("not running in GitHub Actions or missing OIDC token environment variables")
	ErrFailedToGetOIDCToken         = errors.New("failed to get OIDC token")
	ErrFailedToDecodeOIDCResponse   = errors.New("failed to decode OIDC token response")
	ErrFailedToExchangeOIDCToken    = errors.New("failed to exchange OIDC token")
	ErrFailedToDecodeTokenResponse  = errors.New("failed to decode token response")
	ErrFailedToGetGitHubOIDCToken   = errors.New("failed to get GitHub OIDC token")
	ErrFailedToUploadInstances      = errors.New("failed to upload instances")
	ErrFailedToUploadInstanceStatus = errors.New("failed to upload instance status")
	ErrFailedToUnmarshalAPIResponse = errors.New("failed to unmarshal API response")
	ErrNilRequestDTO                = errors.New("nil request DTO")
	ErrAPIResponseError             = errors.New("API response error")

	// Exec package errors.
	ErrComponentAndStackRequired     = errors.New("component and stack are both required")
	ErrFailedToCreateAPIClient       = errors.New("failed to create API client")
	ErrFailedToProcessArgs           = errors.New("failed to process command-line arguments")
	ErrFailedToInitConfig            = errors.New("failed to initialize Atmos configuration")
	ErrFailedToCreateLogger          = errors.New("failed to create logger")
	ErrFailedToGetComponentFlag      = errors.New("failed to get '--component' flag")
	ErrFailedToGetStackFlag          = errors.New("failed to get '--stack' flag")
	ErrOPAPolicyViolations           = errors.New("OPA policy violations detected")
	ErrOPATimeout                    = errors.New("timeout evaluating OPA policy")
	ErrInvalidRegoPolicy             = errors.New("invalid Rego policy")
	ErrInvalidOPAPolicy              = errors.New("invalid OPA policy")
	ErrTerraformEnvCliVarJSON        = errors.New("failed to parse JSON variable from TF_CLI_ARGS environment variable")
	ErrWorkflowBasePathNotConfigured = errors.New("workflow base path is not configured")
	ErrWorkflowDirectoryDoesNotExist = errors.New("workflow directory does not exist")
	ErrInvalidComponentArgument      = errors.New("invalid arguments. The command requires one argument 'componentName'")
	ErrValidation                    = errors.New("validation failed")
	ErrCUEValidationUnsupported      = errors.New("validation using CUE is not supported yet")

	// List package errors.
	ErrExecuteDescribeStacks     = errors.New("failed to execute describe stacks")
	ErrProcessInstances          = errors.New("failed to process instances")
	ErrParseFlag                 = errors.New("failed to parse flag value")
	ErrFailedToFinalizeCSVOutput = errors.New("failed to finalize CSV output")
	ErrParseStacks               = errors.New("could not parse stacks")
	ErrParseComponents           = errors.New("could not parse components")
	ErrNoComponentsFound         = errors.New("no components found")
	ErrStackNotFound             = errors.New("stack not found")
	ErrProcessStack              = errors.New("error processing stack")

	// Cache-related errors.
	ErrCacheLocked    = errors.New("cache file is locked")
	ErrCacheRead      = errors.New("cache read failed")
	ErrCacheWrite     = errors.New("cache write failed")
	ErrCacheUnmarshal = errors.New("cache unmarshal failed")
	ErrCacheMarshal   = errors.New("cache marshal failed")
	ErrCacheDir       = errors.New("cache directory creation failed")

	// Logger errors.
	ErrInvalidLogLevel = errors.New("invalid log level")

	// File operation errors.
	ErrCopyFile            = errors.New("failed to copy file")
	ErrCreateDirectory     = errors.New("failed to create directory")
	ErrOpenFile            = errors.New("failed to open file")
	ErrStatFile            = errors.New("failed to stat file")
	ErrRemoveDirectory     = errors.New("failed to remove directory")
	ErrSetPermissions      = errors.New("failed to set permissions")
	ErrReadDirectory       = errors.New("failed to read directory")
	ErrComputeRelativePath = errors.New("failed to compute relative path")

	// OCI/Container image errors.
	ErrCreateTempDirectory   = ErrCreateTempDir // Alias to avoid duplicate sentinels
	ErrInvalidImageReference = errors.New("invalid image reference")
	ErrPullImage             = errors.New("failed to pull image")
	ErrGetImageDescriptor    = errors.New("cannot get a descriptor for the OCI image")
	ErrGetImageLayers        = errors.New("failed to get image layers")
	ErrProcessLayer          = errors.New("failed to process layer")
	ErrLayerDecompression    = errors.New("layer decompression error")
	ErrTarballExtraction     = errors.New("tarball extraction error")

	// Initialization and configuration errors.
	ErrInitializeCLIConfig = errors.New("error initializing CLI config")
	ErrGetHooks            = errors.New("error getting hooks")
	ErrSetFlag             = errors.New("failed to set flag")
	ErrVersionMismatch     = errors.New("version mismatch")

	// Download and client errors.
	ErrMergeConfiguration = errors.New("failed to merge configuration")

	// Template and documentation errors.
	ErrGenerateTerraformDocs = errors.New("failed to generate terraform docs")
	ErrMergeInputYAMLs       = errors.New("failed to merge input YAMLs")
	ErrRenderTemplate        = errors.New("failed to render template with datasources")
	ErrResolveOutputPath     = errors.New("failed to resolve output path")
	ErrWriteOutput           = errors.New("failed to write output")

	// Import-related errors.
	ErrBasePath             = errors.New("base path required to process imports")
	ErrTempDir              = errors.New("temporary directory required to process imports")
	ErrResolveLocal         = errors.New("failed to resolve local import path")
	ErrSourceDestination    = errors.New("source and destination cannot be nil")
	ErrImportPathRequired   = errors.New("import path required to process imports")
	ErrNoFileMatchPattern   = errors.New("no files matching patterns found")
	ErrMaxImportDepth       = errors.New("maximum import depth reached")
	ErrNoValidAbsolutePaths = errors.New("no valid absolute paths found")

	// Profiler-related errors.
	ErrProfilerStart           = errors.New("profiler start failed")
	ErrProfilerUnsupportedType = errors.New("profiler: unsupported profile type")
	ErrProfilerStartCPU        = errors.New("profiler: failed to start CPU profile")
	ErrProfilerStartTrace      = errors.New("profiler: failed to start trace profile")
	ErrProfilerCreateFile      = errors.New("profiler: failed to create profile file")

	// Auth package errors.
	ErrInvalidAuthConfig            = errors.New("invalid auth config")
	ErrInvalidIdentityKind          = errors.New("invalid identity kind")
	ErrInvalidIdentityConfig        = errors.New("invalid identity config")
	ErrInvalidProviderKind          = errors.New("invalid provider kind")
	ErrInvalidProviderConfig        = errors.New("invalid provider config")
	ErrAuthenticationFailed         = errors.New("authentication failed")
	ErrPostAuthenticationHookFailed = errors.New("post authentication hook failed")
	ErrAuthManager                  = errors.New("auth manager error")
	ErrDefaultIdentity              = errors.New("default identity error")
	ErrAwsAuth                      = errors.New("aws auth error")
	ErrAwsUserNotConfigured         = errors.New("aws user not configured")
	ErrAwsSAMLDecodeFailed          = errors.New("aws saml decode failed")
	ErrAwsMissingEnvVars            = errors.New("missing required AWS environment variables")
	ErrUnsupportedPlatform          = errors.New("unsupported platform")
	ErrUserAborted                  = errors.New("user aborted")

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
	ErrInvalidStackConfig            = errors.New("invalid stack config")
	ErrNoCommandSpecified            = errors.New("no command specified")
	ErrCommandNotFound               = errors.New("command not found")

	ErrInvalidSubcommand = errors.New("invalid subcommand")
	ErrSubcommandFailed  = errors.New("subcommand failed")

	ErrInvalidArgumentError = errors.New("invalid argument error")
	ErrMissingInput         = errors.New("missing input")

	ErrAuthAwsFileManagerFailed = errors.New("failed to create AWS file manager")

	ErrAuthOidcDecodeFailed    = errors.New("failed to decode OIDC token")
	ErrAuthOidcUnmarshalFailed = errors.New("failed to unmarshal oidc claims")

	// Store and hook errors.
	ErrNilTerraformOutput = errors.New("terraform output returned nil")
	ErrNilStoreValue      = errors.New("cannot store nil value")

	// Logout errors.
	ErrLogoutFailed         = errors.New("logout failed")
	ErrPartialLogout        = errors.New("partial logout")
	ErrLogoutNotSupported   = errors.New("logout not supported for this provider")
	ErrLogoutNotImplemented = errors.New("logout not implemented for this provider")
	ErrKeyringDeletion      = errors.New("keyring deletion failed")
	ErrProviderLogout       = errors.New("provider logout failed")
	ErrIdentityLogout       = errors.New("identity logout failed")
	ErrIdentityNotInConfig  = errors.New("identity not found in configuration")
	ErrProviderNotInConfig  = errors.New("provider not found in configuration")
	ErrInvalidLogoutOption  = errors.New("invalid logout option")
)

// ExitCodeError is a typed error that preserves subcommand exit codes.
// This allows the root command to exit with the same code as the subcommand.
type ExitCodeError struct {
	Code int
}

func (e ExitCodeError) Error() string {
	return fmt.Sprintf("command exited with code %d", e.Code)
}
