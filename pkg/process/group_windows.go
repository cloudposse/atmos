//go:build windows

package process

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const managedWaitDelay = 5 * time.Second

type managedCommandState struct {
	job windows.Handle
}

var managedCommands sync.Map

func prepareManagedCommand(cmd *exec.Cmd) (func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP

	state := &managedCommandState{job: job}
	managedCommands.Store(cmd, state)
	if cmd.Cancel != nil {
		cmd.Cancel = func() error {
			if termErr := windows.TerminateJobObject(job, 1); termErr == nil {
				return nil
			}
			if cmd.Process == nil {
				return os.ErrProcessDone
			}
			return cmd.Process.Kill()
		}
		cmd.WaitDelay = managedWaitDelay
	}

	return func() {
		managedCommands.Delete(cmd)
		_ = windows.CloseHandle(job)
	}, nil
}

func activateManagedCommand(cmd *exec.Cmd) error {
	value, ok := managedCommands.Load(cmd)
	if !ok {
		return nil
	}
	state := value.(*managedCommandState)

	processHandle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(processHandle)

	if err := windows.AssignProcessToJobObject(state.job, processHandle); err != nil {
		return err
	}
	return resumeProcessThreads(uint32(cmd.Process.Pid))
}

func killManagedCommand(cmd *exec.Cmd) error {
	value, ok := managedCommands.Load(cmd)
	if ok {
		state := value.(*managedCommandState)
		if err := windows.TerminateJobObject(state.job, 1); err == nil {
			return nil
		}
	}
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func resumeProcessThreads(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	if err := windows.Thread32First(snapshot, &entry); err != nil {
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return nil
		}
		return err
	}

	for {
		if entry.OwnerProcessID == pid {
			thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if openErr == nil {
				_, resumeErr := windows.ResumeThread(thread)
				_ = windows.CloseHandle(thread)
				if resumeErr != nil {
					return resumeErr
				}
			}
		}

		err = windows.Thread32Next(snapshot, &entry)
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
