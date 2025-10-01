package errors

import (
	"errors"
)

const (
	ErrWrappingFormat       = "%w: %w"
	ErrStringWrappingFormat = "%w: %s"
	ErrValueWrappingFormat  = "%w: %v"
)

var (
	ErrNoGitRepo                             = errors.New("not in a git repository")
	ErrDownloadPackage                       = errors.New("failed to download package")
	ErrProcessOCIImage                       = errors.New("failed to process OCI image")
	ErrCopyPackage                           = errors.New("failed to copy package")
	ErrCreateTempDir                         = errors.New("failed to create temp directory")
	ErrUnknownPackageType                    = errors.New("unknown package type")
	ErrLocalMixinURICannotBeEmpty            = errors.New("local mixin URI cannot be empty")
	ErrLocalMixinInstallationNotImplemented  = errors.New("local mixin installation not implemented")
	ErrFailedToInitializeTUIModel            = errors.New("failed to initialize TUI model: verify terminal capabilities and permissions")
	ErrSetTempDirPermissions                 = errors.New("failed to set temp directory permissions")
	ErrCopyPackageToTarget                   = errors.New("failed to copy package to target")
	ErrNoValidInstallerPackage               = errors.New("no valid installer package provided")
	ErrFailedToInitializeTUIModelWithDetails = errors.New("failed to initialize TUI model: verify terminal capabilities and permissions")
	ErrValidPackage                          = errors.New("no valid installer package provided for")
	ErrTUIModel                              = errors.New("failed to initialize TUI model")
	ErrNoFilesFound                          = errors.New("no files found in directory")
	ErrMultipleFilesFound                    = errors.New("multiple files found in directory")
	ErrSourceDirNotExist                     = errors.New("source directory does not exist")
	ErrEmptyFilePath                         = errors.New("file path is empty")
	ErrEmptyWorkdir                          = errors.New("workdir cannot be empty")
	ErrWorkdirNotExist                       = errors.New("workdir does not exist")
	ErrPathResolution                        = errors.New("failed to resolve absolute path")
	ErrInvalidTemplateFunc                   = errors.New("invalid template function")
	ErrRefuseDeleteSymbolicLink              = errors.New("refusing to delete symbolic link")
	ErrNoDocsGenerateEntry                   = errors.New("no docs.generate entry found")
	ErrMissingDocType                        = errors.New("doc-type argument missing")
	ErrUnsupportedInputType                  = errors.New("unsupported input type")
	ErrMissingStackNameTemplateAndPattern    = errors.New("'stacks.name_pattern' or 'stacks.name_template' needs to be specified in 'atmos.yaml'")
	ErrFailedMarshalConfigToYaml             = errors.New("failed to marshal config to YAML")

	// ErrPlanHasDiff is returned when there are differences between two Terraform plan files.
	ErrPlanHasDiff = errors.New("plan files have differences")

	ErrInvalidTerraformFlagsWithAffectedFlag                 = errors.New("the `--affected` flag can't be used with the other multi-component (bulk operations) flags `--all`, `--query` and `--components`")
	ErrInvalidTerraformComponentWithMultiComponentFlags      = errors.New("the `component` argument can't be used with the multi-component (bulk operations) flags `--affected`, `--all`, `--query` and `--components`")
	ErrInvalidTerraformSingleComponentAndMultiComponentFlags = errors.New("the single-component flags (`--from-plan`, `--planfile`) can't be used with the multi-component (bulk operations) flags (`--affected`, `--all`, `--query`, `--components`)")

	ErrYamlFuncInvalidArguments         = errors.New("invalid number of arguments in the Atmos YAML function")
	ErrDescribeComponent                = errors.New("failed to describe component")
	ErrReadTerraformState               = errors.New("failed to read Terraform state")
	ErrEvaluateTerraformBackendVariable = errors.New("failed to evaluate terraform backend variable")
	ErrUnsupportedBackendType           = errors.New("unsupported backend type")
	ErrProcessTerraformStateFile        = errors.New("error processing terraform state file")

	ErrLoadAwsConfig    = errors.New("failed to load AWS config")
	ErrGetObjectFromS3  = errors.New("failed to get object from S3")
	ErrReadS3ObjectBody = errors.New("failed to read S3 object body")

	// Git-related errors.
	ErrGitNotAvailable      = errors.New("git must be available and on the PATH")
	ErrInvalidGitPort       = errors.New("invalid port number")
	ErrSSHKeyUsage          = errors.New("error using SSH key")
	ErrGitCommandExited     = errors.New("git command exited with non-zero status")
	ErrGitCommandFailed     = errors.New("failed to execute git command")
	ErrReadDestDir          = errors.New("failed to read the destination directory during git update")
	ErrRemoveGitDir         = errors.New("failed to remove the .git directory in the destination directory during git update")
	ErrUnexpectedGitOutput  = errors.New("unexpected 'git version' output")
	ErrGitVersionMismatch   = errors.New("git version requirement not met")
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
	ErrInvalidURL          = errors.New("invalid URL")
	ErrFailedToFindImport  = errors.New("failed to find import")

	ErrMissingStack                       = errors.New("stack is required; specify it on the command line using the flag `--stack <stack>` (shorthand `-s`)")
	ErrInvalidComponent                   = errors.New("invalid component")
	ErrAbstractComponentCantBeProvisioned = errors.New("abstract component cannot be provisioned")
	ErrLockedComponentCantBeProvisioned   = errors.New("locked component cannot be provisioned")

	ErrMissingPackerTemplate = errors.New("packer template is required; it can be specified in the `settings.packer.template` section in the Atmos component manifest, or on the command line via the flag `--template <template>` (shorthand `-t`)")
	ErrMissingPackerManifest = errors.New("packer manifest is missing")

	ErrAtmosConfigIsNil              = errors.New("atmos config is nil")
	ErrFailedToInitializeAtmosConfig = errors.New("failed to initialize atmos config")
	ErrInvalidListMergeStrategy      = errors.New("invalid list merge strategy")
	ErrMerge                         = errors.New("merge error")
	ErrInvalidStackManifest          = errors.New("invalid stack manifest")
	ErrInvalidLogLevel               = errors.New("invalid log level")

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
	ErrComponentAndStackRequired = errors.New("both '--component' and '--stack' flags must be provided")
	ErrFailedToCreateAPIClient   = errors.New("failed to create API client")
	ErrFailedToProcessArgs       = errors.New("failed to process command-line arguments")
	ErrFailedToInitConfig        = errors.New("failed to initialize Atmos configuration")
	ErrFailedToCreateLogger      = errors.New("failed to create logger")
	ErrFailedToGetComponentFlag  = errors.New("failed to get '--component' flag")
	ErrFailedToGetStackFlag      = errors.New("failed to get '--stack' flag")
	ErrOPAPolicyViolations       = errors.New("OPA policy violations detected")
	ErrInvalidOPAPolicy          = errors.New("invalid OPA policy")
	ErrTerraformEnvCliVarJSON    = errors.New("failed to parse JSON variable from TF_CLI_ARGS environment variable")

	// List package errors.
	ErrExecuteDescribeStacks     = errors.New("failed to execute describe stacks")
	ErrProcessInstances          = errors.New("failed to process instances")
	ErrParseFlag                 = errors.New("failed to parse flag value")
	ErrFailedToFinalizeCSVOutput = errors.New("failed to finalize CSV output")

	// Cache-related errors.
	ErrCacheLocked    = errors.New("cache file is locked")
	ErrCacheRead      = errors.New("cache read failed")
	ErrCacheWrite     = errors.New("cache write failed")
	ErrCacheUnmarshal = errors.New("cache unmarshal failed")
	ErrCacheMarshal   = errors.New("cache marshal failed")

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
	ErrUnsupportedPlatform          = errors.New("unsupported platform")

	// Auth manager and identity/provider resolution errors (centralized sentinels).
	ErrFailedToInitializeAuthManager  = errors.New("failed to initialize auth manager")
	ErrNoCredentialsFound             = errors.New("no credentials found for identity")
	ErrExpiredCredentials             = errors.New("credentials for identity are expired or invalid")
	ErrNilParam                       = errors.New("parameter cannot be nil")
	ErrInitializingProviders          = errors.New("failed to initialize providers")
	ErrInitializingIdentities         = errors.New("failed to initialize identities")
	ErrInitializingCredentialStore    = errors.New("failed to initialize credential store")
	ErrCircularDependency             = errors.New("circular dependency detected in identity chain")
	ErrIdentityNotFound               = errors.New("identity not found")
	ErrNoDefaultIdentity              = errors.New("no default identity configured for authentication")
	ErrMultipleDefaultIdentities      = errors.New("multiple default identities found")
	ErrTerraformPreHook               = errors.New("terraform pre-hook failed")
	ErrNoIdentitiesAvailable          = errors.New("no identities available")
	ErrFailedBuildAuthenticationChain = errors.New("failed to build authentication chain for identity")
	ErrInvalidStackConfig             = errors.New("invalid stack config")
	ErrNoCommandSpecified             = errors.New("no command specified")
	ErrCommandNotFound                = errors.New("command not found")

	ErrInvalidSubcommand = errors.New("invalid subcommand")
	ErrSubcommandFailed  = errors.New("subcommand failed")

	ErrInvalidArgumentError = errors.New("invalid argument error")
	ErrMissingInput         = errors.New("missing input")

	ErrAuthAwsFileManagerFailed = errors.New("failed to create AWS file manager")

	ErrAuthOidcDecodeFailed    = errors.New("failed to decode OIDC token")
	ErrAuthOidcUnmarshalFailed = errors.New("failed to unmarshal oidc claims")

	// Profiler-related errors.
	ErrProfilerStart = errors.New("profiler start failed")
)
