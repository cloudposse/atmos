package schema

// GitConfig is the top-level `git` section in atmos.yaml.
// Git is a foundational Atmos capability (peer to Toolchain, Auth, and Hooks);
// see docs/prd/git-ops.md.
type GitConfig struct {
	// Repositories maps user-defined logical names to managed Git repositories.
	Repositories map[string]GitRepository `yaml:"repositories,omitempty" json:"repositories,omitempty" mapstructure:"repositories"`
	// Hooks maps local Git hook names (pre-commit, commit-msg, ...) to commands
	// executed by `atmos git hooks run` via generated .git/hooks shims.
	Hooks map[string]GitHookEntry `yaml:"hooks,omitempty" json:"hooks,omitempty" mapstructure:"hooks"`
	// List configures `atmos git list` output (columns, format).
	List GitListConfig `yaml:"list,omitempty" json:"list,omitempty" mapstructure:"list"`
}

// GitRepository describes a managed Git repository under git.repositories.<name>.
type GitRepository struct {
	// Provider selects the execution backend. Default and only v1 value: "cli".
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	// URI is the remote repository URI. Credentials must never be embedded.
	URI string `yaml:"uri" json:"uri" mapstructure:"uri"`
	// Branch to check out; empty means the remote default branch.
	Branch string `yaml:"branch,omitempty" json:"branch,omitempty" mapstructure:"branch"`
	// Remote name. Default: "origin".
	Remote string `yaml:"remote,omitempty" json:"remote,omitempty" mapstructure:"remote"`
	// Workdir overrides the automatic XDG workdir location.
	Workdir string `yaml:"workdir,omitempty" json:"workdir,omitempty" mapstructure:"workdir"`
	// Clone controls clone depth and related options.
	Clone GitCloneConfig `yaml:"clone,omitempty" json:"clone,omitempty" mapstructure:"clone"`
	// Auth selects the Atmos Auth identity used for Git operations on this repository.
	Auth GitAuthConfig `yaml:"auth,omitempty" json:"auth,omitempty" mapstructure:"auth"`
	// Commit controls commit signing and author identity.
	Commit GitCommitConfig `yaml:"commit,omitempty" json:"commit,omitempty" mapstructure:"commit"`
	// Push controls push retry behavior.
	Push GitPushConfig `yaml:"push,omitempty" json:"push,omitempty" mapstructure:"push"`
	// Init sets defaults for `atmos git init` on this repository.
	Init GitInitConfig `yaml:"init,omitempty" json:"init,omitempty" mapstructure:"init"`
}

// GitInitConfig sets defaults for `atmos git init` on a managed repository.
type GitInitConfig struct {
	// From seeds the new repository from another repository's content. It acts
	// as the default for the `--from` flag; the flag overrides it when set.
	From string `yaml:"from,omitempty" json:"from,omitempty" mapstructure:"from"`
	// KeepHistory preserves the source's full history and keeps the source
	// reachable as the 'upstream' remote. Only valid together with From.
	// The `--keep-history` flag also enables it.
	KeepHistory bool `yaml:"keep_history,omitempty" json:"keep_history,omitempty" mapstructure:"keep_history"`
}

// GitCloneConfig controls clone behavior for a managed repository.
type GitCloneConfig struct {
	// Depth is the shallow-clone depth; 0 means full history (default).
	Depth int `yaml:"depth,omitempty" json:"depth,omitempty" mapstructure:"depth"`
	// Filter is an optional partial-clone filter spec (e.g. "blob:none").
	Filter string `yaml:"filter,omitempty" json:"filter,omitempty" mapstructure:"filter"`
	// SingleBranch limits the clone to the configured branch.
	SingleBranch bool `yaml:"single_branch,omitempty" json:"single_branch,omitempty" mapstructure:"single_branch"`
	// Submodules enables submodule initialization. Default: false.
	Submodules bool `yaml:"submodules,omitempty" json:"submodules,omitempty" mapstructure:"submodules"`
}

// GitAuthConfig selects the Atmos Auth identity for a repository.
// Integrations are never referenced by name; they attach via identity links.
type GitAuthConfig struct {
	Identity string `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
}

// GitCommitConfig controls commit signing and author identity.
type GitCommitConfig struct {
	// Signing mode: "auto" (default; Git config decides), "always" (-S), or "never" (--no-gpg-sign).
	Signing string `yaml:"signing,omitempty" json:"signing,omitempty" mapstructure:"signing"`
	// Author overrides the commit author/committer identity (required in CI
	// where no user.name/user.email is configured).
	Author GitAuthorConfig `yaml:"author,omitempty" json:"author,omitempty" mapstructure:"author"`
}

// GitAuthorConfig is the commit author/committer identity.
type GitAuthorConfig struct {
	Name  string `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Email string `yaml:"email,omitempty" json:"email,omitempty" mapstructure:"email"`
}

// GitPushConfig controls push behavior.
type GitPushConfig struct {
	// Retries bounds the pull-ff/re-push loop on non-fast-forward rejection.
	// nil means the default (3). 0 disables retries.
	Retries *int `yaml:"retries,omitempty" json:"retries,omitempty" mapstructure:"retries"`
}

// GitHookEntry configures a local Git hook executed via `atmos git hooks run`.
type GitHookEntry struct {
	Command string `yaml:"command" json:"command" mapstructure:"command"`
}

// GitListConfig configures `atmos git list` output.
type GitListConfig struct {
	Format  string             `yaml:"format,omitempty" json:"format,omitempty" mapstructure:"format"`
	Columns []ListColumnConfig `yaml:"columns,omitempty" json:"columns,omitempty" mapstructure:"columns"`
}
