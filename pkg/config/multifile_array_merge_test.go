package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLoadConfigFromCLIArgs_ArrayFieldMergeAcrossFiles reproduces cloudposse/atmos#2867's
// exact three-scenario matrix: a second --config file setting the SAME, a SUPERSET, or a
// completely DISJOINT value for stacks.included_paths (an array-typed key) relative to the
// first file. Only `commands` gets a manual merge workaround in mergeConfigFile(); every
// other array-typed key -- including stacks.included_paths -- goes through plain
// v.MergeConfig(). This asserts what the FINAL merged atmosConfig.Stacks.IncludedPaths
// actually is for each case, to establish ground truth before designing a fix.
func TestLoadConfigFromCLIArgs_ArrayFieldMergeAcrossFiles(t *testing.T) {
	tests := []struct {
		name             string
		fragmentIncluded string
		wantIncluded     []string
	}{
		{
			name:             "identical value",
			fragmentIncluded: `["deploy/**/*"]`,
			wantIncluded:     []string{"deploy/**/*"},
		},
		{
			name:             "superset value (still contains original)",
			fragmentIncluded: `["deploy/**/*", "other/**/*"]`,
			wantIncluded:     []string{"deploy/**/*", "other/**/*"},
		},
		{
			name:             "disjoint value",
			fragmentIncluded: `["other/**/*"]`,
			wantIncluded:     []string{"other/**/*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			mainFile := filepath.Join(tmpDir, "main.yaml")
			fragmentFile := filepath.Join(tmpDir, "fragment.yaml")

			require.NoError(t, os.WriteFile(mainFile, []byte(`
base_path: "."
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
components:
  terraform:
    base_path: "components/terraform"
`), 0o644))

			require.NoError(t, os.WriteFile(fragmentFile, []byte(`
stacks:
  included_paths: `+tt.fragmentIncluded+`
`), 0o644))

			v := viper.New()
			v.SetConfigType("yaml")

			configAndStacksInfo := &schema.ConfigAndStacksInfo{
				AtmosConfigFilesFromArg: []string{mainFile, fragmentFile},
			}

			var atmosConfig schema.AtmosConfiguration
			err := loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
			require.NoError(t, err)

			assert.Equal(t, tt.wantIncluded, atmosConfig.Stacks.IncludedPaths,
				"the second --config file's value for stacks.included_paths must be what atmos "+
					"actually uses -- last file wins, matching the documented/expected 'later file "+
					"overrides earlier' semantics")
		})
	}
}

