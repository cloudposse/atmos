package marketplace

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/skills"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// Installer manages skill installation.
type Installer struct {
	downloader    DownloaderInterface
	validator     *Validator
	localRegistry *LocalRegistry
	atmosVersion  string
}

// InstallOptions configures the installation process.
type InstallOptions struct {
	Force       bool     // Reinstall if already installed.
	SkipConfirm bool     // Skip security confirmation prompt.
	CustomName  string   // Install with custom name (--as).
	Path        string   // Override install base directory (--path); relative paths resolve against CWD.
	BasePath    string   // Project base path used for client-signal detection and distribution (default: CWD).
	Scope       string   // Distribution scope: "project" or "user" (--scope/--global); always populated by the CLI layer, "project" is the flag default and is never left empty.
	Clients     []string // Explicit AI clients to distribute to (--client); empty means auto-detect.
	AllClients  bool     // Distribute to every supported client (--all-clients).
}

// NewInstaller creates a new skill installer.
func NewInstaller(atmosVersion string) (*Installer, error) {
	defer perf.Track(nil, "marketplace.NewInstaller")()

	localRegistry, err := NewLocalRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load local registry: %w", err)
	}

	return &Installer{
		downloader:    NewDownloader(),
		validator:     NewValidator(atmosVersion),
		localRegistry: localRegistry,
		atmosVersion:  atmosVersion,
	}, nil
}

// Install installs a skill from a source URL.
// It supports both single-skill repos (with SKILL.md at root) and multi-skill packages
// (with skills/*/SKILL.md pattern).
func (i *Installer) Install(ctx context.Context, source string, opts InstallOptions) error {
	defer perf.Track(nil, "marketplace.Installer.Install")()

	// Offline fast path: an official skill referenced by bare name (e.g.
	// "atmos-terraform") installs from the embedded catalog with no network or
	// Git clone. Any source with slashes/dots (a real URL) falls through to the
	// Git flow below.
	if available, ok := LookupBundledSkill(source); ok {
		return i.installBundledSkill(&available, opts)
	}

	// 1. Parse source.
	sourceInfo, err := ParseSource(source)
	if err != nil {
		return fmt.Errorf("invalid source: %w", err)
	}

	// 2. Check if already installed (for single-skill only; multi-skill checked per skill).
	if !opts.Force {
		if _, err := i.localRegistry.Get(sourceInfo.Name); err == nil {
			return fmt.Errorf("%w: use --force to reinstall", ErrSkillAlreadyInstalled)
		}
	}

	// 3. Download to temporary directory.
	ui.Infof("Downloading skills from %s...", sourceInfo.URL)
	tempDir, err := i.downloader.Download(ctx, sourceInfo)
	if err != nil {
		return fmt.Errorf("failed to download skill: %w", err)
	}
	defer os.RemoveAll(tempDir) // Cleanup on error.

	// 4. Check if this is a single-skill repo or a multi-skill package.
	skillMDPath := filepath.Join(tempDir, skillFileName)
	if _, err := os.Stat(skillMDPath); err == nil {
		// Single-skill repo: SKILL.md at root.
		return i.installSingleSkill(tempDir, sourceInfo, opts)
	}

	// Check for multi-skill package: skills/*/SKILL.md pattern.
	pattern := filepath.Join(tempDir, "agent-skills", "skills", "*", skillFileName)
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		// Also try skills/*/SKILL.md as alternative layout.
		pattern = filepath.Join(tempDir, "skills", "*", skillFileName)
		matches, err = filepath.Glob(pattern)
	}
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("%w: no %s found at root or in skills/*/%s pattern", ErrInvalidMetadata, skillFileName, skillFileName)
	}

	return i.installMultiSkillPackage(matches, sourceInfo, opts)
}

// parseAndValidateSkill parses metadata from SKILL.md and validates the skill.
func (i *Installer) parseAndValidateSkill(tempDir string) (*SkillMetadata, error) {
	skillMDPath := filepath.Join(tempDir, skillFileName)
	metadata, err := ParseSkillMetadata(skillMDPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidMetadata, err)
	}

	if err := i.validator.Validate(tempDir, metadata); err != nil {
		return nil, fmt.Errorf("skill validation failed: %w", err)
	}

	return metadata, nil
}

