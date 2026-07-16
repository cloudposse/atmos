package schema

// VendorUpdateConfig configures the local part of component updates. Publishing
// configuration intentionally lives under VendorCIConfig so normal updates stay
// independent of a CI provider.
type VendorUpdateConfig struct {
	Execution VendorUpdateExecutionConfig        `yaml:"execution,omitempty" json:"execution,omitempty" mapstructure:"execution"`
	Batching  VendorUpdateBatchingConfig         `yaml:"batching,omitempty" json:"batching,omitempty" mapstructure:"batching"`
	Groups    map[string]VendorUpdateGroupConfig `yaml:"groups,omitempty" json:"groups,omitempty" mapstructure:"groups"`
}

// VendorUpdateExecutionConfig configures the execution mode for vendor updates.
type VendorUpdateExecutionConfig struct {
	// Mode is "current" (the default) or "worktree".
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty" mapstructure:"mode" validate:"omitempty,oneof=current worktree"`
}

// VendorUpdateBatchingConfig configures the batching strategy for vendor updates.
type VendorUpdateBatchingConfig struct {
	// Mode is "scope" (the default) or "component". Component batching requires
	// execution.mode=worktree because every component receives an isolated branch.
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty" mapstructure:"mode" validate:"omitempty,oneof=scope component"`
}

// VendorUpdateGroupConfig selects components for a named vendor update group.
type VendorUpdateGroupConfig struct {
	Include []string `yaml:"include,omitempty" json:"include,omitempty" mapstructure:"include"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty" mapstructure:"exclude"`
}

// VendorCIConfig configures provider-backed publishing for vendor updates.
type VendorCIConfig struct {
	PullRequest VendorPullRequestConfig `yaml:"pull_request,omitempty" json:"pull_request,omitempty" mapstructure:"pull_request"`
	Summary     VendorSummaryConfig     `yaml:"summary,omitempty" json:"summary,omitempty" mapstructure:"summary"`
}

// VendorPullRequestConfig configures pull request publishing for vendor CI.
type VendorPullRequestConfig struct {
	Provider     string   `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider" validate:"omitempty,oneof=github"`
	BaseBranch   string   `yaml:"base_branch,omitempty" json:"base_branch,omitempty" mapstructure:"base_branch"`
	BranchPrefix string   `yaml:"branch_prefix,omitempty" json:"branch_prefix,omitempty" mapstructure:"branch_prefix"`
	Title        string   `yaml:"title,omitempty" json:"title,omitempty" mapstructure:"title"`
	Body         string   `yaml:"body,omitempty" json:"body,omitempty" mapstructure:"body"`
	Labels       []string `yaml:"labels,omitempty" json:"labels,omitempty" mapstructure:"labels"`
	Draft        bool     `yaml:"draft,omitempty" json:"draft,omitempty" mapstructure:"draft"`
	Reviewers    []string `yaml:"reviewers,omitempty" json:"reviewers,omitempty" mapstructure:"reviewers"`
	Assignees    []string `yaml:"assignees,omitempty" json:"assignees,omitempty" mapstructure:"assignees"`
}

// VendorSummaryConfig configures CI summary output for vendor updates.
type VendorSummaryConfig struct {
	// Enabled defaults to true when omitted. The command treats false as an
	// explicit opt-out only when the summary map is present in configuration.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
}