// TestLoadConfigFromCLIArgs_ArrayFieldMerge_IntermediateStates instruments each stage of
// mergeFiles (the function --config a.yaml,b.yaml actually goes through) to pinpoint exactly
// where a superset stacks.included_paths value from the second file diverges from what ends
// up in the final unmarshaled atmosConfig -- since static tracing of viper's own MergeConfig
// suggests a bare merge call should already fully replace (not revert) the slice.
func TestLoadConfigFromCLIArgs_ArrayFieldMerge_IntermediateStates(t *testing.T) {
	tmpDir := t.TempDir()
	mainFile := filepath.Join(tmpDir, "main.yaml")
	fragmentFile := filepath.Join(tmpDir, "fragment.yaml")

	require.NoError(t, os.WriteFile(mainFile, []byte(`
base_path: "."
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
components:
  terraform:
    base_path: "components/terraform"
`), 0o644))

	require.NoError(t, os.WriteFile(fragmentFile, []byte(`
stacks:
  included_paths:
    - "deploy/**/*"
    - "other/**/*"
`), 0o644))

	v := viper.New()
	v.SetConfigType("yaml")
	setDefaultConfiguration(v)
	require.NoError(t, loadEmbeddedConfig(v))

	// Stage 1: after merging main.yaml alone.
	require.NoError(t, mergeConfigFile(mainFile, v))
	require.Equal(t, []interface{}{"deploy/**/*"}, v.Get("stacks.included_paths"),
		"stage 1 (after main.yaml): sanity check on the starting value")

	// Stage 2: after merging fragment.yaml on top (mergeConfigFile only, no mergeImports yet).
	// Plain v.MergeConfig() already replaces the whole slice with fragment.yaml's superset here
	// -- if a regression made this stage revert to main.yaml's original single-element value (or
	// anything else), this must fail here rather than only at the final stage 4 assertion below.
	require.NoError(t, mergeConfigFile(fragmentFile, v))
	afterMergeConfigFile := v.Get("stacks.included_paths")
	require.Equal(t, []interface{}{"deploy/**/*", "other/**/*"}, afterMergeConfigFile,
		"stage 2 (after mergeConfigFile(fragment.yaml)): the raw viper value")

	// Stage 3: after mergeImports runs (a no-op for files with no `import:` key, per
	// processConfigImportsWithFSAndBasePathSource's early return). Asserting stage 2's exact
	// value against stage 3's confirms that no-op claim directly, instead of only logging it.
	_, err := mergeImports(v, tmpDir, "", "")
	require.NoError(t, err)
	afterMergeImports := v.Get("stacks.included_paths")
	require.Equal(t, afterMergeConfigFile, afterMergeImports,
		"stage 3 (after mergeImports): must be unchanged from stage 2 -- mergeImports is a no-op here")

	// Stage 4: after the final v.Unmarshal into the typed struct (what loadConfigFromCLIArgs
	// itself does last).
	var atmosConfig schema.AtmosConfiguration
	require.NoError(t, v.Unmarshal(&atmosConfig, atmosDecodeHook()))
	t.Logf("stage 4 (after v.Unmarshal): %#v", atmosConfig.Stacks.IncludedPaths)

	assert.Equal(t, []string{"deploy/**/*", "other/**/*"}, atmosConfig.Stacks.IncludedPaths,
		"final unmarshaled value must match the superset fragment.yaml declared")
}

// TestInitCliConfig_ConfigMultiFileArraySupersetUsedDuringDiscovery reproduces the full
// end-to-end symptom from cloudposse/atmos#2867: `atmos --config main.yaml,fragment.yaml
// list stacks` reports "No stacks found" when fragment.yaml's stacks.included_paths is a
// superset of main.yaml's, even though the superset still contains the glob that covers the
// only stack manifest present.
func TestInitCliConfig_ConfigMultiFileArraySupersetUsedDuringDiscovery(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)

	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "stacks", "deploy"), 0o755))

	mainFile := filepath.Join(tempDir, "main.yaml")
	fragmentFile := filepath.Join(tempDir, "fragment.yaml")

	require.NoError(t, os.WriteFile(mainFile, []byte(`
base_path: ''
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths:
    - deploy/**/*
  name_pattern: '{stage}'
`), 0o644))

	require.NoError(t, os.WriteFile(fragmentFile, []byte(`
stacks:
  included_paths:
    - deploy/**/*
    - other/**/*
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "stacks", "deploy", "dev.yaml"), []byte(`
vars:
  stage: dev
components:
  terraform:
    my-component:
      vars: {}
`), 0o644))

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{mainFile, fragmentFile},
	}
	atmosConfig, err := InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err,
		"list stacks should discover dev.yaml under the superset included_paths, which still "+
			"contains the original deploy/**/* glob")
	assert.NotEmpty(t, atmosConfig.StackConfigFilesAbsolutePaths,
		"stack discovery must find dev.yaml -- it's covered by deploy/**/*, present in both files")
}

// TestInitCliConfig_SingleFileMultipleIncludedPathsOneEmpty reproduces the actual root cause
// behind cloudposse/atmos#2867's "No stacks found" symptom: it is NOT a config-file-merge bug
// (see TestLoadConfigFromCLIArgs_ArrayFieldMergeAcrossFiles above, which proves the merge
// itself is correct) -- it reproduces with a single atmos.yaml and no --config merging at
// all. When stacks.included_paths has multiple glob entries and one of them currently
// matches zero files (e.g. its directory doesn't exist yet), FindAllStackConfigsInPaths
// aborts discovery entirely instead of just skipping that one empty entry, discarding
// matches already found via the other entries.
func TestInitCliConfig_SingleFileMultipleIncludedPathsOneEmpty(t *testing.T) {
	setupTestAdapters()
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "stacks", "deploy"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(`
base_path: ''
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths:
    - deploy/**/*
    - other/**/*
  name_pattern: '{stage}'
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "stacks", "deploy", "dev.yaml"), []byte(`
vars:
  stage: dev
components:
  terraform:
    my-component:
      vars: {}
`), 0o644))

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err,
		"a single atmos.yaml with one included_paths entry that currently matches nothing "+
			"must not abort discovery of manifests covered by the other entries")
	assert.NotEmpty(t, atmosConfig.StackConfigFilesAbsolutePaths)
}

