package schema

type AtmosSectionMapType = map[string]any

// CliConfiguration structure represents schema for `atmos.yaml` CLI config
type CliConfiguration struct {
	BasePath                      string         `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	Components                    Components     `yaml:"components" json:"components" mapstructure:"components"`
	Stacks                        Stacks         `yaml:"stacks" json:"stacks" mapstructure:"stacks"`
	Workflows                     Workflows      `yaml:"workflows,omitempty" json:"workflows,omitempty" mapstructure:"workflows"`
	Logs                          Logs           `yaml:"logs,omitempty" json:"logs,omitempty" mapstructure:"logs"`
	Commands                      []Command      `yaml:"commands,omitempty" json:"commands,omitempty" mapstructure:"commands"`
	CommandAliases                CommandAliases `yaml:"aliases,omitempty" json:"aliases,omitempty" mapstructure:"aliases"`
	Integrations                  Integrations   `yaml:"integrations,omitempty" json:"integrations,omitempty" mapstructure:"integrations"`
	Schemas                       Schemas        `yaml:"schemas,omitempty" json:"schemas,omitempty" mapstructure:"schemas"`
	Templates                     Templates      `yaml:"templates,omitempty" json:"templates,omitempty" mapstructure:"templates"`
	Settings                      CliSettings    `yaml:"settings,omitempty" json:"settings,omitempty" mapstructure:"settings"`
	Vendor                        Vendor         `yaml:"vendor,omitempty" json:"vendor,omitempty" mapstructure:"vendor"`
	Initialized                   bool           `yaml:"initialized" json:"initialized" mapstructure:"initialized"`
	StacksBaseAbsolutePath        string         `yaml:"stacksBaseAbsolutePath,omitempty" json:"stacksBaseAbsolutePath,omitempty" mapstructure:"stacksBaseAbsolutePath"`
	IncludeStackAbsolutePaths     []string       `yaml:"includeStackAbsolutePaths,omitempty" json:"includeStackAbsolutePaths,omitempty" mapstructure:"includeStackAbsolutePaths"`
	ExcludeStackAbsolutePaths     []string       `yaml:"excludeStackAbsolutePaths,omitempty" json:"excludeStackAbsolutePaths,omitempty" mapstructure:"excludeStackAbsolutePaths"`
	TerraformDirAbsolutePath      string         `yaml:"terraformDirAbsolutePath,omitempty" json:"terraformDirAbsolutePath,omitempty" mapstructure:"terraformDirAbsolutePath"`
	HelmfileDirAbsolutePath       string         `yaml:"helmfileDirAbsolutePath,omitempty" json:"helmfileDirAbsolutePath,omitempty" mapstructure:"helmfileDirAbsolutePath"`
	StackConfigFilesRelativePaths []string       `yaml:"stackConfigFilesRelativePaths,omitempty" json:"stackConfigFilesRelativePaths,omitempty" mapstructure:"stackConfigFilesRelativePaths"`
	StackConfigFilesAbsolutePaths []string       `yaml:"stackConfigFilesAbsolutePaths,omitempty" json:"stackConfigFilesAbsolutePaths,omitempty" mapstructure:"stackConfigFilesAbsolutePaths"`
	StackType                     string         `yaml:"stackType,omitempty" json:"StackType,omitempty" mapstructure:"stackType"`
	Default                       bool           `yaml:"default" json:"default" mapstructure:"default"`
	Version                       Version        `yaml:"version,omitempty" json:"version,omitempty" mapstructure:"version"`
}

type CliSettings struct {
	ListMergeStrategy string `yaml:"list_merge_strategy" json:"list_merge_strategy" mapstructure:"list_merge_strategy"`
	Docs              Docs   `yaml:"docs,omitempty" json:"docs,omitempty" mapstructure:"docs"`
}

type Docs struct {
	MaxWidth   int  `yaml:"max-width" json:"max_width" mapstructure:"max-width"`
	Pagination bool `yaml:"pagination" json:"pagination" mapstructure:"pagination"`
}

type Templates struct {
	Settings TemplatesSettings `yaml:"settings" json:"settings" mapstructure:"settings"`
}

type TemplatesSettings struct {
	Enabled     bool                      `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	Sprig       TemplatesSettingsSprig    `yaml:"sprig" json:"sprig" mapstructure:"sprig"`
	Gomplate    TemplatesSettingsGomplate `yaml:"gomplate" json:"gomplate" mapstructure:"gomplate"`
	Delimiters  []string                  `yaml:"delimiters,omitempty" json:"delimiters,omitempty" mapstructure:"delimiters"`
	Evaluations int                       `yaml:"evaluations,omitempty" json:"evaluations,omitempty" mapstructure:"evaluations"`
	Env         map[string]string         `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`
}

