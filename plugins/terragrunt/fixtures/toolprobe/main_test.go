package main

import "testing"

func TestAnswersAsTerragruntIgnoresCaseAndExtension(t *testing.T) {
	for name, want := range map[string]bool{
		"terragrunt":             true,
		"terragrunt.exe":         true,
		"terragrunt.EXE":         true,
		"Terragrunt.Exe":         true,
		"tofu":                   false,
		"tofu.exe":               false,
		"terragrunt-wrapper.exe": false,
		"not-terragrunt":         false,
		"terragrunt.exe.bak":     false,
	} {
		if got := answersAsTerragrunt(name); got != want {
			t.Errorf("answersAsTerragrunt(%q) = %v, want %v", name, got, want)
		}
	}
}