// TestInitCliConfig_ProfileAppliedOnTopOfConfigFlag reproduces a related bug found
// during the #2867/#2868 audit: loadConfigFromCLIArgs (the --config/--config-path path)
// returns immediately after merging the CLI-selected files, without ever reaching
// LoadConfig's profile-loading block. This means --config and --profile currently cannot be
// combined -- profile settings are silently ignored whenever --config/--config-path is used.
func TestInitCliConfig_ProfileAppliedOnTopOfConfigFlag(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	if orig, ok := os.LookupEnv("ATMOS_PROFILE"); ok {
		require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))
		t.Cleanup(func() { require.NoError(t, os.Setenv("ATMOS_PROFILE", orig)) })
	}

	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "profiles", "test"), 0o755))

	mainFile := filepath.Join(tempDir, "main.yaml")
	require.NoError(t, os.WriteFile(mainFile, []byte(`
base_path: .
profiles:
  base_path: profiles
stacks:
  base_path: stacks
  included_paths:
    - "deploy/**/*"
`), 0o644))

	// The profile overrides stacks.base_path to a directory the base config never mentions.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "profiles", "test", "atmos.yaml"), []byte(`
stacks:
  base_path: profile-stacks
`), 0o644))

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{mainFile},
		ProfilesFromArg:         []string{"test"},
	}
	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	require.NoError(t, err)

	assert.Equal(t, "profile-stacks", atmosConfig.Stacks.BasePath,
		"the profile's stacks.base_path override must be applied on top of the --config-selected "+
			"base, matching the documented precedence in docs/prd/atmos-profiles.md")
}

// TestInitCliConfig_ProfilesBasePathResolvesAgainstDeclaringFile reproduces a bug found during a
// field-test pass on cloudposse/atmos#2867/#2868: discoverProfileLocations resolved a relative
// `profiles.base_path` against the FIRST --config file's directory, regardless of which file
// actually declared it. Here, only the SECOND --config file (in a different directory) declares
// profiles.base_path, and the profile directory only exists relative to that second file's
// directory -- so the profile must still be found.
func TestInitCliConfig_ProfilesBasePathResolvesAgainstDeclaringFile(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	if orig, ok := os.LookupEnv("ATMOS_PROFILE"); ok {
		require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))
		t.Cleanup(func() { require.NoError(t, os.Setenv("ATMOS_PROFILE", orig)) })
	}

	viper.Reset()
	t.Cleanup(viper.Reset)

	mainFile := filepath.Join(tempDir, "main.yaml")
	require.NoError(t, os.WriteFile(mainFile, []byte(`
base_path: .
stacks:
  base_path: stacks
  included_paths:
    - "deploy/**/*"
`), 0o644))

	// fragment.yaml lives in a DIFFERENT directory than main.yaml, and is the SECOND --config
	// file. Only it declares profiles.base_path, relative to ITS OWN directory.
	fragmentDir := filepath.Join(tempDir, "region-overrides")
	require.NoError(t, os.MkdirAll(fragmentDir, 0o755))
	fragmentFile := filepath.Join(fragmentDir, "fragment.yaml")
	require.NoError(t, os.WriteFile(fragmentFile, []byte(`
profiles:
  base_path: ./custom-profiles
