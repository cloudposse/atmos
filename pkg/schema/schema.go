package schema

import (
	"encoding/json"

	"github.com/cloudposse/atmos/pkg/store"
	"gopkg.in/yaml.v3"
)

type AtmosSectionMapType = map[string]any

// DescribeSettings contains settings for the describe command output.
type DescribeSettings struct {
	IncludeEmpty *bool `yaml:"include_empty,omitempty" json:"include_empty,omitempty" mapstructure:"include_empty"`
}

// Describe contains configuration for the describe command.
type Describe struct {
	Settings DescribeSettings `yaml:"settings,omitempty" json:"settings,omitempty" mapstructure:"settings"`
}

// AtmosConfiguration structure represents schema for `atmos.yaml` CLI config.
type AtmosConfiguration struct {
	BasePath                      string                 `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	Components                    Components             `yaml:"components" json:"components" mapstructure:"components"`
	Stacks                        Stacks                 `yaml:"stacks" json:"stacks" mapstructure:"stacks"`
	Workflows                     Workflows              `yaml:"workflows,omitempty" json:"workflows,omitempty" mapstructure:"workflows"`
	Logs                          Logs                   `yaml:"logs,omitempty" json:"logs,omitempty" mapstructure:"logs"`
	Commands                      []Command              `yaml:"commands,omitempty" json:"commands,omitempty" mapstructure:"commands"`
	CommandAliases                CommandAliases         `yaml:"aliases,omitempty" json:"aliases,omitempty" mapstructure:"aliases"`
	Integrations                  Integrations           `yaml:"integrations,omitempty" json:"integrations,omitempty" mapstructure:"integrations"`
	Schemas                       map[string]interface{} `yaml:"schemas,omitempty" json:"schemas,omitempty" mapstructure:"schemas"`
	Templates                     Templates              `yaml:"templates,omitempty" json:"templates,omitempty" mapstructure:"templates"`
	Settings                      AtmosSettings          `yaml:"settings,omitempty" json:"settings,omitempty" mapstructure:"settings"`
	Describe                      Describe               `yaml:"describe,omitempty" json:"describe,omitempty" mapstructure:"describe"`
	StoresConfig                  store.StoresConfig     `yaml:"stores,omitempty" json:"stores,omitempty" mapstructure:"stores"`
	Vendor                        Vendor                 `yaml:"vendor,omitempty" json:"vendor,omitempty" mapstructure:"vendor"`
	Initialized                   bool                   `yaml:"initialized" json:"initialized" mapstructure:"initialized"`
	StacksBaseAbsolutePath        string                 `yaml:"stacksBaseAbsolutePath,omitempty" json:"stacksBaseAbsolutePath,omitempty" mapstructure:"stacksBaseAbsolutePath"`
	IncludeStackAbsolutePaths     []string               `yaml:"includeStackAbsolutePaths,omitempty" json:"includeStackAbsolutePaths,omitempty" mapstructure:"includeStackAbsolutePaths"`
	ExcludeStackAbsolutePaths     []string               `yaml:"excludeStackAbsolutePaths,omitempty" json:"excludeStackAbsolutePaths,omitempty" mapstructure:"excludeStackAbsolutePaths"`
	TerraformDirAbsolutePath      string                 `yaml:"terraformDirAbsolutePath,omitempty" json:"terraformDirAbsolutePath,omitempty" mapstructure:"terraformDirAbsolutePath"`
	HelmfileDirAbsolutePath       string                 `yaml:"helmfileDirAbsolutePath,omitempty" json:"helmfileDirAbsolutePath,omitempty" mapstructure:"helmfileDirAbsolutePath"`
	StackConfigFilesRelativePaths []string               `yaml:"stackConfigFilesRelativePaths,omitempty" json:"stackConfigFilesRelativePaths,omitempty" mapstructure:"stackConfigFilesRelativePaths"`
	StackConfigFilesAbsolutePaths []string               `yaml:"stackConfigFilesAbsolutePaths,omitempty" json:"stackConfigFilesAbsolutePaths,omitempty" mapstructure:"stackConfigFilesAbsolutePaths"`
	StackType                     string                 `yaml:"stackType,omitempty" json:"StackType,omitempty" mapstructure:"stackType"`
	Default                       bool                   `yaml:"default" json:"default" mapstructure:"default"`
	Version                       Version                `yaml:"version,omitempty" json:"version,omitempty" mapstructure:"version"`
	Validate                      Validate               `yaml:"validate,omitempty" json:"validate,omitempty" mapstructure:"validate"`
	// Stores is never read from yaml, it is populated in processStoreConfig and it's used to pass to the populated store
	// registry through to the yaml parsing functions when !store is run and to pass the registry to the hooks
	// functions to be able to call stores from within hooks.
	Stores        store.StoreRegistry `yaml:"stores_registry,omitempty" json:"stores_registry,omitempty" mapstructure:"stores_registry"`
	CliConfigPath string              `yaml:"cli_config_path" json:"cli_config_path,omitempty" mapstructure:"cli_config_path"`
	Import        []string            `yaml:"import" json:"import" mapstructure:"import"`
	Docs          Docs                `yaml:"docs,omitempty" json:"docs,omitempty" mapstructure:"docs"`
}

func (m *AtmosConfiguration) GetSchemaRegistry(key string) SchemaRegistry {
	atmosSchemaInterface, interfaceOk := m.Schemas[key]
	var manifestSchema SchemaRegistry
	atmosSchemaFound := false
	if interfaceOk {
		manifestSchema, atmosSchemaFound = atmosSchemaInterface.(SchemaRegistry)
	}
	if atmosSchemaFound {
		return manifestSchema
	}
	return SchemaRegistry{}
}

func (m *AtmosConfiguration) GetResourcePath(key string) ResourcePath {
	atmosSchemaInterface, interfaceOk := m.Schemas[key]
	var resourcePath ResourcePath
	atmosSchemaFound := false
	if interfaceOk {
		resourcePath, atmosSchemaFound = atmosSchemaInterface.(ResourcePath)
	}
	if atmosSchemaFound {
		return resourcePath
	}
	return ResourcePath{}
}

// Custom YAML unmarshaler for `Schemas`.
func (m *AtmosConfiguration) UnmarshalYAML(value *yaml.Node) error {
	type Alias AtmosConfiguration // Prevent recursion
	aux := &struct {
		Schemas map[string]yaml.Node `yaml:"schemas"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	// Decode the full struct (preserves other fields)
	if err := value.Decode(aux); err != nil {
		return err
	}

	// Process Schemas map and pre-cast values
	m.Schemas = make(map[string]interface{})
	for key := range aux.Schemas {
		node := aux.Schemas[key]
		// Try decoding as string
		var strVal string
		if err := node.Decode(&strVal); err == nil {
			m.Schemas[key] = strVal
			continue
		}

		if key == "cue" || key == "opa" || key == "jsonschema" {
			var temp ResourcePath
			if err := node.Decode(&temp); err == nil {
				m.Schemas[key] = temp
				continue
			}
		}

		// Try decoding as Manifest struct
		var manifest SchemaRegistry
		if err := node.Decode(&manifest); err == nil {
			m.Schemas[key] = manifest
			continue
		}

		// If neither works, keep it as raw YAML node (fallback)
		m.Schemas[key] = node
	}

	return nil
}

