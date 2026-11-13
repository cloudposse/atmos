package terraform

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestPlanBindingPrivateACLUnderPermissiveParent(t *testing.T) {
	directory := t.TempDir()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	require.NoError(t, err)
	// Grant Everyone read access to the parent, so inherited permissions
	// would expose the binding if its own ACL were not protected.
	security, err := windows.SecurityDescriptorFromString("D:(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;GR;;;WD)")
	require.NoError(t, err)
	dacl, _, err := security.DACL()
	require.NoError(t, err)
	require.NoError(t, windows.SetNamedSecurityInfo(directory, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil))

	path := filepath.Join(directory, "reviewed.plan.atmos.json")
	data := []byte("private plan binding")
	require.NoError(t, writePlanBinding(path, data))
	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, data, actual)

	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	acl, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	control, _, err := acl.Control()
	require.NoError(t, err)
	require.NotZero(t, control&windows.SE_DACL_PROTECTED)
	entries, _, err := acl.DACL()
	require.NoError(t, err)
	require.NotNil(t, entries)
	require.EqualValues(t, 1, entries.AceCount, "no other local user may read the binding")
	var ace *windows.ACCESS_ALLOWED_ACE
	require.NoError(t, windows.GetAce(entries, 0, &ace))
	require.EqualValues(t, windows.ACCESS_ALLOWED_ACE_TYPE, ace.Header.AceType)
	require.NotZero(t, ace.Mask&windows.FILE_GENERIC_READ)
	require.True(t, (*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(user.User.Sid))
}
