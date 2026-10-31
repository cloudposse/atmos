package artifact

import (
	"errors"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Repository kinds defined by V1. They name repository backends and are independent of artifact
// kinds such as "files" or "component/terraform".
const (
	// RepositoryKindLocalDir stores copied files below a local directory.
	RepositoryKindLocalDir = "local/dir"

	// RepositoryKindLocalArchive stores one archive per artifact below a local directory.
	RepositoryKindLocalArchive = "local/archive"

	// RepositoryKindS3 stores objects in an S3 bucket.
	RepositoryKindS3 = "aws/s3"

	// RepositoryKindOCIRegistry stores artifacts in an OCI registry.
	RepositoryKindOCIRegistry = "oci/registry"

	// RepositoryKindOCILayout stores artifacts in an OCI image layout directory.
	RepositoryKindOCILayout = "oci/layout"
)

// KindDescriptor is the static, network-free description of a repository kind. Configuration
// validation, path anchoring, and lock delegation checks use it before any backend exists or
// any credential is resolved.
//
// The V1 kinds below have built-in descriptors so that validation works before backend packages
// register. A backend that registers its kind supplies its own descriptor, which then takes
// precedence (typically a copy of the built-in one with refinements).
type KindDescriptor struct {
	// Kind is the repository kind name, for example "aws/s3".
	Kind string

	// Capabilities are the capabilities known from configuration alone (supported packagings and
	// lock scopes). A constructed repository's own Capabilities refine them.
	Capabilities Capabilities

	// DefaultPackaging is the packaging used when the configuration omits it.
	DefaultPackaging Packaging

	// PathOptions names the option keys that are local destination paths. Relative values resolve
	// against the declaring configuration file when the configuration is loaded.
	PathOptions []string

	// AllowsPermissions reports that the kind accepts the `permissions` field.
	AllowsPermissions bool

	// AllowsCatalog reports that the kind accepts the `catalog` field.
	AllowsCatalog bool

	// NewOptions returns a fresh typed options struct for strict decoding and validation.
	NewOptions func() Options
}

// UsesLocalPath reports whether the named option of the kind is a local destination path.
func (d *KindDescriptor) UsesLocalPath(option string) bool {
	defer perf.Track(nil, "artifact.KindDescriptor.UsesLocalPath")()

	for _, name := range d.PathOptions {
		if name == option {
			return true
		}
	}
	return false
}

// EffectivePackaging returns the configured packaging, or the kind's default when it is omitted.
func (d *KindDescriptor) EffectivePackaging(configured string) Packaging {
	defer perf.Track(nil, "artifact.KindDescriptor.EffectivePackaging")()

	if configured == "" {
		return d.DefaultPackaging
	}
	return Packaging(configured)
}

// builtinKinds returns the documented static set of V1 repository kinds.
func builtinKinds() map[string]KindDescriptor {
	archive := []Packaging{PackagingTar, PackagingTgz, PackagingZip}
	return map[string]KindDescriptor{
		RepositoryKindLocalDir: {
			Kind:             RepositoryKindLocalDir,
			Capabilities:     Capabilities{Read: true, Write: true, Packagings: []Packaging{PackagingNone}, LockScopes: []LockScope{LockScopeHost}},
			DefaultPackaging: PackagingNone,
			PathOptions:      []string{OptionPath},
			NewOptions:       func() Options { return &LocalDirOptions{} },
		},
		RepositoryKindLocalArchive: {
			Kind:              RepositoryKindLocalArchive,
			Capabilities:      Capabilities{Read: true, Write: true, Packagings: archive, LockScopes: []LockScope{LockScopeHost}},
			DefaultPackaging:  PackagingTgz,
			PathOptions:       []string{OptionPath},
			AllowsPermissions: true,
			NewOptions:        func() Options { return &LocalArchiveOptions{} },
		},
		RepositoryKindS3: {
			Kind:             RepositoryKindS3,
			Capabilities:     Capabilities{Read: true, Write: true, Packagings: append([]Packaging{PackagingNone}, archive...), LockScopes: []LockScope{LockScopeHost, LockScopeRepository}},
			DefaultPackaging: PackagingTgz,
			NewOptions:       func() Options { return &S3Options{} },
		},
		RepositoryKindOCIRegistry: {
			Kind:             RepositoryKindOCIRegistry,
			Capabilities:     Capabilities{Read: true, Write: true, Packagings: archive, LockScopes: []LockScope{LockScopeHost}},
			DefaultPackaging: PackagingTgz,
			AllowsCatalog:    true,
			NewOptions:       func() Options { return &OCIRegistryOptions{} },
		},
		RepositoryKindOCILayout: {
			Kind:             RepositoryKindOCILayout,
			Capabilities:     Capabilities{Read: true, Write: true, Packagings: archive, LockScopes: []LockScope{LockScopeHost}},
			DefaultPackaging: PackagingTgz,
			PathOptions:      []string{OptionPath},
			NewOptions:       func() Options { return &OCILayoutOptions{} },
		},
	}
}

// BuiltinKind returns the built-in descriptor of a V1 repository kind.
func BuiltinKind(kind string) (*KindDescriptor, bool) {
	defer perf.Track(nil, "artifact.BuiltinKind")()

	desc, ok := builtinKinds()[kind]
	if !ok {
		return nil, false
	}
	return &desc, true
}

// BuiltinKinds returns the sorted names of the V1 repository kinds.
func BuiltinKinds() []string {
	defer perf.Track(nil, "artifact.BuiltinKinds")()

	kinds := builtinKinds()
	names := make([]string, 0, len(kinds))
	for name := range kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateRepositoryConfig checks one repository configuration against its kind descriptor:
// the packaging matrix, permissions (accepted only where the kind allows them, and only as a
// quoted octal string), and strict typed options with their required values. It performs no
// network access, credential resolution, or directory creation.
func ValidateRepositoryConfig(desc *KindDescriptor, cfg *schema.ArtifactRepositoryConfig) error {
	defer perf.Track(nil, "artifact.ValidateRepositoryConfig")()

	if desc == nil || cfg == nil {
		return fmt.Errorf("%w: repository configuration is nil", errUtils.ErrArtifactConfigInvalid)
	}
	var errs []error
	packaging, err := validatePackagingFor(desc, cfg.Packaging)
	errs = append(errs, err)
	errs = append(errs, validatePermissionsFor(desc, cfg.Permissions))
	errs = append(errs, validateOptionsFor(desc, cfg, packaging))
	return errors.Join(errs...)
}

// validatePackagingFor checks the packaging against the kind's matrix and returns the effective packaging.
func validatePackagingFor(desc *KindDescriptor, configured string) (Packaging, error) {
	packaging := desc.EffectivePackaging(configured)
	if configured == "" {
		return packaging, nil
	}
	if !desc.Capabilities.HasPackaging(packaging) {
		return packaging, fmt.Errorf("%w: %w: kind %q does not support packaging %q (supported: %v)", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactPackagingInvalid, desc.Kind, configured, desc.Capabilities.Packagings)
	}
	return packaging, nil
}

func validatePermissionsFor(desc *KindDescriptor, permissions string) error {
	if permissions == "" {
		return nil
	}
	if !desc.AllowsPermissions {
		return fmt.Errorf("%w: kind %q does not accept permissions; they apply only to local archive files", errUtils.ErrArtifactConfigInvalid, desc.Kind)
	}
	_, err := ParsePermissions(permissions)
	return err
}

func validateOptionsFor(desc *KindDescriptor, cfg *schema.ArtifactRepositoryConfig, packaging Packaging) error {
	if desc.NewOptions == nil {
		return nil
	}
	options := desc.NewOptions()
	if err := DecodeOptions(cfg.Options, options); err != nil {
		return err
	}
	errs := []error{options.Validate()}
	if aware, ok := options.(PackagingValidator); ok {
		errs = append(errs, aware.ValidateForPackaging(packaging))
	}
	return errors.Join(errs...)
}
