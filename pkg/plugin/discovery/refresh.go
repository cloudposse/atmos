package discovery

import (
	"context"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

func (s *Service) refresh(ctx context.Context, item source, old *snapshot) (*snapshot, error) {
	if item.kind == "catalog" {
		return s.refreshCatalog(ctx, item.reference, old)
	}
	return s.refreshRepository(ctx, item.reference, old)
}

func (s *Service) refreshCatalog(ctx context.Context, source string, old *snapshot) (*snapshot, error) {
	ref, err := name.ParseReference(strings.TrimPrefix(source, "oci://"), name.StrictValidation)
	if err != nil {
		return nil, err
	}
	descriptor, err := s.Registry.Head(ctx, ref)
	if err != nil {
		return nil, err
	}
	digest := descriptor.Digest.String()
	if old != nil && old.Digest == digest {
		return old, nil
	}
	file, actual, err := catalog.Read(ctx, s.Registry, "oci://"+ref.Context().Digest(digest).Name())
	if err != nil {
		return nil, err
	}
	return &snapshot{Digest: actual, Entries: file.Spec.Entries}, nil
}

func (s *Service) refreshRepository(ctx context.Context, source string, old *snapshot) (*snapshot, error) {
	repo, err := name.NewRepository(strings.TrimPrefix(source, "oci://"), name.StrictValidation)
	if err != nil {
		return nil, err
	}
	tags, err := s.Registry.Tags(ctx, repo)
	if err != nil {
		return nil, err
	}
	tags = slices.DeleteFunc(tags, func(tag string) bool {
		_, err := semver.StrictNewVersion(strings.TrimPrefix(tag, "v"))
		return err != nil
	})
	sortTags(tags)
	current := &snapshot{Entries: []catalog.Entry{}, Releases: make(map[string]catalog.Entry)}
	for _, tag := range slices.Compact(tags) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry, err := s.readRelease(ctx, repo.Tag(tag), source, old, current)
		if err != nil {
			return nil, err
		}
		current.Entries, err = mergeEntry(current.Entries, &entry)
		if err != nil {
			return nil, err
		}
	}
	return current, nil
}

func (s *Service) readRelease(ctx context.Context, ref name.Tag, source string, old, current *snapshot) (catalog.Entry, error) {
	descriptor, err := s.Registry.Head(ctx, ref)
	if err != nil {
		return catalog.Entry{}, err
	}
	digest := descriptor.Digest.String()
	entry, found := current.Releases[digest]
	if !found && old != nil {
		entry, found = old.Releases[digest]
	}
	if !found {
		entry, err = s.inspectRelease(ctx, ref.Context().Digest(digest), source)
		if err != nil {
			return catalog.Entry{}, err
		}
	}
	if len(entry.Versions) != 1 || entry.Versions[0].Version != strings.TrimPrefix(ref.TagStr(), "v") || entry.Source != source {
		return catalog.Entry{}, errUtils.ErrOCIIntegrity
	}
	current.Releases[digest] = entry
	return entry, nil
}

func (s *Service) inspectRelease(ctx context.Context, ref name.Digest, source string) (catalog.Entry, error) {
	descriptor, err := s.Registry.Get(ctx, ref)
	if err != nil {
		return catalog.Entry{}, err
	}
	index, err := descriptor.ImageIndex()
	if err != nil {
		return catalog.Entry{}, err
	}
	release, err := pluginoci.Inspect(index, ref.DigestStr())
	if err != nil {
		return catalog.Entry{}, err
	}
	file := catalog.New("discovery")
	if err := catalog.Add(file, source, release, s.Descriptors); err != nil {
		return catalog.Entry{}, err
	}
	return file.Spec.Entries[0], nil
}

func sortTags(tags []string) {
	slices.SortFunc(tags, func(a, b string) int {
		av, ae := semver.StrictNewVersion(strings.TrimPrefix(a, "v"))
		bv, be := semver.StrictNewVersion(strings.TrimPrefix(b, "v"))
		if ae == nil && be == nil {
			if order := av.Compare(bv); order != 0 {
				return order
			}
		}
		return strings.Compare(a, b)
	})
}

func mergeEntry(entries []catalog.Entry, added *catalog.Entry) ([]catalog.Entry, error) {
	for i := range entries {
		existing := &entries[i]
		if existing.Name != added.Name {
			continue
		}
		if existing.Source != added.Source {
			return nil, errUtils.ErrPluginCatalogConflict
		}
		versions := slices.Clone(existing.Versions)
		for _, version := range added.Versions {
			var err error
			versions, err = mergeVersion(versions, &version)
			if err != nil {
				return nil, err
			}
		}
		merged := *added
		merged.Versions = versions
		*existing = merged
		return entries, nil
	}
	return append(entries, *added), nil
}

func mergeVersion(versions []catalog.Version, added *catalog.Version) ([]catalog.Version, error) {
	for i, previous := range versions {
		if previous.Version != added.Version {
			continue
		}
		if previous.Digest != added.Digest {
			return nil, errUtils.ErrPluginImmutableRelease
		}
		if added.Yanked != nil {
			versions[i].Yanked = added.Yanked
		}
		return versions, nil
	}
	return append(versions, *added), nil
}