// moveSkillToInstallPath moves the skill from temp dir to its final install path.
func (i *Installer) moveSkillToInstallPath(tempDir, installPath string, force bool) error {
	if force {
		if err := os.RemoveAll(installPath); err != nil {
			log.Warnf("Failed to remove existing installation: %v", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(installPath), dirPermissions); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	if err := os.Rename(tempDir, installPath); err != nil {
		return fmt.Errorf("failed to install skill: %w", err)
	}

	return nil
}

// registerSkill registers a skill in the local registry.
func (i *Installer) registerSkill(metadata *SkillMetadata, sourceInfo *SourceInfo, installPath string, force bool) error {
	installedSkill := &InstalledSkill{
		Name:        metadata.Name,
		DisplayName: metadata.GetDisplayName(),
		Source:      sourceInfo.FullPath,
		Version:     metadata.GetVersion(),
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        installPath,
		IsBuiltIn:   false,
		Enabled:     true,
	}

	if force {
		_ = i.localRegistry.Remove(metadata.Name) // Ignore error if not exists.
	}

	if err := i.localRegistry.Add(installedSkill); err != nil {
		return fmt.Errorf("failed to register skill: %w", err)
	}

	return nil
}

// installSingleSkill installs a single skill from a downloaded repository.
func (i *Installer) installSingleSkill(tempDir string, sourceInfo *SourceInfo, opts InstallOptions) error {
	metadata, err := i.parseAndValidateSkill(tempDir)
	if err != nil {
		return err
	}

	// Security check (interactive prompt).
	if !opts.SkipConfirm {
		if err := i.confirmInstallation(metadata); err != nil {
			return err // User cancelled.
		}
	}

	installPath, err := i.getInstallPath(sourceInfo, opts.Path)
	if err != nil {
		return err
	}

	if err := i.moveSkillToInstallPath(tempDir, installPath, opts.Force); err != nil {
		return err
	}

	if err := i.registerSkill(metadata, sourceInfo, installPath, opts.Force); err != nil {
		return err
	}

	// An explicit --path takes full manual control and skips auto-distribution
	// to avoid surprise double-writes.
	if opts.Path == "" {
		clients := i.resolveDistributionClients(opts.BasePath, &opts)
		for _, r := range i.distributeToClients(opts.BasePath, installPath, metadata.Name, clients, opts.Scope) {
			ui.Infof("Also installed for `%s`: %s", r.Client, r.Target)
		}
	}

	return printInstallSuccess(metadata.GetDisplayName(), metadata.GetVersion(), installPath)
}

// printInstallSuccess prints the standard post-install confirmation, shared by
// the single-skill and bundled-skill install paths.
func printInstallSuccess(displayName, version, installPath string) error {
	ui.Successf("Skill %q installed successfully", displayName)
	ui.Infof("Version: %s", version)
	ui.Infof("Location: %s", redactHomePath(installPath))
	return nil
}

// installBundledSkill installs an official skill from the embedded catalog,
// fully offline. It mirrors installSingleSkill but sources files from the
// embedded FS instead of a Git clone.
func (i *Installer) installBundledSkill(available *AvailableSkill, opts InstallOptions) error {
	defer perf.Track(nil, "marketplace.Installer.installBundledSkill")()

	// Resolve the install name: honor --as when set, otherwise use the
	// canonical embedded-directory name so list/install/Source stay consistent.
	installName := available.Name
	if opts.CustomName != "" {
		installName = opts.CustomName
	}

	if !opts.Force {
		if _, err := i.localRegistry.Get(installName); err == nil {
			return fmt.Errorf("%w: use --force to reinstall", ErrSkillAlreadyInstalled)
		}
	}

	metadata, err := readBundledMetadata(available.Name)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMetadata, err)
	}

	// Security check (interactive prompt).
	if !opts.SkipConfirm {
		if err := i.confirmInstallation(metadata); err != nil {
			return err // User cancelled.
		}
	}

	installPath, err := i.materializeBundledSkill(available.Name, installName, opts.Path, opts.Force)
	if err != nil {
		return err
	}

	if err := i.registerBundledSkill(available, installName, installPath); err != nil {
		return err
	}

	// An explicit --path takes full manual control and skips auto-distribution
	// to avoid surprise double-writes.
	if opts.Path == "" {
		clients := i.resolveDistributionClients(opts.BasePath, &opts)
		for _, r := range i.distributeToClients(opts.BasePath, installPath, installName, clients, opts.Scope) {
			ui.Infof("Also installed for `%s`: %s", r.Client, r.Target)
		}
	}

	return printInstallSuccess(available.DisplayName, available.Version, installPath)
}

// printItemColumns writes a short header line followed by items laid out in
// evenly spaced columns sized to the terminal width, instead of a single
// comma-separated line that wraps unpredictably (and can break mid-word)
// once there are more than a handful of entries.
func printItemColumns(header string, items []string) {
	ui.Infof("%s", header)
	ui.Write(ui.FormatColumns(items, ui.TerminalWidth()) + "\n")
}

// BacktickJoin renders items as a comma-separated, markdown-code-formatted
// list (e.g. `claude-code`, `vscode`) for toast messages, so client/skill
// names read as code rather than plain prose.
func BacktickJoin(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "`" + item + "`"
	}
	return strings.Join(quoted, ", ")
}

// batchInstallOutcome classifies the result of one skill in a batch install
// (InstallAllBundled/installMultiSkillPackage), so the final summary can
// distinguish freshly-installed skills from --force updates and
// already-installed skips instead of a single opaque count.
type batchInstallOutcome int

const (
	outcomeInstalled batchInstallOutcome = iota // Freshly installed; wasn't registered before.
	outcomeUpdated                              // Was already registered; --force reinstalled it.
	outcomeSkipped                              // Was already registered; --force not set.
	outcomeFailed                               // Install attempt failed (see the logged warning).
)

// tallyBatchOutcomes counts each outcome type from a batch install loop.
func tallyBatchOutcomes(outcomes []batchInstallOutcome) (installed, updated, skipped int) {
	for _, outcome := range outcomes {
		switch outcome {
		case outcomeInstalled:
			installed++
		case outcomeUpdated:
			updated++
		case outcomeSkipped:
			skipped++
		case outcomeFailed:
			// Already logged a warning; not counted in any tally.
		}
	}
	return installed, updated, skipped
}

