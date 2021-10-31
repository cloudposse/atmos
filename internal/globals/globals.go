package globals

const (
	DefaultStackConfigFileExtension = ".yaml"
	ConfigFileName                  = "atmos.yaml"
	SystemDirConfigFilePath         = "/usr/local/etc/atmos"
	WindowsAppDataEnvVar            = "LOCALAPPDATA"

	// Custom flag to specify helmfile `GLOBAL OPTIONS`
	// https://github.com/roboll/helmfile#cli-reference
	GlobalOptionsFlag = "--global-options"

	TerraformDirFlag            = "--terraform-dir"
	HelmfileDirFlag             = "--helmfile-dir"
	ConfigDirFlag               = "--config-dir"
	StackDirFlag                = "--stacks-dir"
	DeployRunInitFlag           = "--deploy-run-init"
	AutoGenerateBackendFileFlag = "--auto-generate-backend-file"
)

var (
	LogVerbose = false
)