func (a *AtmosConfiguration) ProcessSchemas() {
	for key := range a.Schemas {
		if key == "cue" || key == "opa" || key == "jsonschema" {
			a.processResourceSchema(key)
			continue
		}
		a.processManifestSchemas(key)
	}
}

func (a *AtmosConfiguration) processManifestSchemas(key string) {
	val, exists := a.Schemas[key]
	if !exists {
		return
	}
	// Marshal the interface{} to JSON
	data, err := json.Marshal(val)
	if err != nil {
		return
	}
	// Unmarshal JSON into ResourcePath struct
	var schemasStruct SchemaRegistry
	if err := json.Unmarshal(data, &schemasStruct); err != nil {
		return
	}
	a.Schemas[key] = schemasStruct
}

func (a *AtmosConfiguration) processResourceSchema(key string) {
	val, exists := a.Schemas[key]
	if !exists {
		return
	}
	// Marshal the interface{} to JSON
	data, err := json.Marshal(val)
	if err != nil {
		return
	}

	// Unmarshal JSON into ResourcePath struct
	var resource ResourcePath
	if err := json.Unmarshal(data, &resource); err != nil {
		return
	}
	a.Schemas[key] = resource
}

type Validate struct {
	EditorConfig EditorConfig `yaml:"editorconfig,omitempty" json:"editorconfig,omitempty" mapstructure:"editorconfig"`
}

