package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestParseFlagValue tests the parseFlagValue helper function.
func TestParseFlagValue(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		arg       string
		args      []string
		index     int
		wantValue string
		wantFound bool
		wantErr   bool
	}{
		{
			name:      "space-separated: matches and returns next arg",
			flag:      "--foo",
			arg:       "--foo",
			args:      []string{"--foo", "bar"},
			index:     0,
			wantValue: "bar",
			wantFound: true,
		},
		{
			name:      "equals-separated: matches and returns value",
			flag:      "--foo",
			arg:       "--foo=bar",
			args:      []string{"--foo=bar"},
			index:     0,
			wantValue: "bar",
			wantFound: true,
		},
		{
			name:    "space-separated: missing value returns error",
			flag:    "--foo",
			arg:     "--foo",
			args:    []string{"--foo"},
			index:   0,
			wantErr: true,
		},
		{
			name:    "equals-separated: multiple equals returns error",
			flag:    "--foo",
			arg:     "--foo=a=b",
			args:    []string{"--foo=a=b"},
			index:   0,
			wantErr: true,
		},
		{
			name:      "no match returns empty and false",
			flag:      "--foo",
			arg:       "--bar",
			args:      []string{"--bar", "baz"},
			index:     0,
			wantValue: "",
			wantFound: false,
		},
		{
			name:      "space-separated: index has next arg at different position",
			flag:      "--foo",
			arg:       "--foo",
			args:      []string{"plan", "--foo", "val"},
			index:     1,
			wantValue: "val",
			wantFound: true,
		},
		{
			name:    "space-separated: flag at last position errors",
			flag:    "--foo",
			arg:     "--foo",
			args:    []string{"plan", "--foo"},
			index:   1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found, err := parseFlagValue(tt.flag, tt.arg, tt.args, tt.index)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantValue, value)
		})
	}
}

// TestParseIdentityFlag tests the parseIdentityFlag helper function directly.
func TestParseIdentityFlag(t *testing.T) {
	tests := []struct {
		name             string
		arg              string
		args             []string
		index            int
		expectedIdentity string
		wantErr          bool
	}{
		{
			name:             "exact flag without next arg uses SELECT",
			arg:              cfg.IdentityFlag,
			args:             []string{cfg.IdentityFlag},
			index:            0,
			expectedIdentity: cfg.IdentityFlagSelectValue,
		},
		{
			name:             "exact flag with next arg being another flag uses SELECT",
			arg:              cfg.IdentityFlag,
			args:             []string{cfg.IdentityFlag, "--other-flag"},
			index:            0,
			expectedIdentity: cfg.IdentityFlagSelectValue,
		},
		{
			name:             "exact flag followed by non-flag value sets value",
			arg:              cfg.IdentityFlag,
			args:             []string{cfg.IdentityFlag, "my-identity"},
			index:            0,
			expectedIdentity: "my-identity",
		},
		{
			name:             "equals form with value sets value",
			arg:              cfg.IdentityFlag + "=my-identity",
			args:             []string{cfg.IdentityFlag + "=my-identity"},
			index:            0,
			expectedIdentity: "my-identity",
		},
		{
			name:             "equals form with empty value uses SELECT",
			arg:              cfg.IdentityFlag + "=",
			args:             []string{cfg.IdentityFlag + "="},
			index:            0,
			expectedIdentity: cfg.IdentityFlagSelectValue,
		},
		{
			name:    "equals form with multiple equals returns error",
			arg:     cfg.IdentityFlag + "=a=b",
			args:    []string{cfg.IdentityFlag + "=a=b"},
			index:   0,
			wantErr: true,
		},
		{
			name:             "non-matching arg does not modify identity",
			arg:              "--other-flag",
			args:             []string{"--other-flag", "val"},
			index:            0,
			expectedIdentity: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info schema.ArgsAndFlagsInfo
			err := parseIdentityFlag(&info, tt.arg, tt.args, tt.index)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedIdentity, info.Identity)
		})
	}
}