type TemplatesSettingsSprig struct {
	Enabled bool `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
}

type TemplatesSettingsGomplateDatasource struct {
	Url     string              `yaml:"url" json:"url" mapstructure:"url"`
	Headers map[string][]string `yaml:"headers" json:"headers" mapstructure:"headers"`
}

type TemplatesSettingsGomplate struct {
	Enabled     bool                                           `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	Timeout     int                                            `yaml:"timeout" json:"timeout" mapstructure:"timeout"`
	Datasources map[string]TemplatesSettingsGomplateDatasource `yaml:"datasources" json:"datasources" mapstructure:"datasources"`
}

type Terraform struct {
	BasePath                string      `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	ApplyAutoApprove        bool        `yaml:"apply_auto_approve" json:"apply_auto_approve" mapstructure:"apply_auto_approve"`
	AppendUserAgent         string      `yaml:"append_user_agent" json:"append_user_agent" mapstructure:"append_user_agent"`
	DeployRunInit           bool        `yaml:"deploy_run_init" json:"deploy_run_init" mapstructure:"deploy_run_init"`
	InitRunReconfigure      bool        `yaml:"init_run_reconfigure" json:"init_run_reconfigure" mapstructure:"init_run_reconfigure"`
	AutoGenerateBackendFile bool        `yaml:"auto_generate_backend_file" json:"auto_generate_backend_file" mapstructure:"auto_generate_backend_file"`
	Command                 string      `yaml:"command" json:"command" mapstructure:"command"`
	Shell                   ShellConfig `yaml:"shell" json:"shell" mapstructure:"shell"`
}

type ShellConfig struct {
	Prompt string `yaml:"prompt" json:"prompt" mapstructure:"prompt"`
}

type Helmfile struct {
	BasePath              string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	UseEKS                bool   `yaml:"use_eks" json:"use_eks" mapstructure:"use_eks"`
	KubeconfigPath        string `yaml:"kubeconfig_path" json:"kubeconfig_path" mapstructure:"kubeconfig_path"`
	HelmAwsProfilePattern string `yaml:"helm_aws_profile_pattern" json:"helm_aws_profile_pattern" mapstructure:"helm_aws_profile_pattern"`
	ClusterNamePattern    string `yaml:"cluster_name_pattern" json:"cluster_name_pattern" mapstructure:"cluster_name_pattern"`
	Command               string `yaml:"command" json:"command" mapstructure:"command"`
}

type Components struct {
	Terraform Terraform  `yaml:"terraform" json:"terraform" mapstructure:"terraform"`
	Helmfile  Helmfile   `yaml:"helmfile" json:"helmfile" mapstructure:"helmfile"`
	List      ListConfig `yaml:"list" json:"list" mapstructure:"list"`
}

type ListConfig struct {
	Columns []ListColumnConfig `yaml:"columns" json:"columns" mapstructure:"columns"`
}

type ListColumnConfig struct {
	Name  string `yaml:"name" json:"name" mapstructure:"name"`
	Value string `yaml:"value" json:"value" mapstructure:"value"`
}

type Stacks struct {
	BasePath      string   `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	IncludedPaths []string `yaml:"included_paths" json:"included_paths" mapstructure:"included_paths"`
	ExcludedPaths []string `yaml:"excluded_paths" json:"excluded_paths" mapstructure:"excluded_paths"`
	NamePattern   string   `yaml:"name_pattern" json:"name_pattern" mapstructure:"name_pattern"`
	NameTemplate  string   `yaml:"name_template" json:"name_template" mapstructure:"name_template"`
}

type Workflows struct {
	BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
}

type Logs struct {
	File  string `yaml:"file" json:"file" mapstructure:"file"`
	Level string `yaml:"level" json:"level" mapstructure:"level"`
}

