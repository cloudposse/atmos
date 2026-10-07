package target

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PublishFile is a repeatable, streaming source with a portable destination name.
type PublishFile struct {
	Name string
	Size int64
	Open func() (io.ReadSeekCloser, error)
}

// PublishInput carries files and an independently resolved destination identity.
type PublishInput struct {
	AtmosConfig  *schema.AtmosConfiguration
	TargetConfig map[string]any
	Files        []PublishFile
	AuthContext  *schema.AuthContext
	EnvProvider  IdentityEnvironmentProvider
	Env          map[string]string
	Metadata     ArtifactMetadata
}

// PublishResult reports all published locations, including unchanged files.
type PublishResult struct {
	Locations []string
	Changed   int
	Unchanged int
	Metadata  map[string]any
}

// FilePublisher is an optional target capability; deployment is never a fallback.
type FilePublisher interface {
	ValidatePublish(*PublishInput) error
	Publish(context.Context, *PublishInput) (*PublishResult, error)
}

// Publisher finds a target's file-publishing capability without invoking delivery.
func Publisher(kind string) (FilePublisher, error) {
	defer perf.Track(nil, "target.Publisher")()
	p, ok := Get(kind)
	if !ok {
		return nil, fmt.Errorf("%w: %q", errUtils.ErrPublishTarget, kind)
	}
	publisher, ok := p.(FilePublisher)
	if !ok {
		return nil, fmt.Errorf("%w: %q does not publish files", errUtils.ErrPublishTarget, kind)
	}
	return publisher, nil
}

// ValidatePublishPath rejects paths that are not portable relative file names.
func ValidatePublishPath(name string) error {
	defer perf.Track(nil, "target.ValidatePublishPath")()
	if name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
		return fmt.Errorf("%w: invalid relative path %q", errUtils.ErrPublishSource, name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return fmt.Errorf("%w: unsafe path %q", errUtils.ErrPublishSource, name)
		}
	}
	return nil
}

// LocalPublishFiles validates and inventories files without reading them into memory.
func LocalPublishFiles(source, destination string) ([]PublishFile, error) {
	defer perf.Track(nil, "target.LocalPublishFiles")()
	if destination != "" {
		if err := ValidatePublishPath(destination); err != nil {
			return nil, err
		}
	}
	info, err := os.Lstat(source)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPublishSource, err)
	}
	if info.Mode().IsRegular() {
		name := destination
		if name == "" || strings.HasSuffix(name, "/") {
			name += filepath.Base(source)
		}
		file, err := localPublishFile(source, name, info)
		if err != nil {
			return nil, err
		}
		return []PublishFile{file}, nil
	}
	if !info.IsDir() {
		return nil, errUtils.ErrPublishSource
	}
	return localPublishDirectory(source, destination)
}

func localPublishDirectory(source, destination string) ([]PublishFile, error) {
	var files []PublishFile
	err := filepath.WalkDir(source, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		stat, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, filename)
		if err != nil {
			return err
		}
		file, err := localPublishFile(filename, filepath.ToSlash(filepath.Join(destination, relative)), stat)
		if err != nil {
			return err
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPublishSource, err)
	}
	return files, nil
}

func localPublishFile(filename, name string, info fs.FileInfo) (PublishFile, error) {
	if !info.Mode().IsRegular() {
		return PublishFile{}, fmt.Errorf("%w: %s is not a regular file", errUtils.ErrPublishSource, filename)
	}
	if err := ValidatePublishPath(name); err != nil {
		return PublishFile{}, err
	}
	return PublishFile{Name: name, Size: info.Size(), Open: func() (io.ReadSeekCloser, error) {
		current, err := os.Lstat(filename)
		if err != nil {
			return nil, err
		}
		if !current.Mode().IsRegular() || !os.SameFile(info, current) || current.Size() != info.Size() {
			return nil, errUtils.ErrPublishSource
		}
		return os.Open(filename)
	}}, nil
}