// TestParseFromPlanFlag tests the parseFromPlanFlag helper function directly.
func TestParseFromPlanFlag(t *testing.T) {
	tests := []struct {
		name                 string
		arg                  string
		args                 []string
		index                int
		expectedUsePlan      bool
		expectedPlanFile     string
	}{
		{
			name:            "exact flag alone enables plan mode without path",
			arg:             cfg.FromPlanFlag,
			args:            []string{cfg.FromPlanFlag},
			index:           0,
			expectedUsePlan: true,
			expectedPlanFile: "",
		},
		{
			name:             "exact flag followed by non-flag path uses path",
			arg:              cfg.FromPlanFlag,
			args:             []string{cfg.FromPlanFlag, "my-plan.tfplan"},
			index:            0,
			expectedUsePlan:  true,
			expectedPlanFile: "my-plan.tfplan",
		},
		{
			name:            "exact flag followed by another flag uses no path",
			arg:             cfg.FromPlanFlag,
			args:            []string{cfg.FromPlanFlag, "--dry-run"},
			index:           0,
			expectedUsePlan: true,
			expectedPlanFile: "",
		},
		{
			name:             "equals form with path sets plan mode and path",
			arg:              cfg.FromPlanFlag + "=my-plan.tfplan",
			args:             []string{cfg.FromPlanFlag + "=my-plan.tfplan"},
			index:            0,
			expectedUsePlan:  true,
			expectedPlanFile: "my-plan.tfplan",
		},
		{
			name:            "equals form with empty path enables plan mode without path",
			arg:             cfg.FromPlanFlag + "=",
			args:            []string{cfg.FromPlanFlag + "="},
			index:           0,
			expectedUsePlan: true,
			expectedPlanFile: "",
		},
		{
			name:            "non-matching arg does not modify fields",
			arg:             "--other-flag",
			args:            []string{"--other-flag", "val"},
			index:           0,
			expectedUsePlan: false,
			expectedPlanFile: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info schema.ArgsAndFlagsInfo
			parseFromPlanFlag(&info, tt.arg, tt.args, tt.index)
			assert.Equal(t, tt.expectedUsePlan, info.UseTerraformPlan)
			assert.Equal(t, tt.expectedPlanFile, info.PlanFile)
		})
	}
}

