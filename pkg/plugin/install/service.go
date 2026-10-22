package install

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/registryauth"
	"github.com/cloudposse/atmos/pkg/keyring"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/signature"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginconfig "github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Service prepares verified installs; configuration transactions are separate.
type Service struct {
	Verifier    signature.Verifier
	Config      *schema.AtmosConfiguration
	Descriptors *extension.Registry
	Registry    atmosoci.Registry
	Cache       *Cache
	Platform    string
}

// Request selects a release or restores a previously locked one.
type Request struct {
	PinnedDigest    string
	SelectedVersion string
	Source          string
	Constraint      string
	Locked          *pluginlock.Entry
	// Previous informs update feedback without constraining release selection.
	Previous *pluginlock.Entry
	Layout   string
	// Alias names the package in repair hints; it defaults to the locked package name.
	Alias string
	// Frozen forbids changing the cache: a corrupt locked release fails instead of being repaired.
	Frozen bool
}

// NewService composes shared registry auth and the existing XDG/keyring services.
func NewService(config *schema.AtmosConfiguration, descriptors *extension.Registry, options ...Option) (*Service, error) {
	defer perf.Track(config, "plugininstall.NewService")()

	if err := pluginconfig.Validate(&config.Plugins); err != nil {
		return nil, err
	}

	cache, err := NewCache(config.Plugins.CacheDir)
	if err != nil {
		return nil, err
	}
	chain := atmosoci.CredentialKeychain{Open: func() (keyring.Keyring, error) { return credentials.OpenKeyring(&config.Auth) }, Optional: config.Auth.Keyring.Type == "" && credentials.KeyringType(&config.Auth) == "system"}
	bindings, err := registryauth.PluginBindings(config)
	if err != nil {
		return nil, err
	}
	settings := serviceOptions{}
	for _, option := range options {
		option(&settings)
	}
	registry := pluginpolicy.Registry{Next: atmosoci.NewClient(config, atmosoci.WithCredentialFallback(chain), atmosoci.WithAuthenticationResolver(registryauth.New(config, bindings, registryauth.WithPreparation(settings.registryPreparation))))}
	return &Service{Config: config, Descriptors: descriptors, Registry: registry, Cache: cache, Platform: CurrentPlatform()}, nil
}

// Prepare resolves immutable metadata and installs bytes without editing a lock
// or declaration. Callers commit the returned entry only after all packages pass.
func (s *Service) Prepare(ctx context.Context, request *Request) (*pluginlock.Entry, error) {
	defer perf.Track(s.Config, "plugininstall.Service.Prepare")()

	return s.prepare(ctx, request, func(string) {})
}

func (s *Service) prepare(ctx context.Context, request *Request, reportPhase func(string)) (*pluginlock.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := pluginconfig.Validate(&s.Config.Plugins); err != nil {
		return nil, err
	}
	if err := pluginpolicy.Source(ctx, "install", request.Source, request.policyOptions()...); err != nil {
		return nil, err
	}
	cached, err := s.cachedEntry(request)
	if err != nil {
		return nil, err
	}
	if cached != nil {
		if err := s.Cache.CheckPolicy(ctx, "install", cached, s.Platform, s.Descriptors); err != nil {
			return nil, err
		}
		verification, err := s.VerifyInstalled(ctx, cached)
		if err != nil {
			return nil, err
		}
		result := *cached
		result.Verification = verification
		reportPhase("Cached")
		return &result, nil
	}
	reportPhase("Resolving")
	release, err := s.resolve(ctx, request)
	if err != nil {
		return nil, err
	}
	if err := checkRequestedRelease(request, release); err != nil {
		return nil, err
	}
	return s.prepareRelease(ctx, request, release, reportPhase)
}

func (s *Service) prepareRelease(ctx context.Context, request *Request, release *pluginoci.Release, reportPhase func(string)) (*pluginlock.Entry, error) {
	options := append(request.policyOptions(), pluginpolicy.WithExtensions(s.Descriptors), pluginpolicy.WithDigest(release.Digest))
	if err := pluginpolicy.Package(ctx, "install", request.Source, release.Definition, options...); err != nil {
		return nil, err
	}
	evidence := &verifiedEvidence{verification: pluginlock.Verification{Signature: "unverified"}}
	if SignatureRequired(&s.Config.Plugins.Policy) {
		reportPhase("Verifying publisher")
		var err error
		evidence, err = s.loadEvidence(ctx, releaseEntry(request, release, nil), request.Layout, request.Layout != "")
		if err != nil {
			return nil, err
		}
	}
	reportPhase("Downloading and verifying")
	directory, err := s.Cache.Store(ctx, release, s.Platform, s.Descriptors)
	if err != nil {
		return nil, err
	}
	reportPhase("Validating")
	bindings, err := s.Descriptors.Validate(directory, release.Definition.Spec.Capabilities)
	if err != nil {
		return nil, err
	}
	if len(evidence.bundles) != 0 {
		if err := signature.Save(filepath.Dir(directory), evidence.bundles); err != nil {
			return nil, err
		}
	}
	entry := releaseEntry(request, release, bindings)
	entry.Verification = evidence.verification
	return entry, nil
}

