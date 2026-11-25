package atmosbridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"
)

// Catalog identifies an immutable checkout and the selected directory within it.
type Catalog struct {
	Root      string
	Directory string
	URI       string
	Commit    string
}

// CatalogRepository preserves the revision query when separating the unit directory.
func CatalogRepository(source string) (string, string) { return getter.SourceDirSubdir(source) }

// FetchCatalog uses Terragrunt's Git getter. Only fully populated snapshots are published.
func FetchCatalog(ctx context.Context, logger log.Logger, environment map[string]string, repository, subdir, commit string) (*Catalog, error) {
	if !ValidCatalogCommit(commit) {
		return nil, errors.New("catalog needs a full Git commit SHA")
	}
	normalized, err := cas.DetectRemoteSource(repository)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimPrefix(normalized, "git::"))
	if err != nil {
		return nil, err
	}
	if (u.Scheme != "https" && u.Scheme != "ssh" && u.Scheme != "http") || u.Host == "" || u.Fragment != "" {
		return nil, errors.New("catalog must use a Git repository URL")
	}
	if _, password := u.User.Password(); password || (u.User != nil && u.Scheme != "ssh") {
		return nil, errors.New("use Git credentials outside the catalog URL")
	}
	for k := range u.Query() {
		if k != "ref" {
			return nil, errors.New("only the ref query parameter is supported for catalogs")
		}
	}
	// A commit written directly in HCL cannot be silently overridden by configuration.
	if ref := u.Query().Get("ref"); len(ref) == 40 && !strings.EqualFold(ref, commit) {
		return nil, errors.New("catalog commit differs from source ref")
	}
	u.RawQuery = ""
	uri := "git::" + u.String()
	cache, err := catalogCacheDir(environment)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(uri+"\x00"+commit)))
	parent := filepath.Join(cache, "atmos", "terragrunt", "catalogs")
	root := filepath.Join(parent, key)
	if subdir == "" {
		return nil, errors.New("catalog source requires an explicit //subdirectory selector (use //. for the repository root)")
	}
	if !filepath.IsLocal(subdir) {
		return nil, errors.New("catalog directory escapes checkout")
	}
	if _, err = os.Stat(root); errors.Is(err, os.ErrNotExist) {
		if err := fetchSnapshot(ctx, logger, environment, uri, commit, root); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, subdir)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("catalog source must select a directory")
	}
	return &Catalog{Root: root, Directory: dir, URI: uri, Commit: commit}, nil
}