// formatBatchInstallSummary renders the final tally for a batch install as
// the spinner's completion message, distinguishing new installs from --force
// updates, where distributionDesc is the comma-joined "`client` (dir)"
// fragment from resolveDistributionClientsForBatch. When non-empty it's
// woven into this same message rather than left to the spinner's
// in-progress line, because ExecWithSpinnerDynamic only prints the progress
// message while a TTY is animating it -- in non-TTY output (CI, piped,
// tests) only this completion message is ever shown, so it must carry the
// "where did my skills go for my client" answer itself, and skillsDir
// (Atmos's own canonical store) is reported instead only when
// distributionDesc is empty (no client got a copy, e.g. --path was used or
// no clients were resolved) -- it's the only location that matters then.
func formatBatchInstallSummary(installed, updated int, skillsDir, distributionDesc string) string {
	total := installed + updated
	suffix := " in " + redactHomePath(skillsDir)
	if distributionDesc != "" {
		suffix = " → " + distributionDesc
	}
	switch {
	case total == 0:
		return "0 skills installed"
	case updated == 0:
		return fmt.Sprintf("%d skills installed successfully%s", installed, suffix)
	case installed == 0:
		return fmt.Sprintf("%d skills updated successfully%s", updated, suffix)
	default:
		return fmt.Sprintf("%d skills installed successfully%s (%d new, %d updated)", total, suffix, installed, updated)
	}
}

// InstallAllBundled installs every skill in the embedded catalog. It mirrors
// `atmos mcp install` with no server names given: omitting <source> entirely
// means "act on the whole built-in set" rather than requiring a separate
// --all flag. Like installMultiSkillPackage, it shows a single upfront
// confirmation instead of one per skill, and skips (rather than erroring on)
// a skill that's already installed unless --force is set, so one already-
// installed skill doesn't abort the whole batch.
func (i *Installer) InstallAllBundled(opts *InstallOptions) error {
	defer perf.Track(nil, "marketplace.Installer.InstallAllBundled")()

	catalog, err := Catalog()
	if err != nil {
		return fmt.Errorf("failed to load skill catalog: %w", err)
	}
	if len(catalog) == 0 {
		return ErrNoValidSkillsFound
	}

	names := make([]string, 0, len(catalog))
	for _, available := range catalog {
		names = append(names, available.Name)
	}
	printItemColumns(fmt.Sprintf("Discovered %d bundled skills:", len(catalog)), names)

	if !opts.SkipConfirm {
		if err := confirmMultiSkillInstall(len(catalog)); err != nil {
			return err
		}
	}

	return i.runBatchInstallWithSpinner(opts.BasePath, opts, func(clients []string) []batchInstallOutcome {
		outcomes := make([]batchInstallOutcome, len(catalog))
		for idx := range catalog {
			outcomes[idx] = i.installOneBundledSkill(&catalog[idx], opts, clients)
		}
		return outcomes
	})
}

// installOneBundledSkill installs a single bundled skill as part of an
// InstallAllBundled batch. It skips the single-skill security confirmation
// prompt (bundled skills are Atmos's own, already-reviewed content, and the
// batch already got one upfront confirmation) and returns outcomeFailed with
// a logged warning on a per-skill issue instead of returning an error, so
// the rest of the batch keeps going; clients is the batch's pre-resolved
// distribution list (empty when opts.Path is set); success is intentionally
// silent per skill -- the batch-level summary and "Distributing to" line
// already cover it, so only failures are worth a line.
func (i *Installer) installOneBundledSkill(available *AvailableSkill, opts *InstallOptions, clients []string) batchInstallOutcome {
	installName := available.Name
	if opts.CustomName != "" {
		installName = opts.CustomName
	}

	_, getErr := i.localRegistry.Get(installName)
	wasInstalled := getErr == nil
	if wasInstalled && !opts.Force {
		// Silent: the batch summary's aggregate "N already installed" line
		// already covers this, and printing per-skill here would interleave
		// with the progress spinner wrapping the batch loop.
		return outcomeSkipped
	}

	if _, err := readBundledMetadata(available.Name); err != nil {
		log.Warnf("Skipping %s: %v", available.Name, err)
		return outcomeFailed
	}

	installPath, err := i.materializeBundledSkill(available.Name, installName, opts.Path, opts.Force)
	if err != nil {
		log.Warnf("Failed to install %s: %v", available.Name, err)
		return outcomeFailed
	}

	if err := i.registerBundledSkill(available, installName, installPath); err != nil {
		log.Warnf("Failed to register %s: %v", available.Name, err)
		return outcomeFailed
	}

	if len(clients) > 0 {
		i.distributeToClients(opts.BasePath, installPath, installName, clients, opts.Scope)
	}

	if wasInstalled {
		return outcomeUpdated
	}
	return outcomeInstalled
}

