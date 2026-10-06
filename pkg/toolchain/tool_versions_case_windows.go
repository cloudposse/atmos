package toolchain

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// toolVersionsPreserveCase honors Windows' per-directory case-sensitive flag,
// including WSL directories, without creating or modifying project files.
func toolVersionsPreserveCase(path string) (bool, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return false, err
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	// FILE_CASE_SENSITIVE_INFO contains an ULONG. A byte array has no ULONG
	// alignment guarantee and can make the native query return ERROR_NOACCESS.
	var flags uint32
	err = windows.GetFileInformationByHandleEx(handle, windows.FileCaseSensitiveInfo,
		(*byte)(unsafe.Pointer(&flags)), uint32(unsafe.Sizeof(flags)))
	// Older Windows versions and filesystems without this feature use the
	// normal case-insensitive Windows lookup behavior.
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_SUPPORTED) {
		return false, nil
	}
	const caseSensitiveDirectory = 1
	return flags&caseSensitiveDirectory != 0, err
}
