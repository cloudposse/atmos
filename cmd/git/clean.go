package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// cleanParser handles flag parsing for `atmos git clean`.
var cleanParser = newCleanParser()

const (
	envAtmosXDGCacheHome = "ATMOS_XDG_CACHE_HOME"
	envXDGCacheHome      = "XDG_CACHE_HOME"
)

// cleanCmd is the `atmos git clean` subcommand.
var cleanCmd = &cobra.Command{
	Use:   "clean [name]",
	Short: "Remove managed Git repository workdirs",
	Long: `Remove workdirs for repositories configured under git.repositories.

Named repositories without an explicit workdir are cleaned from the automatic
Atmos XDG cache location. The command only accepts configured repository names,
not arbitrary filesystem paths. Use --dry-run to preview the resolved path and
--force to delete dirty workdirs.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "git.clean.RunE")()

		v := viper.GetViper()
		if err := cleanParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &cleanOptions{
			All:    v.GetBool(flagAll),
			Force:  v.GetBool(flagForce),
			DryRun: v.GetBool(flagDryRun),
		}
		return runClean(cmd.Context(), opts, args)
	},
}

type cleanOptions struct {
	All    bool
	Force  bool
	DryRun bool
}

func runClean(ctx context.Context, opts *cleanOptions, args []string) error {
	defer perf.Track(nil, "git.runClean")()

	if opts.All && len(args) > 0 {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHint("--all is mutually exclusive with a positional repository name.").
			WithExitCode(2).
			Err()
	}

	if !opts.All && len(args) == 0 {
		name, ok := singleConfiguredRepositoryName(gitConfig())
		if !ok {
			return errUtils.Build(errUtils.ErrGitRepositoryRequired).
				WithHint("Provide a repository name, or use --all to clean all configured repository workdirs.").
				WithExitCode(2).
				Err()
		}
		args = []string{name}
	}

	if opts.All {
		return runCleanAll(ctx, opts)
	}

	return runCleanOne(args[0], opts)
}

func runCleanOne(name string, opts *cleanOptions) error {
	defer perf.Track(nil, "git.runCleanOne")()

	cfg := gitConfig()
	if classifyArg(name, cfg) != argKindName {
		return wrapRepoNotFound(errUtils.ErrGitRepositoryNotFound, name)
	}

	repo := cfg.Repositories[name]
	workdir := repo.Workdir
	explicitWorkdir := workdir != ""
	if !explicitWorkdir {
		workdir = atmosgit.DefaultWorkdirPath(name)
	}

	return cleanGitWorkdir(name, repo.URI, workdir, explicitWorkdir, opts)
}

func runCleanAll(ctx context.Context, opts *cleanOptions) error {
	defer perf.Track(nil, "git.runCleanAll")()

	cfg := gitConfig()
	if cfg == nil || len(cfg.Repositories) == 0 {
		ui.Info("No repositories configured under git.repositories.")
		return nil
	}

	names := atmosgit.ConfiguredRepositoryNames(cfg)
	return runConcurrent(ctx, names, func(_ context.Context, name string) error {
		return runCleanOne(name, opts)
	})
}

func cleanGitWorkdir(name, uri, workdir string, explicitWorkdir bool, opts *cleanOptions) error {
	defer perf.Track(nil, "git.cleanGitWorkdir")()

	abs, err := resolveCleanWorkdirPath(name, workdir, explicitWorkdir)
	if err != nil {
		return err
	}

	exists, err := validateCleanableGitWorkdir(name, abs, uri)
	if err != nil || !exists {
		return err
	}

	if opts.DryRun {
		ui.Infof("[dry-run] Would clean git workdir for %s at %s.", name, abs)
		return nil
	}

	if !opts.Force {
		dirty, err := isGitWorkdirDirty(abs)
		if err != nil {
			return err
		}
		if dirty {
			return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
				WithHint("Workdir has uncommitted changes. Pass --force to delete it anyway.").
				WithExitCode(2).
				Err()
		}
	}

	return removeCleanWorkdir(name, abs, explicitWorkdir, opts.All)
}

func resolveCleanWorkdirPath(name, workdir string, explicitWorkdir bool) (string, error) {
	if workdir == "" {
		return "", errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Repository %q resolved to an empty workdir.", name).
			WithExitCode(2).
			Err()
	}

	abs, err := filepath.Abs(workdir)
	if err != nil {
		return "", fmt.Errorf("resolving git workdir %q: %w", workdir, err)
	}
	if err := validateCleanWorkdirPath(abs); err != nil {
		return "", err
	}
	if !explicitWorkdir {
		if err := validateAutomaticCleanWorkdirPath(abs); err != nil {
			return "", err
		}
	}

	return abs, nil
}

func validateCleanableGitWorkdir(name, abs, uri string) (bool, error) {
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		ui.Infof("No git workdir found for %s at %s.", name, abs)
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking git workdir %q: %w", abs, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("%w: %s", errUtils.ErrRefuseDeleteSymbolicLink, abs)
	}
	if !info.IsDir() {
		return false, errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Refusing to clean %q because it is not a directory.", abs).
			WithExitCode(2).
			Err()
	}

	if err := validateManagedGitWorkdir(abs, uri); err != nil {
		return false, err
	}

	return true, nil
}

func removeCleanWorkdir(name, abs string, explicitWorkdir, skipSpinner bool) error {
	remove := func() error {
		if err := os.RemoveAll(abs); err != nil {
			return fmt.Errorf("%w: failed to clean git workdir %q: %w", errUtils.ErrFileOperation, abs, err)
		}
		return nil
	}

	message := cleanWorkdirSuccessMessage(name, abs, explicitWorkdir)
	if !skipSpinner {
		return spinner.ExecWithSpinner(fmt.Sprintf("Cleaning %s", name), message, remove)
	}
	if err := remove(); err != nil {
		return err
	}
	ui.Success(message)
	return nil
}

func cleanWorkdirSuccessMessage(name, abs string, explicitWorkdir bool) string {
	if explicitWorkdir {
		return fmt.Sprintf("Cleaned configured git workdir for %s: %s", name, abs)
	}
	return fmt.Sprintf("Cleaned XDG git workdir for %s: %s", name, abs)
}

func validateCleanWorkdirPath(path string) error {
	cleaned := filepath.Clean(path)
	if cleaned == "" || cleaned == "." || cleaned == string(os.PathSeparator) || isVolumeRoot(cleaned) {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Refusing to delete dangerous path %q.", path).
			WithExitCode(2).
			Err()
	}
	if pathContainsCurrentProject(cleaned) {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Refusing to delete %q because it is the current project directory or one of its parents.", path).
			WithHint("Set git.repositories.<name>.workdir to a dedicated managed clone directory.").
			WithExitCode(2).
			Err()
	}
	return nil
}

func validateAutomaticCleanWorkdirPath(path string) error {
	base, source := gitCacheHomeEnv()
	if base == "" {
		return nil
	}
	if !filepath.IsAbs(base) {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Refusing to clean automatic git workdir because %s must be an absolute path, got %q.", source, base).
			WithExitCode(2).
			Err()
	}

	absBase, err := filepath.Abs(base)
	if err != nil {
		return fmt.Errorf("resolving %s %q: %w", source, base, err)
	}
	absBase = filepath.Clean(absBase)
	if absBase == string(os.PathSeparator) || isVolumeRoot(absBase) || pathContainsCurrentProject(absBase) {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Refusing to clean automatic git workdir because %s points to a dangerous cache root %q.", source, base).
			WithHint("Set ATMOS_XDG_CACHE_HOME or XDG_CACHE_HOME to a dedicated cache directory.").
			WithExitCode(2).
			Err()
	}

	rel, err := filepath.Rel(absBase, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Refusing to clean automatic git workdir %q because it is outside %s %q.", path, source, base).
			WithExitCode(2).
			Err()
	}

	return nil
}

func gitCacheHomeEnv() (string, string) {
	v := viper.New()
	_ = v.BindEnv(envAtmosXDGCacheHome, envAtmosXDGCacheHome)
	_ = v.BindEnv(envXDGCacheHome, envXDGCacheHome)

	if value := v.GetString(envAtmosXDGCacheHome); value != "" {
		return value, envAtmosXDGCacheHome
	}
	if value := v.GetString(envXDGCacheHome); value != "" {
		return value, envXDGCacheHome
	}
	return "", ""
}

func isVolumeRoot(path string) bool {
	volume := filepath.VolumeName(path)
	if volume == "" {
		return false
	}
	rest := path[len(volume):]
	return rest == "" || rest == string(os.PathSeparator)
}

func pathContainsCurrentProject(target string) bool {
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if atmosConfigPtr != nil && atmosConfigPtr.BasePath != "" {
		candidates = append(candidates, atmosConfigPtr.BasePath)
	}

	for _, candidate := range candidates {
		absCandidate, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if sameOrParentDir(target, absCandidate) {
			return true
		}
	}
	return false
}

func sameOrParentDir(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func validateManagedGitWorkdir(workdir, uri string) error {
	if uri == "" {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHint("Repository URI is required before a git workdir can be cleaned.").
			WithExitCode(2).
			Err()
	}

	if err := ensureGitWorktree(workdir); err != nil {
		return err
	}

	actualURI, err := gitRemoteOriginURL(workdir)
	if err != nil {
		return err
	}
	if !sameGitRemote(uri, actualURI) {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Refusing to clean %q because its origin remote %q does not match configured URI %q.", workdir, actualURI, uri).
			WithHint("Check git.repositories.<name>.workdir before running clean.").
			WithExitCode(2).
			Err()
	}

	return nil
}

func ensureGitWorktree(workdir string) error {
	out, err := exec.Command("git", "-C", workdir, "rev-parse", "--is-inside-work-tree").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Refusing to clean %q because it is not a Git worktree.", workdir).
			WithExitCode(2).
			Err()
	}
	return nil
}

func gitRemoteOriginURL(workdir string) (string, error) {
	out, err := exec.Command("git", "-C", workdir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return "", errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Refusing to clean %q because remote.origin.url could not be read.", workdir).
			WithExitCode(2).
			Err()
	}
	return strings.TrimSpace(string(out)), nil
}

func isGitWorkdirDirty(workdir string) (bool, error) {
	out, err := exec.Command("git", "-C", workdir, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("%w: checking git workdir status for %q: %w", errUtils.ErrGitCommandFailed, workdir, err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func sameGitRemote(configured, actual string) bool {
	if configured == actual {
		return true
	}
	if IsURI(configured) || IsURI(actual) {
		return false
	}

	configuredAbs, configuredErr := filepath.Abs(configured)
	actualAbs, actualErr := filepath.Abs(actual)
	if configuredErr != nil || actualErr != nil {
		return false
	}
	return filepath.Clean(configuredAbs) == filepath.Clean(actualAbs)
}

func init() {
	cleanParser.RegisterFlags(cleanCmd)
	if err := cleanParser.BindToViper(viper.GetViper()); err != nil {
		panic(fmt.Sprintf("git clean: BindToViper: %v", err))
	}
}
