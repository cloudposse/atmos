package config

const (
	DefaultStackConfigFileExtension       = ".yaml"
	DefaultVendoringManifestFileExtension = ".yaml"
	CliConfigFileName                     = "atmos.yaml"
	SystemDirConfigFilePath               = "/usr/local/etc/atmos"
	WindowsAppDataEnvVar                  = "LOCALAPPDATA"

	// GlobalOptionsFlag is a custom flag to specify helmfile `GLOBAL OPTIONS`
	// https://github.com/roboll/helmfile#cli-reference
	GlobalOptionsFlag = "--global-options"

	TerraformDirFlag     = "--terraform-dir"
	HelmfileDirFlag      = "--helmfile-dir"
	CliConfigDirFlag     = "--config-dir"
	StackDirFlag         = "--stacks-dir"
	BasePathFlag         = "--base-path"
	WorkflowDirFlag      = "--workflows-dir"
	KubeConfigConfigFlag = "--kubeconfig-path"
	JsonSchemaDirFlag    = "--schemas-jsonschema-dir"
	OpaDirFlag           = "--schemas-opa-dir"
	CueDirFlag           = "--schemas-cue-dir"

	DeployRunInitFlag           = "--deploy-run-init"
	AutoGenerateBackendFileFlag = "--auto-generate-backend-file"
	InitRunReconfigure          = "--init-run-reconfigure"

	FromPlanFlag       = "--from-plan"
	PlanFileFlag       = "--planfile"
	DryRunFlag         = "--dry-run"
	SkipInitFlag       = "--skip-init"
	RedirectStdErrFlag = "--redirect-stderr"

	HelpFlag1 = "-h"
	HelpFlag2 = "--help"

	ComponentVendorConfigFileName = "component.yaml"
	AtmosVendorConfigFileName     = "vendor.yaml"

	ImportSectionName = "import"
)
