package config

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// The longest auth identity reference.
	maxIdentityLength = 256

	// The longest default artifact description.
	maxDescriptionLength = 4096
)

// aliasPattern is the shape of a repository alias, which appears in `--repository` and step fields.
var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// Validator validates artifact configuration. The zero value is not usable; use [NewValidator].
type Validator struct {
	registry       *artifact.RepositoryRegistry
	identityExists func(name string) bool
}

// Option configures a Validator.
type Option func(*Validator)

// WithRegistry validates kinds against a specific registry instead of the process-wide one.
func WithRegistry(registry *artifact.RepositoryRegistry) Option {
	defer perf.Track(nil, "artifactconfig.WithRegistry")()

	return func(v *Validator) { v.registry = registry }
}

// WithIdentityLookup also checks that each referenced identity exists. Without it only the shape
// of the identity name is checked: identities are resolved lazily and may be provisioned later.
func WithIdentityLookup(exists func(name string) bool) Option {
	defer perf.Track(nil, "artifactconfig.WithIdentityLookup")()

	return func(v *Validator) { v.identityExists = exists }
}

// NewValidator returns a Validator.
func NewValidator(opts ...Option) *Validator {
	defer perf.Track(nil, "artifactconfig.NewValidator")()

	v := &Validator{registry: artifact.DefaultRepositoryRegistry()}
	for _, opt := range opts {
		if opt != nil {
			opt(v)
		}
	}
	return v
}

// Validate validates a whole artifact configuration with a default Validator.
func Validate(cfg *schema.ArtifactConfig, opts ...Option) error {
	defer perf.Track(nil, "artifactconfig.Validate")()

	return NewValidator(opts...).Validate(cfg)
}

// Validate checks the global verification policy and every repository, returning all problems
// joined in a deterministic order. Every error wraps errUtils.ErrArtifactConfigInvalid. It
// performs no network access, credential resolution, or directory creation.
func (v *Validator) Validate(cfg *schema.ArtifactConfig) error {
	defer perf.Track(nil, "artifactconfig.Validator.Validate")()

	if cfg == nil {
		return nil
	}
	var errs []error
	if err := validateVerification("artifacts.verification", cfg.Verification); err != nil {
		errs = append(errs, err)
	}
	for _, alias := range sortedAliases(cfg) {
		repo := cfg.Repositories[alias]
		if err := v.validateRepository(cfg, alias, &repo); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ValidateRepository validates one repository in the context of the whole configuration, which
// the lock delegation and verification checks need.
func (v *Validator) ValidateRepository(cfg *schema.ArtifactConfig, alias string) error {
	defer perf.Track(nil, "artifactconfig.Validator.ValidateRepository")()

	repo, ok := cfg.Repositories[alias]
	if !ok {
		return fmt.Errorf("%w: repository %q is not configured (configured: %v)", errUtils.ErrArtifactConfigInvalid, alias, sortedAliases(cfg))
	}
	return v.validateRepository(cfg, alias, &repo)
}

func (v *Validator) validateRepository(cfg *schema.ArtifactConfig, alias string, repo *schema.ArtifactRepositoryConfig) error {
	prefix := fmt.Sprintf("repository %q", alias)
	if !aliasPattern.MatchString(alias) {
		return fmt.Errorf("%w: %s: alias must start with a letter or digit and contain only letters, digits, '_', '.' and '-'", errUtils.ErrArtifactConfigInvalid, prefix)
	}
	desc, err := v.resolveKind(repo.Kind)
	if err != nil {
		return fmt.Errorf("%s: %w", prefix, err)
	}
	errs := []error{
		v.validateIdentity(repo.Identity),
		artifact.ValidateRepositoryConfig(desc, repo),
		validatePathOptions(desc, repo),
		validateCatalog(desc, repo),
		validateDefaults(repo.Defaults),
		validateVerification("verification", repo.Verification),
		validateWeakening(cfg.Verification, repo.Verification),
		validateLocking(v, cfg, alias, repo),
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("%s: %w", prefix, err)
	}
	return nil
}

// resolveKind finds the descriptor for a kind, rejecting empty and unknown kinds.
func (v *Validator) resolveKind(kind string) (*artifact.KindDescriptor, error) {
	if kind == "" {
		return nil, fmt.Errorf("%w: kind is required (known kinds: %v)", errUtils.ErrArtifactConfigInvalid, v.registry.Kinds())
	}
	desc, ok := v.registry.Lookup(kind)
	if !ok {
		return nil, fmt.Errorf("%w: unknown kind %q (known kinds: %v)", errUtils.ErrArtifactConfigInvalid, kind, v.registry.Kinds())
	}
	return desc, nil
}

// validateIdentity checks the shape of an auth identity reference. Resolution is lazy.
func (v *Validator) validateIdentity(identity string) error {
	if identity == "" {
		return nil
	}
	switch {
	case len(identity) > maxIdentityLength:
		return fmt.Errorf("%w: identity exceeds %d characters", errUtils.ErrArtifactConfigInvalid, maxIdentityLength)
	case strings.IndexFunc(identity, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0:
		return fmt.Errorf("%w: identity %q must be an auth identity name without whitespace", errUtils.ErrArtifactConfigInvalid, identity)
	case strings.Contains(identity, "://"):
		return fmt.Errorf("%w: identity %q looks like a credential or URL; reference an Atmos auth identity name", errUtils.ErrArtifactConfigInvalid, identity)
	case v.identityExists != nil && !v.identityExists(identity):
		return fmt.Errorf("%w: identity %q is not defined under auth.identities", errUtils.ErrArtifactConfigInvalid, identity)
	}
	return nil
}

// validatePathOptions rejects local path options that cannot be anchored to a local file.
func validatePathOptions(desc *artifact.KindDescriptor, repo *schema.ArtifactRepositoryConfig) error {
	var errs []error
	for _, option := range desc.PathOptions {
		value, ok := repo.Options[option].(string)
		if !ok || value == "" {
			continue
		}
		if _, err := AnchorPath(repo.PathOrigin, value); err != nil {
			errs = append(errs, fmt.Errorf("options.%s: %w", option, err))
		}
	}
	return errors.Join(errs...)
}

// validateDefaults checks the default artifact metadata.
func validateDefaults(defaults *schema.ArtifactRepositoryDefaults) error {
	if defaults == nil || defaults.Metadata == nil {
		return nil
	}
	var errs []error
	if len(defaults.Metadata.Description) > maxDescriptionLength {
		errs = append(errs, fmt.Errorf("%w: defaults.metadata.description exceeds %d bytes", errUtils.ErrArtifactConfigInvalid, maxDescriptionLength))
	}
	if err := artifact.ValidateLabels(defaults.Metadata.Labels); err != nil {
		errs = append(errs, fmt.Errorf("defaults.metadata.labels: %w", err))
	}
	return errors.Join(errs...)
}

func sortedAliases(cfg *schema.ArtifactConfig) []string {
	aliases := make([]string, 0, len(cfg.Repositories))
	for alias := range cfg.Repositories {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}
