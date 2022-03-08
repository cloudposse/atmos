package config

type Terraform struct {
	BasePath                string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	ApplyAutoApprove        bool   `yaml:"apply_auto_approve" json:"apply_auto_approve" mapstructure:"apply_auto_approve"`
	DeployRunInit           bool   `yaml:"deploy_run_init" json:"deploy_run_init" mapstructure:"deploy_run_init"`
	AutoGenerateBackendFile bool   `yaml:"auto_generate_backend_file" json:"auto_generate_backend_file" mapstructure:"auto_generate_backend_file"`
}

type Helmfile struct {
	BasePath              string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	KubeconfigPath        string `yaml:"kubeconfig_path" json:"kubeconfig_path" mapstructure:"kubeconfig_path"`
	HelmAwsProfilePattern string `yaml:"helm_aws_profile_pattern" json:"helm_aws_profile_pattern" mapstructure:"helm_aws_profile_pattern"`
	ClusterNamePattern    string `yaml:"cluster_name_pattern" json:"cluster_name_pattern" mapstructure:"cluster_name_pattern"`
}

type Components struct {
	Terraform Terraform
	Helmfile  Helmfile
}

type Stacks struct {
	BasePath      string   `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	IncludedPaths []string `yaml:"included_paths" json:"included_paths" mapstructure:"included_paths"`
	ExcludedPaths []string `yaml:"excluded_paths" json:"excluded_paths" mapstructure:"excluded_paths"`
	NamePattern   string   `yaml:"name_pattern" json:"name_pattern" mapstructure:"name_pattern"`
}

type Workflows struct {
	BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
}

type Logs struct {
	Verbose bool `yaml:"verbose" json:"verbose" mapstructure:"verbose"`
	Colors  bool `yaml:"colors" json:"colors" mapstructure:"colors"`
}

type Configuration struct {
	BasePath   string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	Components Components
	Stacks     Stacks
	Workflows  Workflows
	Logs       Logs
}

type ProcessedConfiguration struct {
	StacksBaseAbsolutePath        string   `yaml:"StacksBaseAbsolutePath" json:"StacksBaseAbsolutePath"`
	IncludeStackAbsolutePaths     []string `yaml:"IncludeStackAbsolutePaths" json:"IncludeStackAbsolutePaths"`
	ExcludeStackAbsolutePaths     []string `yaml:"ExcludeStackAbsolutePaths" json:"ExcludeStackAbsolutePaths"`
	TerraformDirAbsolutePath      string   `yaml:"TerraformDirAbsolutePath" json:"TerraformDirAbsolutePath"`
	HelmfileDirAbsolutePath       string   `yaml:"HelmfileDirAbsolutePath" json:"HelmfileDirAbsolutePath"`
	StackConfigFilesRelativePaths []string `yaml:"StackConfigFilesRelativePaths" json:"StackConfigFilesRelativePaths"`
	StackConfigFilesAbsolutePaths []string `yaml:"StackConfigFilesAbsolutePaths" json:"StackConfigFilesAbsolutePaths"`
	StackType                     string   `yaml:"StackType" json:"StackType"`
}

type Context struct {
	Namespace   string
	Tenant      string
	Environment string
	Stage       string
	Region      string
}

type ArgsAndFlagsInfo struct {
	AdditionalArgsAndFlags  []string
	SubCommand              string
	ComponentFromArg        string
	GlobalOptions           []string
	TerraformDir            string
	HelmfileDir             string
	ConfigDir               string
	StacksDir               string
	WorkflowsDir            string
	BasePath                string
	DeployRunInit           string
	AutoGenerateBackendFile string
	UseTerraformPlan        bool
	NeedHelp                bool
}

type ConfigAndStacksInfo struct {
	StackFromArg              string
	Stack                     string
	ComponentType             string
	ComponentFromArg          string
	Component                 string
	ComponentFolderPrefix     string
	BaseComponentPath         string
	BaseComponent             string
	FinalComponent            string
	Command                   string
	SubCommand                string
	ComponentSection          map[string]interface{}
	ComponentVarsSection      map[interface{}]interface{}
	ComponentEnvSection       map[interface{}]interface{}
	ComponentEnvList          []string
	ComponentBackendSection   map[interface{}]interface{}
	ComponentBackendType      string
	AdditionalArgsAndFlags    []string
	GlobalOptions             []string
	BasePath                  string
	TerraformDir              string
	HelmfileDir               string
	ConfigDir                 string
	StacksDir                 string
	WorkflowsDir              string
	Context                   Context
	ContextPrefix             string
	DeployRunInit             string
	AutoGenerateBackendFile   string
	UseTerraformPlan          bool
	ComponentInheritanceChain []string
	NeedHelp                  bool
	ComponentIsAbstract       bool
	ComponentMetadataSection  map[interface{}]interface{}
	TerraformWorkspace        string
}

type WorkflowStep struct {
	Command string `yaml:"command" json:"command" mapstructure:"command"`
	Stack   string `yaml:"stack" json:"stack" mapstructure:"stack"`
	Type    string `yaml:"type" json:"type" mapstructure:"type"`
}

type Workflow struct {
	Description string         `yaml:"description" json:"description" mapstructure:"description"`
	Steps       []WorkflowStep `yaml:"steps" json:"steps" mapstructure:"steps"`
}