// TestProcessArgsAndFlags_AllStringFlagsDefs tests that every stringFlagDef entry correctly
// sets the corresponding ArgsAndFlagsInfo field using both space-separated and equals-separated forms.
func TestProcessArgsAndFlags_AllStringFlagsDefs(t *testing.T) {
	tests := []struct {
		name              string
		componentType     string
		inputArgsAndFlags []string
		want              schema.ArgsAndFlagsInfo
	}{
		// HelmfileCommandFlag.
		{
			name:              "helmfile-command space form",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "--helmfile-command", "helmfile3"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "sync", HelmfileCommand: "helmfile3"},
		},
		{
			name:              "helmfile-command equals form",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "nginx", "--helmfile-command=helmfile3"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "sync", ComponentFromArg: "nginx", HelmfileCommand: "helmfile3"},
		},
		// HelmfileDirFlag.
		{
			name:              "helmfile-dir equals form",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "nginx", "--helmfile-dir=/charts"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "sync", ComponentFromArg: "nginx", HelmfileDir: "/charts"},
		},
		// CliConfigDirFlag.
		{
			name:              "config-dir equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--config-dir=/etc/atmos"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", ConfigDir: "/etc/atmos"},
		},
		// StackDirFlag.
		{
			name:              "stacks-dir equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--stacks-dir=/stacks"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", StacksDir: "/stacks"},
		},
		// BasePathFlag.
		{
			name:              "base-path equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--base-path=/repo"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", BasePath: "/repo"},
		},
		// VendorBasePathFlag.
		{
			name:              "vendor-base-path equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--vendor-base-path=/vendor"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", VendorBasePath: "/vendor"},
		},
		// AutoGenerateBackendFileFlag.
		{
			name:              "auto-generate-backend-file equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--auto-generate-backend-file=true"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", AutoGenerateBackendFile: "true"},
		},
		// WorkflowDirFlag.
		{
			name:              "workflows-dir equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--workflows-dir=/workflows"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", WorkflowsDir: "/workflows"},
		},
		// InitRunReconfigure.
		{
			name:              "init-run-reconfigure equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--init-run-reconfigure=true"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", InitRunReconfigure: "true"},
		},
		// JsonSchemaDirFlag.
		{
			name:              "schemas-jsonschema-dir equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--schemas-jsonschema-dir=/schemas"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", JsonSchemaDir: "/schemas"},
		},
		// OpaDirFlag.
		{
			name:              "schemas-opa-dir equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--schemas-opa-dir=/opa"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", OpaDir: "/opa"},
		},
		// CueDirFlag.
		{
			name:              "schemas-cue-dir equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--schemas-cue-dir=/cue"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", CueDir: "/cue"},
		},
		// AtmosManifestJsonSchemaFlag.
		{
			name:              "schemas-atmos-manifest equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--schemas-atmos-manifest=/manifest.json"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", AtmosManifestJsonSchema: "/manifest.json"},
		},
		// RedirectStdErrFlag.
		{
			name:              "redirect-stderr equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--redirect-stderr=/tmp/stderr.log"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", RedirectStdErr: "/tmp/stderr.log"},
		},
		// LogsFileFlag.
		{
			name:              "logs-file equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--logs-file=/tmp/atmos.log"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", LogsFile: "/tmp/atmos.log"},
		},
		// SettingsListMergeStrategyFlag.
		// Note: --settings-list-merge-strategy is NOT in commonFlags, so it is NOT stripped from
		// AdditionalArgsAndFlags; it appears in both the parsed field and the pass-through args.
		{
			name:              "settings-list-merge-strategy equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--settings-list-merge-strategy=append"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand:                "plan",
				ComponentFromArg:          "vpc",
				SettingsListMergeStrategy: "append",
				AdditionalArgsAndFlags:    []string{"--settings-list-merge-strategy=append"},
			},
		},
		// QueryFlag.
		{
			name:              "query equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--query=.tags"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", Query: ".tags"},
		},
		// PlanFileFlag.
		{
			name:              "planfile equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--planfile=plan.tfplan"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand:       "plan",
				ComponentFromArg: "vpc",
				PlanFile:         "plan.tfplan",
				UseTerraformPlan: true,
			},
		},
		// PlanFileFlag space form (also sets UseTerraformPlan).
		{
			name:              "planfile space form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--planfile", "plan.tfplan"},
			want: schema.ArgsAndFlagsInfo{
				SubCommand:       "plan",
				ComponentFromArg: "vpc",
				PlanFile:         "plan.tfplan",
				UseTerraformPlan: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags(tt.componentType, tt.inputArgsAndFlags)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestProcessArgsAndFlags_BooleanFlags tests that boolean CLI flags are correctly parsed.
func TestProcessArgsAndFlags_BooleanFlags(t *testing.T) {
	tests := []struct {
		name              string
		componentType     string
		inputArgsAndFlags []string
		checkFn           func(t *testing.T, got schema.ArgsAndFlagsInfo)
	}{
		{
			name:              "--dry-run sets DryRun=true",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--dry-run"},
			checkFn: func(t *testing.T, got schema.ArgsAndFlagsInfo) {
				t.Helper()
				assert.True(t, got.DryRun)
			},
		},
		{
			name:              "--skip-init sets SkipInit=true",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--skip-init"},
			checkFn: func(t *testing.T, got schema.ArgsAndFlagsInfo) {
				t.Helper()
				assert.True(t, got.SkipInit)
			},
		},
		{
			name:              "-h sets NeedHelp=true",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "-h"},
			checkFn: func(t *testing.T, got schema.ArgsAndFlagsInfo) {
				t.Helper()
				assert.True(t, got.NeedHelp)
			},
		},
		{
			name:              "--help sets NeedHelp=true",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--help"},
			checkFn: func(t *testing.T, got schema.ArgsAndFlagsInfo) {
				t.Helper()
				assert.True(t, got.NeedHelp)
			},
		},
		{
			name:              "--affected sets Affected=true",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--affected"},
			checkFn: func(t *testing.T, got schema.ArgsAndFlagsInfo) {
				t.Helper()
				assert.True(t, got.Affected)
			},
		},
		{
			name:              "--all sets All=true",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--all"},
			checkFn: func(t *testing.T, got schema.ArgsAndFlagsInfo) {
				t.Helper()
				assert.True(t, got.All)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags(tt.componentType, tt.inputArgsAndFlags)
			require.NoError(t, err)
			tt.checkFn(t, got)
		})
	}
}

// TestProcessArgsAndFlags_GlobalOptions tests global options flag handling including second-pass collection.
func TestProcessArgsAndFlags_GlobalOptions(t *testing.T) {
	tests := []struct {
		name              string
		componentType     string
		inputArgsAndFlags []string
		wantGlobalOptions []string
	}{
		{
			name:              "global-options space form collects options",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "nginx", "--global-options", "--log-level=debug --no-color"},
			wantGlobalOptions: []string{"--log-level=debug", "--no-color"},
		},
		{
			name:              "global-options equals form collects options",
			componentType:     "helmfile",
			inputArgsAndFlags: []string{"sync", "nginx", "--global-options=--log-level=debug --no-color"},
			wantGlobalOptions: []string{"--log-level=debug", "--no-color"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags(tt.componentType, tt.inputArgsAndFlags)
			require.NoError(t, err)
			assert.Equal(t, tt.wantGlobalOptions, got.GlobalOptions)
		})
	}
}

