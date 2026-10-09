// Package source reconciles declarative skill sources with owned installations.
package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	privateFileMode = 0o600
	scopeProject    = "project"
	scopeUser       = "user"
	canonicalClient = "atmos"
	skillsDir       = "skills"
	directoryMode   = 0o755
	fileMode        = 0o644
	executableMode  = 0o111
	gitSHA1Length   = 40
	gitSHA256Length = 64
	editRemove      = "remove"
)

var (
	ErrInvalid   = errUtils.ErrAISkillSourceInvalid
	ErrDrift     = errUtils.ErrAISkillSourceDrift
	ErrOwnership = errUtils.ErrAISkillSourceOwnership
	ErrRecovery  = errUtils.ErrAISkillSourceRecovery
)

// Lock is portable resolution evidence, partitioned by track and declaration.
type Lock struct {
	Version int                              `yaml:"version"`
	Tracks  map[string]map[string]Resolution `yaml:"tracks"`
}

// Resolution pins every repository contributing to a declaration.
type Resolution struct {
	Declaration  schema.AISkillConfig `yaml:"declaration"`
	Version      string               `yaml:"resolved_version,omitempty"`
	Repositories []Repository         `yaml:"repositories"`
	Skills       []LockedSkill        `yaml:"skills"`
}

// Repository records immutable Git provenance or a local content digest.
type Repository struct {
	Source string `yaml:"source"`
	Ref    string `yaml:"ref,omitempty"`
	Commit string `yaml:"commit,omitempty"`
	Digest string `yaml:"digest,omitempty"`
}

// LockedSkill identifies one complete skill tree within a resolved repository.
type LockedSkill struct {
	Name       string `yaml:"name"`
	Plugin     string `yaml:"plugin,omitempty"`
	Repository int    `yaml:"repository"`
	Path       string `yaml:"path"`
	Digest     string `yaml:"digest"`
}

// Record owns one actual destination, including the canonical Atmos copy.
type Record struct {
	Project    string `json:"project"`
	Source     string `json:"source"`
	Track      string `json:"track"`
	Scope      string `json:"scope"`
	Name       string `json:"name"`
	Client     string `json:"client"`
	Path       string `json:"path"`
	Digest     string `json:"digest"`
	ManualRoot string `json:"manual_root,omitempty"`
}

// State contains only machine-local installation records.
type State struct {
	Version int       `json:"version"`
	Records []*Record `json:"records"`
}

// Options controls reconciliation without changing declarations.
type Options struct {
	Source, Name, Track, Scope, Path                       string
	Clients                                                []string
	Update, Prune, Frozen, Check, DryRun, Force, Uninstall bool
}

// Status describes one desired or recorded destination.
type Status struct {
	Source, Name, Track, Scope, Client, Path, Status string
}

// Fetcher stages a repository and returns its exact provenance.
type Fetcher interface {
	Fetch(context.Context, Repository, string) (Repository, error)
}

// Engine owns dependencies and project identity, never global process configuration.
type Engine struct {
	AdHoc         bool
	Config        *schema.AtmosConfiguration
	Project, Home string
	Fetcher       Fetcher
	manualRoots   []string
	// Rename permits deterministic injection of apply and rollback failures.
	Rename func(string, string) error
}

// New constructs an engine without creating files or contacting sources.
func New(config *schema.AtmosConfiguration) (*Engine, error) {
	base := config.BasePath
	if base == "" {
		base = config.CliConfigPath
	}
	if base == "" {
		base = "."
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	home, err := homedir.Dir()
	if err != nil {
		return nil, err
	}
	// Canonicalize the explicitly selected roots (macOS /var aliases /private/var).
	// Descendant symlinks are still rejected before every filesystem operation.
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	copyConfig := *config
	copyConfig.BasePath = base
	config = &copyConfig
	return &Engine{Config: config, Project: base, Home: home, Fetcher: &GitFetcher{Config: config}, Rename: os.Rename}, nil
}

func (e *Engine) stateDir(scope string) string {
	base := e.Project
	if scope == "user" {
		base = e.Home
	}
	return filepath.Join(base, ".atmos", "skills")
}

func (e *Engine) lockPath() string {
	if e.AdHoc {
		return filepath.Join(e.stateDir("project"), "ad-hoc.lock.yaml")
	}
	return filepath.Join(e.Project, "skills.lock.yaml")
}

func readYAML(path string, value any) error {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return yaml.Unmarshal(raw, value)
}

func validateDeclaration(d *schema.AISkillConfig) error {
	if d.Source == "" {
		return fmt.Errorf("%w: source is required", ErrInvalid)
	}
	if d.SystemPrompt != "" || len(d.AllowedTools)+len(d.RestrictedTools) > 0 || d.DisplayName != "" || d.Description != "" || d.Category != "" {
		return fmt.Errorf("%w: source and inline fields cannot be combined", ErrInvalid)
	}
	if !contains([]string{"auto", "skills", "marketplace"}, d.Kind) {
		return fmt.Errorf("%w: kind %q", ErrInvalid, d.Kind)
	}
	if !contains([]string{scopeProject, scopeUser}, d.Scope) {
		return fmt.Errorf("%w: scope %q", ErrInvalid, d.Scope)
	}
	return nil
}

func normalize(original *schema.AISkillConfig) schema.AISkillConfig {
	d := *original
	if d.Kind == "" {
		d.Kind = "auto"
	}
	if d.Scope == "" {
		d.Scope = "project"
	}
	return d
}
