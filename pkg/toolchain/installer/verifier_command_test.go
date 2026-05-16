package installer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/verification"
)

func TestVerifierTool(t *testing.T) {
	tests := []struct {
		name      string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{name: "cosign", wantOwner: "sigstore", wantRepo: "cosign", wantOK: true},
		{name: "slsa-verifier", wantOwner: "slsa-framework", wantRepo: "slsa-verifier", wantOK: true},
		{name: "gh", wantOwner: "cli", wantRepo: "cli", wantOK: true},
		{name: "minisign", wantOwner: "jedisct1", wantRepo: "minisign", wantOK: true},
		{name: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := verifierTool(tt.name)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestVerifierCommandRunnerRequiresPathWhenAutoInstallDisabled(t *testing.T) {
	err := verifierCommandRunner{
		installer: &Installer{},
		policy: verification.Policy{
			VerifierInstall: verification.VerifierInstallPathOnly,
		},
	}.Run(context.Background(), "definitely-not-an-atmos-verifier")

	require.ErrorIs(t, err, verification.ErrVerifierCommandRequired)
}

func TestVerifierCommandRunnerRequiresKnownVerifierForAutoInstall(t *testing.T) {
	err := verifierCommandRunner{
		installer: &Installer{},
		policy: verification.Policy{
			VerifierInstall: verification.VerifierInstallAuto,
		},
	}.Run(context.Background(), "definitely-not-an-atmos-verifier")

	require.ErrorIs(t, err, verification.ErrVerifierCommandRequired)
}

func TestVerifierCommandRunnerUsesExistingCommandOnPath(t *testing.T) {
	exe := os.Args[0]
	t.Setenv("PATH", filepath.Dir(exe)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ATMOS_VERIFIER_HELPER_PROCESS", "1")

	err := verifierCommandRunner{
		installer: &Installer{},
		policy:    verification.Policy{VerifierInstall: verification.VerifierInstallAuto},
	}.Run(context.Background(), filepath.Base(exe), "-test.run=TestVerifierCommandHelperProcess", "--", "success")

	require.NoError(t, err)
}

func TestRunVerifierCommandFailure(t *testing.T) {
	t.Setenv("ATMOS_VERIFIER_HELPER_PROCESS", "1")

	err := runVerifierCommand(context.Background(), os.Args[0], "-test.run=TestVerifierCommandHelperProcess", "--", "fail")

	require.ErrorIs(t, err, verification.ErrSignatureFailed)
}

func TestVerifierCommandHelperProcess(t *testing.T) {
	if os.Getenv("ATMOS_VERIFIER_HELPER_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) && os.Args[i+1] == "success" {
			os.Exit(0)
		}
	}
	os.Exit(1)
}
