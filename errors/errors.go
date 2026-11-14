package errors

import (
	"errors"

	schemaPkg "github.com/cloudposse/atmos/pkg/schema"
)

var (
	ErrAISkillSourceInvalid                  = errors.New("invalid skill source")
	ErrAISkillSourceDrift                    = errors.New("skill installation is not current")
	ErrAISkillSourceOwnership                = errors.New("skill destination is not owned by this source")
	ErrAISkillSourceRecovery                 = errors.New("skill transaction requires recovery")
	ErrDownloadPackage                       = errors.New("failed to download package")
	ErrDownloadFile                          = errors.New("failed to download file")
	ErrInvalidClientMode                     = errors.New("invalid client mode for operation")
	ErrInvalidErrorMode                      = errors.New("invalid error mode")
	ErrParseFile                             = errors.New("failed to parse file")
	ErrParseURL                              = errors.New("failed to parse URL")
	ErrParseHexColor                         = errors.New("failed to parse hex color")
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
	ErrStdoutPipe                            = errors.New("failed to get stdout pipe")
	ErrStderrPipe                            = errors.New("failed to get stderr pipe")
	ErrCommandStart                          = errors.New("failed to start command")
	ErrUIFormatterNotInitialized             = errors.New("ui formatter not initialized")
	ErrMarkdownRendererInit                  = errors.New("failed to initialize markdown renderer")
	ErrMarkdownRender                        = errors.New("failed to render markdown content")
	ErrCastOutputExists                      = errors.New("cast output already exists")
	ErrEmptyCastFile                         = errors.New("empty cast file")
	ErrUnsupportedCastOutputExtension        = errors.New("unsupported cast output extension")
	ErrRenderOutputExists                    = errors.New("render output already exists")
	ErrMissingAgg                            = errors.New("missing required tool `agg`; install asciinema agg and retry")
	ErrMissingFFmpeg                         = errors.New("missing required tool `ffmpeg`; install FFmpeg and retry")
	ErrMissingRenderOutput                   = errors.New("specify an output path with --output")
	ErrRenderToolExecFailed                  = errors.New("managed renderer execution failed")
	ErrUnknownSessionAction                  = errors.New("unknown cast session action type")
	ErrSimulateActionMissingCallback         = errors.New("simulate session action has no callback")
	ErrWaitTimeout                           = errors.New("timed out waiting for cast output")
	ErrUnsupportedCastKey                    = errors.New("unsupported cast key")
	ErrMissingExecCommand                    = errors.New("exec recording requires a command")
	ErrIOContextNotInitialized               = errors.New("global I/O context is nil after initialization")
	ErrNoFilesFound                          = errors.New("no files found in directory")
	ErrMultipleFilesFound                    = errors.New("multiple files found in directory")
	ErrSourceDirNotExist                     = errors.New("source directory does not exist")
	ErrEmptyFilePath                         = errors.New("file path is empty")
	ErrEmptyWorkdir                          = errors.New("workdir cannot be empty")
	ErrWorkdirNotExist                       = errors.New("workdir does not exist")
	ErrPathResolution                        = errors.New("failed to resolve absolute path")
	ErrInvalidTemplateFunc                   = errors.New("invalid template function")
	ErrInvalidTemplateSettings               = errors.New("invalid template settings")
	ErrTemplateEvaluation                    = errors.New("template evaluation failed")
	ErrTemplateConversion                    = errors.New("template value conversion failed")
	ErrCommandEnvDecodeFailed                = schemaPkg.ErrCommandEnvDecodeFailed
	ErrComponentDependencyMissingComponent   = schemaPkg.ErrComponentDependencyMissingComponent
	ErrInvalidCastPlayback                   = errors.New("invalid cast playback group")
	ErrCastStepRequiresSteps                 = errors.New("cast step requires nested steps")
	ErrCastSessionRequiresActions            = errors.New("cast session step requires session actions")
	ErrInvalidCastMode                       = errors.New("cast step has invalid mode")
	ErrInvalidCastShellInit                  = errors.New("invalid cast session shell_init")
	ErrInvalidCastShell                      = errors.New("invalid cast session shell")
	ErrCastSessionShellExited                = errors.New("cast session shell exited")
	ErrWriteActionRequiresText               = errors.New("write action requires text")
	ErrKeyActionRequiresKey                  = errors.New("key action requires key")
	ErrPauseActionRequiresDuration           = errors.New("pause action requires duration")
	ErrWaitActionRequiresTextOrRegex         = errors.New("wait action requires exactly one of text or regex")
	ErrUnsupportedSessionAction              = errors.New("unsupported session action type")
	ErrInvalidSimulateMode                   = errors.New("simulate step has invalid mode")
	ErrSimulateTypedRequiresText             = errors.New("simulate typed step requires text")
	ErrInvalidSimulateJitter                 = errors.New("simulate typed step jitter must be between 0 and 1")
	ErrUnsupportedPromptStyle                = errors.New("unsupported simulate prompt style")
	ErrTapeParseFailed                       = errors.New("failed to parse tape script")
	ErrUnsupportedTapeDirective              = errors.New("unsupported tape directive")
	ErrTapeDirectiveRequiresSessionMode      = errors.New("tape directive requires mode: session")
	ErrUnsupportedTapeOutputExtension        = errors.New("tape output directive uses an unsupported file extension")
	ErrTapeSourceNotFound                    = errors.New("tape source file not found")
	ErrTapeSourceCycle                       = errors.New("tape source directive forms a cycle")
	ErrCastTapeSourceConflict                = errors.New("cast step cannot set both tape and source")
	ErrCastTapeSourceInvalid                 = errors.New("cast step source must be a path string")
	ErrCastTapeStepsConflict                 = errors.New("cast step cannot combine tape/source with explicit steps")
	ErrScreenshotActionRequiresPath          = errors.New("screenshot action requires a path")
	ErrScreenshotRenderFailed                = errors.New("failed to render marker screenshot")
	ErrCastTapeAndInputConflict              = errors.New("cast record cannot combine --tape input with another input source")
	ErrWorkdirPathRequired                   = errors.New("workdir path is required")
	ErrWorkdirSourceRequired                 = errors.New("workdir source is required")
	ErrWorkdirSourceKeyInvalid               = errors.New("workdir source map keys must be strings")
	ErrUnsafeVendorTarget                    = errors.New("unsafe vendor target directory")
	ErrInvalidConfig                         = errors.New("invalid configuration")
	ErrRefuseDeleteSymbolicLink              = errors.New("refusing to delete symbolic link")
	ErrRefuseWriteThroughSymlink             = errors.New("refusing to write through symbolic link")
	ErrNoDocsGenerateEntry                   = errors.New("no docs.generate entry found")
	ErrMissingDocType                        = errors.New("doc-type argument missing")
	ErrUnsupportedInputType                  = errors.New("unsupported input type")
	ErrMissingStackNameTemplateAndPattern    = errors.New("'stacks.name_template' (or the deprecated 'stacks.name_pattern') needs to be specified in 'atmos.yaml'")
	ErrStackNamePatternPartMissing           = errors.New("stack name pattern references a context part that is not defined in the stack file")
	ErrFailedMarshalConfigToYaml             = errors.New("failed to marshal config to YAML")
	ErrStacksDirectoryDoesNotExist           = errors.New("directory for Atmos stacks does not exist")
	ErrMissingAtmosConfig                    = errors.New("atmos configuration not found or invalid")
	ErrNotInGitRepository                    = errors.New("not inside a git repository")
	ErrAffectedBaseUnresolved                = errors.New("cannot resolve the base for --affected")
	ErrCommandNil                            = errors.New("command cannot be nil")
	ErrProcessStartFailed                    = errors.New("process start failed")
	ErrProcessWaitFailed                     = errors.New("process wait failed")
	ErrGitHubRateLimitExceeded               = errors.New("GitHub API rate limit exceeded")
	ErrInvalidLimit                          = errors.New("limit must be between 1 and 100")
	ErrInvalidOffset                         = errors.New("offset must be >= 0")
	ErrDuplicateFlagRegistration             = errors.New("duplicate flag registration")
	ErrReservedFlagName                      = errors.New("reserved flag name")
	ErrInvalidSinceDate                      = errors.New("invalid date format for --since")
	ErrTerminalTooNarrow                     = errors.New("terminal too narrow")
	ErrSpinnerReturnedNilModel               = errors.New("spinner returned nil model")
	ErrSpinnerUnexpectedModelType            = errors.New("spinner returned unexpected model type")
	ErrUnexpectedModelType                   = errors.New("unexpected model type")
	ErrSpinnerOperationInterrupted           = errors.New("operation was interrupted")

	// Version Tracker errors.
	ErrVersionTrackNotFound             = errors.New("version track not found")
	ErrVersionNotFound                  = errors.New("version not found")
	ErrVersionNotLocked                 = errors.New("version not locked")
	ErrVersionTrackNotVerified          = errors.New("version track is not verified")
	ErrDesiredVersionRequired           = errors.New("desired version is required")
	ErrVersionEntryExists               = errors.New("version entry already exists")
	ErrVersionEntryNotFound             = errors.New("version entry not found")
	ErrInvalidVersionCooldown           = errors.New("invalid cooldown")
	ErrUnsupportedVersionTrackFormat    = errors.New("unsupported output format (supported: yaml, json)")
	ErrUnsupportedVersionShow           = errors.New("unsupported --show value (supported: desired, locked)")
	ErrUnsupportedVersionField          = errors.New("unsupported --show value (supported: name, ecosystem, datasource, provider, package, desired, group, update, include, exclude, prerelease, labels, locked)")
	ErrVersionFilesDrift                = errors.New("version-managed files are out of date; run `atmos version track apply`")
	ErrUnknownVersionFileManager        = errors.New("unknown file manager")
	ErrDuplicateVersionFileManager      = errors.New("duplicate file manager registration")
	ErrVersionMarkerBadMatch            = errors.New("marker match expression must compile and contain one capture group")
	ErrVersionJSONOptionsInvalid        = errors.New("json manager options are invalid")
	ErrVersionJSONInvalidContent        = errors.New("json manager target file is not valid JSON")
	ErrVersionJSONSetFailed             = errors.New("json manager failed to set value at path")
	ErrVersionJSONAppendPathUnsupported = errors.New("json manager does not support array-append (-1) paths: they cannot be applied idempotently")
	ErrVersionJSONPathTypeMismatch      = errors.New("json manager refused to overwrite a container (object or array) with a scalar value")
	ErrVersionJSONPathNotFound          = errors.New("json manager path did not match anything in the target file")
	ErrVersionJSONDuplicatePath         = errors.New("json manager has more than one set entry targeting the same path")
	ErrVersionJSONExpandPathsFailed     = errors.New("json manager failed to expand configured paths")
	ErrVersionJSONReadFailed            = errors.New("json manager failed to read target file")
	ErrVersionJSONFormatInvalid         = errors.New("json manager set entry format template is invalid")
	ErrVersionYAMLOptionsInvalid        = errors.New("yaml manager options are invalid")
	ErrVersionYAMLSetFailed             = errors.New("yaml manager failed to set value at path")
	ErrVersionYAMLPathTypeMismatch      = errors.New("yaml manager refused to overwrite a map or list with a scalar value")
	ErrVersionYAMLDuplicatePath         = errors.New("yaml manager has more than one set entry targeting the same path")
	ErrVersionYAMLExpandPathsFailed     = errors.New("yaml manager failed to expand configured paths")
	ErrVersionYAMLReadFailed            = errors.New("yaml manager failed to read target file")
	ErrVersionYAMLFormatInvalid         = errors.New("yaml manager set entry format template is invalid")
	ErrVersionLockWriteFailed           = errors.New("failed to write version lock file")

	// Theme-related errors.
	ErrThemeNotFound = errors.New("theme not found")
	ErrInvalidTheme  = errors.New("invalid theme")

	// Experimental feature errors.
	ErrExperimentalDisabled   = errors.New("experimental command is disabled")
	ErrExperimentalRequiresIn = errors.New("experimental command requires explicit opt-in")

	// Authentication and TTY errors.
	ErrAuthConsole          = errors.New("auth console operation failed")
	ErrProviderNotSupported = errors.New("provider does not support this operation")
	// ErrWebflowRequiresAWSUser indicates --webflow was used without a direct aws/user identity.
	ErrWebflowRequiresAWSUser = errors.New("--webflow requires an aws/user identity")
	ErrUnknownServiceAlias    = errors.New("unknown service alias")
	ErrUnknownHelpTopic       = errors.New("unknown help topic")
	ErrTTYRequired            = errors.New("requires a TTY")
	ErrInvalidAuthManagerType = errors.New("invalid authManager type")
	ErrStreamingNotSupported  = errors.New("streaming UI not supported in this environment")

	// Component and positional argument errors.
	ErrComponentRequired          = errors.New("component is required")
	ErrInvalidPositionalArgs      = errors.New("invalid positional arguments")
	ErrWorkflowNameRequired       = errors.New("workflow name is required")
	ErrInvalidStackConfiguration  = errors.New("invalid stack configuration")
	ErrPathNotWithinComponentBase = errors.New("path is not within component base path")
	ErrStackRequired              = errors.New("--stack flag is required")
	ErrStackHasNoLocals           = errors.New("stack has no locals defined")
	ErrNoStackManifestsFound      = errors.New("no stack manifests found")

	// ErrPlanHasDiff is returned when there are differences between two Terraform plan files.
	ErrPlanHasDiff = errors.New("plan files have differences")

	// ErrPlanVerificationFailed is returned when a stored planfile differs from the current state during --verify-plan.
	ErrPlanVerificationFailed = errors.New("plan verification failed: stored plan differs from current state")

	// ErrSavedPlanNotFound is returned when applying a plugin-managed component's saved plan and none exists.
	ErrSavedPlanNotFound = errors.New("no saved plan found")

	// ErrStoredPlanfileMissing is returned when a deploy expected a stored planfile to verify against but none was found.
	ErrStoredPlanfileMissing = errors.New("plan verification failed: no stored planfile was found to verify against")

	// ErrPlanfileStorageNotConfigured is returned when planfile verification is explicitly requested (`--verify-plan`) but no planfile storage is configured.
	ErrPlanfileStorageNotConfigured = errors.New("planfile verification was requested but planfile storage is not configured")

	ErrInvalidTerraformFlagsWithAffectedFlag                 = errors.New("the `--affected` flag can't be used with the other multi-component (bulk operations) flags `--all`, `--query` and `--components`")
	ErrInvalidTerraformComponentWithMultiComponentFlags      = errors.New("the component argument can't be used with the multi-component (bulk operations) flags `--affected`, `--all`, `--query`, `--components`, `--tags` and `--labels`")
	ErrInvalidTerraformSingleComponentAndMultiComponentFlags = errors.New("the single-component flags (`--from-plan`, `--planfile`) can't be used with the multi-component (bulk operations) flags (`--affected`, `--all`, `--query`, `--components`)")
	ErrClosureFlagsRequireMultiComponent                     = errors.New("the `--include-dependencies` and `--include-dependents` flags expand a multi-component selection and require one of `--all`, `--components`, `--query`, `-s`, `--tags`, `--labels`, or `--affected`")

	ErrLabelNotFound             = errors.New("component label not found")
	ErrYamlFuncInvalidArguments  = errors.New("invalid number of arguments in the Atmos YAML function")
	ErrYamlFuncTemplateEmbedding = errors.New("a YAML function value cannot be used in this template expression")
	//nolint:staticcheck // Error starts with a proper product name.
	ErrYamlFuncMaxResolutionDepth       = errors.New("Atmos YAML function resolution exceeded the maximum dependency depth (likely an undetected circular dependency)")
	ErrComponentFuncDuringManifestLoad  = errors.New("atmos.Component cannot be evaluated while stack manifests are being loaded")
	ErrDeferredTemplateContextMissing   = errors.New("cannot resolve deferred !template value: template context is unavailable because template processing was disabled for this invocation")
	ErrAwsGetCallerIdentity             = errors.New("failed to get AWS caller identity")
	ErrUnsupportedYamlTag               = errors.New("unsupported YAML tag")
	ErrAwsDescribeOrganization          = errors.New("failed to describe AWS organization")
	ErrDescribeComponent                = errors.New("failed to describe component")
	ErrReadTerraformState               = errors.New("failed to read Terraform state")
	ErrEvaluateTerraformBackendVariable = errors.New("failed to evaluate terraform backend variable")
	ErrEvaluateOutput                   = errors.New("failed to evaluate output expression")

	// Recoverable YAML function errors - use YQ default if available.
	// These errors indicate the data is not available but do not represent API failures.
	ErrTerraformStateNotProvisioned = errors.New("terraform state not provisioned")
	ErrTerraformOutputNotFound      = errors.New("terraform output not found")
	ErrTerraformOutputFailed        = errors.New("failed to retrieve terraform outputs")
	ErrParseTerraformOutput         = errors.New("failed to parse terraform output")

	// Terraform output component configuration errors.
	ErrMissingExecutable      = errors.New("component does not have 'command' (executable) defined")
	ErrMissingWorkspace       = errors.New("component does not have terraform workspace defined")
	ErrMissingComponentInfo   = errors.New("component does not have 'component_info' defined")
	ErrInvalidComponentInfoS  = errors.New("component has invalid 'component_info' section")
	ErrMissingComponentPath   = errors.New("component has invalid 'component_info.component_path'")
	ErrBackendFileGeneration  = errors.New("failed to generate backend file")
	ErrProviderFileGeneration = errors.New("failed to generate provider override file")
	ErrTerraformInit          = errors.New("terraform init failed")
	ErrTerraformWorkspaceOp   = errors.New("terraform workspace operation failed")

	// Terraform auto-init errors.
	ErrTerraformInitRequired            = errors.New("terraform reports the working directory must be initialized")
	ErrTerraformInitReconfigureRequired = errors.New("terraform init -reconfigure is required because the backend configuration changed")
	ErrTerraformInitUpgradeRequired     = errors.New("terraform init -upgrade is required to satisfy provider or module constraints")
	ErrInitFingerprint                  = errors.New("failed to compute terraform init fingerprint")
	ErrInitMarker                       = errors.New("failed to read or write terraform init marker")
	ErrInvalidMocksMode                 = errors.New("invalid components.terraform.mocks.mode (expected fallback or always)")
	ErrInvalidInitMode                  = errors.New("invalid components.terraform.init.mode (expected auto, always, or never)")
	ErrInvalidInitReconfigure           = errors.New("invalid components.terraform.init.reconfigure (expected auto, always, or never)")
	ErrInvalidInitUpgrade               = errors.New("invalid components.terraform.init.upgrade (expected auto, always, or never)")

	// Terraform lint errors.
	ErrTerraformLint             = errors.New("terraform lint failed")
	ErrTerraformLintAuth         = errors.New("failed to initialize authentication for terraform lint")
	ErrTerraformLintAffected     = errors.New("failed to determine affected terraform lint targets")
	ErrBuildTerraformLintTargets = errors.New("failed to build terraform lint targets")

	// --use-mocks errors.
	ErrTerraformComponentMocksNotDeclared = errors.New("terraform component does not declare `mocks` required by --use-mocks")
	ErrTerraformMockOutputNotDeclared     = errors.New("mocked terraform output is not declared")

	// API/infrastructure errors - should cause non-zero exit.
	// These errors indicate backend API failures that should not use YQ defaults.
	ErrTerraformBackendAPIError  = errors.New("terraform backend API error")
	ErrUnsupportedBackendType    = errors.New("unsupported backend type")
	ErrProcessTerraformStateFile = errors.New("error processing terraform state file")
	ErrGetObjectFromS3           = errors.New("failed to get object from S3")
	ErrReadS3ObjectBody          = errors.New("failed to read S3 object body")
	ErrS3BucketAccessDenied      = errors.New("access denied to S3 bucket")
	ErrInvalidSSECustomerKey     = errors.New("invalid SSE-C customer encryption key")
	ErrCreateGCSClient           = errors.New("failed to create GCS client")
	ErrGetObjectFromGCS          = errors.New("failed to get object from GCS")
	ErrReadGCSObjectBody         = errors.New("failed to read GCS object body")
	ErrGCSBucketRequired         = errors.New("bucket is required for gcs backend")
	ErrInvalidBackendConfig      = errors.New("invalid backend configuration")

	// Azure Blob Storage specific errors.
	ErrGetBlobFromAzure       = errors.New("failed to get blob from Azure Blob Storage")
	ErrReadAzureBlobBody      = errors.New("failed to read Azure blob body")
	ErrCreateAzureCredential  = errors.New("failed to create Azure credential")
	ErrCreateAzureClient      = errors.New("failed to create Azure Blob Storage client")
	ErrAzureContainerRequired = errors.New("container_name is required for azurerm backend")
	ErrStorageAccountRequired = errors.New("storage_account_name is required for azurerm backend")
	ErrAzurePermissionDenied  = errors.New("permission denied accessing Azure blob")

	// Azure authentication errors.
	ErrAzureOIDClaimNotFound      = errors.New("oid claim not found in token")
	ErrAzureUsernameClaimNotFound = errors.New("no username claim found in token (tried upn, unique_name, email)")
	ErrAzureInvalidJWTFormat      = errors.New("invalid JWT format")
	ErrAzureExpirationTimeEmpty   = errors.New("expiration time is empty")
	ErrAzureTimeParseFailure      = errors.New("unable to parse time: tried RFC3339, local time formats, and Unix timestamp")
	ErrAzureNoAccountsInCache     = errors.New("no accounts found in cache")
	ErrAzureNoAccountForTenant    = errors.New("no account found for tenant")

	// Azure PIM (Privileged Identity Management) role activation errors.
	// ErrAzurePIMNotEligible indicates the principal has no eligible assignment for the
	// requested role at the requested scope, so there is nothing to activate.
	ErrAzurePIMNotEligible = errors.New("principal is not eligible for the requested PIM role at the given scope")
	// ErrAzurePIMJustificationRequired indicates activation needs a justification but none
	// was supplied and no interactive prompt could be shown (CI, exec, MCP startup).
	ErrAzurePIMJustificationRequired = errors.New("a justification is required to activate the PIM role")
	// ErrAzurePIMActivationFailed indicates the PIM self-activation request was denied or
	// otherwise failed server-side.
	ErrAzurePIMActivationFailed = errors.New("PIM role activation failed")
	// ErrAzurePIMActivationTimeout indicates the bounded wait for a pending activation
	// (for example awaiting an approver) elapsed before the role became active.
	ErrAzurePIMActivationTimeout = errors.New("timed out waiting for PIM role activation to complete")
	// ErrAzurePIMRequestFailed indicates an ARM PIM REST call returned an unexpected status.
	ErrAzurePIMRequestFailed = errors.New("request to the Azure PIM API failed")
	// ErrAzurePIMInvalidScope indicates the configured scope is not a valid ARM resource path
	// or would redirect the credential-bearing ARM request to an unexpected host. Rejecting it
	// prevents a crafted scope (for example one injecting URL user-information) from sending the
	// parent bearer token to an attacker-controlled host.
	ErrAzurePIMInvalidScope = errors.New("invalid Azure PIM scope")

	ErrBackendConfigRequired       = errors.New("backend configuration is required")
	ErrBackendTypeRequired         = errors.New("backend_type is required")
	ErrBackendSectionMissing       = errors.New("no 'backend' section configured")
	ErrBackendTypeMissing          = errors.New("no 'backend_type' configured")
	ErrBackendTypeEmptyAfterRender = errors.New("'backend_type' is empty after template processing")
	ErrBackendConfigEmpty          = errors.New("'backend' section is empty but 'backend_type' requires configuration")

	// Git-related errors.
	ErrGitNotAvailable             = errors.New("git must be available and on the PATH")
	ErrGitRoot                     = errors.New("failed to get git repository root")
	ErrGitSHA                      = errors.New("failed to get git SHA")
	ErrGitBranch                   = errors.New("failed to get git branch")
	ErrGitRef                      = errors.New("failed to get git ref")
	ErrGitWorktree                 = errors.New("failed to get git worktree")
	ErrDetachedHead                = errors.New("git HEAD is detached")
	ErrEmptyBranchName             = errors.New("git branch name is empty")
	ErrInvalidGitPort              = errors.New("invalid port number")
	ErrSSHKeyUsage                 = errors.New("error using SSH key")
	ErrGitCommandExited            = errors.New("git command exited with non-zero status")
	ErrGitCommandFailed            = errors.New("failed to execute git command")
	ErrReadDestDir                 = errors.New("failed to read the destination directory during git update")
	ErrRemoveGitDir                = errors.New("failed to remove the .git directory in the destination directory during git update")
	ErrUnexpectedGitOutput         = errors.New("unexpected 'git version' output")
	ErrUnexpectedGitRevParseOutput = errors.New("unexpected 'git rev-parse' output")
	ErrGitVersionMismatch          = errors.New("git version requirement not met")
	ErrRemoteRepoNotGitRepo        = errors.New("target remote repository is not a git repository")
	ErrFailedToGetLocalRepo        = errors.New("failed to get local repository")
	ErrFailedToGetRepoInfo         = errors.New("failed to get repository info")
	ErrLocalRepoFetch              = errors.New("local repo unavailable")
	ErrGitRefNotFound              = errors.New("git reference not found on local filesystem")
	ErrGitFileNotFound             = errors.New("file not found in git reference")
	ErrGitWorktreeAdd              = errors.New("failed to create git worktree")
	ErrGitWorktreePruneIncomplete  = errors.New("git worktree prune completed but worktree path still exists")
	ErrFetchOrigin                 = errors.New("failed to fetch from origin")
	ErrDeepenOrigin                = errors.New("failed to deepen fetch from origin")
	ErrGitRepositoryNotFound       = errors.New("git repository not configured")
	ErrGitAuthFailed               = errors.New("git authentication failed")
	ErrGitPushRejected             = errors.New("git push rejected: non-fast-forward")
	ErrGitDirtyUnmanagedFiles      = errors.New("unmanaged dirty files detected outside commit paths")
	ErrGitPathEscapesWorktree      = errors.New("path resolves outside git worktree")
	ErrGitHookNotConfigured        = errors.New("git hook not configured")
	ErrGitRepositoryRequired       = errors.New("git repository name or URI is required")
	ErrGitProviderNotFound         = errors.New("git provider not registered")
	ErrGitWorkdirExists            = errors.New("git workdir already exists")
	ErrGitNoTrackingBranch         = errors.New("no branch to pull: the current branch has no upstream")
	ErrGitWorkdirNotInitialized    = errors.New("git repository not cloned or initialized")
	ErrGitTargetPathInvalid        = errors.New("git target path must not be empty or the repository root")
	ErrGitTargetSplitInvalid       = errors.New("git target 'split' must be a boolean")
	ErrGitArtifactWrite            = errors.New("failed to write provision artifact")
	ErrUnsafeForkCheckout          = errors.New("refusing to clone untrusted fork content in an elevated CI event")
	ErrGitArtifactRead             = errors.New("failed to read provision artifact")

	// I/O and output errors.
	ErrBuildIOConfig        = errors.New("failed to build I/O config")
	ErrUnknownStream        = errors.New("unknown I/O stream")
	ErrWriteToStream        = errors.New("failed to write to stream")
	ErrMaskingContent       = errors.New("failed to mask content")
	ErrUnboundedMaskPattern = errors.New("mask patterns that can match carriage returns or newlines must have a finite maximum match length")
	ErrHeadLookup           = errors.New("HEAD not found")
	ErrInvalidFormat        = errors.New("invalid format")
	ErrOutputFormat         = errors.New("output format error")

	// File operation errors.
	ErrRefusingToDeleteSymlink = ErrRefuseDeleteSymbolicLink

	// Scheduler errors.
	ErrNilGraph      = errors.New("scheduler graph cannot be nil")
	ErrNilDispatcher = errors.New("scheduler dispatcher cannot be nil")
	ErrNodeFailed    = errors.New("scheduler node failed")
	ErrNodeSkipped   = errors.New("scheduler node skipped")
	ErrNodeNotFound  = errors.New("scheduler node not found")
	ErrInvalidGraph  = errors.New("scheduler graph is invalid")
	ErrInvalidWorker = errors.New("scheduler max concurrency must be greater than zero")

	// Slice utility errors.
	ErrNilInput         = errors.New("input must not be nil")
	ErrNonStringElement = errors.New("element is not a string")

	// Merge-related errors.
	ErrEmptyPath                = errors.New("empty path")
	ErrCannotNavigatePath       = errors.New("cannot navigate path: field is not a map")
	ErrUnknownListMergeStrategy = errors.New("unknown list merge strategy")

	ErrReadFile    = errors.New("error reading file")
	ErrInvalidFlag = errors.New("invalid flag")

	// ErrForbiddenSelectorFunction enforces the selector purity contract:
	// metadata.tags/metadata.labels drive scoping decisions before evaluation,
	// so by design they may not contain constructs that require authentication
	// or process execution (e.g. !terraform.state, !store, !exec, atmos.Component).
	ErrForbiddenSelectorFunction = errors.New("forbidden function in labels/tags selector")

	// Dependency management errors.
	ErrDependencyConstraint = errors.New("dependency constraint validation failed")
	ErrDependencyResolution = errors.New("dependency resolution failed")
	ErrToolInstall          = errors.New("tool installation failed")

	// ErrToolVersionsConflict reports .tool-versions entries that identify the same
	// tool (for example `tofu` and `opentofu/opentofu`) but pin different versions.
	ErrToolVersionsConflict = errors.New("conflicting .tool-versions entries for the same tool")

	// ErrInvalidToolchainInstall reports an unknown toolchain.install policy.
	ErrInvalidToolchainInstall = errors.New("invalid toolchain.install (expected never, declared, auto, or always)")

	// ErrToolNotInstalled reports an explicit dependency that is not installed while
	// toolchain.install is never, which forbids installing it.
	ErrToolNotInstalled = errors.New("required tool is not installed and toolchain.install is never")

	// Helm plugin errors.
	ErrInvalidHelmPluginSpec = errors.New("invalid helm plugin specification")
	ErrHelmPluginInstall     = errors.New("helm plugin installation failed")
	ErrHelmBinaryNotFound    = errors.New("helm binary not found")
)
