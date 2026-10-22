package install

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
)

func (s *Service) selectVersion(ctx context.Context, repository name.Repository, constraint string) (string, error) {
	requested, err := semver.NewConstraint(constraint)
	if err != nil {
		return "", errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	tags, err := s.Registry.Tags(ctx, repository)
	if err != nil {
		return "", err
	}
	// Stable sorting resolves aliases such as 1.0.0 and v1.0.0 deterministically.
	sort.Strings(tags)
	// A wildcard falls back to pre-releases only when nothing else matches, so a repository that
	// has published only release candidates remains installable.
	if result := highestMatching(tags, requested.Check); result != "" {
		return result, nil
	}
	if isWildcard(constraint) {
		if result := highestMatching(tags, func(*semver.Version) bool { return true }); result != "" {
			return result, nil
		}
	}
	return "", errUtils.ErrPluginVersionNotFound
}

func highestMatching(tags []string, matches func(*semver.Version) bool) string {
	var selected *semver.Version
	var result string
	for _, tag := range tags {
		candidate, err := semver.StrictNewVersion(strings.TrimPrefix(tag, "v"))
		if err != nil || !matches(candidate) {
			continue
		}
		if selected == nil || candidate.GreaterThan(selected) {
			selected, result = candidate, tag
		}
	}
	return result
}

func isWildcard(constraint string) bool {
	trimmed := strings.TrimSpace(constraint)
	return trimmed == "*" || trimmed == "x" || trimmed == "X"
}

func versionMatches(version, constraint string) error {
	requested, err := semver.NewConstraint(constraint)
	if err != nil {
		return err
	}
	selected, err := semver.StrictNewVersion(version)
	if err != nil {
		return err
	}
	if !requested.Check(selected) && (!isWildcard(constraint) || selected.Prerelease() == "") {
		return errUtils.ErrPluginVersionNotFound
	}
	return nil
}
