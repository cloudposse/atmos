package atmosbridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/spf13/afero"
)

// ErrUnsupportedOverlay reports a unit file that the Atmos preparation contract
// cannot carry faithfully.
var ErrUnsupportedOverlay = errors.New("unit file cannot be exported through the overlay")

// UnitOverlay applies upstream default copy rules without writing to the unit or module.
// It returns file contents and, for files whose permission bits differ from the
// default, the unit file's mode so executables keep their execute bit and
// owner-only or otherwise unreadable-by-others files, such as copied keys, stay
// private. The host ignores modes on Windows, so the exported modes only take
// effect on platforms with POSIX permission bits.
func UnitOverlay(logger log.Logger, env *Venv, unit string, terraform *config.TerraformConfig) (map[string][]byte, map[string]uint32, error) {
	memory := afero.NewMemMapFs()
	// Upstream joins the copy source with each include and exclude pattern and expands the result as
	// a glob, so a `*` or `{` anywhere in the unit's real path would turn part of the path into a
	// pattern and widen the walk to its parent. The copy runs on a view of the unit rooted at a
	// fixed path that holds no glob characters, which bounds every read to the unit's own tree.
	view := newUnitView(unit)
	fsys := afero.NewCopyOnWriteFs(afero.NewReadOnlyFs(view.mount(env.FS)), memory)
	unit, destination := view.source, view.destination
	include := []string{".tflint.hcl"}
	var exclude []string
	if terraform.IncludeInCopy != nil {
		include = append(include, (*terraform.IncludeInCopy)...)
	}
	if terraform.ExcludeFromCopy != nil {
		exclude = append(exclude, (*terraform.ExcludeFromCopy)...)
	}
	if err := util.CopyFolderContents(logger, fsys, unit, destination, ".terragrunt-module-manifest",
		util.WithIncludeInCopy(include...), util.WithExcludeFromCopy(exclude...)); err != nil {
		return nil, nil, err
	}
	files := map[string][]byte{}
	modes := map[string]uint32{}
	err := afero.Walk(memory, destination, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name, err := filepath.Rel(destination, path)
		if err != nil {
			return err
		}
		if name == ".terragrunt-module-manifest" {
			return nil
		}
		// The lock file beside the unit is handed to the host as a unit file copy with its own
		// ownership rules, so the overlay leaves it out rather than rejecting it.
		if filepath.ToSlash(name) == util.TerraformLockFile {
			return nil
		}
		if err := checkOverlayFile(name); err != nil {
			return err
		}
		content, err := afero.ReadFile(memory, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(name)] = content
		// The directories are read through the unit view, because unit is the view's fixed source path
		// and does not exist in the host filesystem.
		perm := restrictByDirectories(fsys, unit, filepath.Dir(name), info.Mode().Perm())
		if exportsMode(perm) {
			modes[filepath.ToSlash(name)] = uint32(perm)
		}
		return nil
	})
	return files, modes, err
}

// unitView maps a fixed, glob-safe source path onto the unit's real directory.
type unitView struct {
	unit, source, destination string
}

// newUnitView places the view's source and destination side by side at the root of the unit's volume.
func newUnitView(unit string) unitView {
	root := filepath.VolumeName(unit) + string(filepath.Separator)
	return unitView{
		unit:        filepath.Clean(unit),
		source:      filepath.Join(root, "atmos-unit-source"),
		destination: filepath.Join(root, "atmos-unit-overlay"),
	}
}

// mount returns base with the view's source path served from the unit directory.
func (v unitView) mount(base afero.Fs) afero.Fs {
	return rebaseFs{Fs: base, view: v}
}

// rebaseFs serves paths under the view's source from the unit directory and reports the view's
// destination as absent, because that tree exists only in the in-memory layer above it. Every other
// path passes through, so a symlink target outside the unit still reads normally. The overlay copy
// only reads through it, behind a read-only layer.
type rebaseFs struct {
	afero.Fs
	view unitView
}

