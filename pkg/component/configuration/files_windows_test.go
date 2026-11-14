package configuration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/cloudposse/atmos/pkg/schema"
)

func assertPrivateGeneratedACL(t *testing.T, handle windows.Handle) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	require.NoError(t, err)
	actual, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	expected, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	require.NoError(t, err)
	dacl, _, err := expected.DACL()
	require.NoError(t, err)
	require.NoError(t, verifyGeneratedFileACL(actual, user.User.Sid, dacl))
}

func TestSensitiveGeneratedFileWindowsACLBeforeWrite(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "overwrite"}[existing], func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "secret.tf")
			previous := []byte("old content remains until access is restricted")
			if existing {
				require.NoError(t, os.WriteFile(path, previous, 0o644))
				// Explicit Everyone read access must be removed on overwrite.
				security, err := windows.SecurityDescriptorFromString("D:(A;;FA;;;OW)(A;;GR;;;WD)")
				require.NoError(t, err)
				dacl, _, err := security.DACL()
				require.NoError(t, err)
				require.NoError(t, windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
					windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil))
			}
			root, err := os.OpenRoot(directory)
			require.NoError(t, err)
			defer root.Close()
			file := schema.ComponentConfigurationFile{Path: "secret.tf", IfExists: "overwrite", Sensitive: true, Content: []byte("new")}
			output, err := openGeneratedFile(root, file, existing)
			require.NoError(t, err)
			t.Cleanup(func() { _ = output.Close() })
			assertPrivateGeneratedACL(t, windows.Handle(output.Fd()))
			info, err := output.Stat()
			require.NoError(t, err)
			if existing {
				require.EqualValues(t, len(previous), info.Size(), "opening must not truncate before ACL restriction")
			} else {
				require.Zero(t, info.Size())
			}
			reader, err := os.Open(path)
			if reader != nil {
				_ = reader.Close()
			}
			require.ErrorIs(t, err, windows.ERROR_SHARING_VIOLATION)
			require.NoError(t, output.Close())
			require.NoError(t, WriteFiles(directory, []schema.ComponentConfigurationFile{file}))
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, file.Content, content)
			reader, err = os.Open(path)
			require.NoError(t, err)
			defer reader.Close()
			assertPrivateGeneratedACL(t, windows.Handle(reader.Fd()))
		})
	}
}

func TestSensitiveGeneratedFileWindowsExistingReader(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "secret.tf")
	previous := []byte("original")
	require.NoError(t, os.WriteFile(path, previous, 0o644))
	reader, err := os.Open(path)
	require.NoError(t, err)
	defer reader.Close()
	err = WriteFiles(directory, []schema.ComponentConfigurationFile{{Path: "secret.tf", IfExists: "overwrite", Sensitive: true, Content: []byte("secret")}})
	require.ErrorIs(t, err, windows.ERROR_SHARING_VIOLATION)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, previous, content)
}

func TestSensitiveGeneratedFileWindowsRejectsHardLink(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "original.tf")
	previous := []byte("original")
	require.NoError(t, os.WriteFile(path, previous, 0o644))
	require.NoError(t, os.Link(path, filepath.Join(directory, "secret.tf")))
	require.Error(t, WriteFiles(directory, []schema.ComponentConfigurationFile{{Path: "secret.tf", IfExists: "overwrite", Sensitive: true, Content: []byte("secret")}}))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, previous, content)
}

func TestSensitiveGeneratedFileWindowsACLFailurePreservesContent(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "secret.tf")
	previous := []byte("original")
	require.NoError(t, os.WriteFile(path, previous, 0o644))
	// A read-only handle cannot set the DACL. Exercise the real Windows failure.
	reader, err := os.Open(path)
	require.NoError(t, err)
	defer reader.Close()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	require.NoError(t, err)
	security, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	require.NoError(t, err)
	require.Error(t, restrictGeneratedFile(windows.Handle(reader.Fd()), user.User.Sid, security))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, previous, content)
}

func TestSensitiveGeneratedFileWindowsRejectsDifferentOwner(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "secret.tf")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))
	reader, err := os.Open(path)
	require.NoError(t, err)
	defer reader.Close()
	other, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	require.NoError(t, err)
	require.ErrorContains(t, validateSensitiveGeneratedFile(windows.Handle(reader.Fd()), other), "owned by the current user")
}

func TestSensitiveGeneratedFileWindowsRejectsReparsePoint(t *testing.T) {
	directory := t.TempDir()
	outside := filepath.Join(t.TempDir(), "original.tf")
	previous := []byte("original")
	require.NoError(t, os.WriteFile(outside, previous, 0o644))
	if err := os.Symlink(outside, filepath.Join(directory, "secret.tf")); err != nil {
		t.Skipf("symlink creation requires Windows developer mode or privilege: %v", err)
	}
	require.Error(t, WriteFiles(directory, []schema.ComponentConfigurationFile{{Path: "secret.tf", IfExists: "overwrite", Sensitive: true, Content: []byte("secret")}}))
	content, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, previous, content)
}

func TestSensitiveGeneratedFileWindowsACLVerification(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	require.NoError(t, err)
	entry := "(A;;FA;;;" + user.User.Sid.String() + ")"
	expected, err := windows.SecurityDescriptorFromString("D:P" + entry)
	require.NoError(t, err)
	dacl, _, err := expected.DACL()
	require.NoError(t, err)
	for _, tc := range []struct {
		name  string
		sddl  string
		valid bool
	}{
		{"protected", "D:P" + entry, true},
		{"auto-inheritance-history", "D:PAI" + entry, true},
		{"unprotected", "D:" + entry, false},
		{"everyone", "D:P(A;;FA;;;WD)", false},
		{"extra-reader", "D:P" + entry + "(A;;GR;;;WD)", false},
		{"empty", "D:P", false},
		{"null", "D:NO_ACCESS_CONTROL", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := windows.SecurityDescriptorFromString(tc.sddl)
			require.NoError(t, err)
			err = verifyGeneratedFileACL(actual, user.User.Sid, dacl)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
