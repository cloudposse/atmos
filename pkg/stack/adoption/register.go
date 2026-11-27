package adoption

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	configFileMode = 0o644
	configDirMode  = 0o755
	lockDirMode    = 0o700
)

// StarterConfig locates standalone stacks and preserves native Terraform backends in new projects.
const StarterConfig = `base_path: .
components:
  terraform:
    base_path: .
    auto_generate_backend_file: false
stacks:
  base_path: stacks
  included_paths: ["*.yaml"]
`

var stackNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Registration describes one component addition to a standalone Atmos stack.
type Registration struct {
	AtlantisProject               string
	Adapters                      Adapters
	Project, Dir, Stack, Explicit string
	Name, ReleaseName, Namespace  string
	SourcePath, Workspace         string // SourcePath retains the original CLI input when a wrapper delegates.
	ShareState, ReplaceVarFiles   bool
	OnPreview                     func(Registration)
	reviewed                      map[string]reviewedFile
	VarFiles, Values              []string
	Config                        *schema.AtmosConfiguration // Nil for an unconfigured project.
	Defaults                      *schema.AtmosConfiguration // CLI defaults, including plugin policy in a fresh project.
	Interactive, DryRun           bool
	Output                        io.Writer
}

// Save previews or atomically updates the selected manifest under a project lock.
func (r *Registration) Save(ctx context.Context) error {
	defer perf.Track(r.Config, "stackadoption.Save")()

	if !stackNamePattern.MatchString(r.Stack) {
		return fmt.Errorf("%w: invalid stack name %q", errUtils.ErrInvalidStackConfig, r.Stack)
	}
	// Use one lock identity even when the checkout is reached through a symlink.
	project, err := filepath.EvalSymlinks(r.Project)
	if err != nil {
		return err
	}
	request := *r
	request.Project = project
	if r.DryRun {
		if r.OnPreview != nil {
			request.reviewed = make(map[string]reviewedFile)
		}
		return request.adoptReviewed(ctx)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	locks := filepath.Join(cache, "atmos", "locks")
	if err := os.MkdirAll(locks, lockDirMode); err != nil {
		return err
	}
	lock := filelock.New(filepath.Join(locks, fmt.Sprintf("stack-add-%x.lock", sha256.Sum256([]byte(project)))))
	return lock.WithExclusive(ctx, func() error { return request.adoptReviewed(ctx) })
}

// StandaloneStacks reports whether a filename identifies one independently writable stack.
func StandaloneStacks(stacks *schema.Stacks) bool {
	defer perf.Track(nil, "adoption.StandaloneStacks")()

	return stacks.NameTemplate == "" && stacks.NamePattern == "" && slices.Equal(stacks.IncludedPaths, []string{"*.yaml"}) && len(stacks.ExcludedPaths) == 0
}

func printChange(out io.Writer, file string, before, after []byte) {
	edits := myers.ComputeEdits(span.URIFromPath(file), string(before), string(after))
	if len(edits) > 0 {
		fmt.Fprint(out, gotextdiff.ToUnified("a/"+file, "b/"+file, string(before), edits))
	}
}

func writeStarter(project string) error {
	fs := filesystem.NewOSFileSystem()

	configFile := filepath.Join(project, "atmos.yaml")
	current, _, err := ReadRegular(configFile)
	if err != nil {
		return err
	}
	if current != nil && string(current) != StarterConfig {
		return fmt.Errorf("%w: configuration already exists at %s; reload and retry", errUtils.ErrInvalidStackConfig, configFile)
	}
	if current == nil {
		if err := fs.WriteFileAtomic(configFile, []byte(StarterConfig), configFileMode); err != nil {
			return err
		}
	}
	return nil
}

// StandaloneDestination resolves a named standalone manifest or its new destination.
func StandaloneDestination(dir, stackName string) (string, error) {
	defer perf.Track(nil, "adoption.StandaloneDestination")()

	target := filepath.Join(dir, stackName+".yaml")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return target, nil
	}
	if err != nil {
		return "", err
	}
	var match string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		name, err := manifestName(path, entry.Name())
		if err != nil {
			return "", err
		}
		if name == stackName {
			if match != "" {
				return "", fmt.Errorf("%w: stack %s is declared in both %s and %s", errUtils.ErrInvalidStackConfig, stackName, match, path)
			}
			match = path
		}
	}
	if match != "" {
		return match, nil
	}
	return target, nil
}

// ReadRegular reads an existing regular file. A missing destination returns nil data and the default creation mode.
func ReadRegular(path string) ([]byte, os.FileMode, error) {
	defer perf.Track(nil, "adoption.ReadRegular")()

	stat, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, configFileMode, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if !stat.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("%w: expected a regular file: %s", errUtils.ErrInvalidStackConfig, path)
	}
	data, err := os.ReadFile(path)
	return data, stat.Mode().Perm(), err
}

// WriteManifest previews or persists a validated standalone manifest with its original permissions.
func (r *Registration) WriteManifest(file string, original, data []byte, mode os.FileMode) error {
	defer perf.Track(nil, "adoption.Registration.WriteManifest")()

	if err := r.reviewFile(file, original); err != nil {
		return err
	}
	stacksDir := filepath.Dir(file)
	if r.DryRun {
		fmt.Fprintln(r.Output, "Preview only.")
		if r.Config == nil {
			printChange(r.Output, "atmos.yaml", nil, []byte(StarterConfig))
		}
		path, err := filepath.Rel(r.Project, file)
		if err != nil {
			return err
		}
		printChange(r.Output, filepath.ToSlash(path), original, data)
		return nil
	}
	if r.Config == nil {
		if err := writeStarter(r.Project); err != nil {
			return err
		}
	}
	if bytes.Equal(data, original) {
		fmt.Fprintln(r.Output, "Already configured.")
		return nil
	}
	if err := os.MkdirAll(stacksDir, configDirMode); err != nil {
		return err
	}
	return filesystem.NewOSFileSystem().WriteFileAtomic(file, data, mode)
}

func manifestName(path, filename string) (string, error) {
	data, _, err := ReadRegular(path)
	if err != nil {
		return "", err
	}
	var header struct{ Name string }
	if err := yaml.Unmarshal(data, &header); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	name := header.Name
	if name == "" {
		name = strings.TrimSuffix(filename, ".yaml")
	}
	return name, nil
}

// reviewFile compares the bytes read under lock with the baselines shown in the interactive preview.
func (r *Registration) reviewFile(path string, before []byte) error {
	if r.reviewed == nil {
		return nil
	}
	version := reviewedFile{digest: sha256.Sum256(before), exists: before != nil}
	if r.DryRun {
		r.reviewed[path] = version
		return nil
	}
	if prior, ok := r.reviewed[path]; !ok || prior != version {
		return fmt.Errorf("%w: %s changed after preview; review the registration again", errUtils.ErrInvalidStackConfig, path)
	}
	return nil
}

type reviewedFile struct {
	digest [sha256.Size]byte
	exists bool
}

func (r *Registration) adoptReviewed(ctx context.Context) error {
	if r.reviewed != nil {
		path := filepath.Join(r.Project, "atmos.yaml")
		before, _, err := ReadRegular(path)
		if err != nil {
			return err
		}
		if err := r.reviewFile(path, before); err != nil {
			return err
		}
	}
	return r.adopt(ctx)
}