type EditorConfig struct {
	IgnoreDefaults  bool     `yaml:"ignore_defaults,omitempty" json:"ignore_defaults,omitempty" mapstructure:"ignore_defaults"`
	DryRun          bool     `yaml:"dry_run,omitempty" json:"dry_run,omitempty" mapstructure:"dry_run"`
	Format          string   `yaml:"format,omitempty" json:"format,omitempty" mapstructure:"format"`
	ConfigFilePaths []string `yaml:"config_file_paths,omitempty" json:"config_file_paths,omitempty" mapstructure:"config_file_paths"`
	Exclude         []string `yaml:"exclude,omitempty" json:"exclude,omitempty" mapstructure:"exclude"`
	Init            bool     `yaml:"init,omitempty" json:"init,omitempty" mapstructure:"init"`

	DisableEndOfLine              bool `yaml:"disable_end_of_line,omitempty" json:"disable_end_of_line,omitempty" mapstructure:"disable_end_of_line"`
	DisableInsertFinalNewline     bool `yaml:"disable_insert_final_newline,omitempty" json:"disable_insert_final_newline,omitempty" mapstructure:"disable_insert_final_newline"`
	DisableIndentation            bool `yaml:"disable_indentation,omitempty" json:"disable_indentation,omitempty" mapstructure:"disable_indentation"`
	DisableIndentSize             bool `yaml:"disable_indent_size,omitempty" json:"disable_indent_size,omitempty" mapstructure:"disable_indent_size"`
	DisableMaxLineLength          bool `yaml:"disable_max_line_length,omitempty" json:"disable_max_line_length,omitempty" mapstructure:"disable_max_line_length"`
	DisableTrimTrailingWhitespace bool `yaml:"disable_trim_trailing_whitespace,omitempty" json:"disable_trim_trailing_whitespace,omitempty" mapstructure:"disable_trim_trailing_whitespace"`
}

type Terminal struct {
	MaxWidth           int                `yaml:"max_width" json:"max_width" mapstructure:"max_width"`
	Pager              string             `yaml:"pager" json:"pager" mapstructure:"pager"`
	Unicode            bool               `yaml:"unicode" json:"unicode" mapstructure:"unicode"`
	SyntaxHighlighting SyntaxHighlighting `yaml:"syntax_highlighting" json:"syntax_highlighting" mapstructure:"syntax_highlighting"`
	NoColor            bool               `yaml:"no_color" json:"no_color" mapstructure:"no_color"`
	TabWidth           int                `yaml:"tab_width,omitempty" json:"tab_width,omitempty" mapstructure:"tab_width"`
}

func (t *Terminal) IsPagerEnabled() bool {
	return t.Pager == "" || t.Pager == "on" || t.Pager == "less" || t.Pager == "true" || t.Pager == "yes" || t.Pager == "y" || t.Pager == "1"
}