type Context struct {
	Namespace          string `yaml:"namespace" json:"namespace" mapstructure:"namespace"`
	Tenant             string `yaml:"tenant" json:"tenant" mapstructure:"tenant"`
	Environment        string `yaml:"environment" json:"environment" mapstructure:"environment"`
	Stage              string `yaml:"stage" json:"stage" mapstructure:"stage"`
	Region             string `yaml:"region" json:"region" mapstructure:"region"`
	Component          string `yaml:"component" json:"component" mapstructure:"component"`
	BaseComponent      string `yaml:"base_component" json:"base_component" mapstructure:"base_component"`
	ComponentPath      string `yaml:"component_path" json:"component_path" mapstructure:"component_path"`
	Workspace          string `yaml:"workspace" json:"workspace" mapstructure:"workspace"`
	Attributes         []any  `yaml:"attributes" json:"attributes" mapstructure:"attributes"`
	File               string `yaml:"file" json:"file" mapstructure:"file"`
	Folder             string `yaml:"folder" json:"folder" mapstructure:"folder"`
	TerraformWorkspace string `yaml:"terraform_workspace" json:"terraform_workspace" mapstructure:"terraform_workspace"`
}

type VersionCheck struct {
	Enabled   bool   `yaml:"enabled,omitempty" mapstructure:"enabled"`
	Timeout   int    `yaml:"timeout,omitempty" mapstructure:"timeout"`
	Frequency string `yaml:"frequency,omitempty" mapstructure:"frequency"`
}

type Version struct {
	Check VersionCheck `yaml:"check,omitempty" mapstructure:"check"`
}

type ArgsAndFlagsInfo struct {
	AdditionalArgsAndFlags    []string
	SubCommand                string
	SubCommand2               string
	ComponentFromArg          string
	GlobalOptions             []string
	TerraformCommand          string
	TerraformDir              string
	HelmfileCommand           string
	HelmfileDir               string
	ConfigDir                 string
	StacksDir                 string
	WorkflowsDir              string
	BasePath                  string
	VendorBasePath            string
	DeployRunInit             string
	InitRunReconfigure        string
	AutoGenerateBackendFile   string
	AppendUserAgent           string
	UseTerraformPlan          bool
	PlanFile                  string
	DryRun                    bool
	SkipInit                  bool
	NeedHelp                  bool
	JsonSchemaDir             string
	OpaDir                    string
	CueDir                    string
	AtmosManifestJsonSchema   string
	RedirectStdErr            string
	LogsLevel                 string
	LogsFile                  string
	SettingsListMergeStrategy string
}

type ConfigAndStacksInfo struct {
	StackFromArg                  string
	Stack                         string
	StackFile                     string
	ComponentType                 string
	ComponentFromArg              string
	Component                     string
	ComponentFolderPrefix         string
	ComponentFolderPrefixReplaced string
	BaseComponentPath             string
	BaseComponent                 string
	FinalComponent                string
	Command                       string
	SubCommand                    string
	SubCommand2                   string
	ComponentSection              AtmosSectionMapType
	ComponentVarsSection          AtmosSectionMapType
	ComponentSettingsSection      AtmosSectionMapType
	ComponentOverridesSection     AtmosSectionMapType
	ComponentProvidersSection     AtmosSectionMapType
	ComponentEnvSection           AtmosSectionMapType
	ComponentEnvList              []string
	ComponentBackendSection       AtmosSectionMapType
	ComponentBackendType          string
	AdditionalArgsAndFlags        []string
	GlobalOptions                 []string
	BasePath                      string
	VendorBasePathFlag            string
	TerraformCommand              string
	TerraformDir                  string
	HelmfileCommand               string
	HelmfileDir                   string
	ConfigDir                     string
	StacksDir                     string
	WorkflowsDir                  string
	Context                       Context
	ContextPrefix                 string
	DeployRunInit                 string
	InitRunReconfigure            string
	AutoGenerateBackendFile       string
	UseTerraformPlan              bool
	PlanFile                      string
	DryRun                        bool
	SkipInit                      bool
	ComponentInheritanceChain     []string
	ComponentImportsSection       []string
	NeedHelp                      bool
	ComponentIsAbstract           bool
	ComponentIsEnabled            bool
	ComponentMetadataSection      AtmosSectionMapType
	TerraformWorkspace            string
	JsonSchemaDir                 string
	OpaDir                        string
	CueDir                        string
	AtmosManifestJsonSchema       string
	AtmosCliConfigPath            string
	AtmosBasePath                 string
	RedirectStdErr                string
	LogsLevel                     string
	LogsFile                      string
	SettingsListMergeStrategy     string
}