// materializeBundledSkill copies a bundled skill's files from the embedded FS
// to its install path, handling --force replacement, and returns the install
// path.
//
// The sourceName argument is the embedded directory name, and installName is
// the on-disk name, which may differ from sourceName when --as is set.
// PathOverride honors --path / ATMOS_AI_SKILL_PATH. Bundled skills already
// install flat (skillsDir/installName, no owner/repo nesting), so no extra
// flattening is needed here.
func (i *Installer) materializeBundledSkill(sourceName, installName, pathOverride string, force bool) (string, error) {
	skillFS, err := bundledSkillFS(sourceName)
	if err != nil {
		return "", err
	}

	skillsDir, err := ResolveSkillsDir(pathOverride)
	if err != nil {
		return "", fmt.Errorf("failed to resolve skills directory: %w", err)
	}
	installPath := filepath.Join(skillsDir, installName)

	if err := i.prepareInstallPath(installPath, installName, force); err != nil {
		return "", err
	}

	if err := os.MkdirAll(installPath, dirPermissions); err != nil {
		return "", fmt.Errorf("failed to create install directory: %w", err)
	}

	if err := copyFS(skillFS, installPath); err != nil {
		return "", fmt.Errorf("failed to install bundled skill: %w", err)
	}

	return installPath, nil
}

// prepareInstallPath clears or validates the install directory before a bundled
// skill is materialized. With --force a failed removal is a hard error, since
// copying into a partially-removed directory risks a corrupted install. Without
// --force an existing directory is rejected to avoid merging into stale content.
func (i *Installer) prepareInstallPath(installPath, installName string, force bool) error {
	defer perf.Track(nil, "marketplace.Installer.prepareInstallPath")()

	if force {
		if isSymlink(installPath) {
			return fmt.Errorf("%w: %s", errUtils.ErrRefuseDeleteSymbolicLink, installPath)
		}
		if err := os.RemoveAll(installPath); err != nil {
			return fmt.Errorf("failed to remove existing installation: %w", err)
		}
		_ = i.localRegistry.Remove(installName) // Ignore error if not exists.
		return nil
	}

	_, statErr := os.Stat(installPath)
	if statErr == nil {
		return fmt.Errorf("%w: use --force to reinstall", ErrSkillAlreadyInstalled)
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("failed to check install directory: %w", statErr)
	}
	return nil
}

// registerBundledSkill records an installed bundled skill in the local registry.
// The installName is the name under which the skill is registered (it may differ
// from available.Name when the skill was installed with --as).
func (i *Installer) registerBundledSkill(available *AvailableSkill, installName, installPath string) error {
	installedSkill := &InstalledSkill{
		Name:        installName,
		DisplayName: available.DisplayName,
		Source:      available.Source,
		Version:     available.Version,
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        installPath,
		IsBuiltIn:   false,
		Enabled:     true,
	}
	if err := i.localRegistry.Add(installedSkill); err != nil {
		return fmt.Errorf("failed to register skill: %w", err)
	}

	return nil
}

// discoveredSkill holds the directory and metadata for a discovered skill.
type discoveredSkill struct {
	dir      string
	metadata *SkillMetadata
}

// discoverSkillsFromPaths parses metadata from each SKILL.md path and returns valid skills.
func discoverSkillsFromPaths(skillMDPaths []string) ([]discoveredSkill, []string) {
	var discovered []discoveredSkill
	var skillNames []string

	for _, skillMDPath := range skillMDPaths {
		skillDir := filepath.Dir(skillMDPath)
		metadata, err := ParseSkillMetadata(skillMDPath)
		if err != nil {
			log.Warnf("Skipping invalid skill at %s: %v", skillMDPath, err)
			continue
		}

		discovered = append(discovered, discoveredSkill{dir: skillDir, metadata: metadata})
		skillNames = append(skillNames, metadata.Name)
	}

	return discovered, skillNames
}

// confirmMultiSkillInstall prompts the user for multi-skill install confirmation.
func confirmMultiSkillInstall(count int) error {
	return requireConfirmation(fmt.Sprintf("Install all %d skills?", count), ErrInstallationCancelled)
}

// prepareOverwrite checks whether skillName is already registered and, if
// so, either warns and reports skip=true (no --force) or best-effort removes
// the stale install path and registry entry so the caller can reinstall
// fresh (--force). Returns wasInstalled so the caller can distinguish
// outcomeInstalled from outcomeUpdated once the reinstall succeeds.
func (i *Installer) prepareOverwrite(skillName, installPath string, force bool) (wasInstalled, skip bool) {
	_, getErr := i.localRegistry.Get(skillName)
	wasInstalled = getErr == nil
	if !wasInstalled {
		return false, false
	}
	if !force {
		// Silent: see the matching comment in installOneBundledSkill.
		return true, true
	}
	if err := os.RemoveAll(installPath); err != nil {
		log.Warnf("Failed to remove existing installation for %s: %v", skillName, err)
	}
	_ = i.localRegistry.Remove(skillName)
	return true, false
}

