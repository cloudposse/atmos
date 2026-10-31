package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
// explains direct named operations still work.
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
	if err := checkEnumeration(alias, &info.Capabilities, request.name); err != nil {
		return fail(err)
	}
	repo, err := s.repos.Open(ctx, alias)
	if err != nil {
		return fail(err)
	}
	entries, err := consumePages(ctx, repo, request)
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

func checkEnumeration(alias string, caps *artifact.Capabilities, name string) error {
	if !caps.Read {
		return artifact.NewUnsupportedCapability(alias, opList, "read", "the repository is write-only")
	}
	if name != "" && !caps.EnumerateNamed {
		return artifact.NewUnsupportedCapability(alias, opList, "enumerate/named", "this repository cannot list versions of a name; direct named operations such as inspect still work")
	}
	if name == "" && !caps.EnumerateCollection {
		return artifact.NewUnsupportedCapability(alias, opList, "enumerate/collection", "this repository has no native collection listing or catalog; direct named operations such as inspect still work")
	}
	return nil
}

func consumePages(ctx context.Context, repo artifact.Repository, request *enumeration) ([]artifact.ListEntry, error) {
	var entries []artifact.ListEntry
	cursor := ""
	for page := 0; page < maxListPages; page++ {
		if err := ctx.Err(); err != nil {
			return entries, err
		}
		result, err := repo.List(ctx, artifact.ListQuery{Name: request.name, Labels: request.labels, Cursor: cursor, Refresh: request.refresh})
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
