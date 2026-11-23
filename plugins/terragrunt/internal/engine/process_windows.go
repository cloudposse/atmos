package engine

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processTree uses a job to terminate version probe descendants without launching another command.
type processTree struct {
	cmd *exec.Cmd
	job windows.Handle
}

func prepareProcessTree(cmd *exec.Cmd) (*processTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create version probe process job: %w", err)
	}
	tree := &processTree{cmd: cmd, job: job}
	// Assign the job before running any user code, so even immediately launched children belong to it.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	cmd.Cancel = func() error {
		jobErr := windows.TerminateJobObject(job, 1)
		// Cancellation may race with started() before the version probe has joined the job.
		processErr := cmd.Process.Kill()
		if jobErr != nil {
			return processErr
		}
		return nil
	}
	return tree, nil
}

func (t *processTree) started() error {
	pid := uint32(t.cmd.Process.Pid) //nolint:gosec // Windows process identifiers originate as a DWORD in CreateProcess.
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return fmt.Errorf("open version probe process: %w", err)
	}
	defer func() { _ = windows.CloseHandle(process) }()
	if err := windows.AssignProcessToJobObject(t.job, process); err != nil {
		return fmt.Errorf("assign version probe process job: %w", err)
	}
	return resumeProviderThread(pid)
}

// Go closes the initial thread handle in Start, so retrieve that still-suspended thread by PID.
func resumeProviderThread(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("find version probe thread: %w", err)
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	var entry windows.ThreadEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID == pid {
			return resumeThread(entry.ThreadID)
		}
	}
	return fmt.Errorf("find suspended version probe thread: %w", err)
}

func resumeThread(id uint32) error {
	thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, id)
	if err != nil {
		return fmt.Errorf("open version probe thread: %w", err)
	}
	defer func() { _ = windows.CloseHandle(thread) }()
	if _, err := windows.ResumeThread(thread); err != nil {
		return fmt.Errorf("resume version probe thread: %w", err)
	}
	return nil
}

func (t *processTree) close() {
	// Version probes never own persistent services; close descendants even after a wrapper exits.
	_ = windows.TerminateJobObject(t.job, 1)
	_ = windows.CloseHandle(t.job)
}
