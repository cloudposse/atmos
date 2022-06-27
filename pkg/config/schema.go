package config

type Terraform struct {
	BasePath                string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	ApplyAutoApprove        bool   `yaml:"apply_auto_approve" json:"apply_auto_approve" mapstructure:"apply_auto_approve"`
	DeployRunInit           bool   `yaml:"deploy_run_init" json:"deploy_run_init" mapstructure:"deploy_run_init"`
	InitRunReconfigure      bool   `yaml:"init_run_reconfigure" json:"init_run_reconfigure" mapstructure:"init_run_reconfigure"`
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

type Command struct {
	Name        string            `yaml:"name" json:"name" mapstructure:"name"`
	Description string            `yaml:"description" json:"description" mapstructure:"description"`
	Env         []CommandEnv      `yaml:"env" json:"env" mapstructure:"env"`
	Arguments   []CommandArgument `yaml:"arguments" json:"arguments" mapstructure:"arguments"`
	Flags       []CommandFlag     `yaml:"flags" json:"flags" mapstructure:"flags"`
	Steps       []string          `yaml:"steps" json:"steps" mapstructure:"steps"`
	Commands    []Command         `yaml:"commands" json:"commands" mapstructure:"commands"`
}

type CommandArgument struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
}

type CommandFlag struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Shorthand   string `yaml:"shorthand" json:"shorthand" mapstructure:"shorthand"`
	Type        string `yaml:"type" json:"type" mapstructure:"type"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
	Usage       string `yaml:"usage" json:"usage" mapstructure:"usage"`
	Required    bool   `yaml:"required" json:"required" mapstructure:"required"`
}

type CommandEnv struct {
	Key          string `yaml:"key" json:"key" mapstructure:"key"`
	Value        string `yaml:"value" json:"value" mapstructure:"value"`
	ValueCommand string `yaml:"valueCommand" json:"valueCommand" mapstructure:"valueCommand"`
}

type Configuration struct {
	BasePath    string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	Components  Components
	Stacks      Stacks
	Workflows   Workflows
	Logs        Logs
	Commands    []Command
	Initialized bool
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
	Namespace     string
	Tenant        string
	Environment   string
	Stage         string
	Region        string
	Component     string
	BaseComponent string
	Attributes    []string
}

type ArgsAndFlagsInfo struct {
	AdditionalArgsAndFlags  []string
	SubCommand              string
	SubCommand2             string
	ComponentFromArg        string
	GlobalOptions           []string
	TerraformDir            string
	HelmfileDir             string
	ConfigDir               string
	StacksDir               string
	WorkflowsDir            string
	BasePath                string
	DeployRunInit           string
	InitRunReconfigure      string
	AutoGenerateBackendFile string
	UseTerraformPlan        bool
	DryRun                  bool
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
	SubCommand2               string
	ComponentSection          map[string]any
	ComponentVarsSection      map[any]any
	ComponentEnvSection       map[any]any
	ComponentEnvList          []string
	ComponentBackendSection   map[any]any
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
	InitRunReconfigure        string
	AutoGenerateBackendFile   string
	UseTerraformPlan          bool
	DryRun                    bool
	ComponentInheritanceChain []string
	NeedHelp                  bool
	ComponentIsAbstract       bool
	ComponentMetadataSection  map[any]any
	TerraformWorkspace        string
}

type WorkflowStep struct {
	Command string `yaml:"command" json:"command" mapstructure:"command"`
	Stack   string `yaml:"stack" json:"stack" mapstructure:"stack"`
	Type    string `yaml:"type" json:"type" mapstructure:"type"`
}

type WorkflowDefinition struct {
	Description string         `yaml:"description" json:"description" mapstructure:"description"`
	Steps       []WorkflowStep `yaml:"steps" json:"steps" mapstructure:"steps"`
	Stack       string         `yaml:"stack" json:"stack" mapstructure:"stack"`
}

type WorkflowConfig map[string]WorkflowDefinition

type WorkflowFile map[string]WorkflowConfig

type AwsEksUpdateKubeconfigContext struct {
	Component   string
	Stack       string
	Profile     string
	ClusterName string
	Kubeconfig  string
	RoleArn     string
	DryRun      bool
	Verbose     bool
	Alias       string
	Tenant      string
	Environment string
	Stage       string
	Region      string
}

// Component vendoring (`component.yaml` file)

type VendorComponentSource struct {
	Type          string   `yaml:"type" json:"type" mapstructure:"type"`
	Uri           string   `yaml:"uri" json:"uri" mapstructure:"uri"`
	Version       string   `yaml:"version" json:"version" mapstructure:"version"`
	IncludedPaths []string `yaml:"included_paths" json:"included_paths" mapstructure:"included_paths"`
	ExcludedPaths []string `yaml:"excluded_paths" json:"excluded_paths" mapstructure:"excluded_paths"`
}

type VendorComponentMixins struct {
	Type     string `yaml:"type" json:"type" mapstructure:"type"`
	Uri      string `yaml:"uri" json:"uri" mapstructure:"uri"`
	Version  string `yaml:"version" json:"version" mapstructure:"version"`
	Filename string `yaml:"filename" json:"filename" mapstructure:"filename"`
}

type VendorComponentSpec struct {
	Source VendorComponentSource
	Mixins []VendorComponentMixins
}

type VendorComponentMetadata struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
}

type VendorComponentConfig struct {
	ApiVersion string `yaml:"apiVersion" json:"apiVersion" mapstructure:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind" mapstructure:"kind"`
	Metadata   VendorComponentMetadata
	Spec       VendorComponentSpec `yaml:"spec" json:"spec" mapstructure:"spec"`
}