// Workflows

type WorkflowStep struct {
	Name    string `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Command string `yaml:"command" json:"command" mapstructure:"command"`
	Stack   string `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
	Type    string `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"`
}

type WorkflowDefinition struct {
	Description string         `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	Steps       []WorkflowStep `yaml:"steps" json:"steps" mapstructure:"steps"`
	Stack       string         `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
}

type WorkflowConfig map[string]WorkflowDefinition

type WorkflowManifest struct {
	Name        string         `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	Workflows   WorkflowConfig `yaml:"workflows" json:"workflows" mapstructure:"workflows"`
}

type DescribeWorkflowsItem struct {
	File     string `yaml:"file" json:"file" mapstructure:"file"`
	Workflow string `yaml:"workflow" json:"workflow" mapstructure:"workflow"`
}

// EKS update-kubeconfig

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
	Namespace   string
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
	Source VendorComponentSource   `yaml:"source" json:"source" mapstructure:"source"`
	Mixins []VendorComponentMixins `yaml:"mixins" json:"mixins" mapstructure:"mixins"`
}

type VendorComponentMetadata struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
}

type VendorComponentConfig struct {
	ApiVersion string                  `yaml:"apiVersion" json:"apiVersion" mapstructure:"apiVersion"`
	Kind       string                  `yaml:"kind" json:"kind" mapstructure:"kind"`
	Metadata   VendorComponentMetadata `yaml:"metadata" json:"metadata" mapstructure:"metadata"`
	Spec       VendorComponentSpec     `yaml:"spec" json:"spec" mapstructure:"spec"`
}

// Custom CLI commands

type Command struct {
	Name            string                 `yaml:"name" json:"name" mapstructure:"name"`
	Description     string                 `yaml:"description" json:"description" mapstructure:"description"`
	Env             []CommandEnv           `yaml:"env" json:"env" mapstructure:"env"`
	Arguments       []CommandArgument      `yaml:"arguments" json:"arguments" mapstructure:"arguments"`
	Flags           []CommandFlag          `yaml:"flags" json:"flags" mapstructure:"flags"`
	ComponentConfig CommandComponentConfig `yaml:"component_config" json:"component_config" mapstructure:"component_config"`
	Steps           []string               `yaml:"steps" json:"steps" mapstructure:"steps"`
	Commands        []Command              `yaml:"commands" json:"commands" mapstructure:"commands"`
	Verbose         bool                   `yaml:"verbose" json:"verbose" mapstructure:"verbose"`
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

type CommandComponentConfig struct {
	Component string `yaml:"component" json:"component" mapstructure:"component"`
	Stack     string `yaml:"stack" json:"stack" mapstructure:"stack"`
}

// CLI command aliases

type CommandAliases map[string]string

// Integrations

type Integrations struct {
	Atlantis Atlantis            `yaml:"atlantis,omitempty" json:"atlantis,omitempty" mapstructure:"atlantis"`
	GitHub   AtmosSectionMapType `yaml:"github,omitempty" json:"github,omitempty" mapstructure:"github"`
	Pro      AtmosSectionMapType `yaml:"pro,omitempty" json:"pro,omitempty" mapstructure:"pro"`
}

// Atlantis integration

type Atlantis struct {
	Path              string                           `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`
	ConfigTemplates   map[string]AtlantisRepoConfig    `yaml:"config_templates,omitempty" json:"config_templates,omitempty" mapstructure:"config_templates"`
	ProjectTemplates  map[string]AtlantisProjectConfig `yaml:"project_templates,omitempty" json:"project_templates,omitempty" mapstructure:"project_templates"`
	WorkflowTemplates AtmosSectionMapType              `yaml:"workflow_templates,omitempty" json:"workflow_templates,omitempty" mapstructure:"workflow_templates"`
}

type AtlantisRepoConfig struct {
	Version                   int      `yaml:"version" json:"version" mapstructure:"version"`
	Automerge                 bool     `yaml:"automerge" json:"automerge" mapstructure:"automerge"`
	DeleteSourceBranchOnMerge bool     `yaml:"delete_source_branch_on_merge" json:"delete_source_branch_on_merge" mapstructure:"delete_source_branch_on_merge"`
	ParallelPlan              bool     `yaml:"parallel_plan" json:"parallel_plan" mapstructure:"parallel_plan"`
	ParallelApply             bool     `yaml:"parallel_apply" json:"parallel_apply" mapstructure:"parallel_apply"`
	AllowedRegexpPrefixes     []string `yaml:"allowed_regexp_prefixes" json:"allowed_regexp_prefixes" mapstructure:"allowed_regexp_prefixes"`
}

type AtlantisProjectConfig struct {
	Name                      string                        `yaml:"name" json:"name" mapstructure:"name"`
	Workspace                 string                        `yaml:"workspace" json:"workspace" mapstructure:"workspace"`
	Workflow                  string                        `yaml:"workflow,omitempty" json:"workflow,omitempty" mapstructure:"workflow"`
	Dir                       string                        `yaml:"dir" json:"dir" mapstructure:"dir"`
	TerraformVersion          string                        `yaml:"terraform_version" json:"terraform_version" mapstructure:"terraform_version"`
	DeleteSourceBranchOnMerge bool                          `yaml:"delete_source_branch_on_merge" json:"delete_source_branch_on_merge" mapstructure:"delete_source_branch_on_merge"`
	Autoplan                  AtlantisProjectAutoplanConfig `yaml:"autoplan" json:"autoplan" mapstructure:"autoplan"`
	ApplyRequirements         []string                      `yaml:"apply_requirements" json:"apply_requirements" mapstructure:"apply_requirements"`
}

type AtlantisProjectAutoplanConfig struct {
	Enabled      bool     `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	WhenModified []string `yaml:"when_modified" json:"when_modified" mapstructure:"when_modified"`
}

