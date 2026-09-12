package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerraformInitMode_IsValid(t *testing.T) {
	tests := []struct {
		name string
		mode TerraformInitMode
		want bool
	}{
		{"empty (unset) is valid", "", true},
		{"auto is valid", TerraformInitModeAuto, true},
		{"always is valid", TerraformInitModeAlways, true},
		{"never is valid", TerraformInitModeNever, true},
		{"unknown value is invalid", TerraformInitMode("bogus"), false},
		{"uppercase is invalid (case-sensitive)", TerraformInitMode("AUTO"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.mode.IsValid())
		})
	}
}

func TestTerraformInitReconfigure_IsValid(t *testing.T) {
	tests := []struct {
		name string
		mode TerraformInitReconfigure
		want bool
	}{
		{"empty (unset) is valid", "", true},
		{"auto is valid", TerraformInitReconfigureAuto, true},
		{"always is valid", TerraformInitReconfigureAlways, true},
		{"never is valid", TerraformInitReconfigureNever, true},
		{"unknown value is invalid", TerraformInitReconfigure("bogus"), false},
		{"uppercase is invalid (case-sensitive)", TerraformInitReconfigure("ALWAYS"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.mode.IsValid())
		})
	}
}

func TestTerraformInitUpgrade_IsValid(t *testing.T) {
	tests := []struct {
		name string
		mode TerraformInitUpgrade
		want bool
	}{
		{"empty (unset) is valid", "", true},
		{"auto is valid", TerraformInitUpgradeAuto, true},
		{"always is valid", TerraformInitUpgradeAlways, true},
		{"never is valid", TerraformInitUpgradeNever, true},
		{"unknown value is invalid", TerraformInitUpgrade("bogus"), false},
		{"uppercase is invalid (case-sensitive)", TerraformInitUpgrade("NEVER"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.mode.IsValid())
		})
	}
}

func TestTerraform_EffectiveInitMode(t *testing.T) {
	tests := []struct {
		name string
		tf   Terraform
		want TerraformInitMode
	}{
		{"unset defaults to auto", Terraform{}, TerraformInitModeAuto},
		{"explicit auto", Terraform{Init: TerraformInit{Mode: TerraformInitModeAuto}}, TerraformInitModeAuto},
		{"explicit always", Terraform{Init: TerraformInit{Mode: TerraformInitModeAlways}}, TerraformInitModeAlways},
		{"explicit never", Terraform{Init: TerraformInit{Mode: TerraformInitModeNever}}, TerraformInitModeNever},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.tf.EffectiveInitMode())
		})
	}
}

func TestTerraform_EffectiveInitReconfigure(t *testing.T) {
	tests := []struct {
		name string
		tf   Terraform
		want TerraformInitReconfigure
	}{
		{
			name: "explicit reconfigure wins over legacy true",
			tf: Terraform{
				InitRunReconfigure: true,
				Init:               TerraformInit{Reconfigure: TerraformInitReconfigureNever},
			},
			want: TerraformInitReconfigureNever,
		},
		{
			name: "explicit reconfigure wins over legacy false",
			tf: Terraform{
				InitRunReconfigure: false,
				Init:               TerraformInit{Reconfigure: TerraformInitReconfigureAlways},
			},
			want: TerraformInitReconfigureAlways,
		},
		{
			name: "legacy false maps to never when explicit is unset",
			tf:   Terraform{InitRunReconfigure: false},
			want: TerraformInitReconfigureNever,
		},
		{
			name: "legacy true (default) maps to auto when explicit is unset",
			tf:   Terraform{InitRunReconfigure: true},
			want: TerraformInitReconfigureAuto,
		},
		{
			name: "zero-value Terraform (no legacy, no explicit) maps to never",
			tf:   Terraform{},
			want: TerraformInitReconfigureNever,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.tf.EffectiveInitReconfigure())
		})
	}
}

func TestTerraform_EffectiveInitUpgrade(t *testing.T) {
	tests := []struct {
		name string
		tf   Terraform
		want TerraformInitUpgrade
	}{
		{"unset defaults to auto", Terraform{}, TerraformInitUpgradeAuto},
		{"explicit auto", Terraform{Init: TerraformInit{Upgrade: TerraformInitUpgradeAuto}}, TerraformInitUpgradeAuto},
		{"explicit always", Terraform{Init: TerraformInit{Upgrade: TerraformInitUpgradeAlways}}, TerraformInitUpgradeAlways},
		{"explicit never", Terraform{Init: TerraformInit{Upgrade: TerraformInitUpgradeNever}}, TerraformInitUpgradeNever},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.tf.EffectiveInitUpgrade())
		})
	}
}