// fetchSnapshot stages a Git checkout beside its final path so publication is atomic
// even when separate plugin processes discover the same pinned catalog concurrently.
func fetchSnapshot(ctx context.Context, logger log.Logger, environment map[string]string, uri, commit, root string) error {
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, "fetch-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	checkout := filepath.Join(stage, "checkout")
	env := maps.Clone(environment)
	if env == nil {
		env = map[string]string{}
	}

	env["GIT_TERMINAL_PROMPT"] = "0"
	env["GIT_ALLOW_PROTOCOL"] = "https:http:ssh"
	v := Environment(env, io.Discard, io.Discard)
	if _, err = getter.Get(ctx, logger, v, checkout, uri+"?ref="+commit); err != nil {
		return err
	}
	if err = os.RemoveAll(filepath.Join(checkout, ".git")); err != nil {
		return err
	}
	// Generated units are copied through a virtual filesystem that can follow links.
	// Reject links before publishing a checkout that could read outside its tree.
	err = filepath.WalkDir(checkout, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 || d.Name() == ".git" {
			return errors.New("catalog contains a symlink or nested Git checkout")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err = os.Rename(checkout, root); err != nil {
		if _, statErr := os.Stat(root); statErr != nil {
			return err
		}
	}
	return nil
}

// Plugins start with a minimal process environment; the host supplies user directories
// on the invocation. An explicit XDG cache also makes offline CI runs portable.
func catalogCacheDir(env map[string]string) (string, error) {
	if path := env["XDG_CACHE_HOME"]; filepath.IsAbs(path) {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		if path := env["LocalAppData"]; filepath.IsAbs(path) {
			return path, nil
		}
	}
	if home := env["HOME"]; filepath.IsAbs(home) {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Caches"), nil
		}
		return filepath.Join(home, ".cache"), nil
	}
	return os.UserCacheDir()
}

// ValidCatalogCommit accepts a full immutable Git SHA-1.
func ValidCatalogCommit(commit string) bool {
	_, err := hex.DecodeString(commit)
	return len(commit) == 40 && err == nil
}

// CatalogFiles returns a read-through view of catalog definitions. Only the files whose
// relative references need rebasing are shadowed; generated output writes go to the caller's FS.
type CatalogFiles struct {
	afero.Fs
	overrides map[string][]byte
}

// NewCatalogFiles preserves writes to the upstream stack generation filesystem.
func NewCatalogFiles(base afero.Fs) *CatalogFiles {
	return &CatalogFiles{Fs: base, overrides: make(map[string][]byte)}
}

func (f *CatalogFiles) Open(name string) (afero.File, error) {
	return f.OpenFile(name, os.O_RDONLY, 0)
}

func (f *CatalogFiles) OpenFile(name string, flag int, mode os.FileMode) (afero.File, error) {
	if content, ok := f.overrides[filepath.Clean(name)]; ok && flag == os.O_RDONLY {
		mem := afero.NewMemMapFs()
		if err := afero.WriteFile(mem, name, content, 0o600); err != nil {
			return nil, err
		}
		return mem.Open(name)
	}
	return f.Fs.OpenFile(name, flag, mode)
}

// Include projects catalog-relative source attributes before the generator copies a unit or stack.
func (f *CatalogFiles) Include(c *Catalog, name string) error {
	path := filepath.Join(c.Directory, name)
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if name == "terragrunt.stack.hcl" {
		file, blocks, err := cas.ReadStackBlocks(original)
		if err != nil {
			return err
		}
		changed := false
		for _, block := range blocks {
			if block.UpdateSourceWithCAS {
				source, err := c.source(block.Source)
				if err != nil {
					return err
				}
				block.SetSource(source)
				changed = true
			}
		}
		if changed {
			clearCatalogUpdates(file.Body())
			f.overrides[path] = file.Bytes()
		}
		return nil
	}
	source, update, err := cas.ReadTerraformSourceInfo(original)
	if err != nil || !update {
		return err
	}
	uri, err := c.source(source)
	if err != nil {
		return err
	}
	rewritten, err := cas.RewriteTerraformSource(original, uri)
	if err != nil {
		return err
	}
	file, diags := hclwrite.ParseConfig(rewritten, path, hcl.InitialPos)
	if diags.HasErrors() {
		return diags
	}
	clearCatalogUpdates(file.Body())
	f.overrides[path] = file.Bytes()
	return nil
}

// The source now carries the immutable commit. Upstream needs no further CAS rewrite.
func clearCatalogUpdates(body *hclwrite.Body) {
	for _, block := range body.Blocks() {
		switch block.Type() {
		case "terraform", "unit", "stack":
			if block.Body().GetAttribute("update_source_with_cas") != nil {
				block.Body().SetAttributeValue("update_source_with_cas", cty.False)
			}
		case "autoinclude":
		default:
			continue
		}
		clearCatalogUpdates(block.Body())
	}
}

func (c *Catalog) source(s string) (string, error) {
	target, err := cas.ResolveInRepoSource(c.Root, c.Directory, s)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("catalog source must be a directory")
	}
	rel, err := filepath.Rel(c.Root, target)
	if err != nil {
		return "", err
	}
	return c.URI + "//" + filepath.ToSlash(rel) + "?ref=" + c.Commit, nil
}

// PinnedCatalogURI is the canonical repository spelling used for nested references.
func PinnedCatalogURI(repository, commit string) string {
	normalized, err := cas.DetectRemoteSource(repository)
	if err != nil {
		return ""
	}
	base, _, _ := strings.Cut(normalized, "?")
	return "git::" + strings.TrimPrefix(base, "git::") + "?ref=" + commit
}
