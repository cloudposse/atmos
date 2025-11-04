package schema

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
	TerraformVersion          string                        `yaml:"terraform_version,omitempty" json:"terraform_version,omitempty" mapstructure:"terraform_version,omitempty"`
	DeleteSourceBranchOnMerge bool                          `yaml:"delete_source_branch_on_merge" json:"delete_source_branch_on_merge" mapstructure:"delete_source_branch_on_merge"`
	Autoplan                  AtlantisProjectAutoplanConfig `yaml:"autoplan" json:"autoplan" mapstructure:"autoplan"`
	ApplyRequirements         []string                      `yaml:"apply_requirements,omitempty" json:"apply_requirements,omitempty" mapstructure:"apply_requirements,omitempty"`
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
