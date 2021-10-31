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

type Logs struct {
	Verbose bool `yaml:"verbose" json:"verbose" mapstructure:"verbose"`
	Colors  bool `yaml:"colors" json:"colors" mapstructure:"colors"`
}

type Configuration struct {
	Components Components
	Stacks     Stacks
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
	DeployRunInit           string
	AutoGenerateBackendFile string
}

type ConfigAndStacksInfo struct {
	Stack                   string
	ComponentFromArg        string
	ComponentFolderPrefix   string
	ComponentNamePrefix     string
	Component               string
	BaseComponentPath       string
	BaseComponent           string
	Command                 string
	SubCommand              string
	ComponentVarsSection    map[interface{}]interface{}
	ComponentBackendSection map[interface{}]interface{}
	AdditionalArgsAndFlags  []string
	GlobalOptions           []string
	TerraformDir            string
	HelmfileDir             string
	ConfigDir               string
	StacksDir               string
	Context                 Context
	ContextPrefix           string
	DeployRunInit           string
	AutoGenerateBackendFile string
}