// installOneSkillFromPackage installs a single skill from a multi-skill
// package; clients is the batch's pre-resolved distribution list (empty when
// opts.Path is set); success is intentionally silent per skill -- the
// batch-level summary and "Distributing to" line already cover it, so only
// failures are worth a line.
func (i *Installer) installOneSkillFromPackage(skill discoveredSkill, sourceInfo *SourceInfo, opts *InstallOptions, clients []string) batchInstallOutcome {
	skillName := skill.metadata.Name
	skillsDir, err := ResolveSkillsDir(opts.Path)
	if err != nil {
		log.Warnf("Failed to resolve install directory for %s: %v", skillName, err)
		return outcomeFailed
	}
	// An explicit --path flattens the layout to <path>/<skillName>, dropping
	// the default owner/repo nesting VS Code/Copilot don't expect.
	installPath := filepath.Join(skillsDir, sourceInfo.Owner, sourceInfo.Repo, skillName)
	if opts.Path != "" {
		installPath = filepath.Join(skillsDir, skillName)
	}

	wasInstalled, skip := i.prepareOverwrite(skillName, installPath, opts.Force)
	if skip {
		return outcomeSkipped
	}

	if err := os.MkdirAll(installPath, dirPermissions); err != nil {
		log.Warnf("Failed to create directory for %s: %v", skillName, err)
		return outcomeFailed
	}

	if err := copyDir(skill.dir, installPath); err != nil {
		log.Warnf("Failed to install skill %s: %v", skillName, err)
		return outcomeFailed
	}

	installedSkill := &InstalledSkill{
		Name:        skillName,
		DisplayName: skill.metadata.GetDisplayName(),
		Source:      sourceInfo.FullPath,
		Version:     skill.metadata.GetVersion(),
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        installPath,
		IsBuiltIn:   false,
		Enabled:     true,
	}

	if err := i.localRegistry.Add(installedSkill); err != nil {
		log.Warnf("Failed to register skill %s: %v", skillName, err)
		return outcomeFailed
	}

	if len(clients) > 0 {
		i.distributeToClients(opts.BasePath, installPath, skillName, clients, opts.Scope)
	}

	if wasInstalled {
		return outcomeUpdated
	}
	return outcomeInstalled
}

// installMultiSkillPackage installs multiple skills discovered in a package.
func (i *Installer) installMultiSkillPackage(skillMDPaths []string, sourceInfo *SourceInfo, opts InstallOptions) error {
	discovered, skillNames := discoverSkillsFromPaths(skillMDPaths)
	if len(discovered) == 0 {
		return ErrNoValidSkillsFound
	}

	// Show discovery summary.
	printItemColumns(fmt.Sprintf("Discovered %d skills in package:", len(discovered)), skillNames)

	if !opts.SkipConfirm {
		if err := confirmMultiSkillInstall(len(discovered)); err != nil {
			return err
		}
	}

	return i.runBatchInstallWithSpinner(opts.BasePath, &opts, func(clients []string) []batchInstallOutcome {
		outcomes := make([]batchInstallOutcome, len(discovered))
		for idx, skill := range discovered {
			outcomes[idx] = i.installOneSkillFromPackage(skill, sourceInfo, &opts, clients)
		}
		return outcomes
	})
}

// copyFS copies the entire tree of srcFS into the dst directory on disk. It is
// the fs.FS analogue of copyDir, letting bundled (embedded) skills install
// through the same write logic as Git-cloned ones. Embedded paths are always
// forward-slash separated, so they are translated to OS paths under dst.
func copyFS(srcFS fs.FS, dst string) error {
	return fs.WalkDir(srcFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		target := filepath.Join(dst, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, dirPermissions)
		}

		data, err := fs.ReadFile(srcFS, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), dirPermissions); err != nil {
			return err
		}
		return os.WriteFile(target, data, filePermissions)
	})
}

// copyDir copies the contents of a source directory to a destination directory.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if err := copyEntry(srcPath, dstPath, entry.IsDir()); err != nil {
			return err
		}
	}

	return nil
}

// copyEntry copies a single directory entry (file or subdirectory).
func copyEntry(srcPath, dstPath string, isDir bool) error {
	if isDir {
		if err := os.MkdirAll(dstPath, dirPermissions); err != nil {
			return err
		}
		return copyDir(srcPath, dstPath)
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, data, filePermissions)
}

// Uninstall removes an installed skill.
//
// The basePath and clients arguments drive best-effort cleanup of any
// per-client copies distributed by a prior install (see distributeToClients);
// pass a nil/empty clients slice to skip client cleanup. Scope is the
// distribution scope ("project" or "user") the client copies were installed
// under.
func (i *Installer) Uninstall(name string, force bool, basePath string, clients []string, scopes []string) error {
	defer perf.Track(nil, "marketplace.Installer.Uninstall")()

	// 1. Get skill from registry.
	skill, err := i.localRegistry.Get(name)
	if err != nil {
		return err
	}

	// 2. Confirm uninstallation (unless force).
	if !force {
		title := fmt.Sprintf("Uninstall skill %q (version %s)?", skill.DisplayName, skill.Version)
		if err := requireConfirmation(title, ErrUninstallationCancelled); err != nil {
			return err
		}
	}

	// 3. Remove skill directory.
	if err := os.RemoveAll(skill.Path); err != nil {
		return fmt.Errorf("failed to remove skill directory: %w", err)
	}

	// 4. Remove from registry.
	if err := i.localRegistry.Remove(name); err != nil {
		return err
	}

	// 5. Best-effort cleanup of any distributed client copies.
	skipped := removeClientCopies(basePath, name, clients, scopes)

	ui.Successf("Skill %q uninstalled successfully", skill.DisplayName)
	reportSkippedSymlinks(skipped)
	return nil
}

