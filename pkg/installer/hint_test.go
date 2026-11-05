package installer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpgradeHint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind                      Kind
		manager, command, message string
	}{
		{Homebrew, "brew", "brew upgrade atmos", ""},
		{Scoop, "scoop", "scoop update atmos", ""},
		{Mise, "mise", "mise install atmos@1.2.3", "mise configuration"},
		{ASDF, "asdf", "asdf install atmos 1.2.3", ".tool-versions"},
		{Aqua, "aqua", "aqua install", ""},
		{Go, "go", "go install github.com/cloudposse/atmos@v1.2.3", ""},
		{Native, "atmos", "atmos version install 1.2.3", "version.use"},
		{DEB, "apt-get", "sudo apt-get update && sudo apt-get install --only-upgrade atmos", "newer .deb"},
		{RPM, "dnf", "sudo dnf upgrade atmos", "newer .rpm"},
		{RPM, "yum", "sudo yum update atmos", "newer .rpm"},
		{APK, "apk", "apk upgrade atmos", "newer .apk"},
		{Nix, "nix", "", "controlling Nix configuration"},
		{Unknown, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind)+tt.manager, func(t *testing.T) {
			t.Parallel()
			installation := Installation{Kind: tt.kind, Manager: tt.manager}
			hint := installation.UpgradeHint("1.2.3")
			assert.Equal(t, tt.command, hint.Command)
			assert.Contains(t, hint.Message, tt.message)
			assert.NotEmpty(t, hint.URL)
			assert.Equal(t, hint, installation.UpgradeHint("v1.2.3"))
			if tt.kind == Aqua {
				assert.Contains(t, hint.Condition, "After updating")
			}
			if tt.kind == APK {
				assert.Contains(t, hint.Condition, "administrative privileges")
			}
		})
	}
	assert.Equal(t, "scoop update atmos --global", (Installation{Kind: Scoop, Manager: "scoop", Global: true}).UpgradeHint("1.2.3").Command)
}

func TestUnavailableManagers(t *testing.T) {
	t.Parallel()
	for _, kind := range []Kind{Homebrew, Scoop, Mise, ASDF, Aqua, Go, Native, DEB, RPM, APK} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			hint := (Installation{Kind: kind}).UpgradeHint("1.2.3")
			assert.Empty(t, hint.Command)
			assert.Empty(t, hint.Condition)
			assert.NotEmpty(t, hint.Message)
			assert.NotEmpty(t, hint.URL)
		})
	}
	assert.Equal(t, Hint{URL: InstallURL}, (Installation{}).UpgradeHint("1.2.3"))
}