`), 0o644))

	// The profile directory only exists relative to fragmentDir, NOT relative to tempDir (where
	// main.yaml, the first --config file, lives) -- proving resolution uses the declaring file's
	// directory, not just the first --config file's directory.
	profileDir := filepath.Join(fragmentDir, "custom-profiles", "test")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "atmos.yaml"), []byte(`
stacks:
  base_path: profile-stacks
`), 0o644))

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{mainFile, fragmentFile},
		ProfilesFromArg:         []string{"test"},
	}
	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	require.NoError(t, err)

	assert.Equal(t, "profile-stacks", atmosConfig.Stacks.BasePath,
		"the profile must be found relative to fragment.yaml's directory (where profiles.base_path "+
			"was actually declared), not main.yaml's directory (just the first --config file)")
}

// TestInitCliConfig_MultipleConfigFilesBasePathResolves reproduces a field-tested regression:
// connectPaths joins every --config file's directory into a ";"-delimited CliConfigPath string
// ("dirA;dirB;") once 2+ files are given. Before BasePathConfigDir existed,
// AtmosConfigAbsolutePaths joined a dot-prefixed base_path directly against that malformed
// string, producing a nonexistent path -- so stack discovery silently found nothing (`list
// stacks` printed "No stacks found" with no error at all). Both --config files here live in the
// SAME directory (the simplest case that still corrupts the naive ";"-join), and base_path is
// the common explicit "." convention.
func TestInitCliConfig_MultipleConfigFilesBasePathResolves(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "stacks", "deploy"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "components", "terraform"), 0o755))

	mainFile := filepath.Join(tempDir, "main.yaml")
	require.NoError(t, os.WriteFile(mainFile, []byte(`
base_path: "./"
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths:
    - "deploy/**/*"
  name_template: "{{.vars.tenant}}-{{.vars.environment}}-{{.vars.stage}}"
`), 0o644))

	fragmentFile := filepath.Join(tempDir, "fragment.yaml")
	require.NoError(t, os.WriteFile(fragmentFile, []byte(`
logs:
  level: Debug
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "stacks", "deploy", "test.yaml"), []byte(`
vars:
  tenant: acme
  environment: ue1
  stage: test
components:
  terraform:
    test-component:
      vars: {}
`), 0o644))

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{mainFile, fragmentFile},
	}
	atmosConfig, err := InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err,
		"a dot-prefixed base_path must still resolve correctly once a SECOND --config file "+
			"joins CliConfigPath into a multi-directory string -- it must not silently corrupt "+
			"stack discovery")
	assert.NotEmpty(t, atmosConfig.StackConfigFilesAbsolutePaths,
		"the real stack manifest must be found; an empty result here means base_path resolved "+
			"to a nonexistent path built from the raw \";\"-joined CliConfigPath")
}

// TestInitCliConfig_MultipleConfigPathDirsBasePathResolves is the --config-path analogue of
// TestInitCliConfig_MultipleConfigFilesBasePathResolves: mergeConfigFromDirectories builds the
// same ";"-joined CliConfigPath as mergeFiles does, so the same corruption applies when 2+
// --config-path directories are given.
func TestInitCliConfig_MultipleConfigPathDirsBasePathResolves(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	dirX := filepath.Join(tempDir, "dirX")
	dirY := filepath.Join(tempDir, "dirY")
	require.NoError(t, os.MkdirAll(filepath.Join(dirX, "stacks", "deploy"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dirX, "components", "terraform"), 0o755))
	require.NoError(t, os.MkdirAll(dirY, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(dirX, "atmos.yaml"), []byte(`
base_path: "./"
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths:
    - "deploy/**/*"
  name_template: "{{.vars.tenant}}-{{.vars.environment}}-{{.vars.stage}}"
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(dirY, "atmos.yaml"), []byte(`
logs:
  level: Debug
`), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(dirX, "stacks", "deploy", "test.yaml"), []byte(`
vars:
  tenant: acme
  environment: ue1
  stage: test
components:
  terraform:
    test-component:
      vars: {}
`), 0o644))

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosConfigDirsFromArg: []string{dirX, dirY},
	}
	atmosConfig, err := InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err,
		"a dot-prefixed base_path must still resolve correctly once a SECOND --config-path "+
			"directory joins CliConfigPath into a multi-directory string")
	assert.NotEmpty(t, atmosConfig.StackConfigFilesAbsolutePaths,
		"the real stack manifest under dirX must be found; an empty result here means base_path "+
			"resolved to a nonexistent path built from the raw \";\"-joined CliConfigPath")
}