// reportSkippedSymlinks prints a single aggregate note for client-distributed
// copies left in place because they're symbolic links (see
// removeClientCopies), instead of one warning line per skill.
func reportSkippedSymlinks(count int) {
	if count == 0 {
		return
	}
	noun := "copy"
	if count > 1 {
		noun = "copies"
	}
	ui.Infof("%d client-distributed %s left in place (symbolic link, use `--path` to manage a plain directory instead)", count, noun)
}

// UninstallAll removes every installed skill. It mirrors `atmos mcp
// uninstall` with no server names given: omitting <name> entirely means "act
// on everything installed" rather than requiring a separate --all flag. A
// per-skill failure is logged and skipped so the rest of the batch keeps
// going, matching InstallAllBundled's lenient behavior; scopes are the
// distribution scope(s) ("project" and/or "user") to check for client
// copies -- see resolveUninstallScopes for why this is a slice rather than a
// single guessed value.
func (i *Installer) UninstallAll(force bool, basePath string, clients []string, scopes []string) error {
	defer perf.Track(nil, "marketplace.Installer.UninstallAll")()

	installed := i.localRegistry.List()
	if len(installed) == 0 {
		ui.Info("No skills installed.")
		return nil
	}

	names := make([]string, 0, len(installed))
	for _, skill := range installed {
		names = append(names, skill.Name)
	}
	printItemColumns("Installed skills:", names)

	if !force {
		title := fmt.Sprintf("Uninstall all %d skills?", len(installed))
		if err := requireConfirmation(title, ErrUninstallationCancelled); err != nil {
			return err
		}
	}

	removed := 0
	skippedSymlinks := 0
	for _, skill := range installed {
		if err := os.RemoveAll(skill.Path); err != nil {
			log.Warnf("Failed to remove skill directory for %s: %v", skill.Name, err)
			continue
		}
		if err := i.localRegistry.Remove(skill.Name); err != nil {
			log.Warnf("Failed to remove %s from registry: %v", skill.Name, err)
			continue
		}
		skippedSymlinks += removeClientCopies(basePath, skill.Name, clients, scopes)
		removed++
	}

	ui.Successf("%d skills uninstalled successfully", removed)
	reportSkippedSymlinks(skippedSymlinks)
	return nil
}

// List returns all installed skills.
func (i *Installer) List() []*InstalledSkill {
	defer perf.Track(nil, "marketplace.Installer.List")()
	return i.localRegistry.List()
}

// Get retrieves an installed skill.
func (i *Installer) Get(name string) (*InstalledSkill, error) {
	defer perf.Track(nil, "marketplace.Installer.Get")()
	return i.localRegistry.Get(name)
}

// LoadInstalledSkills loads all installed community skills into the skill registry.
func (i *Installer) LoadInstalledSkills(registry *skills.Registry) error {
	defer perf.Track(nil, "marketplace.Installer.LoadInstalledSkills")()

	for _, installed := range i.localRegistry.List() {
		if !installed.Enabled {
			continue // Skip disabled skills.
		}

		// Parse metadata from SKILL.md.
		skillMDPath := filepath.Join(installed.Path, skillFileName)
		metadata, err := ParseSkillMetadata(skillMDPath)
		if err != nil {
			log.Warnf("Failed to load community skill %q: %v", installed.Name, err)
			continue
		}

		// Read prompt content with references.
		promptContent, err := readSkillPromptWithReferences(installed.Path, metadata)
		if err != nil {
			log.Warnf("Failed to read prompt for skill %q: %v", installed.Name, err)
			continue
		}

		// Create skill.
		skill := &skills.Skill{
			Name:            metadata.Name,
			DisplayName:     metadata.GetDisplayName(),
			Description:     metadata.Description,
			SystemPrompt:    promptContent,
			Category:        metadata.GetCategory(),
			IsBuiltIn:       false,
			AllowedTools:    metadata.AllowedTools,
			RestrictedTools: metadata.RestrictedTools,
		}

		// Register.
		if err := registry.Register(skill); err != nil {
			log.Warnf("Failed to register community skill %q: %v", installed.Name, err)
			continue
		}
	}

	return nil
}

// readSkillPromptWithReferences reads the skill prompt body and appends any referenced files.
func readSkillPromptWithReferences(skillDir string, metadata *SkillMetadata) (string, error) {
	skillMDPath := filepath.Join(skillDir, skillFileName)
	body, err := readSkillPrompt(skillMDPath)
	if err != nil {
		return "", err
	}

	// Append reference files if any.
	for _, ref := range metadata.References {
		refPath := filepath.Join(skillDir, ref)
		refContent, err := os.ReadFile(refPath)
		if err != nil {
			log.Warnf("Failed to read reference file %q: %v", ref, err)
			continue
		}

		refName := filepath.Base(ref)
		body += "\n\n---\n\n## Reference: " + refName + "\n\n" + strings.TrimSpace(string(refContent))
	}

	return body, nil
}

// readSkillPrompt reads the markdown body from a SKILL.md file (after frontmatter).
func readSkillPrompt(skillMDPath string) (string, error) {
	content, err := os.ReadFile(skillMDPath)
	if err != nil {
		return "", err
	}

	// Extract body after frontmatter.
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var lines []string
	inFrontmatter := false
	frontmatterEnded := false
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter && lineNum == 1 {
				inFrontmatter = true
				continue
			} else if inFrontmatter {
				inFrontmatter = false
				frontmatterEnded = true
				continue
			}
		}

		if inFrontmatter {
			continue
		}

		if frontmatterEnded {
			lines = append(lines, line)
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n")), scanner.Err()
}

