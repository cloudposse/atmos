//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
)

// store is the content-addressed graph store of one OCI repository or layout.
type store = atmosoci.GraphStore

// backend is what differs between the registry and the layout kinds. The shared core in
// repository.go does everything else.
type backend interface {
	// kind returns the repository kind.
	kind() string

	// locator returns the human-readable location of a name: registry/repository or layout path.
	locator(name string) string

	// open returns the content store holding a name. write reports that the caller will write; a
	// read of an absent layout returns a not-found error instead of creating it.
	open(ctx context.Context, name string, write bool) (store, error)

	// reference returns the store reference that names a version of a name.
	reference(name, version string) string

	// versions lists the versions (tags) published for a name, unsorted.
	versions(ctx context.Context, name string) ([]string, error)

	// names enumerates the artifact names of the collection, or returns an unsupported-capability
	// error when the backend cannot enumerate.
	names(ctx context.Context) ([]string, error)

	// deleteScope describes what deleting a reference removes in this backend.
	deleteScope() artifact.DeleteSemantics

	// remove deletes a reference. For a registry it deletes the manifest by digest, which removes
	// every tag of it; for a layout it removes only the named reference.
	remove(ctx context.Context, name, version string, root ocispec.Descriptor) error

	// enumeratesCollection reports whether collection enumeration is available.
	enumeratesCollection() bool
}

// registryBackend maps names onto repositories of one registry below a collection prefix.
type registryBackend struct {
	registry  string
	prefix    string
	transport atmosoci.Transport
	noCatalog bool
	mu        sync.Mutex
	stores    map[string]atmosoci.RepositoryStore
}

var _ backend = (*registryBackend)(nil)

func newRegistryBackend(registry, prefix string, transport atmosoci.Transport, noCatalog bool) *registryBackend {
	return &registryBackend{registry: registry, prefix: prefix, transport: transport, noCatalog: noCatalog, stores: map[string]atmosoci.RepositoryStore{}}
}

func (b *registryBackend) kind() string { return artifact.RepositoryKindOCIRegistry }

// repositoryPath maps a name onto the OCI repository path below the collection prefix.
func (b *registryBackend) repositoryPath(name string) string {
	if b.prefix == "" {
		return name
	}
	return b.prefix + "/" + name
}

func (b *registryBackend) locator(name string) string {
	return b.registry + "/" + b.repositoryPath(name)
}

func (b *registryBackend) open(ctx context.Context, name string, _ bool) (store, error) {
	return b.repository(ctx, name)
}

// repository opens (once per name) the registry repository. Credentials are resolved by the
// transport here, not at construction, so describing configuration never authenticates.
func (b *registryBackend) repository(ctx context.Context, name string) (atmosoci.RepositoryStore, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if opened, ok := b.stores[name]; ok {
		return opened, nil
	}
	opened, err := b.transport.Repository(ctx, atmosoci.Repository{Registry: b.registry, Path: b.repositoryPath(name)})
	if err != nil {
		return nil, fmt.Errorf("%w: open %s: %w", errUtils.ErrArtifactDownloadFailed, b.locator(name), err)
	}
	b.stores[name] = opened
	return opened, nil
}

func (b *registryBackend) reference(_, version string) string { return version }

func (b *registryBackend) versions(ctx context.Context, name string) ([]string, error) {
	repository, err := b.repository(ctx, name)
	if err != nil {
		return nil, err
	}
	tags, err := repository.Tags(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: list tags of %s: %w", errUtils.ErrArtifactListFailed, b.locator(name), err)
	}
	return tags, nil
}

func (b *registryBackend) deleteScope() artifact.DeleteSemantics { return artifact.DeleteManifest }

func (b *registryBackend) remove(ctx context.Context, name, _ string, root ocispec.Descriptor) error {
	repository, err := b.repository(ctx, name)
	if err != nil {
		return err
	}
	if err := repository.Delete(ctx, root); err != nil {
		return fmt.Errorf("%w: delete manifest %s: %w", errUtils.ErrArtifactDeleteFailed, root.Digest, err)
	}
	return nil
}

func (b *registryBackend) enumeratesCollection() bool {
	if b.noCatalog {
		return false
	}
	_, ok := b.transport.(atmosoci.RegistryCatalog)
	return ok
}