func (s *Service) cachedEntry(request *Request) (*pluginlock.Entry, error) {
	if request.Locked == nil {
		return nil, nil
	}
	if !matchesLock(request) {
		return nil, errUtils.ErrPluginFrozenDrift
	}
	_, err := s.Cache.Installed(request.Locked, s.Platform)
	if err == nil {
		return request.Locked, nil
	}
	if errors.Is(err, errUtils.ErrPluginCacheCorrupt) {
		return nil, s.checkRepairable(request, err)
	}
	return nil, nil
}

// checkRepairable lets Store quarantine and re-fetch a corrupt locked release only when
// the caller may change the cache and the verified bytes can be obtained again.
func (s *Service) checkRepairable(request *Request, cause error) error {
	switch {
	case request.Frozen:
		return s.Cache.CorruptReleaseError(cause, request.Alias, request.Locked, s.Platform, RepairFrozen)
	case s.Config.Plugins.Policy.Offline && request.Layout == "":
		return s.Cache.CorruptReleaseError(cause, request.Alias, request.Locked, s.Platform, RepairOffline)
	}
	return nil
}

func (s *Service) resolve(ctx context.Context, request *Request) (*pluginoci.Release, error) {
	if request.Layout != "" {
		return resolveLayout(request)
	}
	if s.Config.Plugins.Policy.Offline {
		return nil, errUtils.ErrPluginOfflineArtifact
	}
	repository, err := name.NewRepository(strings.TrimPrefix(request.Source, "oci://"), name.StrictValidation)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	ref, err := s.resolveReference(ctx, repository, request)
	if err != nil {
		return nil, err
	}
	descriptor, err := s.Registry.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	index, err := descriptor.ImageIndex()
	if err != nil {
		return nil, err
	}
	release, err := pluginoci.Inspect(index, descriptor.Digest.String())
	if err != nil {
		return nil, err
	}
	if tag, ok := ref.(name.Tag); ok && strings.TrimPrefix(tag.TagStr(), "v") != release.Version {
		return nil, errUtils.ErrOCIIntegrity
	}
	return release, nil
}

func checkRequestedRelease(request *Request, release *pluginoci.Release) error {
	if request.SelectedVersion != "" && release.Version != request.SelectedVersion {
		return errUtils.ErrOCIIntegrity
	}
	if err := versionMatches(release.Version, request.Constraint); err != nil {
		return err
	}
	if request.Locked != nil {
		if err := pluginlock.CheckRelease(request.Locked, release); err != nil {
			return err
		}
	}

	return nil
}

func resolveLayout(request *Request) (*pluginoci.Release, error) {
	expected := ""
	if request.Locked != nil {
		expected = request.Locked.IndexDigest
	}
	index, err := atmosoci.LoadIndex(request.Layout, expected)
	if err != nil {
		return nil, err
	}
	return pluginoci.Inspect(index, expected)
}

func (s *Service) resolveReference(ctx context.Context, repository name.Repository, request *Request) (name.Reference, error) {
	var ref name.Reference
	var err error
	switch {
	case request.Locked != nil:
		ref = repository.Digest(request.Locked.IndexDigest)
	case request.PinnedDigest != "":
		ref, err = name.NewDigest(repository.Name() + "@" + request.PinnedDigest)
		if err != nil {
			return nil, err
		}
	default:
		tag, err := s.selectVersion(ctx, repository, request.Constraint)
		if err != nil {
			return nil, err
		}
		ref = repository.Tag(tag)
	}
	return ref, nil
}

func releaseEntry(request *Request, release *pluginoci.Release, bindings []extension.Binding) *pluginlock.Entry {
	entry := &pluginlock.Entry{Plugin: release.Name, Source: request.Source, Constraint: request.Constraint, Version: release.Version, IndexDigest: release.Digest, Runtime: release.Definition.Spec.Runtime, Platforms: make(map[string]string), Exports: make([]pluginlock.Export, 0, len(bindings)), Verification: pluginlock.Verification{Signature: "unverified"}}
	for platform := range release.Platforms {
		descriptor := release.Platforms[platform]
		entry.Platforms[platform] = descriptor.Digest.String()
	}
	for _, binding := range bindings {
		entry.Exports = append(entry.Exports, pluginlock.Export{Capability: binding.Capability, Name: binding.Name})
	}

	return entry
}

func matchesLock(request *Request) bool {
	return request.Source == request.Locked.Source && request.Constraint == request.Locked.Constraint
}
