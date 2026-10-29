// Package policy supplies plugin collectors for the shared CEL and Starlark policy engine.
package policy

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// OperationTest gates running a plugin's conformance fixtures and authored tests. It acts on an
// explicit local development directory with no release provenance, so it is deliberately not
// subject to plugins.policy.require_signature: an author
// must be able to test an unsigned local plugin before there is anything to sign. Organizations
// restrict it with rules such as `input.operation == 'test'`, optionally combined with
// `input.development`. Compiling a plugin (`plugin build`) is not a policy operation: it admits
// nothing and runs no plugin code.
const OperationTest = "test"

// Option supplies inspected evidence and the artifact origin.
type Option func(*options)

type options struct {
	origin     string
	digest     string
	path       string
	extensions *extension.Registry
}

// WithLayout identifies an explicit local OCI import, whose claimed repository
// has not been contacted by this operation.
//
//nolint:lintroller // Pure functional option.
func WithLayout() Option { return func(settings *options) { settings.origin = "layout" } }

// WithCache identifies verified cached bytes selected through a project lock.
//
//nolint:lintroller // Pure functional option.
func WithCache() Option { return func(settings *options) { settings.origin = "cache" } }

// WithDigest supplies the inspected release index digest.
//
//nolint:lintroller // Pure functional option.
func WithDigest(digest string) Option {
	return func(settings *options) { settings.digest = digest }
}

// WithPath supplies the policy path of a local build, as returned by DevelopmentPath. It is exposed
// as input.path only for origin development and is ignored for every other origin.
//
//nolint:lintroller // Pure functional option.
func WithPath(path string) Option {
	return func(settings *options) { settings.path = path }
}

// DevelopmentPath renders a local plugin directory for policy input.
//
// Inside the project directory it is a clean forward-slash path relative to it, with symlinks
// resolved, so a rule can compare it against relative allow-roots such as `build/`. A directory
// outside the project, or one that cannot be related to it, is returned absolute and slash-normalized
// instead. The result never contains a `..` segment, so a relative allow-root cannot match a
// directory that escapes the project.
func DevelopmentPath(project, directory string) string {
	defer perf.Track(nil, "pluginpolicy.DevelopmentPath")()

	resolved := resolveSymlinks(directory)
	root := resolveSymlinks(project)
	if relative, err := filepath.Rel(root, resolved); err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(resolved)
}

// resolveSymlinks makes a path absolute and follows symlinks. A path that does not exist yet, such as a
// build directory that is still to be created, keeps its missing tail below the resolved nearest
// existing ancestor, so it compares consistently with a resolved project directory.
func resolveSymlinks(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	remainder := ""
	for current := absolute; ; {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(resolved, remainder)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absolute
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
	}
}

// WithExtensions supplies registered contracts for metadata-only capability discovery.
//
//nolint:lintroller // Pure functional option.
func WithExtensions(registry *extension.Registry) Option {
	return func(settings *options) { settings.extensions = registry }
}

// Source checks the actual repository before network access or cached reuse.
// An empty source represents local development, including conformance testing.
func Source(ctx context.Context, operation, source string, options ...Option) error {
	defer perf.Track(nil, "pluginpolicy.Source")()

	enabled, err := guard.Enabled("plugin")
	if err != nil || !enabled {
		return err
	}
	input, err := describe(operation, source)
	if err != nil {
		return err
	}
	applyOptions(input, options)
	return guard.Plugin(ctx, input)
}

// Login admits storing registry credentials without exposing the username or token.
func Login(ctx context.Context, host string) error {
	defer perf.Track(nil, "pluginpolicy.Login")()

	enabled, err := guard.Enabled("plugin")
	if err != nil || !enabled {
		return err
	}
	registry, err := name.NewRegistry(host, name.StrictValidation)
	if err != nil {
		return errUtils.ErrPolicyInputInvalid
	}
	return guard.Plugin(ctx, &guard.PluginInput{
		Operation: "registry.login", Phase: "source", Origin: "registry", Source: "oci://" + registry.Name(),
		Registry: registry.Name(), Capabilities: []string{},
	})
}

// Package checks inspected metadata before installing or activating executable code.
// It never treats a catalog entry or lockfile verification label as authenticated metadata.
func Package(ctx context.Context, operation, source string, definition *manifest.Config, options ...Option) error {
	defer perf.Track(nil, "pluginpolicy.Package")()

	enabled, err := guard.Enabled("plugin")
	if err != nil || !enabled {
		return err
	}
	input, err := describe(operation, source)
	if err != nil {
		return err
	}
	if definition == nil {
		return errUtils.ErrPolicyInputInvalid
	}
	input.Phase = "package"
	input.Name, input.Version = definition.Metadata.Name, definition.Metadata.Version
	input.Runtime = definition.Spec.Runtime.Type
	settings := applyOptions(input, options)
	if settings.extensions == nil {
		return errUtils.ErrPolicyInputUnavailable
	}
	bindings, err := settings.extensions.Discover(definition.Spec.Capabilities)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		input.Capabilities = append(input.Capabilities, binding.Capability)
	}
	slices.Sort(input.Capabilities)
	input.Capabilities = slices.Compact(input.Capabilities)
	return guard.Plugin(ctx, input)
}

func applyOptions(input *guard.PluginInput, opts []Option) options {
	settings := options{}
	for _, option := range opts {
		option(&settings)
	}
	input.Digest = settings.digest
	if settings.origin != "" {
		input.Origin = settings.origin
	}
	if input.Origin == "development" {
		input.Path = settings.path
	}
	return settings
}

func describe(operation, source string) (*guard.PluginInput, error) {
	input := &guard.PluginInput{Operation: operation, Phase: "source", Origin: "registry", Capabilities: []string{}, Development: source == ""}
	if source == "" {
		input.Origin = "development"
		return input, nil
	}
	raw := strings.TrimPrefix(source, "oci://")
	repository, err := name.NewRepository(raw, name.StrictValidation)
	if err != nil {
		ref, refErr := name.ParseReference(raw, name.StrictValidation)
		if refErr == nil {
			repository, err = ref.Context(), nil
		}
	}
	if err != nil {
		// A malformed reference could contain credentials. Do not echo it.
		return nil, errUtils.ErrPolicyInputInvalid
	}
	input.Source = "oci://" + repository.Name()
	input.Registry, input.Repository = repository.RegistryStr(), repository.RepositoryStr()
	return input, nil
}
