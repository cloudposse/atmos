package proexec

import "testing"

func TestIsSyncCommand(t *testing.T) {
	tests := []struct {
		name        string
		commandPath string
		want        bool
	}{
		{"terraform plan is sync", "atmos terraform plan", true},
		{"terraform apply is sync", "atmos terraform apply", true},
		{"terraform deploy is sync", "atmos terraform deploy", true},
		{"describe affected is sync", "atmos describe affected", true},
		{"version is not sync", "atmos version", false},
		{"terraform validate is not sync", "atmos terraform validate", false},
		{"terraform output is not sync", "atmos terraform output", false},
		{"empty command path is not sync", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSyncCommand(tt.commandPath); got != tt.want {
				t.Errorf("IsSyncCommand(%q) = %v, want %v", tt.commandPath, got, tt.want)
			}
		})
	}
}
