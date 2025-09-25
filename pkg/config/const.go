package config

const (
	AtmosCommand         = "atmos"
	CliConfigFileName    = "atmos"
	DotCliConfigFileName = ".atmos"

	SystemDirConfigFilePath = "/usr/local/etc/atmos"
	WindowsAppDataEnvVar    = "LOCALAPPDATA"

	// GlobalOptionsFlag is a custom flag to specify helmfile `GLOBAL OPTIONS`
	// https://github.com/roboll/helmfile#cli-reference
	GlobalOptionsFlag = "--global-options"

	TerraformCommandFlag        = "--terraform-command"
	TerraformDirFlag            = "--terraform-dir"
	HelmfileCommandFlag         = "--helmfile-command"
	HelmfileDirFlag             = "--helmfile-dir"
	PackerCommandFlag           = "--packer-command"
	PackerDirFlag               = "--packer-dir"
	CliConfigDirFlag            = "--config-dir"
	StackDirFlag                = "--stacks-dir"
	BasePathFlag                = "--base-path"
	VendorBasePathFlag          = "--vendor-base-path"
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
	InitPassVars                = "--init-pass-vars"
	PlanSkipPlanfile            = "--skip-planfile"

	FromPlanFlag       = "--from-plan"
	PlanFileFlag       = "--planfile"
	DryRunFlag         = "--dry-run"
	SkipInitFlag       = "--skip-init"
	RedirectStdErrFlag = "--redirect-stderr"

	HelpFlag1 = "-h"
	HelpFlag2 = "--help"

	TerraformComponentType = "terraform"
	HelmfileComponentType  = "helmfile"
	PackerComponentType    = "packer"

	ComponentVendorConfigFileName = "component.yaml"
	AtmosVendorConfigFileName     = "vendor"

	ImportSectionName                 = "import"
	OverridesSectionName              = "overrides"
	ProvidersSectionName              = "providers"
	HooksSectionName                  = "hooks"
	VarsSectionName                   = "vars"
	SettingsSectionName               = "settings"
	EnvSectionName                    = "env"
	BackendSectionName                = "backend"
	BackendTypeSectionName            = "backend_type"
	RemoteStateBackendSectionName     = "remote_state_backend"
	RemoteStateBackendTypeSectionName = "remote_state_backend_type"
	MetadataSectionName               = "metadata"
	ComponentSectionName              = "component"
	ComponentsSectionName             = "components"
	CommandSectionName                = "command"
	TerraformSectionName              = "terraform"
	HelmfileSectionName               = "helmfile"
	PackerSectionName                 = "packer"
	PackerTemplateSectionName         = "template"
	WorkspaceSectionName              = "workspace"
	InheritanceSectionName            = "inheritance"
	IntegrationsSectionName           = "integrations"
	GithubSectionName                 = "github"
	TerraformCliVarsSectionName       = "tf_cli_vars"
	CliArgsSectionName                = "cli_args"
	ComponentTypeSectionName          = "component_type"
	OutputsSectionName                = "outputs"
	StaticSectionName                 = "static"
	BackendTypeLocal                  = "local"
	BackendTypeS3                     = "s3"
	BackendTypeAzurerm                = "azurerm"
	BackendTypeGCS                    = "gcs"
	BackendTypeCloud                  = "cloud"
	ComponentPathSectionName          = "component_path"

	LogsLevelFlag = "--logs-level"
	LogsFileFlag  = "--logs-file"

	QueryFlag    = "--query"
	AffectedFlag = "--affected"
	AllFlag      = "--all"

	ProcessTemplatesFlag = "--process-templates"
	ProcessFunctionsFlag = "--process-functions"
	SkipFlag             = "--skip"

	SettingsListMergeStrategyFlag = "--settings-list-merge-strategy"

	// Atmos Pro.
	AtmosProBaseUrlEnvVarName     = "ATMOS_PRO_BASE_URL"
	AtmosProEndpointEnvVarName    = "ATMOS_PRO_ENDPOINT"
	AtmosProTokenEnvVarName       = "ATMOS_PRO_TOKEN"
	AtmosProWorkspaceIDEnvVarName = "ATMOS_PRO_WORKSPACE_ID"
	AtmosProRunIDEnvVarName       = "ATMOS_PRO_RUN_ID"
	AtmosProDefaultBaseUrl        = "https://atmos-pro.com"
	AtmosProDefaultEndpoint       = "api/v1"
	UploadStatusFlag              = "upload-status"

	TerraformDefaultWorkspace = "default"

	ComponentStr = "component"
	StackStr     = "stack"
)
