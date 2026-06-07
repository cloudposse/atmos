package cache

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// trustSeams overrides the trust-command seams for a test and restores them after.
type trustSeams struct {
	certPath  string
	certErr   error
	required  bool
	note      string
	installFn func(string) error
	removeFn  func(string) error
}

func applyTrustSeams(t *testing.T, s trustSeams) {
	t.Helper()
	origResolve, origInstr, origInstall, origRemove := resolveCertPath, trustInstructions, installTrust, removeTrust
	t.Cleanup(func() {
		resolveCertPath, trustInstructions, installTrust, removeTrust = origResolve, origInstr, origInstall, origRemove
	})
	resolveCertPath = func(*cobra.Command) (string, error) { return s.certPath, s.certErr }
	trustInstructions = func() (bool, string) { return s.required, s.note }
	if s.installFn != nil {
		installTrust = s.installFn
	}
	if s.removeFn != nil {
		removeTrust = s.removeFn
	}
}

func TestTrustUntrustArgs(t *testing.T) {
	assert.NoError(t, trustCmd.Args(trustCmd, []string{}))
	assert.Error(t, trustCmd.Args(trustCmd, []string{"extra"}))
	assert.NoError(t, untrustCmd.Args(untrustCmd, []string{}))
	assert.Error(t, untrustCmd.Args(untrustCmd, []string{"extra"}))
}

func TestTrustCmd_NotRequired(t *testing.T) {
	applyTrustSeams(t, trustSeams{
		certPath: "/tmp/proxy.pem",
		required: false,
		note:     "not required here",
		installFn: func(string) error {
			t.Fatal("InstallTrust must not be called when trust is not required")
			return nil
		},
	})

	cacheCmd.SetArgs([]string{"trust"})
	require.NoError(t, cacheCmd.Execute())
}

func TestTrustCmd_Success(t *testing.T) {
	var got string
	applyTrustSeams(t, trustSeams{
		certPath:  "/tmp/proxy.pem",
		required:  true,
		installFn: func(p string) error { got = p; return nil },
	})

	cacheCmd.SetArgs([]string{"trust"})
	require.NoError(t, cacheCmd.Execute())
	assert.Equal(t, "/tmp/proxy.pem", got)
}

func TestTrustCmd_InstallErrorWrapsTrustStore(t *testing.T) {
	applyTrustSeams(t, trustSeams{
		certPath:  "/tmp/proxy.pem",
		required:  true,
		installFn: func(string) error { return errors.New("keychain denied") },
	})

	cacheCmd.SetArgs([]string{"trust"})
	require.ErrorIs(t, cacheCmd.Execute(), errUtils.ErrTrustStore)
}

func TestTrustCmd_CertPathError(t *testing.T) {
	wantErr := errors.New("config load failed")
	applyTrustSeams(t, trustSeams{certErr: wantErr, required: true})

	cacheCmd.SetArgs([]string{"trust"})
	require.ErrorIs(t, cacheCmd.Execute(), wantErr)
}

func TestUntrustCmd_Success(t *testing.T) {
	var got string
	applyTrustSeams(t, trustSeams{
		certPath: "/tmp/proxy.pem",
		required: true,
		removeFn: func(p string) error { got = p; return nil },
	})

	cacheCmd.SetArgs([]string{"untrust"})
	require.NoError(t, cacheCmd.Execute())
	assert.Equal(t, "/tmp/proxy.pem", got)
}

func TestUntrustCmd_RemoveErrorWrapsTrustStore(t *testing.T) {
	applyTrustSeams(t, trustSeams{
		certPath: "/tmp/proxy.pem",
		required: true,
		removeFn: func(string) error { return errors.New("certutil failed") },
	})

	cacheCmd.SetArgs([]string{"untrust"})
	require.ErrorIs(t, cacheCmd.Execute(), errUtils.ErrTrustStore)
}

func TestUntrustCmd_NotRequired(t *testing.T) {
	applyTrustSeams(t, trustSeams{
		required: false,
		note:     "not required here",
		removeFn: func(string) error {
			t.Fatal("RemoveTrust must not be called when trust is not required")
			return nil
		},
	})

	cacheCmd.SetArgs([]string{"untrust"})
	require.NoError(t, cacheCmd.Execute())
}