type SyntaxHighlighting struct {
	Enabled                bool   `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	Lexer                  string `yaml:"lexer" json:"lexer" mapstructure:"lexer"`
	Formatter              string `yaml:"formatter" json:"formatter" mapstructure:"formatter"`
	Theme                  string `yaml:"theme" json:"theme" mapstructure:"theme"`
	HighlightedOutputPager bool   `yaml:"pager" json:"pager" mapstructure:"pager"`
	LineNumbers            bool   `yaml:"line_numbers" json:"line_numbers" mapstructure:"line_numbers"`
	Wrap                   bool   `yaml:"wrap" json:"wrap" mapstructure:"wrap"`
}

type AtmosSettings struct {
	ListMergeStrategy string   `yaml:"list_merge_strategy" json:"list_merge_strategy" mapstructure:"list_merge_strategy"`
	Terminal          Terminal `yaml:"terminal,omitempty" json:"terminal,omitempty" mapstructure:"terminal"`
	// Deprecated: this was moved to top-level Atmos config
	Docs                 Docs             `yaml:"docs,omitempty" json:"docs,omitempty" mapstructure:"docs"`
	Markdown             MarkdownSettings `yaml:"markdown,omitempty" json:"markdown,omitempty" mapstructure:"markdown"`
	InjectGithubToken    bool             `yaml:"inject_github_token,omitempty" mapstructure:"inject_github_token"`
	GithubToken          string           `yaml:"github_token,omitempty" mapstructure:"github_token"`
	AtmosGithubToken     string           `yaml:"atmos_github_token,omitempty" mapstructure:"atmos_github_token"`
	InjectBitbucketToken bool             `yaml:"inject_bitbucket_token,omitempty" mapstructure:"inject_bitbucket_token"`
	BitbucketToken       string           `yaml:"bitbucket_token,omitempty" mapstructure:"bitbucket_token"`
	AtmosBitbucketToken  string           `yaml:"atmos_bitbucket_token,omitempty" mapstructure:"atmos_bitbucket_token"`
	BitbucketUsername    string           `yaml:"bitbucket_username,omitempty" mapstructure:"bitbucket_username"`
	InjectGitlabToken    bool             `yaml:"inject_gitlab_token,omitempty" mapstructure:"inject_gitlab_token"`
	AtmosGitlabToken     string           `yaml:"atmos_gitlab_token,omitempty" mapstructure:"atmos_gitlab_token"`
	GitlabToken          string           `yaml:"gitlab_token,omitempty" mapstructure:"gitlab_token"`
	// Atmos Pro integration settings
	Pro ProSettings `yaml:"pro,omitempty" json:"pro,omitempty" mapstructure:"pro"`
	// Telemetry settings
	Telemetry TelemetrySettings `yaml:"telemetry,omitempty" json:"telemetry,omitempty" mapstructure:"telemetry"`
}

// TelemetrySettings contains configuration for telemetry collection.
type TelemetrySettings struct {
	Enabled  bool   `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Endpoint string `yaml:"endpoint,omitempty" json:"endpoint,omitempty" mapstructure:"endpoint"`
	Token    string `yaml:"token,omitempty" json:"token,omitempty" mapstructure:"token"`
}

