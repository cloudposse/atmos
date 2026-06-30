package types

import "testing"

func TestIsStandaloneIdentityKind(t *testing.T) {
	tests := []struct {
		name string
		kind string
		want bool
	}{
		{"aws user", IdentityKindAWSUser, true},
		{"aws ambient", IdentityKindAWSAmbient, true},
		{"generic ambient", IdentityKindAmbient, true},
		{"aws emulator", IdentityKindAWSEmulator, true},
		{"gcp emulator", IdentityKindGCPEmulator, true},
		{"azure emulator", IdentityKindAzureEmulator, true},
		{"kubernetes emulator", IdentityKindKubernetesEmulator, true},
		{"aws permission-set is not standalone", "aws/permission-set", false},
		{"gcp service account is not standalone", IdentityKindGCPServiceAccount, false},
		{"empty kind is not standalone", "", false},
		{"unknown kind is not standalone", "made/up", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsStandaloneIdentityKind(tt.kind); got != tt.want {
				t.Errorf("IsStandaloneIdentityKind(%q) = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

func TestStandaloneProviderName(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		wantName string
		wantOK   bool
	}{
		{"aws user maps to synthetic provider", IdentityKindAWSUser, ProviderNameAWSUser, true},
		{"aws ambient reports its own name", IdentityKindAWSAmbient, "", false},
		{"generic ambient reports its own name", IdentityKindAmbient, "", false},
		{"emulator reports its own name", IdentityKindAWSEmulator, "", false},
		{"non-standalone kind", "aws/permission-set", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ok := StandaloneProviderName(tt.kind)
			if name != tt.wantName || ok != tt.wantOK {
				t.Errorf("StandaloneProviderName(%q) = (%q, %v), want (%q, %v)",
					tt.kind, name, ok, tt.wantName, tt.wantOK)
			}
		})
	}
}
