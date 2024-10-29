package config

const (
	DefaultStackConfigFileExtension       = ".yaml"
	DefaultVendoringManifestFileExtension = ".yaml"
	CliConfigFileName                     = "atmos"
	SystemDirConfigFilePath               = "/usr/local/etc/atmos"
	WindowsAppDataEnvVar                  = "LOCALAPPDATA"

	// GlobalOptionsFlag is a custom flag to specify helmfile `GLOBAL OPTIONS`
	// https://github.com/roboll/helmfile#cli-reference
	GlobalOptionsFlag = "--global-options"

	TerraformCommandFlag        = "--terraform-command"
	TerraformDirFlag            = "--terraform-dir"
	HelmfileCommandFlag         = "--helmfile-command"
	HelmfileDirFlag             = "--helmfile-dir"
	CliConfigDirFlag            = "--config-dir"
	StackDirFlag                = "--stacks-dir"
	BasePathFlag                = "--base-path"
	WorkflowDirFlag             = "--workflows-dir"
	KubeConfigConfigFlag        = "--kubeconfig-path"
	JsonSchemaDirFlag           = "--schemas-jsonschema-dir"
	OpaDirFlag                  = "--schemas-opa-dir"
	CueDirFlag                  = "--schemas-cue-dir"
	AtmosManifestJsonSchemaFlag = "--schemas-atmos-manifest"

	DeployRunInitFlag           = "--deploy-run-init"
	AutoGenerateBackendFileFlag = "--auto-generate-backend-file"
	AppendUserAgentFlag         = "--append-user-agent"
	InitRunReconfigure          = "--init-run-reconfigure"

	FromPlanFlag       = "--from-plan"
	PlanFileFlag       = "--planfile"
	DryRunFlag         = "--dry-run"
	SkipInitFlag       = "--skip-init"
	RedirectStdErrFlag = "--redirect-stderr"

	HelpFlag1 = "-h"
	HelpFlag2 = "--help"

	ComponentVendorConfigFileName = "component.yaml"
	AtmosVendorConfigFileName     = "vendor"

	ImportSectionName      = "import"
	OverridesSectionName   = "overrides"
	ProvidersSectionName   = "providers"
	VarsSectionName        = "vars"
	SettingsSectionName    = "settings"
	EnvSectionName         = "env"
	BackendSectionName     = "backend"
	BackendTypeSectionName = "backend_type"
	MetadataSectionName    = "metadata"
	ComponentSectionName   = "component"
	ComponentsSectionName  = "components"
	CommandSectionName     = "command"
	TerraformSectionName   = "terraform"
	HelmfileSectionName    = "helmfile"
	WorkspaceSectionName   = "workspace"

	LogsLevelFlag = "--logs-level"
	LogsFileFlag  = "--logs-file"

	SettingsListMergeStrategyFlag = "--settings-list-merge-strategy"

	// Atmos Pro
	AtmosProBaseUrlEnvVarName  = "ATMOS_PRO_BASE_URL"
	AtmosProEndpointEnvVarName = "ATMOS_PRO_ENDPOINT"
	AtmosProTokenEnvVarName    = "ATMOS_PRO_TOKEN"
	AtmosProDefaultBaseUrl     = "https://app.cloudposse.com"
	AtmosProDefaultEndpoint    = "api"
)
