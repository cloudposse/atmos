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
			name:      "equals-separated: value containing '=' is handled correctly",
			flag:      "--foo",
			arg:       "--foo=a=b",
			args:      []string{"--foo=a=b"},
			index:     0,
			wantValue: "a=b",
			wantFound: true,
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
			name:             "equals form with value containing '=' is handled correctly",
			arg:              cfg.IdentityFlag + "=arn:aws:sts::123:assumed-role/MyRole/session",
			args:             []string{cfg.IdentityFlag + "=arn:aws:sts::123:assumed-role/MyRole/session"},
			index:            0,
			expectedIdentity: "arn:aws:sts::123:assumed-role/MyRole/session",
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
			parseIdentityFlag(&info, tt.arg, tt.args, tt.index)
			assert.Equal(t, tt.expectedIdentity, info.Identity)
		})
	}
}

// TestParseFromPlanFlag tests the parseFromPlanFlag helper function directly.
func TestParseFromPlanFlag(t *testing.T) {
	tests := []struct {
		name         string
		arg          string
		args         []string
		index        int
		wantUsePlan  bool
		wantPlanFile string
	}{
		{
			name:         "exact flag alone enables plan mode without path",
			arg:          cfg.FromPlanFlag,
			args:         []string{cfg.FromPlanFlag},
			index:        0,
			wantUsePlan:  true,
			wantPlanFile: "",
		},
		{
			name:         "exact flag followed by non-flag path uses path",
			arg:          cfg.FromPlanFlag,
			args:         []string{cfg.FromPlanFlag, "my-plan.tfplan"},
			index:        0,
			wantUsePlan:  true,
			wantPlanFile: "my-plan.tfplan",
		},
		{
			name:         "exact flag followed by another flag uses no path",
			arg:          cfg.FromPlanFlag,
			args:         []string{cfg.FromPlanFlag, "--dry-run"},
			index:        0,
			wantUsePlan:  true,
			wantPlanFile: "",
		},
		{
			name:         "equals form with path sets plan mode and path",
			arg:          cfg.FromPlanFlag + "=my-plan.tfplan",
			args:         []string{cfg.FromPlanFlag + "=my-plan.tfplan"},
			index:        0,
			wantUsePlan:  true,
			wantPlanFile: "my-plan.tfplan",
		},
		{
			name:         "equals form with empty path enables plan mode without path",
			arg:          cfg.FromPlanFlag + "=",
			args:         []string{cfg.FromPlanFlag + "="},
			index:        0,
			wantUsePlan:  true,
			wantPlanFile: "",
		},
		{
			name:         "non-matching arg does not modify fields",
			arg:          "--other-flag",
			args:         []string{"--other-flag", "val"},
			index:        0,
			wantUsePlan:  false,
			wantPlanFile: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info schema.ArgsAndFlagsInfo
			parseFromPlanFlag(&info, tt.arg, tt.args, tt.index)
			assert.Equal(t, tt.wantUsePlan, info.UseTerraformPlan)
			assert.Equal(t, tt.wantPlanFile, info.PlanFile)
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
		// Now that --settings-list-merge-strategy is in commonFlags it IS stripped from AdditionalArgsAndFlags.
		{
			name:              "settings-list-merge-strategy equals form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--settings-list-merge-strategy=append"},
			want:              schema.ArgsAndFlagsInfo{SubCommand: "plan", ComponentFromArg: "vpc", SettingsListMergeStrategy: "append"},
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

// TestProcessArgsAndFlags_BooleanFlagsDoNotStripNextArg verifies that purely boolean flags
// (--dry-run, --skip-init, --affected, --all, --process-templates, --process-functions,
// --profiler-enabled, --heatmap) do NOT strip the following argument from AdditionalArgsAndFlags.
//
// Before the fix, the stripping loop's else-branch unconditionally stripped i+1 for ALL
// commonFlags entries, silently dropping unrelated Terraform flags like --refresh=false
// when they appeared immediately after an Atmos boolean flag.
func TestProcessArgsAndFlags_BooleanFlagsDoNotStripNextArg(t *testing.T) {
	tests := []struct {
		name                    string
		inputArgsAndFlags       []string
		wantAdditionalArgsFlags []string
	}{
		{
			name:                    "--dry-run does not strip adjacent Terraform flag",
			inputArgsAndFlags:       []string{"plan", "vpc", "--dry-run", "--refresh=false", "--stack", "my-stack"},
			wantAdditionalArgsFlags: []string{"--refresh=false"},
		},
		{
			name:                    "--skip-init does not strip adjacent Terraform flag",
			inputArgsAndFlags:       []string{"plan", "vpc", "--skip-init", "--parallelism=10", "--stack", "my-stack"},
			wantAdditionalArgsFlags: []string{"--parallelism=10"},
		},
		{
			name:                    "--affected does not strip adjacent Terraform flag",
			inputArgsAndFlags:       []string{"plan", "vpc", "--affected", "--refresh=false", "--stack", "my-stack"},
			wantAdditionalArgsFlags: []string{"--refresh=false"},
		},
		{
			name:                    "--all does not strip adjacent Terraform flag",
			inputArgsAndFlags:       []string{"plan", "vpc", "--all", "--parallelism=5", "--stack", "my-stack"},
			wantAdditionalArgsFlags: []string{"--parallelism=5"},
		},
		{
			name:                    "--process-templates does not strip adjacent Terraform flag",
			inputArgsAndFlags:       []string{"plan", "vpc", "--process-templates", "--refresh=false", "--stack", "my-stack"},
			wantAdditionalArgsFlags: []string{"--refresh=false"},
		},
		{
			name:                    "--process-functions does not strip adjacent Terraform flag",
			inputArgsAndFlags:       []string{"plan", "vpc", "--process-functions", "--parallelism=10", "--stack", "my-stack"},
			wantAdditionalArgsFlags: []string{"--parallelism=10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags("terraform", tt.inputArgsAndFlags)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAdditionalArgsFlags, got.AdditionalArgsAndFlags)
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

// TestProcessArgsAndFlags_IdentityWithEqualsInValue tests that --identity=key=value is handled
// correctly (with SplitN fix — values containing '=' no longer cause an error).
func TestProcessArgsAndFlags_IdentityWithEqualsInValue(t *testing.T) {
	// Previously this would have returned an error. Now it should succeed.
	got, err := processArgsAndFlags("terraform", []string{"plan", "vpc", "--identity=user=admin"})
	require.NoError(t, err)
	assert.Equal(t, "user=admin", got.Identity)
}

// TestProcessArgsAndFlags_SingleCommandError tests that processSingleCommand error propagates.
func TestProcessArgsAndFlags_SingleCommandError(t *testing.T) {
	// "--" is a valid flag prefix trigger but invalid as a flag (only "--" with no name).
	// This causes processSingleCommand to return an error.
	_, err := processArgsAndFlags("terraform", []string{"plan", "--"})
	assert.Error(t, err)
}

// TestParseQuotedCompoundSubcommand_DefensiveCheck tests the defensive len(parts) != 2 guard.
// parseCompoundSubcommand (the normal caller) only invokes parseQuotedCompoundSubcommand when
// strings.Contains(arg, " ") is true, which guarantees SplitN yields 2 parts.  The defensive
// guard exists to protect against potential future callers that bypass that contract — and this
// test exercises it directly so that the guard line is covered and can never silently regress.
func TestParseQuotedCompoundSubcommand_DefensiveCheck(t *testing.T) {
	// Calling the function directly with a no-space string triggers the defensive guard.
	// SplitN("plan", " ", 2) returns []string{"plan"} (len 1), so the guard fires and nil is returned.
	result := parseQuotedCompoundSubcommand("plan")
	assert.Nil(t, result, "parseQuotedCompoundSubcommand should return nil for a no-space string")
}

// TestParseFlagValue_EqualsInValue verifies that flag values containing '=' are parsed correctly
// after the strings.SplitN fix. Previously, --query=.tags[?env==prod] would have errored.
func TestParseFlagValue_EqualsInValue(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		arg       string
		wantValue string
	}{
		{
			name:      "JMESPath query with double equals",
			flag:      "--query",
			arg:       "--query=.tags[?env==prod]",
			wantValue: ".tags[?env==prod]",
		},
		{
			name:      "key=value pair as flag value",
			flag:      "--append-user-agent",
			arg:       "--append-user-agent=Env=Production",
			wantValue: "Env=Production",
		},
		{
			name:      "multiple equals signs in value",
			flag:      "--redirect-stderr",
			arg:       "--redirect-stderr=key=val=extra",
			wantValue: "key=val=extra",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found, err := parseFlagValue(tt.flag, tt.arg, []string{tt.arg}, 0)
			require.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, tt.wantValue, value)
		})
	}
}

// TestProcessArgsAndFlags_FlagStripping verifies that atmos-specific flags AND their values are
// stripped from AdditionalArgsAndFlags (the pass-through args sent to Terraform/Helmfile).
// This covers the M1-M3 gaps identified in the CodeRabbit audit.
func TestProcessArgsAndFlags_FlagStripping(t *testing.T) {
	tests := []struct {
		name                    string
		componentType           string
		inputArgsAndFlags       []string
		wantAdditionalArgsFlags []string
	}{
		{
			name:                    "terraform-command space form strips flag and value",
			componentType:           "terraform",
			inputArgsAndFlags:       []string{"plan", "vpc", "--terraform-command", "tofu", "--stack", "my-stack"},
			wantAdditionalArgsFlags: nil,
		},
		{
			name:                    "terraform-dir space form strips flag and value",
			componentType:           "terraform",
			inputArgsAndFlags:       []string{"plan", "vpc", "--terraform-dir", "/my/tf", "--stack", "my-stack"},
			wantAdditionalArgsFlags: nil,
		},
		{
			name:                    "deploy-run-init space form strips flag and value",
			componentType:           "terraform",
			inputArgsAndFlags:       []string{"plan", "vpc", "--deploy-run-init", "false", "--stack", "my-stack"},
			wantAdditionalArgsFlags: nil,
		},
		{
			name:                    "append-user-agent space form strips flag and value",
			componentType:           "terraform",
			inputArgsAndFlags:       []string{"plan", "vpc", "--append-user-agent", "atmos/1.0", "--stack", "my-stack"},
			wantAdditionalArgsFlags: nil,
		},
		{
			name:                    "init-pass-vars space form strips flag and value",
			componentType:           "terraform",
			inputArgsAndFlags:       []string{"plan", "vpc", "--init-pass-vars", "true", "--stack", "my-stack"},
			wantAdditionalArgsFlags: nil,
		},
		{
			name:                    "skip-planfile space form strips flag and value",
			componentType:           "terraform",
			inputArgsAndFlags:       []string{"plan", "vpc", "--skip-planfile", "true", "--stack", "my-stack"},
			wantAdditionalArgsFlags: nil,
		},
		{
			// M2: from-plan with path - path should be stripped from pass-through args.
			name:                    "--from-plan path.tfplan strips both flag and path from pass-through",
			componentType:           "terraform",
			inputArgsAndFlags:       []string{"apply", "vpc", "--from-plan", "plan.tfplan", "--stack", "my-stack"},
			wantAdditionalArgsFlags: nil,
		},
		{
			// M3: identity with value - value should be stripped from pass-through args.
			name:                    "--identity my-identity strips both flag and value from pass-through",
			componentType:           "terraform",
			inputArgsAndFlags:       []string{"plan", "vpc", "--identity", "my-identity", "--stack", "my-stack"},
			wantAdditionalArgsFlags: nil,
		},
		{
			// settings-list-merge-strategy is now in commonFlags and should be stripped.
			name:                    "--settings-list-merge-strategy is stripped from pass-through",
			componentType:           "terraform",
			inputArgsAndFlags:       []string{"plan", "vpc", "--settings-list-merge-strategy", "append", "--stack", "my-stack"},
			wantAdditionalArgsFlags: nil,
		},
		{
			// Non-atmos flags like --refresh=false must not be stripped.
			name:                    "unknown non-atmos flag passes through unchanged",
			componentType:           "terraform",
			inputArgsAndFlags:       []string{"plan", "vpc", "--refresh=false", "--stack", "my-stack"},
			wantAdditionalArgsFlags: []string{"--refresh=false"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags(tt.componentType, tt.inputArgsAndFlags)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAdditionalArgsFlags, got.AdditionalArgsAndFlags)
		})
	}
}

// TestProcessArgsAndFlags_OptionalValueFlagStripping verifies the L2 fix: optional-value flags
// (--from-plan, --identity) in space form must not cause the NEXT arg to be stripped when that
// arg is actually a different flag (e.g., a Terraform flag like --refresh=false).
func TestProcessArgsAndFlags_OptionalValueFlagStripping(t *testing.T) {
	tests := []struct {
		name                    string
		inputArgsAndFlags       []string
		wantAdditionalArgsFlags []string
		wantUseTerraformPlan    bool
		wantIdentity            string
	}{
		{
			// --from-plan followed immediately by a Terraform flag: the Terraform flag must pass through.
			name:                    "--from-plan followed by Terraform flag does not strip the Terraform flag",
			inputArgsAndFlags:       []string{"plan", "vpc", "--from-plan", "--refresh=false", "--stack", "my-stack"},
			wantAdditionalArgsFlags: []string{"--refresh=false"},
			wantUseTerraformPlan:    true,
		},
		{
			// --identity without value followed by Terraform flag: Terraform flag must pass through.
			name:                    "--identity without value followed by Terraform flag does not strip it",
			inputArgsAndFlags:       []string{"plan", "vpc", "--identity", "--terraform-command=tofu", "--stack", "my-stack"},
			wantAdditionalArgsFlags: nil, // --terraform-command=tofu is consumed by stringFlagDefs and stripped
			wantIdentity:            cfg.IdentityFlagSelectValue,
		},
		{
			// --from-plan followed by planfile path: planfile must be stripped (consumed as value).
			name:                    "--from-plan followed by planfile path strips both",
			inputArgsAndFlags:       []string{"plan", "vpc", "--from-plan", "plan.tfplan", "--stack", "my-stack"},
			wantAdditionalArgsFlags: nil,
			wantUseTerraformPlan:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags("terraform", tt.inputArgsAndFlags)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAdditionalArgsFlags, got.AdditionalArgsAndFlags)
			if tt.wantUseTerraformPlan {
				assert.True(t, got.UseTerraformPlan)
			}
			if tt.wantIdentity != "" {
				assert.Equal(t, tt.wantIdentity, got.Identity)
			}
		})
	}
}

// TestProcessArgsAndFlags_AllStringFlagsSpaceForm verifies that the missing M4 flags all correctly
// parse in the space-separated form AND strip both the flag and value from AdditionalArgsAndFlags.
func TestProcessArgsAndFlags_AllStringFlagsSpaceForm(t *testing.T) {
	tests := []struct {
		name              string
		componentType     string
		inputArgsAndFlags []string
		checkField        func(got schema.ArgsAndFlagsInfo) string
		wantFieldValue    string
	}{
		{
			name:              "terraform-command space form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--terraform-command", "tofu", "--stack", "my-stack"},
			checkField:        func(got schema.ArgsAndFlagsInfo) string { return got.TerraformCommand },
			wantFieldValue:    "tofu",
		},
		{
			name:              "terraform-dir space form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--terraform-dir", "/components/tf", "--stack", "my-stack"},
			checkField:        func(got schema.ArgsAndFlagsInfo) string { return got.TerraformDir },
			wantFieldValue:    "/components/tf",
		},
		{
			name:              "deploy-run-init space form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--deploy-run-init", "false", "--stack", "my-stack"},
			checkField:        func(got schema.ArgsAndFlagsInfo) string { return got.DeployRunInit },
			wantFieldValue:    "false",
		},
		{
			name:              "append-user-agent space form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--append-user-agent", "atmos/1.0", "--stack", "my-stack"},
			checkField:        func(got schema.ArgsAndFlagsInfo) string { return got.AppendUserAgent },
			wantFieldValue:    "atmos/1.0",
		},
		{
			name:              "init-pass-vars space form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--init-pass-vars", "true", "--stack", "my-stack"},
			checkField:        func(got schema.ArgsAndFlagsInfo) string { return got.InitPassVars },
			wantFieldValue:    "true",
		},
		{
			name:              "skip-planfile space form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--skip-planfile", "true", "--stack", "my-stack"},
			checkField:        func(got schema.ArgsAndFlagsInfo) string { return got.PlanSkipPlanfile },
			wantFieldValue:    "true",
		},
		{
			name:              "settings-list-merge-strategy space form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--settings-list-merge-strategy", "append", "--stack", "my-stack"},
			checkField:        func(got schema.ArgsAndFlagsInfo) string { return got.SettingsListMergeStrategy },
			wantFieldValue:    "append",
		},
		{
			name:              "query space form",
			componentType:     "terraform",
			inputArgsAndFlags: []string{"plan", "vpc", "--query", ".tags", "--stack", "my-stack"},
			checkField:        func(got schema.ArgsAndFlagsInfo) string { return got.Query },
			wantFieldValue:    ".tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processArgsAndFlags(tt.componentType, tt.inputArgsAndFlags)
			require.NoError(t, err)
			assert.Equal(t, tt.wantFieldValue, tt.checkField(got))
			// Verify the flag and its value are NOT in AdditionalArgsAndFlags.
			assert.Nil(t, got.AdditionalArgsAndFlags, "expected no pass-through args")
		})
	}
}
