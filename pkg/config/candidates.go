package config

import (
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ConfigCandidateKind classifies a candidate Atmos CLI configuration source.
type ConfigCandidateKind string

const (
	// ConfigCandidateBase is an `atmos.yaml` (or `.atmos.yaml`) file selected via --config,
	// --config-path, ATMOS_CLI_CONFIG_PATH, or discovered in the working directory, a parent
	// directory, or the git root.
	ConfigCandidateBase ConfigCandidateKind = "base"
	// ConfigCandidateAtmosD is a file under an `atmos.d/` or `.atmos.d/` directory.
	ConfigCandidateAtmosD ConfigCandidateKind = "atmos.d"
	// ConfigCandidateProfile is a file belonging to an ACTIVE profile directory.
	ConfigCandidateProfile ConfigCandidateKind = "profile"
	// ConfigCandidateAmbient is configuration that is not part of the project: the system-wide
	// config (`/usr/local/etc/atmos` or `%LOCALAPPDATA%`) and the user's `~/.atmos` config.
	ConfigCandidateAmbient ConfigCandidateKind = "ambient"
)

// ConfigCandidate describes one configuration file (or an expected-but-missing location) that the
// Atmos config loader reads for the current invocation.
type ConfigCandidate struct {
	// Path is the absolute path of the file.
	Path string
	// Kind classifies the candidate.
	Kind ConfigCandidateKind
	// Source explains how the candidate was selected, e.g. "--config", "--config-path",
	// "ATMOS_CLI_CONFIG_PATH", "working directory", "parent directory", "git root", "home directory",
	// "system directory", "atmos.d (git root)", "profile 'ci' (project-hidden)".
	Source string
	// Profile is the profile name for ConfigCandidateProfile candidates.
	Profile string
	// Exists reports whether Path exists on disk. Missing candidates are only reported for
	// explicit selections (--config, --config-path, ATMOS_CLI_CONFIG_PATH, an active profile);
	// auto-discovery reports only what it finds.
	Exists bool
}

// systemSource is the Source label of the system-wide config.
const systemSource = "system directory"

// candidateOptions carries the inputs of ResolveConfigCandidates.
type candidateOptions struct {
	configFiles               []string
	configDirs                []string
	cliConfigPath             string
	profiles                  []string
	profilesBasePath          string
	profilesBasePathConfigDir string
	workingDir                string
}

// CandidateOption configures ResolveConfigCandidates.
type CandidateOption func(*candidateOptions)

// WithConfigFiles sets the files selected via --config.
func WithConfigFiles(files ...string) CandidateOption {
	return func(o *candidateOptions) { o.configFiles = files }
}

// WithConfigDirs sets the directories selected via --config-path.
func WithConfigDirs(dirs ...string) CandidateOption {
	return func(o *candidateOptions) { o.configDirs = dirs }
}

// WithCliConfigPath sets the legacy single CLI config directory (ConfigAndStacksInfo.AtmosCliConfigPath).
func WithCliConfigPath(dir string) CandidateOption {
	return func(o *candidateOptions) { o.cliConfigPath = dir }
}

// WithWorkingDir anchors the resolution to dir instead of the process working directory. Relative
// --config / --config-path values, the working-directory and parent-directory searches, and the
// Git-root lookup are all evaluated from dir, so the process working directory is never changed or
// read. A relative dir is resolved against the process working directory.
func WithWorkingDir(dir string) CandidateOption {
	return func(o *candidateOptions) { o.workingDir = dir }
}

// WithProfiles sets the ACTIVE profiles (from --profile / ATMOS_PROFILE / profiles.default).
// Only these profiles are reported; inactive profile directories never appear. The bare
// `--profile` interactive sentinel is ignored.
func WithProfiles(profiles ...string) CandidateOption {
	return func(o *candidateOptions) { o.profiles = profiles }
}

// WithProfilesBasePath sets the declared `profiles.base_path` value and, when it is relative, the
// directory of the config file that declared it. The resolver does not parse configuration, so the
// caller supplies these (an empty basePath means the default profile locations only).
func WithProfilesBasePath(basePath, declaringDir string) CandidateOption {
	return func(o *candidateOptions) {
		o.profilesBasePath = basePath
		o.profilesBasePathConfigDir = declaringDir
	}
}

// ResolveConfigCandidates returns the ordered list of Atmos CLI configuration files the real loader
// (LoadConfig) reads for the current invocation, WITHOUT parsing, merging, or unmarshalling any of
// them. It exists so tooling such as `atmos doctor` can inspect configuration that is malformed.
//
// The list is in merge order: lowest precedence first, highest last. The path computation is shared
// with the loader, so it honors:
//
//   - --config / --config-path selections (WithConfigFiles / WithConfigDirs, falling back to the
//     ATMOS_CONFIG / ATMOS_CONFIG_PATH environment variables as the loader does). When any is given,
//     no auto-discovery happens.
//   - Otherwise discovery in loader order: system dir (ambient), `~/.atmos` (ambient), nearest parent
//     directory containing atmos.yaml (when the working directory has no Atmos config), the git root
//     (same condition), the working directory, ATMOS_CLI_CONFIG_PATH, and WithCliConfigPath.
//     ATMOS_CLI_CONFIG_PATH disables parent-directory and git-root discovery.
//   - `atmos.d/` and `.atmos.d/` relative to each loaded config file's directory (plus the git root's,
//     loaded first at lower priority). Like the loader, none are read for --config files or the
//     system config. Each file under them is its own candidate, preceding the base file it belongs to
//     (the base file is applied last and wins).
//   - Only the ACTIVE profiles (WithProfiles), resolved through the same location search as the loader.
//     A profile that cannot be found yields a single non-existent candidate.
//
// Not covered because it requires parsing: `import:` entries inside config files, `profiles.default`,
// and `profiles.base_path` (pass the latter two via WithProfiles and WithProfilesBasePath).
func ResolveConfigCandidates(opts ...CandidateOption) ([]ConfigCandidate, error) {
	defer perf.Track(nil, "config.ResolveConfigCandidates")()

	var o candidateOptions
	for _, opt := range opts {
		opt(&o)
	}

	wd, err := candidateWorkingDir(o.workingDir)
	if err != nil {
		return nil, err
	}
	r := &candidateResolver{wd: wd}
	if len(o.configFiles) == 0 && len(o.configDirs) == 0 {
		sel := ConfigSelectionFromEnv()
		o.configFiles, o.configDirs = sel.Config, sel.ConfigPath
	}
	if len(o.configFiles) > 0 || len(o.configDirs) > 0 {
		err = r.addSelected(o.configFiles, o.configDirs)
	} else {
		err = r.addDiscovered(o.cliConfigPath)
	}
	if err != nil {
		return nil, err
	}
	if err := r.addProfiles(&o); err != nil {
		return nil, err
	}
	return dedupeCandidates(r.candidates), nil
}

// candidateResolver accumulates candidates in merge order.
type candidateResolver struct {
	// wd is the absolute directory every relative path and search is anchored to.
	wd         string
	candidates []ConfigCandidate
	// configDirs are the directories of the base configs found, in merge order, used to locate profiles.
	configDirs []string
	// selected is true when configs came from --config/--config-path (no discovery).
	selected bool
}

func (r *candidateResolver) add(c ConfigCandidate) {
	r.candidates = append(r.candidates, c)
}

// addSelected mirrors mergeConfigFromCLIArgs: --config files first, then --config-path directories.
func (r *candidateResolver) addSelected(files, dirs []string) error {
	r.selected = true
	for _, f := range files {
		abs, err := absoluteFrom(r.wd, f)
		if err != nil {
			return err
		}
		// mergeFiles calls mergeDefaultImports with the FILE path, which is not a directory,
		// so no atmos.d is read for --config files.
		r.add(ConfigCandidate{Path: abs, Kind: ConfigCandidateBase, Source: "--config", Exists: isFile(abs)})
		r.configDirs = append(r.configDirs, filepath.Dir(abs))
	}
	for _, d := range dirs {
		abs, err := absoluteFrom(r.wd, d)
		if err != nil {
			return err
		}
		r.addDir(abs, "--config-path", true, false, CliConfigFileName, DotCliConfigFileName)
	}
	return nil
}

// addDiscovered mirrors loadConfigSources, lowest precedence first.
func (r *candidateResolver) addDiscovered(cliConfigPath string) error {
	r.addAmbient()
	if err := r.addProjectDiscovery(); err != nil {
		return err
	}
	r.addExplicitPaths(cliConfigPath)

	// With no config file at all the loader falls back to the built-in defaults and still reads the
	// git root's atmos.d (mergeDefaultImports on the git root).
	if !r.hasExistingBase() {
		if root := defaultConfigGitRootDir(r.wd); root != "" {
			r.addAtmosD(root)
		}
	}
	return nil
}

// addAmbient adds the system-wide and per-user configs, which sit below project config.
func (r *candidateResolver) addAmbient() {
	if sys := systemConfigDir(); sys != "" {
		r.addDir(sys, systemSource, false, true, CliConfigFileName)
	}
	if home, err := homeConfigDir(defaultHomeDirProvider); err == nil {
		r.addDir(home, "home directory", false, true, CliConfigFileName)
	}
}

// addProjectDiscovery adds the parent-directory, git-root, and working-directory configs.
func (r *candidateResolver) addProjectDiscovery() error {
	wd := r.wd
	if envConfigPath() == "" {
		if parent := parentConfigSearchDir(wd); parent != "" {
			r.addDir(parent, "parent directory", false, false, CliConfigFileName)
		}
	}
	gitRoot, err := gitRootConfigSearchDir(wd)
	if err != nil {
		return err
	}
	if gitRoot != "" {
		r.addDir(gitRoot, "git root", false, false, CliConfigFileName)
	}
	r.addDir(wd, "working directory", false, false, CliConfigFileName)
	return nil
}

// addExplicitPaths adds ATMOS_CLI_CONFIG_PATH and the legacy CLI config path, the highest-priority discovery sources.
func (r *candidateResolver) addExplicitPaths(cliConfigPath string) {
	if env := envConfigPath(); env != "" {
		if abs, err := absoluteFrom(r.wd, env); err == nil {
			r.addDir(abs, AtmosCliConfigPathEnvVar, true, false, CliConfigFileName)
		}
	}
	if cliConfigPath != "" {
		if abs, err := absoluteFrom(r.wd, cliConfigPath); err == nil {
			r.addDir(abs, "CLI config path", true, false, CliConfigFileName)
		}
	}
}

func (r *candidateResolver) hasExistingBase() bool {
	for _, c := range r.candidates {
		if c.Kind == ConfigCandidateBase || c.Kind == ConfigCandidateAmbient {
			if c.Exists {
				return true
			}
		}
	}
	return false
}

// addDir resolves the config file named by one of names inside dir. When found, its atmos.d
// candidates are added first and the base file after (it is applied last). Missing files are
// reported only when explicit is true.
func (r *candidateResolver) addDir(dir, source string, explicit, ambient bool, names ...string) {
	withAtmosD := source != systemSource
	kind := ConfigCandidateBase
	if ambient {
		kind = ConfigCandidateAmbient
	}

	file := ""
	for _, name := range names {
		if f, err := findConfigFile(dir, name); err == nil {
			file = f
			break
		}
	}
	if file == "" {
		if explicit {
			r.add(ConfigCandidate{Path: filepath.Join(dir, AtmosConfigFileName), Kind: kind, Source: source})
		}
		return
	}

	// System config is merged without imports, so no atmos.d is read for it.
	if withAtmosD {
		r.addAtmosD(dir)
	}
	r.add(ConfigCandidate{Path: file, Kind: kind, Source: source, Exists: true})
	r.configDirs = append(r.configDirs, dir)
}

// addAtmosD adds the files of atmos.d/.atmos.d for a config directory, git root's first (mergeDefaultImports).
func (r *candidateResolver) addAtmosD(dir string) {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() || shouldExcludePathForTesting(dir) {
		return
	}
	if gitRoot := atmosDGitRootDir(dir, r.wd); gitRoot != "" {
		r.addAtmosDDirs(gitRoot, "(git root)")
	}
	r.addAtmosDDirs(dir, "")
}

func (r *candidateResolver) addAtmosDDirs(dir, suffix string) {
	for _, name := range []string{AtmosDefaultImportsDirName, DotAtmosDefaultImportsDirName} {
		d := atmosDDirPath(dir, name)
		if info, err := os.Stat(d); err != nil || !info.IsDir() {
			continue
		}
		files, err := SearchAtmosConfig(filepath.Join(d, "**", "*"))
		if err != nil {
			continue
		}
		source := name
		if suffix != "" {
			source += " " + suffix
		}
		for _, f := range files {
			r.add(ConfigCandidate{Path: f, Kind: ConfigCandidateAtmosD, Source: source, Exists: true})
		}
	}
}

// addProfiles mirrors loadProfiles for the active profiles only.
func (r *candidateResolver) addProfiles(o *candidateOptions) error {
	if len(o.profiles) == 0 {
		return nil
	}
	cfg := &schema.AtmosConfiguration{
		CliConfigPath:             r.profileConfigPath(),
		ProfilesBasePathConfigDir: o.profilesBasePathConfigDir,
	}
	cfg.Profiles.BasePath = o.profilesBasePath
	locations, err := discoverProfileLocations(cfg)
	if err != nil {
		return err
	}
	for _, name := range o.profiles {
		if name == "" || name == ProfileFlagSelectValue {
			continue
		}
		r.addProfile(name, locations)
	}
	return nil
}

// addProfile adds the files of one profile, or a single non-existent candidate when it cannot be found.
func (r *candidateResolver) addProfile(name string, locations []ProfileLocation) {
	dir, locType, err := findProfileDirectory(name, locations)
	if err != nil {
		// locations is sorted by precedence by findProfileDirectory; report the preferred spot.
		missing := filepath.Join(locations[0].Path, name)
		if abs, absErr := absoluteFrom(r.wd, missing); absErr == nil {
			missing = abs
		}
		r.add(ConfigCandidate{Path: missing, Kind: ConfigCandidateProfile, Source: "profile '" + name + "' (not found)", Profile: name})
		return
	}
	absDir, absErr := absoluteFrom(r.wd, dir)
	if absErr != nil {
		absDir = dir
	}
	files, err := SearchAtmosConfig(filepath.Join(absDir, "**", "*"))
	if err != nil {
		return
	}
	for _, f := range files {
		r.add(ConfigCandidate{Path: f, Kind: ConfigCandidateProfile, Source: "profile '" + name + "' (" + locType + ")", Profile: name, Exists: true})
	}
}

// profileConfigPath mirrors atmosConfig.CliConfigPath: the ";"-joined directories for --config /
// --config-path selections, or the directory of the last config merged otherwise.
func (r *candidateResolver) profileConfigPath() string {
	if len(r.configDirs) == 0 {
		return ""
	}
	if r.selected {
		return connectPaths(r.configDirs)
	}
	return r.configDirs[len(r.configDirs)-1]
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dedupeCandidates drops repeated paths (e.g. ATMOS_CLI_CONFIG_PATH pointing at the working
// directory), keeping the last occurrence because it has the highest precedence.
func dedupeCandidates(in []ConfigCandidate) []ConfigCandidate {
	last := make(map[string]int, len(in))
	for i, c := range in {
		last[c.Path] = i
	}
	out := make([]ConfigCandidate, 0, len(in))
	for i, c := range in {
		if last[c.Path] == i {
			out = append(out, c)
		}
	}
	return out
}

// candidateWorkingDir returns the absolute working directory the resolver is anchored to.
func candidateWorkingDir(dir string) (string, error) {
	base, err := osGetwd()
	if dir == "" {
		return base, err
	}
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir), nil
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(base, dir), nil
}