// names enumerates the registry catalog, scoped to the collection prefix. A registry that does
// not provide a catalog is reported as an unsupported capability, never as an empty collection.
func (b *registryBackend) names(ctx context.Context) ([]string, error) {
	catalog, ok := b.transport.(atmosoci.RegistryCatalog)
	if !ok || b.noCatalog {
		return nil, b.unsupportedEnumeration("this repository is configured without registry enumeration")
	}
	repositories, err := catalog.Repositories(ctx, b.registry)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, b.unsupportedEnumeration(fmt.Sprintf("the registry did not provide a repository catalog (%v)", err))
	}
	scope := ""
	if b.prefix != "" {
		scope = b.prefix + "/"
	}
	var names []string
	for _, repository := range repositories {
		name, found := strings.CutPrefix(repository, scope)
		if !found || name == "" || artifact.ValidateName(name) != nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (b *registryBackend) unsupportedEnumeration(reason string) error {
	return artifact.NewUnsupportedCapability(b.registry+"/"+b.prefix, "list", "enumerate/collection",
		reason+"; listing the versions of a named artifact and every other direct named operation still works without a catalog, or configure an artifact catalog for discovery")
}

// layoutBackend keeps every name in one shared OCI layout; a name and version are one named root.
type layoutBackend struct {
	root string
}

var _ backend = (*layoutBackend)(nil)

func newLayoutBackend(root string) *layoutBackend { return &layoutBackend{root: root} }

func (b *layoutBackend) kind() string { return artifact.RepositoryKindOCILayout }

func (b *layoutBackend) locator(string) string { return b.root }

func (b *layoutBackend) reference(name, version string) string { return name + ":" + version }

func (b *layoutBackend) deleteScope() artifact.DeleteSemantics { return artifact.DeleteReference }

func (b *layoutBackend) enumeratesCollection() bool { return true }

// layoutPathLocks serializes in-process writers to one layout; cross-process writers are excluded
// by the host guard the service takes.
var layoutPathLocks sync.Map

func lockLayout(root string) func() {
	value, _ := layoutPathLocks.LoadOrStore(root, &sync.Mutex{})
	mu, _ := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// exists reports whether the layout has been created.
func (b *layoutBackend) exists() bool {
	info, err := os.Stat(filepath.Join(b.root, ocispec.ImageIndexFile))
	return err == nil && info.Mode().IsRegular()
}

// open returns a fresh handle on the layout so a long-lived Repository never serves a stale
// reference table. A read of an absent layout is not found; only a write creates it.
func (b *layoutBackend) open(ctx context.Context, _ string, write bool) (store, error) {
	return b.layout(ctx, write)
}

func (b *layoutBackend) layout(ctx context.Context, write bool) (*atmosoci.Layout, error) {
	if !write && !b.exists() {
		return nil, fmt.Errorf("%w: the OCI layout %q does not exist", errUtils.ErrArtifactNotFound, b.root)
	}
	opened, err := atmosoci.OpenLayout(ctx, b.root)
	if err != nil {
		return nil, fmt.Errorf("%w: open layout %q: %w", errUtils.ErrArtifactDownloadFailed, b.root, err)
	}
	return opened, nil
}

// references returns every named root as name and version, skipping references that are not
// artifact references (for example a layout shared with other tools).
func (b *layoutBackend) references(ctx context.Context) ([]artifact.Ref, error) {
	if !b.exists() {
		return nil, nil
	}
	opened, err := b.layout(ctx, false)
	if err != nil {
		return nil, err
	}
	var refs []artifact.Ref
	for _, reference := range opened.References() {
		index := strings.LastIndex(reference, ":")
		if index <= 0 {
			continue
		}
		ref := artifact.Ref{Name: reference[:index], Version: reference[index+1:]}
		if ref.Validate() != nil {
			continue
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (b *layoutBackend) versions(ctx context.Context, name string) ([]string, error) {
	refs, err := b.references(ctx)
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, ref := range refs {
		if ref.Name == name {
			versions = append(versions, ref.Version)
		}
	}
	return versions, nil
}

func (b *layoutBackend) names(ctx context.Context) ([]string, error) {
	refs, err := b.references(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var names []string
	for _, ref := range refs {
		if !seen[ref.Name] {
			seen[ref.Name] = true
			names = append(names, ref.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// remove untags one reference. Blobs stay: they may be shared with other references. The caller
// holds the in-process layout lock.
func (b *layoutBackend) remove(ctx context.Context, name, version string, _ ocispec.Descriptor) error {
	opened, err := b.layout(ctx, true)
	if err != nil {
		return err
	}
	if err := opened.Untag(ctx, b.reference(name, version)); err != nil {
		return fmt.Errorf("%w: remove reference %s:%s: %w", errUtils.ErrArtifactDeleteFailed, name, version, err)
	}
	return nil
}

// isNotFound reports whether a store error means the content or repository is absent.
func isNotFound(err error) bool {
	return err != nil && (atmosoci.IsNotFound(err) || errors.Is(err, os.ErrNotExist))
}

// locatorDigest returns "locator@digest" for a root.
func locatorDigest(locator string, root ocispec.Descriptor) string {
	return locator + "@" + root.Digest.String()
}

// layoutLocator returns the representation locator of a layout reference.
func layoutLocator(root string, ref artifact.Ref) string {
	return path.Clean(filepath.ToSlash(root)) + "#" + ref.Name + ":" + ref.Version
}