type AtlantisConfigOutput struct {
	Version                   int                     `yaml:"version" json:"version" mapstructure:"version"`
	Automerge                 bool                    `yaml:"automerge" json:"automerge" mapstructure:"automerge"`
	DeleteSourceBranchOnMerge bool                    `yaml:"delete_source_branch_on_merge" json:"delete_source_branch_on_merge" mapstructure:"delete_source_branch_on_merge"`
	ParallelPlan              bool                    `yaml:"parallel_plan" json:"parallel_plan" mapstructure:"parallel_plan"`
	ParallelApply             bool                    `yaml:"parallel_apply" json:"parallel_apply" mapstructure:"parallel_apply"`
	AllowedRegexpPrefixes     []string                `yaml:"allowed_regexp_prefixes" json:"allowed_regexp_prefixes" mapstructure:"allowed_regexp_prefixes"`
	Projects                  []AtlantisProjectConfig `yaml:"projects" json:"projects" mapstructure:"projects"`
	Workflows                 AtmosSectionMapType     `yaml:"workflows,omitempty" json:"workflows,omitempty" mapstructure:"workflows"`
}

// Validation schemas

type JsonSchema struct {
	BasePath string `yaml:"base_path,omitempty" json:"base_path,omitempty" mapstructure:"base_path"`
}

type Cue struct {
	BasePath string `yaml:"base_path,omitempty" json:"base_path,omitempty" mapstructure:"base_path"`
}

type Opa struct {
	BasePath string `yaml:"base_path,omitempty" json:"base_path,omitempty" mapstructure:"base_path"`
}

type AtmosSchema struct {
	Manifest string `yaml:"manifest,omitempty" json:"manifest,omitempty" mapstructure:"manifest"`
}

type Schemas struct {
	JsonSchema JsonSchema  `yaml:"jsonschema,omitempty" json:"jsonschema,omitempty" mapstructure:"jsonschema"`
	Cue        Cue         `yaml:"cue,omitempty" json:"cue,omitempty" mapstructure:"cue"`
	Opa        Opa         `yaml:"opa,omitempty" json:"opa,omitempty" mapstructure:"opa"`
	Atmos      AtmosSchema `yaml:"atmos,omitempty" json:"atmos,omitempty" mapstructure:"atmos"`
}

