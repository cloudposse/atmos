package configuration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func createSensitiveTempFile(root *os.Root, name string) (*os.File, error) {
	return openSensitiveGeneratedFile(root, name, false)
}

func openGeneratedFile(root *os.Root, file schema.ComponentConfigurationFile, exists bool) (*os.File, error) {
	if !file.Sensitive {
		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if exists {
			flags = os.O_WRONLY
		}
		output, err := root.OpenFile(file.Path, flags, secretMode)
		if err != nil {
			return nil, err
		}
		if err := output.Chmod(secretMode); err != nil {
			return nil, errors.Join(err, output.Close())
		}
		return output, nil
	}

	return openSensitiveGeneratedFile(root, file.Path, exists)
}

// restrictExistingGeneratedFile re-secures an existing sensitive file. Other files keep their ACLs.
func restrictExistingGeneratedFile(root *os.Root, file schema.ComponentConfigurationFile) error {
	if !file.Sensitive {
		return nil
	}
	output, err := openSensitiveGeneratedFile(root, file.Path, true)
	if err != nil {
		return err
	}
	return output.Close()
}

func openSensitiveGeneratedFile(root *os.Root, path string, exists bool) (*os.File, error) {
	// Resolve the parent through os.Root, then open only a basename relative to
	// its handle. Never turn the contained path back into an absolute pathname.
	parent, err := root.Open(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	security, err := windows.SecurityDescriptorFromString("O:" + user.User.Sid.String() + "D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return nil, err
	}
	name, err := windows.NewNTUnicodeString(filepath.Base(path))
	if err != nil {
		return nil, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{
		RootDirectory: windows.Handle(parent.Fd()), ObjectName: name,
		Attributes:         windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
		SecurityDescriptor: security,
	}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	disposition := uint32(windows.FILE_CREATE)
	if exists {
		disposition = windows.FILE_OPEN
	}
	var handle windows.Handle
	// Exclusive sharing rejects existing readers as well as new readers until
	// close: replacing a DACL alone cannot revoke an already-open read handle.
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_WRITE|windows.FILE_READ_ATTRIBUTES|windows.WRITE_DAC,
		&attributes, &windows.IO_STATUS_BLOCK{}, nil, windows.FILE_ATTRIBUTE_NORMAL, 0, disposition,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	runtime.KeepAlive(parent)
	if err != nil {
		var status windows.NTStatus
		if errors.As(err, &status) {
			return nil, status.Errno()
		}
		return nil, err
	}
	output := os.NewFile(uintptr(handle), path)
	if err := restrictGeneratedFile(handle, user.User.Sid, security); err != nil {
		return nil, errors.Join(err, output.Close())
	}
	return output, nil
}

func validateSensitiveGeneratedFile(handle windows.Handle, user *windows.SID) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	if info.NumberOfLinks != 1 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%w: sensitive generated file must be a regular file with one link", errUtils.ErrPluginInvalidConfig)
	}
	current, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := current.Owner()
	if err != nil {
		return err
	}
	// A different owner could grant themselves read access again after close.
	if owner == nil || !owner.Equals(user) {
		return fmt.Errorf("%w: sensitive generated file must be owned by the current user", errUtils.ErrPluginInvalidConfig)
	}
	return nil
}

func restrictGeneratedFile(handle windows.Handle, user *windows.SID, security *windows.SECURITY_DESCRIPTOR) error {
	if err := validateSensitiveGeneratedFile(handle, user); err != nil {
		return err
	}
	dacl, _, err := security.DACL()
	if err != nil {
		return err
	}
	if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return err
	}
	// Fail closed on filesystems that cannot persist the requested ACL.
	actual, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	return verifyGeneratedFileACL(actual, user, dacl)
}

func verifyGeneratedFileACL(actual *windows.SECURITY_DESCRIPTOR, user *windows.SID, expected *windows.ACL) error {
	invalid := fmt.Errorf("%w: sensitive generated file ACL was not retained", errUtils.ErrPluginInvalidConfig)
	control, _, err := actual.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return invalid
	}
	dacl, _, err := actual.DACL()
	if err != nil {
		return err
	}
	if dacl == nil || dacl.AceCount != 1 {
		return invalid
	}
	var ace, wanted *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		return err
	}
	if err := windows.GetAce(expected, 0, &wanted); err != nil {
		return err
	}
	if *ace != *wanted {
		return invalid
	}
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !sid.Equals(user) {
		return invalid
	}
	return nil
}
