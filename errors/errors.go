package errors

import (
	"errors"
)

const (
	ErrWrappingFormat       = "%w: %w"
	ErrStringWrappingFormat = "%w: %s"
)

var (
	ErrDownloadPackage                       = errors.New("failed to download package")
	ErrDownloadFile                          = errors.New("failed to download file")
	ErrParseURL                              = errors.New("failed to parse URL")
	ErrInvalidURL                            = errors.New("invalid URL")
	ErrCreateDownloadClient                  = errors.New("failed to create download client")
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
	ErrPathResolution                        = errors.New("failed to resolve absolute path")
	ErrInvalidTemplateFunc                   = errors.New("invalid template function")
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
	ErrGitNotAvailable     = errors.New("git must be available and on the PATH")
	ErrInvalidGitPort      = errors.New("invalid port number")
	ErrSSHKeyUsage         = errors.New("error using ssh key")
	ErrGitCommandExited    = errors.New("git command exited with error")
	ErrGitCommandFailed    = errors.New("error running git command")
	ErrReadDestDir         = errors.New("failed to read the destination directory during git update")
	ErrRemoveGitDir        = errors.New("failed to remove the .git directory in the destination directory during git update")
	ErrUnexpectedGitOutput = errors.New("unexpected 'git version' output")
	ErrGitVersionMismatch  = errors.New("git version requirement not met")

	ErrReadFile    = errors.New("error reading file")
	ErrInvalidFlag = errors.New("invalid flag")

	// Config loading errors.
	ErrAtmosDirConfigNotFound      = errors.New("atmos config directory not found")
	ErrReadConfig                  = errors.New("failed to read config")
	ErrMergeTempConfig             = errors.New("failed to merge temp config")
	ErrPreprocessYAMLFunctions     = errors.New("failed to preprocess YAML functions")
	ErrMergeEmbeddedConfig         = errors.New("failed to merge embedded config")
	ErrExpectedDirOrPattern        = errors.New("--config-path expected directory found file")
	ErrFileNotFound                = errors.New("file not found")
	ErrExpectedFile                = errors.New("--config expected file found directory")
	ErrAtmosArgConfigNotFound      = errors.New("atmos configuration not found")
	ErrAtmosFilesDirConfigNotFound = errors.New("`atmos.yaml` or `.atmos.yaml` configuration file not found on directory")

	ErrMissingStack                       = errors.New("stack is required; specify it on the command line using the flag `--stack <stack>` (shorthand `-s`)\"")
	ErrInvalidComponent                   = errors.New("invalid component")
	ErrAbstractComponentCantBeProvisioned = errors.New("abstract component cannot be provisioned")
	ErrLockedComponentCantBeProvisioned   = errors.New("locked component cannot be provisioned")

	ErrMissingPackerTemplate = errors.New("packer template is required; it can be specified in the `settings.packer.template` section in the Atmos component manifest, or on the command line via the flag `--template <template>` (shorthand `-t`)")
	ErrMissingPackerManifest = errors.New("packer manifest is missing")

	ErrAtmosConfigIsNil         = errors.New("atmos config is nil")
	ErrInvalidListMergeStrategy = errors.New("invalid list merge strategy")
	ErrMerge                    = errors.New("merge error")

	// Stack processing errors.
	ErrInvalidStackManifest         = errors.New("invalid stack manifest")
	ErrDeepMergeStackConfigs        = errors.New("failed to deep-merge stack configs")
	ErrInvalidHooksSection          = errors.New("invalid 'hooks' section in the file")
	ErrInvalidTerraformHooksSection = errors.New("invalid 'terraform.hooks' section in the file")
)