type ValidationItem struct {
	SchemaType  string   `yaml:"schema_type" json:"schema_type" mapstructure:"schema_type"`
	SchemaPath  string   `yaml:"schema_path" json:"schema_path" mapstructure:"schema_path"`
	ModulePaths []string `yaml:"module_paths" json:"module_paths" mapstructure:"module_paths"`
	Description string   `yaml:"description" json:"description" mapstructure:"description"`
	Disabled    bool     `yaml:"disabled" json:"disabled" mapstructure:"disabled"`
	Timeout     int      `yaml:"timeout" json:"timeout" mapstructure:"timeout"`
}

type Validation map[string]ValidationItem

// Affected Atmos components and stacks given two Git commits

type Affected struct {
	Component            string              `yaml:"component" json:"component" mapstructure:"component"`
	ComponentType        string              `yaml:"component_type" json:"component_type" mapstructure:"component_type"`
	ComponentPath        string              `yaml:"component_path" json:"component_path" mapstructure:"component_path"`
	Namespace            string              `yaml:"namespace,omitempty" json:"namespace,omitempty" mapstructure:"namespace"`
	Tenant               string              `yaml:"tenant,omitempty" json:"tenant,omitempty" mapstructure:"tenant"`
	Environment          string              `yaml:"environment,omitempty" json:"environment,omitempty" mapstructure:"environment"`
	Stage                string              `yaml:"stage,omitempty" json:"stage,omitempty" mapstructure:"stage"`
	Stack                string              `yaml:"stack" json:"stack" mapstructure:"stack"`
	StackSlug            string              `yaml:"stack_slug" json:"stack_slug" mapstructure:"stack_slug"`
	SpaceliftStack       string              `yaml:"spacelift_stack,omitempty" json:"spacelift_stack,omitempty" mapstructure:"spacelift_stack"`
	AtlantisProject      string              `yaml:"atlantis_project,omitempty" json:"atlantis_project,omitempty" mapstructure:"atlantis_project"`
	Affected             string              `yaml:"affected" json:"affected" mapstructure:"affected"`
	File                 string              `yaml:"file,omitempty" json:"file,omitempty" mapstructure:"file"`
	Folder               string              `yaml:"folder,omitempty" json:"folder,omitempty" mapstructure:"folder"`
	Dependents           []Dependent         `yaml:"dependents" json:"dependents" mapstructure:"dependents"`
	IncludedInDependents bool                `yaml:"included_in_dependents" json:"included_in_dependents" mapstructure:"included_in_dependents"`
	Settings             AtmosSectionMapType `yaml:"settings" json:"settings" mapstructure:"settings"`
}

type BaseComponentConfig struct {
	BaseComponentVars                      AtmosSectionMapType
	BaseComponentSettings                  AtmosSectionMapType
	BaseComponentEnv                       AtmosSectionMapType
	BaseComponentProviders                 AtmosSectionMapType
	FinalBaseComponentName                 string
	BaseComponentCommand                   string
	BaseComponentBackendType               string
	BaseComponentBackendSection            AtmosSectionMapType
	BaseComponentRemoteStateBackendType    string
	BaseComponentRemoteStateBackendSection AtmosSectionMapType
	ComponentInheritanceChain              []string
}

// Stack imports (`import` section)

type StackImport struct {
	Path                        string              `yaml:"path" json:"path" mapstructure:"path"`
	Context                     AtmosSectionMapType `yaml:"context" json:"context" mapstructure:"context"`
	SkipTemplatesProcessing     bool                `yaml:"skip_templates_processing,omitempty" json:"skip_templates_processing,omitempty" mapstructure:"skip_templates_processing"`
	IgnoreMissingTemplateValues bool                `yaml:"ignore_missing_template_values,omitempty" json:"ignore_missing_template_values,omitempty" mapstructure:"ignore_missing_template_values"`
	SkipIfMissing               bool                `yaml:"skip_if_missing,omitempty" json:"skip_if_missing,omitempty" mapstructure:"skip_if_missing"`
}

// Dependencies

type DependsOn map[any]Context