// getInstallPath returns the installation path for a skill. When pathOverride
// is set (--path / ATMOS_AI_SKILL_PATH), the layout is flattened to
// <override>/<skillName>, dropping the default owner/repo nesting, since
// VS Code/Copilot need a flat layout.
func (i *Installer) getInstallPath(source *SourceInfo, pathOverride string) (string, error) {
	skillsDir, err := ResolveSkillsDir(pathOverride)
	if err != nil {
		return "", err
	}
	if pathOverride != "" {
		return filepath.Join(skillsDir, source.Name), nil
	}
	return filepath.Join(skillsDir, source.FullPath), nil
}

// resolveDistributionClients resolves which AI clients a skill should be
// distributed to: an explicit opts.Clients/opts.AllClients always wins;
// otherwise it auto-detects based on basePath/opts.Scope. Callers that
// install many skills in one batch (e.g. InstallAllBundled) should call this
// once up front rather than once per skill -- the result is the same every
// time for a given basePath/opts.
func (i *Installer) resolveDistributionClients(basePath string, opts *InstallOptions) []string {
	if opts.AllClients {
		return SupportedClients
	}
	if len(opts.Clients) > 0 {
		return opts.Clients
	}
	homeDir, err := homedir.Dir()
	if err != nil {
		log.Warnf("Failed to resolve home directory for client distribution: %v", err)
	}
	return DetectClients(basePath, homeDir, opts.Scope)
}

// resolveDistributionClientsForBatch resolves the batch distribution client
// list once (see resolveDistributionClients) and, if any were resolved,
// builds a "Distributing to" progress message describing each client's
// actual target directory -- shared by InstallAllBundled and
// installMultiSkillPackage, which both feed it to
// spinner.ExecWithSpinnerDynamic as the in-progress message so neither prints
// one line per skill. Showing the real per-client directory here (rather
// than just the client name) avoids it ever being confused with skillsDir,
// Atmos's own canonical store reported separately by
// formatBatchInstallSummary. Returns a nil client list and empty message
// when opts.Path is set, since an explicit --path skips auto-distribution
// entirely.
func (i *Installer) resolveDistributionClientsForBatch(basePath string, opts *InstallOptions) (clients []string, description string) {
	if opts.Path != "" {
		return nil, ""
	}
	clients = i.resolveDistributionClients(basePath, opts)
	if len(clients) == 0 {
		return clients, ""
	}

	homeDir, err := homedir.Dir()
	if err != nil {
		log.Warnf("Failed to resolve home directory for client distribution: %v", err)
	}

	described := make([]string, len(clients))
	for idx, client := range clients {
		dir := clientSkillDir(basePath, homeDir, opts.Scope, client)
		if dir == "" {
			described[idx] = BacktickJoin([]string{client})
			continue
		}
		described[idx] = fmt.Sprintf("`%s` (%s)", client, redactHomePath(dir))
	}
	return clients, strings.Join(described, ", ")
}

// runBatchInstallWithSpinner resolves the batch's distribution clients, then
// runs installFn (the per-skill install loop, shared by InstallAllBundled and
// installMultiSkillPackage) inside a progress spinner that shows where
// skills are being distributed to and finalizes to the tally once done, in
// place of two separate static lines. The distribution description is woven
// into both the in-progress and completion messages (see
// formatBatchInstallSummary for why it can't rely on the progress message
// alone), and installFn must not print anything itself (see
// installOneBundledSkill/installOneSkillFromPackage's silent skip handling)
// since concurrent writes to the terminal while the spinner owns it would
// corrupt the animation; the aggregate "N already installed" line below is
// printed only after the spinner has released the terminal.
func (i *Installer) runBatchInstallWithSpinner(
	basePath string,
	opts *InstallOptions,
	installFn func(clients []string) []batchInstallOutcome,
) error {
	clients, description := i.resolveDistributionClientsForBatch(basePath, opts)
	progressMsg := "Installing skills"
	if description != "" {
		progressMsg = "Distributing to: " + description
	}

	var skipped int
	err := spinner.ExecWithSpinnerDynamic(progressMsg, func() (string, error) {
		var installed, updated int
		installed, updated, skipped = tallyBatchOutcomes(installFn(clients))

		skillsDir, dirErr := ResolveSkillsDir(opts.Path)
		if dirErr != nil {
			skillsDir = opts.Path
		}
		return formatBatchInstallSummary(installed, updated, skillsDir, description), nil
	})
	if err != nil {
		return err
	}
	if skipped > 0 {
		ui.Infof("%d already installed (use `--force` to reinstall)", skipped)
	}
	return nil
}

// distributionResult names one client a skill was successfully distributed
// to and the path it landed at.
type distributionResult struct {
	Client string
	Target string
}

