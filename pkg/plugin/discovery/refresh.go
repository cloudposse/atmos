package discovery

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"syscall"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

const maximumServerErrorStatus = 599

// Winsock uses distinct errno values from syscall's portable Unix constants.
const (
	windowsNetworkUnreachable syscall.Errno = 10051
	windowsConnectionTimeout  syscall.Errno = 10060
	windowsConnectionRefused  syscall.Errno = 10061
	windowsHostUnreachable    syscall.Errno = 10065
)

func (s *Service) refresh(ctx context.Context, item source, old *snapshot) (*snapshot, error) {
	if item.kind == "catalog" {
		return s.refreshCatalog(ctx, item, old)
	}
	return s.refreshRepository(ctx, item.reference, old)
}

func (s *Service) refreshCatalog(ctx context.Context, item source, old *snapshot) (*snapshot, error) {
	ref, err := name.ParseReference(strings.TrimPrefix(item.reference, "oci://"), name.StrictValidation)
	if err != nil {
		return nil, err
	}
	descriptor, err := s.Registry.Head(ctx, ref)
	if err != nil {
		// Only an unavailable implicit catalog is optional. After HEAD succeeds,
		// missing content or invalid metadata must still fail closed.
		if item.optional && ctx.Err() == nil && unavailableCatalog(err) {
			return nil, nil
		}
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

// unavailableCatalog excludes authentication, certificate, policy, and integrity errors.
func unavailableCatalog(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var response *transport.Error
	if errors.As(err, &response) {
		return response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusRequestTimeout ||
			response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError && response.StatusCode <= maximumServerErrorStatus
	}
	return unavailableConnection(err)
}

func unavailableConnection(err error) bool {
	var connection *net.OpError
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return true
	}
	if !errors.As(err, &connection) || connection.Op != "dial" {
		return false
	}
	// A dial wrapper alone is not proof of unavailability: TLS verification
	// failures can carry it too. Only known connection failures may be skipped.
	if errors.Is(connection.Err, os.ErrDeadlineExceeded) {
		return true
	}
	var errno syscall.Errno
	if !errors.As(connection.Err, &errno) {
		return false
	}
	switch errno {
	case syscall.ECONNREFUSED, syscall.ENETUNREACH, syscall.EHOSTUNREACH, syscall.ETIMEDOUT,
		windowsConnectionRefused, windowsNetworkUnreachable, windowsHostUnreachable, windowsConnectionTimeout:
		return true
	default:
		return false
	}
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
