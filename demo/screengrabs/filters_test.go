package main

import (
	"strings"
	"testing"
)

func TestCommandSlugMatchesLegacyPipeline(t *testing.T) {
	cases := map[string]string{
		"atmos about":                       "atmos-about",
		"atmos about --help":                "atmos-about--help",
		"atmos describe config -f yaml":     "atmos-describe-config-f-yaml",
		"atmos list stacks --charset=UTF-8": "atmos-list-stacks",
		"tree -CAF --gitignore":             "tree-CAF--gitignore",
	}
	for command, want := range cases {
		if got := commandSlug(command); got != want {
			t.Errorf("commandSlug(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestFilterNoiseDropsTerraformBootstrapLines(t *testing.T) {
	in := strings.Join([]string{
		"Initializing provider plugins...",
		"- Finding latest version of hashicorp/local...",
		"- Installing hashicorp/local v2.5.1...",
		"- Installed hashicorp/local v2.5.1 (signed by HashiCorp)",
		"Terraform has created a lock file .terraform.lock.hcl to record the provider",
		"Include this file in your version control repository so that Terraform can",
		"guarantee to make the same selections by default when",
		`you run "terraform init" in the future.`,
		"",
		"Terraform has been successfully initialized!",
	}, "\n")
	got := filterNoise(in)
	want := "Initializing provider plugins...\n\nTerraform has been successfully initialized!"
	if got != want {
		t.Fatalf("filterNoise = %q, want %q", got, want)
	}
}

func TestFilterNoiseScrubsResourceIDs(t *testing.T) {
	got := filterNoise("local_file.example: Creation complete after 0s [id=abc123def456]")
	if strings.Contains(got, "abc123def456") {
		t.Fatalf("resource id survived: %q", got)
	}
}

func TestFilterNoiseKeepsRegularContent(t *testing.T) {
	in := "Hello world\n  vars:\n    stage: dev\n"
	if got := filterNoise(in); got != in {
		t.Fatalf("regular content changed: %q", got)
	}
}
