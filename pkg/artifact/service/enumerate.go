package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// maxListPages bounds pagination so a repository with a non-advancing cursor cannot loop forever.
const maxListPages = 100_000

// listOutcome is the result of enumerating one repository.
type listOutcome struct {
	entries []artifact.ListEntry
	listing artifact.RepositoryListing
	err     error
}

// listRepository enumerates one repository, consuming pagination, and applies the label and text
// filters. A repository without the needed enumeration capability yields a capability error that
// explains direct named operations still work. With an enumerator configured, the entries come from
// the discovery cache or catalog and the repository is opened only when the source needs it.
func (s *Service) listRepository(ctx context.Context, alias string, request *enumeration) listOutcome {
	outcome := listOutcome{listing: artifact.RepositoryListing{Repository: alias}}
	fail := func(err error) listOutcome {
		outcome.err = fmt.Errorf("repository %q: %w", alias, err)
		outcome.listing.Diagnostics = append(outcome.listing.Diagnostics, artifact.Diagnostic{Severity: artifact.SeverityError, Code: "list-failed", Message: err.Error()})
		return outcome
	}
	info, err := s.repos.Describe(alias)
	if err != nil {
		return fail(err)
	}
	if err := s.checkEnumeration(alias, &info, request.name); err != nil {
		return fail(err)
	}
	entries, err := s.entriesOf(ctx, alias, &info, request, &outcome.listing)
	if err != nil {
		return fail(err)
	}
	for index := range entries {
		entries[index].Repository = alias
		if matchesFilters(&entries[index], request) {
			outcome.entries = append(outcome.entries, entries[index])
		}
	}
	outcome.listing.Count = len(outcome.entries)
	return outcome
}

// entriesOf returns the entries of one repository: through the enumerator when one is configured,
// otherwise directly from the repository with the label selectors pushed down.
func (s *Service) entriesOf(ctx context.Context, alias string, info *RepositoryInfo, request *enumeration, listing *artifact.RepositoryListing) ([]artifact.ListEntry, error) {
	open := lazyOpen(s.repos, alias)
	if s.enumerator == nil {
		repo, err := open(ctx)
		if err != nil {
			return nil, err
		}
		return consumePages(ctx, repo, request, nil)
	}
	enumerated, err := s.enumerator.Enumerate(ctx, &EnumerateRequest{
		Alias: alias, Info: info, Name: request.name, Refresh: request.refresh, Open: open,
		Live: func(ctx context.Context, known map[digest.Digest]artifact.ListEntry) ([]artifact.ListEntry, error) {
			repo, err := open(ctx)
			if err != nil {
				return nil, err
			}
			return consumePages(ctx, repo, &enumeration{name: request.name, refresh: request.refresh}, known)
		},
	})
	if err != nil {
		return nil, err
	}
	listing.Source, listing.Diagnostics = enumerated.Source, append(listing.Diagnostics, enumerated.Diagnostics...)
	if !enumerated.FetchedAt.IsZero() {
		listing.FetchedAt = enumerated.FetchedAt.UTC().Format(time.RFC3339)
	}
	return enumerated.Entries, nil
}

// lazyOpen opens a repository at most once, on first use, so a cache hit never resolves credentials.
func lazyOpen(repos Repositories, alias string) func(context.Context) (artifact.Repository, error) {
	var (
		once sync.Once
		repo artifact.Repository
		err  error
	)
	return func(ctx context.Context) (artifact.Repository, error) {
		once.Do(func() { repo, err = repos.Open(ctx, alias) })
		return repo, err
	}
}

func (s *Service) checkEnumeration(alias string, info *RepositoryInfo, name string) error {
	caps := &info.Capabilities
	if !caps.Read {
		return artifact.NewUnsupportedCapability(alias, opList, "read", "the repository is write-only")
	}
	if name != "" && !caps.EnumerateNamed {
		return artifact.NewUnsupportedCapability(alias, opList, "enumerate/named", "this repository cannot list versions of a name; direct named operations such as inspect still work")
	}
	if name == "" && !s.canEnumerateCollection(info) {
		return artifact.NewUnsupportedCapability(alias, opList, "enumerate/collection", "this repository has no native collection listing or catalog; direct named operations such as inspect still work")
	}
	return nil
}

func consumePages(ctx context.Context, repo artifact.Repository, request *enumeration, known map[digest.Digest]artifact.ListEntry) ([]artifact.ListEntry, error) {
	var entries []artifact.ListEntry
	cursor := ""
	for page := 0; page < maxListPages; page++ {
		if err := ctx.Err(); err != nil {
			return entries, err
		}
		result, err := repo.List(ctx, artifact.ListQuery{Name: request.name, Labels: request.labels, Cursor: cursor, Refresh: request.refresh, Known: known})
		if err != nil {
			return entries, err
		}
		entries = append(entries, result.Entries...)
		if result.NextCursor == "" {
			return entries, nil
		}
		if result.NextCursor == cursor {
			return entries, fmt.Errorf("%w: the repository returned the same list cursor twice", errUtils.ErrArtifactPublicationUnavailable)
		}
		cursor = result.NextCursor
	}
	return entries, fmt.Errorf("%w: the listing exceeded %d pages", errUtils.ErrArtifactTooLarge, maxListPages)
}

// matchesFilters applies the exact label selectors (AND semantics) and the search text.
func matchesFilters(entry *artifact.ListEntry, request *enumeration) bool {
	for _, label := range request.labels {
		if value, ok := entry.Labels[label.Key]; !ok || value != label.Value {
			return false
		}
	}
	if request.text == "" {
		return true
	}
	needle := strings.ToLower(request.text)
	haystack := []string{entry.Name, entry.Kind, entry.Description}
	for key, value := range entry.Labels {
		haystack = append(haystack, key, value)
	}
	for _, text := range haystack {
		if strings.Contains(strings.ToLower(text), needle) {
			return true
		}
	}
	return false
}

// sortEntries orders entries by name, then version, then repository alias.
func sortEntries(entries []artifact.ListEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		if entries[i].Version != entries[j].Version {
			return entries[i].Version < entries[j].Version
		}
		return entries[i].Repository < entries[j].Repository
	})
}