// TestInitCliConfig_ConfigPathProfilesBasePathResolvesAgainstDeclaringDir is the --config-path
// analogue of TestInitCliConfig_ProfilesBasePathResolvesAgainstDeclaringFile: a field-tested
// regression found that mergeConfigFromDirectories (the --config-path flag's merge path) never
// tracked which directory declared profiles.base_path -- only mergeFiles (--config) did. So a
// profiles.base_path declared in a NON-first --config-path directory fell back to the first
// directory and the profile was reported as "does not exist" even though it did. Here, only the
// SECOND --config-path directory declares profiles.base_path, and the profile only exists
// relative to that second directory.
func TestInitCliConfig_ConfigPathProfilesBasePathResolvesAgainstDeclaringDir(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	if orig, ok := os.LookupEnv("ATMOS_PROFILE"); ok {
		require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))
		t.Cleanup(func() { require.NoError(t, os.Setenv("ATMOS_PROFILE", orig)) })
	}
	viper.Reset()
	t.Cleanup(viper.Reset)

	// dirB is the FIRST --config-path directory (the naive "primary" fallback) and declares no
	// profiles.base_path.
	dirB := filepath.Join(tempDir, "dirB")
	require.NoError(t, os.MkdirAll(dirB, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dirB, "atmos.yaml"), []byte(`
base_path: "./"
stacks:
  base_path: stacks
  included_paths:
    - "deploy/**/*"
`), 0o644))

	// dirA is the SECOND --config-path directory and is the only one that declares
	// profiles.base_path, relative to ITS OWN directory.
	dirA := filepath.Join(tempDir, "dirA")
	require.NoError(t, os.MkdirAll(dirA, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dirA, "atmos.yaml"), []byte(`
profiles:
  base_path: ./myprofiles
`), 0o644))

	// The profile directory only exists relative to dirA, NOT relative to dirB (the first
	// --config-path directory) -- proving resolution uses the declaring directory, not just the
	// first --config-path directory.
	profileDir := filepath.Join(dirA, "myprofiles", "testprof")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "atmos.yaml"), []byte(`
logs:
  level: Trace
`), 0o644))

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosConfigDirsFromArg: []string{dirB, dirA},
		ProfilesFromArg:        []string{"testprof"},
	}
	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	require.NoError(t, err,
		"the profile must be found relative to dirA (where profiles.base_path was actually "+
			"declared), not dirB's directory (just the first --config-path directory)")
	assert.Equal(t, "Trace", atmosConfig.Logs.Level,
		"the profile's logs.level override must be applied, proving it was found")
}

// TestDeclaresProfilesBasePath covers declaresProfilesBasePath's branches directly: malformed
// YAML, no top-level mapping, no profiles key, profiles present but not itself a mapping,
// profiles a mapping without base_path, and the true-positive case.
func TestDeclaresProfilesBasePath(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
		wantErr bool
	}{
		{
			name:    "malformed YAML returns error",
			content: "profiles:\n  base_path: [unterminated\n",
			wantErr: true,
		},
		{
			name:    "empty content has no top-level mapping",
			content: "",
			want:    false,
		},
		{
			name:    "no profiles key at all",
			content: "logs:\n  level: Info\n",
			want:    false,
		},
		{
			name:    "profiles key present but not a mapping",
			content: "profiles: not-a-mapping\n",
			want:    false,
		},
		{
			name:    "profiles is a mapping without base_path",
			content: "profiles:\n  default: developer\n",
			want:    false,
		},
		{
			name:    "profiles declares base_path",
			content: "profiles:\n  base_path: ./custom-profiles\n",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := declaresProfilesBasePath([]byte(tt.content))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