// distributeToClients copies an installed skill into each of the given
// clients' well-known skill directory (e.g. .github/skills/<name> for VS
// Code), so clients like VS Code/Copilot pick it up with zero extra flags.
// Callers only invoke this when opts.Path is unset -- an explicit --path
// takes full manual control and skips auto-distribution to avoid surprise
// double-writes. A distribution failure is a warning, not a hard error: the
// canonical install already succeeded and remains the source of truth.
// Returns the clients actually distributed to (skipping a client with no
// resolved directory or a pre-existing symlink); it never prints a
// per-client success line itself -- that's left to the caller, so a batch
// installer can announce the client list once instead of once per skill.
func (i *Installer) distributeToClients(basePath, installPath, skillName string, clients []string, scope string) []distributionResult {
	defer perf.Track(nil, "marketplace.Installer.distributeToClients")()

	homeDir, err := homedir.Dir()
	if err != nil {
		log.Warnf("Failed to resolve home directory for client distribution: %v", err)
	}

	var results []distributionResult
	for _, client := range clients {
		dir := clientSkillDir(basePath, homeDir, scope, client)
		if dir == "" {
			continue
		}
		target := filepath.Join(dir, skillName)
		if isSymlink(target) {
			ui.Warningf("Skipping `%s`: `%s` is a symbolic link (%s) -- use `--path` to install to a plain directory instead",
				client, target, errUtils.ErrRefuseWriteThroughSymlink)
			continue
		}
		if err := os.MkdirAll(target, dirPermissions); err != nil {
			log.Warnf("Failed to create client skill directory for %s (%s): %v", client, target, err)
			continue
		}
		if err := copyDir(installPath, target); err != nil {
			log.Warnf("Failed to distribute skill %q to %s (%s): %v", skillName, client, target, err)
			continue
		}
		results = append(results, distributionResult{Client: client, Target: target})
	}
	return results
}

// removeClientCopies best-effort removes any per-client distributed copies of
// a skill, at every scope in scopes, and returns how many existing targets
// were left in place because they're symbolic links. This is a stateless
// recompute -- the registry never tracks which clients (or which scope) a
// skill was distributed to, so removing a client copy that was never
// created (or was already removed, or lives at a scope not in scopes)
// simply no-ops. Checking every scope means this may legitimately encounter
// symlinks that have nothing to do with the current uninstall session (e.g.
// this very repo's own bundled-skill symlinks for contributor
// auto-discovery, present at project scope regardless of where anything was
// actually distributed) -- returning a count instead of warning per skill
// lets the caller report one aggregate line rather than one alarming
// "refusing to delete" line per skill, most of which are for an unrelated
// scope the user never touched.
func removeClientCopies(basePath, skillName string, clients []string, scopes []string) (skippedSymlinks int) {
	homeDir, err := homedir.Dir()
	if err != nil {
		log.Warnf("Failed to resolve home directory for client cleanup: %v", err)
	}

	for _, scope := range scopes {
		for _, client := range clients {
			dir := clientSkillDir(basePath, homeDir, scope, client)
			if dir == "" {
				continue
			}
			target := filepath.Join(dir, skillName)
			if isSymlink(target) {
				skippedSymlinks++
				continue
			}
			if err := os.RemoveAll(target); err != nil {
				log.Warnf("Failed to remove distributed skill %q from %s (%s): %v", skillName, client, target, err)
			}
		}
	}
	return skippedSymlinks
}

// redactHomePath replaces the home directory portion of a path with ~ for display.
// This avoids logging sensitive home directory paths while still providing useful information.
func redactHomePath(path string) string {
	homeDir, err := homedir.Dir()
	if err != nil {
		return "~/.atmos/skills/..."
	}
	if strings.HasPrefix(path, homeDir) {
		return "~" + strings.TrimPrefix(path, homeDir)
	}
	return path
}

// hasDestructiveTools checks if any of the allowed tools are destructive.
func hasDestructiveTools(allowedTools []string) bool {
	destructiveTools := []string{"terraform_apply", "terraform_destroy", "helmfile_apply"}
	for _, tool := range allowedTools {
		for _, destructive := range destructiveTools {
			if tool == destructive {
				return true
			}
		}
	}
	return false
}

// printToolAccessWarnings prints tool access information and warnings for destructive tools.
func printToolAccessWarnings(metadata *SkillMetadata) error {
	if len(metadata.AllowedTools) == 0 {
		return nil
	}

	ui.Infof("Tool access allowed: %s", strings.Join(metadata.AllowedTools, ", "))

	if !hasDestructiveTools(metadata.AllowedTools) {
		return nil
	}

	ui.Warningf("This skill requests access to destructive operations. Review the skill source before using: %s", metadata.GetRepository())
	return nil
}

// confirmInstallation prompts user to confirm skill installation.
func (i *Installer) confirmInstallation(metadata *SkillMetadata) error {
	// Display skill info.
	ui.Infof("Skill: %s", metadata.GetDisplayName())
	ui.Infof("Author: %s", metadata.GetAuthor())
	ui.Infof("Version: %s", metadata.GetVersion())
	ui.Infof("Repository: %s", metadata.GetRepository())

	if err := printToolAccessWarnings(metadata); err != nil {
		return err
	}

	return requireConfirmation("Install this skill?", ErrInstallationCancelled)
}

func requireConfirmation(title string, cancelErr error) error {
	confirmed, err := flags.PromptForConfirmation(title, false)
	if err != nil {
		return err
	}
	if !confirmed {
		return cancelErr
	}
	return nil
}
