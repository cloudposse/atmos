package install

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/signature"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
)

// SignatureRequired treats a publisher allowlist as enforcement even without the boolean flag.
//
//nolint:lintroller // Pure policy lookup without I/O.
func SignatureRequired(policy *schema.PluginPolicy) bool {
	return policy.RequireSignature || len(policy.TrustedPublishers) != 0 || len(policy.PublisherConstraints) != 0
}

// VerifyInstalled checks current publisher policy against cryptographic evidence.
// Lockfile verification labels are informational and can never authorize execution.
func (s *Service) VerifyInstalled(ctx context.Context, entry *pluginlock.Entry) (pluginlock.Verification, error) {
	defer perf.Track(s.Config, "plugininstall.Service.VerifyInstalled")()
	if !SignatureRequired(&s.Config.Plugins.Policy) {
		return pluginlock.Verification{Signature: "unverified"}, nil
	}
	if _, err := s.Cache.Installed(entry, s.Platform); err != nil {
		return pluginlock.Verification{}, err
	}
	directory, err := s.Cache.location(entry.IndexDigest, s.Platform)
	if err != nil {
		return pluginlock.Verification{}, err
	}
	return s.verifyEvidence(ctx, entry, directory, false)
}

func (s *Service) verifyEvidence(ctx context.Context, entry *pluginlock.Entry, directory string, layout bool) (pluginlock.Verification, error) {
	evidence, err := s.loadEvidence(ctx, entry, directory, layout)
	if err != nil {
		return pluginlock.Verification{}, err
	}
	if !evidence.refreshed {
		return evidence.verification, nil
	}
	if err := os.MkdirAll(directory, privateDirectoryMode); err != nil {
		return pluginlock.Verification{}, err
	}
	if err := signature.Save(directory, evidence.bundles); err != nil {
		return pluginlock.Verification{}, err
	}
	return evidence.verification, nil
}

// Keep the exact verified bytes in memory until the release cache is committed.
// In particular, never reread an offline layout after verifying it.
type verifiedEvidence struct {
	verification pluginlock.Verification
	bundles      [][]byte
	refreshed    bool
}

func (s *Service) loadEvidence(ctx context.Context, entry *pluginlock.Entry, directory string, layout bool) (*verifiedEvidence, error) {
	identities, err := requiredIdentities(&s.Config.Plugins.Policy, entry.Plugin)
	if err != nil {
		return nil, err
	}
	var bundles [][]byte
	if directory != "" {
		bundles, err = signature.Load(directory)
	}
	if err == nil && len(bundles) != 0 {
		result, verifyErr := s.verifyBundles(ctx, entry.IndexDigest, identities, bundles)
		if verifyErr == nil {
			return &verifiedEvidence{verification: result, bundles: bundles}, nil
		}
		err = verifyErr
	}
	if layout || s.Config.Plugins.Policy.Offline {
		return nil, errors.Join(errUtils.ErrPluginSignatureInvalid, err)
	}
	// Cached evidence may predate a tightened policy or a newly attached signature.
	bundles, err = s.fetchEvidence(ctx, entry)
	if err != nil {
		return nil, err
	}
	result, err := s.verifyBundles(ctx, entry.IndexDigest, identities, bundles)
	if err != nil {
		return nil, err
	}
	return &verifiedEvidence{verification: result, bundles: bundles, refreshed: true}, nil
}

func (s *Service) fetchEvidence(ctx context.Context, entry *pluginlock.Entry) ([][]byte, error) {
	registry, ok := s.Registry.(oci.ReferrerRegistry)
	if !ok {
		return nil, errUtils.ErrPluginSignatureInvalid
	}
	ref, err := name.NewDigest(strings.TrimPrefix(entry.Source, "oci://") + "@" + entry.IndexDigest)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	bundles, err := signature.Fetch(ctx, registry, ref)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginSignatureInvalid, err)
	}
	return bundles, nil
}

func requiredIdentities(policy *schema.PluginPolicy, plugin string) ([][]signature.Identity, error) {
	groups := append([][]schema.PluginPublisher(nil), policy.PublisherConstraints...)
	if len(policy.TrustedPublishers) != 0 {
		groups = append(groups, policy.TrustedPublishers)
	}
	if len(groups) == 0 {
		return nil, errUtils.ErrPluginSignatureInvalid
	}
	identities := make([][]signature.Identity, 0, len(groups))
	for _, group := range groups {
		matches := publisherIdentities(plugin, group)
		if len(matches) == 0 {
			return nil, errUtils.ErrPluginSignatureInvalid
		}
		identities = append(identities, matches)
	}
	return identities, nil
}

func (s *Service) verifyBundles(ctx context.Context, digest string, identities [][]signature.Identity, bundles [][]byte) (pluginlock.Verification, error) {
	if len(identities) == 0 {
		return pluginlock.Verification{}, errUtils.ErrPluginSignatureInvalid
	}
	var last error
	for _, raw := range bundles {
		var result *signature.Result
		var err error
		for _, group := range identities {
			result, err = s.Verifier.Verify(ctx, raw, digest, group, s.Config.Plugins.Policy.Offline)
			if err != nil {
				break
			}
		}
		if err == nil {
			return pluginlock.Verification{Signature: "verified", Identity: result.Identity, Issuer: result.Issuer}, nil
		}
		last = err
		if ctx.Err() != nil {
			return pluginlock.Verification{}, ctx.Err()
		}
	}
	return pluginlock.Verification{}, errors.Join(errUtils.ErrPluginSignatureInvalid, last)
}

func publisherIdentities(plugin string, publishers []schema.PluginPublisher) []signature.Identity {
	namespace, _, found := strings.Cut(plugin, "/")
	if !found {
		return nil
	}
	var result []signature.Identity
	for _, publisher := range publishers {
		if publisher.Publisher == namespace {
			result = append(result, signature.Identity{Identity: publisher.Identity, Issuer: publisher.Issuer})
		}
	}
	return result
}