type Dependent struct {
	Component       string              `yaml:"component" json:"component" mapstructure:"component"`
	ComponentType   string              `yaml:"component_type" json:"component_type" mapstructure:"component_type"`
	ComponentPath   string              `yaml:"component_path" json:"component_path" mapstructure:"component_path"`
	Namespace       string              `yaml:"namespace,omitempty" json:"namespace,omitempty" mapstructure:"namespace"`
	Tenant          string              `yaml:"tenant,omitempty" json:"tenant,omitempty" mapstructure:"tenant"`
	Environment     string              `yaml:"environment,omitempty" json:"environment,omitempty" mapstructure:"environment"`
	Stage           string              `yaml:"stage,omitempty" json:"stage,omitempty" mapstructure:"stage"`
	Stack           string              `yaml:"stack" json:"stack" mapstructure:"stack"`
	StackSlug       string              `yaml:"stack_slug" json:"stack_slug" mapstructure:"stack_slug"`
	SpaceliftStack  string              `yaml:"spacelift_stack,omitempty" json:"spacelift_stack,omitempty" mapstructure:"spacelift_stack"`
	AtlantisProject string              `yaml:"atlantis_project,omitempty" json:"atlantis_project,omitempty" mapstructure:"atlantis_project"`
	Dependents      []Dependent         `yaml:"dependents" json:"dependents" mapstructure:"dependents"`
	Settings        AtmosSectionMapType `yaml:"settings" json:"settings" mapstructure:"settings"`
}

// Settings

type SettingsSpacelift AtmosSectionMapType

type Settings struct {
	DependsOn DependsOn         `yaml:"depends_on,omitempty" json:"depends_on,omitempty" mapstructure:"depends_on"`
	Spacelift SettingsSpacelift `yaml:"spacelift,omitempty" json:"spacelift,omitempty" mapstructure:"spacelift"`
	Templates Templates         `yaml:"templates,omitempty" json:"templates,omitempty" mapstructure:"templates"`
}

// ConfigSourcesStackDependency defines schema for sources of config sections
type ConfigSourcesStackDependency struct {
	StackFile        string `yaml:"stack_file" json:"stack_file" mapstructure:"stack_file"`
	StackFileSection string `yaml:"stack_file_section" json:"stack_file_section" mapstructure:"stack_file_section"`
	DependencyType   string `yaml:"dependency_type" json:"dependency_type" mapstructure:"dependency_type"`
	VariableValue    any    `yaml:"variable_value" json:"variable_value" mapstructure:"variable_value"`
}

type ConfigSourcesStackDependencies []ConfigSourcesStackDependency

type ConfigSourcesItem struct {
	FinalValue        any                            `yaml:"final_value" json:"final_value" mapstructure:"final_value"`
	Name              string                         `yaml:"name" json:"name" mapstructure:"name"`
	StackDependencies ConfigSourcesStackDependencies `yaml:"stack_dependencies" json:"stack_dependencies" mapstructure:"stack_dependencies"`
}

type ConfigSources map[string]map[string]ConfigSourcesItem

// Atmos vendoring (`vendor.yaml` file)

type AtmosVendorSource struct {
	Component     string   `yaml:"component" json:"component" mapstructure:"component"`
	Source        string   `yaml:"source" json:"source" mapstructure:"source"`
	Version       string   `yaml:"version" json:"version" mapstructure:"version"`
	File          string   `yaml:"file" json:"file" mapstructure:"file"`
	Targets       []string `yaml:"targets" json:"targets" mapstructure:"targets"`
	IncludedPaths []string `yaml:"included_paths,omitempty" json:"included_paths,omitempty" mapstructure:"included_paths"`
	ExcludedPaths []string `yaml:"excluded_paths,omitempty" json:"excluded_paths,omitempty" mapstructure:"excluded_paths"`
	Tags          []string `yaml:"tags" json:"tags" mapstructure:"tags"`
}

type AtmosVendorSpec struct {
	Imports []string            `yaml:"imports,omitempty" json:"imports,omitempty" mapstructure:"imports"`
	Sources []AtmosVendorSource `yaml:"sources" json:"sources" mapstructure:"sources"`
}

type AtmosVendorMetadata struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
}

type AtmosVendorConfig struct {
	ApiVersion string `yaml:"apiVersion" json:"apiVersion" mapstructure:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind" mapstructure:"kind"`
	Metadata   AtmosVendorMetadata
	Spec       AtmosVendorSpec `yaml:"spec" json:"spec" mapstructure:"spec"`
}

type Vendor struct {
	// Path to vendor configuration file or directory containing vendor files
	// If a directory is specified, all .yaml files in the directory will be processed in lexicographical order
	BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
}