type Docs struct {
	// Deprecated: this has moved to `settings.terminal.max-width`
	MaxWidth int `yaml:"max-width" json:"max_width" mapstructure:"max-width"`
	// Deprecated: this has moved to `settings.terminal.pagination`
	Pagination bool                    `yaml:"pagination" json:"pagination" mapstructure:"pagination"`
	Generate   map[string]DocsGenerate `yaml:"generate,omitempty" json:"generate,omitempty" mapstructure:"generate"`
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
	BasePath                string        `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	ApplyAutoApprove        bool          `yaml:"apply_auto_approve" json:"apply_auto_approve" mapstructure:"apply_auto_approve"`
	AppendUserAgent         string        `yaml:"append_user_agent" json:"append_user_agent" mapstructure:"append_user_agent"`
	DeployRunInit           bool          `yaml:"deploy_run_init" json:"deploy_run_init" mapstructure:"deploy_run_init"`
	InitRunReconfigure      bool          `yaml:"init_run_reconfigure" json:"init_run_reconfigure" mapstructure:"init_run_reconfigure"`
	AutoGenerateBackendFile bool          `yaml:"auto_generate_backend_file" json:"auto_generate_backend_file" mapstructure:"auto_generate_backend_file"`
	WorkspacesEnabled       *bool         `yaml:"workspaces_enabled,omitempty" json:"workspaces_enabled,omitempty" mapstructure:"workspaces_enabled,omitempty"`
	Command                 string        `yaml:"command" json:"command" mapstructure:"command"`
	Shell                   ShellConfig   `yaml:"shell" json:"shell" mapstructure:"shell"`
	Init                    TerraformInit `yaml:"init" json:"init" mapstructure:"init"`
	Plan                    TerraformPlan `yaml:"plan" json:"plan" mapstructure:"plan"`
}

type TerraformInit struct {
	PassVars bool `yaml:"pass_vars" json:"pass_vars" mapstructure:"pass_vars"`
}

type TerraformPlan struct {
	SkipPlanfile bool `yaml:"skip_planfile" json:"skip_planfile" mapstructure:"skip_planfile"`
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
	Terraform Terraform `yaml:"terraform" json:"terraform" mapstructure:"terraform"`
	Helmfile  Helmfile  `yaml:"helmfile" json:"helmfile" mapstructure:"helmfile"`
}

type Stacks struct {
	BasePath      string   `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	IncludedPaths []string `yaml:"included_paths" json:"included_paths" mapstructure:"included_paths"`
	ExcludedPaths []string `yaml:"excluded_paths" json:"excluded_paths" mapstructure:"excluded_paths"`
	NamePattern   string   `yaml:"name_pattern" json:"name_pattern" mapstructure:"name_pattern"`
	NameTemplate  string   `yaml:"name_template" json:"name_template" mapstructure:"name_template"`
}

type Workflows struct {
	BasePath string     `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	List     ListConfig `yaml:"list" json:"list" mapstructure:"list"`
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

type TerraformDocsReadmeSettings struct {
	Source        string `yaml:"source,omitempty" json:"source,omitempty" mapstructure:"source"`
	Enabled       bool   `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Format        string `yaml:"format,omitempty" json:"format,omitempty" mapstructure:"format"`
	ShowProviders bool   `yaml:"show_providers,omitempty" json:"show_providers,omitempty" mapstructure:"show_providers"`
	ShowInputs    bool   `yaml:"show_inputs,omitempty" json:"show_inputs,omitempty" mapstructure:"show_inputs"`
	ShowOutputs   bool   `yaml:"show_outputs,omitempty" json:"show_outputs,omitempty" mapstructure:"show_outputs"`
	SortBy        string `yaml:"sort_by,omitempty" json:"sort_by,omitempty" mapstructure:"sort_by"`
	HideEmpty     bool   `yaml:"hide_empty,omitempty" json:"hide_empty,omitempty" mapstructure:"hide_empty"`
	IndentLevel   int    `yaml:"indent_level,omitempty" json:"indent_level,omitempty" mapstructure:"indent_level"`
}

type DocsGenerate struct {
	BaseDir   string                      `yaml:"base-dir,omitempty" json:"base-dir,omitempty" mapstructure:"base-dir"`
	Input     []any                       `yaml:"input,omitempty" json:"input,omitempty" mapstructure:"input"`
	Template  string                      `yaml:"template,omitempty" json:"template,omitempty" mapstructure:"template"`
	Output    string                      `yaml:"output,omitempty" json:"output,omitempty" mapstructure:"output"`
	Terraform TerraformDocsReadmeSettings `yaml:"terraform,omitempty" json:"terraform,omitempty" mapstructure:"terraform"`
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
	InitPassVars              string
	PlanSkipPlanfile          string
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
	Query                     string
	Affected                  bool
	All                       bool
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
	ComponentHooksSection         AtmosSectionMapType
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
	InitPassVars                  string
	PlanSkipPlanfile              string
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
	ComponentIsLocked             bool
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
	Query                         string
	AtmosConfigFilesFromArg       []string
	AtmosConfigDirsFromArg        []string
	ProcessTemplates              bool
	ProcessFunctions              bool
	Skip                          []string
	CliArgs                       []string
	Affected                      bool
	All                           bool
	Components                    []string
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
	Required    bool   `yaml:"required" json:"required" mapstructure:"required"`
	Default     string `yaml:"default" json:"default" mapstructure:"default"`
}

