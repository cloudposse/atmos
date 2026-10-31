package service

import (
	"context"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

var _ Repositories = (*StaticRepositories)(nil)

// Describe implements [Repositories].
func (s *StaticRepositories) Describe(alias string) (RepositoryInfo, error) {
	defer perf.Track(nil, "service.StaticRepositories.Describe")()

	repo, ok := s.Repositories[alias]
	if !ok {
		return RepositoryInfo{}, fmt.Errorf("%w: repository %q is not configured", errUtils.ErrArtifactConfigInvalid, alias)
	}
	info := s.Info[alias]
	info.Alias = alias
	if info.Capabilities.Packagings == nil && !info.Capabilities.Read && !info.Capabilities.Write {
		info.Capabilities = repo.Capabilities()
	}
	if info.Packaging == "" && len(info.Capabilities.Packagings) > 0 {
		info.Packaging = info.Capabilities.Packagings[0]
	}
	return info, nil
}

// Open implements [Repositories].
func (s *StaticRepositories) Open(_ context.Context, alias string) (artifact.Repository, error) {
	defer perf.Track(nil, "service.StaticRepositories.Open")()

	repo, ok := s.Repositories[alias]
	if !ok {
		return nil, fmt.Errorf("%w: repository %q is not configured", errUtils.ErrArtifactConfigInvalid, alias)
	}
	return repo, nil
}

// Aliases implements [Repositories].
func (s *StaticRepositories) Aliases() []string {
	defer perf.Track(nil, "service.StaticRepositories.Aliases")()

	aliases := make([]string, 0, len(s.Repositories))
	for alias := range s.Repositories {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}
