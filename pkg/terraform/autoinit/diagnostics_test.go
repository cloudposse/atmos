package autoinit

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestClassify_Signatures(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   Diagnosis
	}{
		{
			name:   "terraform upgrade required",
			output: "Error: this configuration must use terraform init -upgrade",
			want:   Diagnosis{InitRequired: true, UpgradeRequired: true, Matched: "must use terraform init -upgrade"},
		},
		{
			name:   "tofu upgrade required",
			output: "Error: this configuration must use tofu init -upgrade",
			want:   Diagnosis{InitRequired: true, UpgradeRequired: true, Matched: "must use tofu init -upgrade"},
		},
		{
			name:   "backend configuration changed",
			output: "Error: Backend configuration changed\n\nA change in the backend...",
			want:   Diagnosis{InitRequired: true, ReconfigureRequired: true, Matched: "Backend configuration changed"},
		},
		{
			name:   "terraform backend init required",
			output: `Backend initialization required, please run "terraform init"`,
			want:   Diagnosis{InitRequired: true, Matched: `Backend initialization required, please run "terraform init"`},
		},
		{
			name:   "tofu backend init required",
			output: `Backend initialization required, please run "tofu init"`,
			want:   Diagnosis{InitRequired: true, Matched: `Backend initialization required, please run "tofu init"`},
		},
		{
			name:   "required plugins not installed",
			output: "Error: Required plugins are not installed",
			want:   Diagnosis{InitRequired: true, Matched: "Required plugins are not installed"},
		},
		{
			name:   "inconsistent dependency lock file",
			output: "Error: Inconsistent dependency lock file",
			want:   Diagnosis{InitRequired: true, Matched: "Inconsistent dependency lock file"},
		},
		{
			name:   "module not installed",
			output: "Error: Module not installed",
			want:   Diagnosis{InitRequired: true, Matched: "Module not installed"},
		},
		{
			name:   "module source has changed",
			output: "Error: Module source has changed",
			want:   Diagnosis{InitRequired: true, Matched: "Module source has changed"},
		},
		{
			name:   "module version requirements have changed",
			output: "Error: Module version requirements have changed",
			want:   Diagnosis{InitRequired: true, Matched: "Module version requirements have changed"},
		},
		{
			name:   "no match",
			output: "Error: some unrelated validation failure",
			want:   Diagnosis{},
		},
		{
			name:   "empty output",
			output: "",
			want:   Diagnosis{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.output)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestClassify_CombinesMultipleSignatures verifies that when output contains more than one
// matching signature (e.g. both a backend change and an upgrade requirement), Classify combines
// their flags instead of stopping at the first match -- a single recovery init then requests both
// -upgrade and -reconfigure instead of only whichever signature happened to appear first.
func TestClassify_CombinesMultipleSignatures(t *testing.T) {
	output := "Error: Backend configuration changed\n\n" +
		"Error: this configuration must use terraform init -upgrade"

	got := Classify(output)

	assert.True(t, got.InitRequired)
	assert.True(t, got.ReconfigureRequired)
	assert.True(t, got.UpgradeRequired)
	assert.Contains(t, got.Matched, "Backend configuration changed")
	assert.Contains(t, got.Matched, "must use terraform init -upgrade")
}

func TestClassify_ANSIWrapped(t *testing.T) {
	output := "\x1b[31mError:\x1b[0m Backend configuration changed\x1b[0m"

	got := Classify(output)

	assert.True(t, got.InitRequired)
	assert.True(t, got.ReconfigureRequired)
	assert.Equal(t, "Backend configuration changed", got.Matched)
}

func TestShouldRecover_NoDiagnosisIsNoOp(t *testing.T) {
	rec, err := ShouldRecover(Diagnosis{}, schema.TerraformInitModeAuto, schema.TerraformInitReconfigureAuto, schema.TerraformInitUpgradeAuto, false)

	require.NoError(t, err)
	assert.Equal(t, Recovery{}, rec)
}

func TestShouldRecover_OptedOutErrors(t *testing.T) {
	d := Diagnosis{InitRequired: true}

	rec, err := ShouldRecover(d, schema.TerraformInitModeNever, schema.TerraformInitReconfigureAuto, schema.TerraformInitUpgradeAuto, true)

	require.Error(t, err)
	assert.Equal(t, Recovery{}, rec)
	assert.True(t, errors.Is(err, errUtils.ErrTerraformInitRequired))
}

func TestShouldRecover_UpgradeRequiredButDisabledErrors(t *testing.T) {
	d := Diagnosis{InitRequired: true, UpgradeRequired: true}

	rec, err := ShouldRecover(d, schema.TerraformInitModeAuto, schema.TerraformInitReconfigureAuto, schema.TerraformInitUpgradeNever, false)

	require.Error(t, err)
	assert.Equal(t, Recovery{}, rec)
	assert.True(t, errors.Is(err, errUtils.ErrTerraformInitUpgradeRequired))
}

func TestShouldRecover_ReconfigureRequiredButDisabledErrors(t *testing.T) {
	d := Diagnosis{InitRequired: true, ReconfigureRequired: true}

	rec, err := ShouldRecover(d, schema.TerraformInitModeAuto, schema.TerraformInitReconfigureNever, schema.TerraformInitUpgradeAuto, false)

	require.Error(t, err)
	assert.Equal(t, Recovery{}, rec)
	assert.True(t, errors.Is(err, errUtils.ErrTerraformInitReconfigureRequired))
}

func TestShouldRecover_RecoversWithAppropriateFlags(t *testing.T) {
	tests := []struct {
		name        string
		diagnosis   Diagnosis
		reconfigure schema.TerraformInitReconfigure
		upgrade     schema.TerraformInitUpgrade
		want        Recovery
	}{
		{
			name:        "plain init required",
			diagnosis:   Diagnosis{InitRequired: true},
			reconfigure: schema.TerraformInitReconfigureAuto,
			upgrade:     schema.TerraformInitUpgradeAuto,
			want:        Recovery{Run: true},
		},
		{
			name:        "upgrade required and allowed",
			diagnosis:   Diagnosis{InitRequired: true, UpgradeRequired: true},
			reconfigure: schema.TerraformInitReconfigureAuto,
			upgrade:     schema.TerraformInitUpgradeAuto,
			want:        Recovery{Run: true, WithUpgrade: true},
		},
		{
			name:        "reconfigure required and allowed",
			diagnosis:   Diagnosis{InitRequired: true, ReconfigureRequired: true},
			reconfigure: schema.TerraformInitReconfigureAuto,
			upgrade:     schema.TerraformInitUpgradeAuto,
			want:        Recovery{Run: true, WithReconfigure: true},
		},
		{
			name:        "upgrade forced by policy even without a diagnosed need",
			diagnosis:   Diagnosis{InitRequired: true},
			reconfigure: schema.TerraformInitReconfigureAuto,
			upgrade:     schema.TerraformInitUpgradeAlways,
			want:        Recovery{Run: true, WithUpgrade: true},
		},
		{
			name:        "reconfigure forced by policy even without a diagnosed need",
			diagnosis:   Diagnosis{InitRequired: true},
			reconfigure: schema.TerraformInitReconfigureAlways,
			upgrade:     schema.TerraformInitUpgradeAuto,
			want:        Recovery{Run: true, WithReconfigure: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := ShouldRecover(tt.diagnosis, schema.TerraformInitModeAuto, tt.reconfigure, tt.upgrade, false)

			require.NoError(t, err)
			assert.Equal(t, tt.want, rec)
		})
	}
}