type CommandFlag struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Shorthand   string `yaml:"shorthand" json:"shorthand" mapstructure:"shorthand"`
	Type        string `yaml:"type" json:"type" mapstructure:"type"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
	Usage       string `yaml:"usage" json:"usage" mapstructure:"usage"`
	Required    bool   `yaml:"required" json:"required" mapstructure:"required"`
	Default     string `yaml:"default" json:"default" mapstructure:"default"`
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

// Validation schemas

type ResourcePath struct {
	BasePath string `yaml:"base_path,omitempty" json:"base_path,omitempty" mapstructure:"base_path"`
}

type SchemaRegistry struct {
	Manifest string   `yaml:"manifest,omitempty" json:"manifest,omitempty" mapstructure:"manifest"`
	Schema   string   `yaml:"schema,omitempty" json:"schema,omitempty" mapstructure:"schema"`
	Matches  []string `yaml:"matches,omitempty" json:"matches,omitempty" mapstructure:"matches"`
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
	AffectedAll          []string            `yaml:"affected_all" json:"affected_all" mapstructure:"affected_all"`
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
	BaseComponentHooks                     AtmosSectionMapType
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
	Dependents           []Dependent         `yaml:"dependents" json:"dependents" mapstructure:"dependents"`
	IncludedInDependents bool                `yaml:"included_in_dependents" json:"included_in_dependents" mapstructure:"included_in_dependents"`
	Settings             AtmosSectionMapType `yaml:"settings" json:"settings" mapstructure:"settings"`
}

// Settings

type SettingsSpacelift AtmosSectionMapType

type Settings struct {
	DependsOn DependsOn         `yaml:"depends_on,omitempty" json:"depends_on,omitempty" mapstructure:"depends_on"`
	Spacelift SettingsSpacelift `yaml:"spacelift,omitempty" json:"spacelift,omitempty" mapstructure:"spacelift"`
	Templates Templates         `yaml:"templates,omitempty" json:"templates,omitempty" mapstructure:"templates"`
}

// ConfigSourcesStackDependency defines schema for sources of config sections.
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

// ComponentManifest defines the structure of the component manifest file (component.yaml).
type ComponentManifest struct {
	APIVersion string         `yaml:"apiVersion,omitempty" json:"apiVersion,omitempty" mapstructure:"apiVersion,omitempty"`
	Kind       string         `yaml:"kind,omitempty" json:"kind,omitempty" mapstructure:"kind,omitempty"`
	Metadata   map[string]any `yaml:"metadata,omitempty" json:"metadata,omitempty" mapstructure:"metadata,omitempty"`
	Spec       map[string]any `yaml:"spec,omitempty" json:"spec,omitempty" mapstructure:"spec,omitempty"`
	Vars       map[string]any `yaml:"vars,omitempty" json:"vars,omitempty" mapstructure:"vars,omitempty"`
}

type Vendor struct {
	// Path to vendor configuration file or directory containing vendor files
	// If a directory is specified, all .yaml files in the directory will be processed in lexicographical order
	BasePath string     `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	List     ListConfig `yaml:"list,omitempty" json:"list,omitempty" mapstructure:"list"`
}

type ChromaStyle struct {
	Color string `yaml:"color,omitempty" json:"color,omitempty" mapstructure:"color"`
}

type ListConfig struct {
	// Format specifies the output format (table, json, csv)
	// If empty, defaults to table format
	Format  string             `yaml:"format" json:"format" mapstructure:"format" validate:"omitempty,oneof=table json csv"`
	Columns []ListColumnConfig `yaml:"columns" json:"columns" mapstructure:"columns"`
}

type ListColumnConfig struct {
	Name  string `yaml:"name" json:"name" mapstructure:"name"`
	Value string `yaml:"value" json:"value" mapstructure:"value"`
}