// TestProcessArgsAndFlags_NeedHelp tests the NeedHelp flag handling and subcommand extraction.
func TestProcessArgsAndFlags_NeedHelp(t *testing.T) {
	tests := []struct {
		name              string
		inputArgsAndFlags []string
		wantSubCommand    string
		wantNeedHelp      bool
	}{
		{
			name:              "--help with remaining args uses first remaining arg as SubCommand",
			inputArgsAndFlags: []string{"--help", "plan", "vpc"},
			wantSubCommand:    "vpc",
			wantNeedHelp:      true,
		},
		{
			name:              "--help alone leaves SubCommand empty",
			inputArgsAndFlags: []string{"--help", "plan"},
			wantSubCommand:    "",
			wantNeedHelp:      true,
		},
		{
			name:              "-h with remaining arg uses first remaining arg as SubCommand",
			inputArgsAndFlags: []string{"-h", "plan", "vpc"},
			wantSubCommand:    "vpc",
			wantNeedHelp:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags("terraform", tt.inputArgsAndFlags)
			require.NoError(t, err)
			assert.True(t, got.NeedHelp)
			assert.Equal(t, tt.wantSubCommand, got.SubCommand)
		})
	}
}

// TestProcessArgsAndFlags_EmptyAfterFlagRemoval tests that when all args are flags
// (removed during processing), additionalArgsAndFlags is empty and info is returned correctly.
func TestProcessArgsAndFlags_EmptyAfterFlagRemoval(t *testing.T) {
	// All these args are atmos flags that get stripped.
	// After removal, additionalArgsAndFlags is empty, so info is returned as-is.
	inputArgsAndFlags := []string{"--logs-level", "Debug", "--logs-file", "/tmp/atmos.log"}

	got, err := processArgsAndFlags("terraform", inputArgsAndFlags)
	require.NoError(t, err)
	assert.Equal(t, "Debug", got.LogsLevel)
	assert.Equal(t, "/tmp/atmos.log", got.LogsFile)
	assert.Empty(t, got.SubCommand)
	assert.Empty(t, got.AdditionalArgsAndFlags)
}

// TestProcessArgsAndFlags_FromPlan tests all forms of the --from-plan flag.
func TestProcessArgsAndFlags_FromPlan(t *testing.T) {
	tests := []struct {
		name              string
		inputArgsAndFlags []string
		wantUsePlan       bool
		wantPlanFile      string
		wantSubCommand    string
	}{
		{
			name:              "--from-plan alone enables terraform plan mode",
			inputArgsAndFlags: []string{"apply", "vpc", "--from-plan"},
			wantUsePlan:       true,
			wantPlanFile:      "",
			wantSubCommand:    "apply",
		},
		{
			name:              "--from-plan=path uses specified planfile",
			inputArgsAndFlags: []string{"apply", "vpc", "--from-plan=plan.tfplan"},
			wantUsePlan:       true,
			wantPlanFile:      "plan.tfplan",
			wantSubCommand:    "apply",
		},
		{
			name:              "--from-plan= (empty equals) enables plan mode without path",
			inputArgsAndFlags: []string{"apply", "vpc", "--from-plan="},
			wantUsePlan:       true,
			wantPlanFile:      "",
			wantSubCommand:    "apply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags("terraform", tt.inputArgsAndFlags)
			require.NoError(t, err)
			assert.Equal(t, tt.wantUsePlan, got.UseTerraformPlan)
			assert.Equal(t, tt.wantPlanFile, got.PlanFile)
			assert.Equal(t, tt.wantSubCommand, got.SubCommand)
		})
	}
}

// TestProcessArgsAndFlags_IdentityError tests that --identity=a=b returns an error.
func TestProcessArgsAndFlags_IdentityError(t *testing.T) {
	// Multiple equals in --identity flag should be an error.
	_, err := processArgsAndFlags("terraform", []string{"plan", "vpc", "--identity=a=b"})
	assert.Error(t, err)
	assert.ErrorContains(t, err, "--identity=a=b")
}

// TestProcessArgsAndFlags_SingleCommandError tests that processSingleCommand error propagates.
func TestProcessArgsAndFlags_SingleCommandError(t *testing.T) {
	// "--" is a valid flag prefix trigger but invalid as a flag (only "--" with no name).
	// This causes processSingleCommand to return an error.
	_, err := processArgsAndFlags("terraform", []string{"plan", "--"})
	assert.Error(t, err)
}