// resolve returns the real path to read for name.
func (r rebaseFs) resolve(op, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if under(r.view.destination, clean) {
		return "", &os.PathError{Op: op, Path: name, Err: os.ErrNotExist}
	}
	if rel, err := filepath.Rel(r.view.source, clean); err == nil && filepath.IsLocal(rel) {
		return filepath.Join(r.view.unit, rel), nil
	}
	return name, nil
}

// under reports whether path is root or lies below it.
func under(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(rel)
}

func (r rebaseFs) Open(name string) (afero.File, error) {
	target, err := r.resolve("open", name)
	if err != nil {
		return nil, err
	}
	return r.Fs.Open(target)
}

func (r rebaseFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	target, err := r.resolve("open", name)
	if err != nil {
		return nil, err
	}
	return r.Fs.OpenFile(target, flag, perm)
}

func (r rebaseFs) Stat(name string) (os.FileInfo, error) {
	target, err := r.resolve("stat", name)
	if err != nil {
		return nil, err
	}
	return r.Fs.Stat(target)
}

// LstatIfPossible lets the copy tell a symlink from the file it points to.
func (r rebaseFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	target, err := r.resolve("lstat", name)
	if err != nil {
		return nil, false, err
	}
	if lstater, ok := r.Fs.(afero.Lstater); ok {
		return lstater.LstatIfPossible(target)
	}
	info, err := r.Fs.Stat(target)
	return info, false, err
}

// ReadlinkIfPossible reports a link's target, which stays a real path.
func (r rebaseFs) ReadlinkIfPossible(name string) (string, error) {
	target, err := r.resolve("readlink", name)
	if err != nil {
		return "", err
	}
	if reader, ok := r.Fs.(afero.LinkReader); ok {
		return reader.ReadlinkIfPossible(target)
	}
	return "", &os.PathError{Op: "readlink", Path: target, Err: afero.ErrNoReadlink}
}

// restrictByDirectories conservatively narrows a file's permission bits by the
// subdirectories that contain it. The overlay carries file modes only, so a
// readable file inside a private directory (for example 0644 under 0700) would
// otherwise become readable once the directory is recreated with default modes.
// A group or other class that cannot traverse a containing directory loses all
// access to the file. The unit directory itself is not considered because the
// host owns the working directory's mode.
func restrictByDirectories(fsys afero.Fs, unit, dir string, perm os.FileMode) os.FileMode {
	for dir != "." && dir != string(filepath.Separator) && dir != "" {
		if info, err := fsys.Stat(filepath.Join(unit, dir)); err == nil {
			mode := info.Mode().Perm()
			if mode&0o010 == 0 {
				perm &^= 0o070
			}
			if mode&0o001 == 0 {
				perm &^= 0o007
			}
		}
		dir = filepath.Dir(dir)
	}
	return perm
}

// exportsMode reports whether a unit file's permission bits differ from the
// default the host applies to files without a mode (0644): it is executable, or
// group or other lacks read access.
func exportsMode(perm os.FileMode) bool {
	const groupAndOtherRead = 0o044
	return perm&0o111 != 0 || perm&groupAndOtherRead != groupAndOtherRead
}

// checkOverlayFile rejects a committed provider lock file. Atmos manages the
// lock per component instance and the host refuses it as a runtime artifact, so
// failing here reports the unit file by name instead of failing later.
func checkOverlayFile(name string) error {
	if filepath.Base(name) == util.TerraformLockFile {
		return fmt.Errorf("%w: %s is a dependency lock file, which Atmos manages per component instance; list it in exclude_from_copy to proceed without the committed pins", ErrUnsupportedOverlay, filepath.ToSlash(name))
	}
	return nil
}

// UnitFileIncluded applies the upstream copy exclusions to an implicit file copy.
// Mount at a stable path so metacharacters in the unit directory remain literal.
//
//nolint:lintroller // Standalone bridge module cannot import the host performance tracker.
func UnitFileIncluded(logger log.Logger, unit, name string, excluded []string) (bool, error) {
	view := newUnitView(unit)
	filter, err := util.NewCopyFilter(logger, view.mount(afero.NewOsFs()), view.source,
		util.WithIncludeInCopy(name), util.WithExcludeFromCopy(excluded...))
	if err != nil {
		return false, err
	}
	return filter(filepath.Join(view.source, name), false), nil
}
