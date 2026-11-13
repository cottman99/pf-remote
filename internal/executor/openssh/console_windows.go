//go:build windows

package openssh

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"strings"
	"unsafe"
)

// A real console receives its own standard handles; no terminal I/O crosses IPC.
type consoleRunner struct {
	started func()
	desktop string
}

func (r consoleRunner) Run(ctx context.Context, binary string, args []string, _ io.Reader, _ io.Writer, _ io.Writer) (int, error) {
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	var sessionID uint32
	if windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sessionID) != nil || sessionID == 0 {
		return -1, errors.New("terminal requires the signed-in user's desktop session")
	}
	app, e := windows.UTF16PtrFromString(binary)
	if e != nil {
		return -1, e
	}
	parts := []string{windows.EscapeArg(binary)}
	for _, a := range args {
		parts = append(parts, windows.EscapeArg(a))
	}
	line, e := windows.UTF16PtrFromString(strings.Join(parts, " "))
	if e != nil {
		return -1, e
	}
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return -1, e
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); e != nil {
		return -1, e
	}
	si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	if r.desktop != "" {
		si.Desktop, e = windows.UTF16PtrFromString(r.desktop)
		if e != nil {
			return -1, e
		}
	}
	pi := windows.ProcessInformation{}
	if e = windows.CreateProcess(app, line, nil, nil, false, windows.CREATE_NEW_CONSOLE|windows.CREATE_SUSPENDED, nil, nil, &si, &pi); e != nil {
		return -1, errors.New("native terminal could not start")
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)
	if e = windows.AssignProcessToJobObject(job, pi.Process); e != nil {
		windows.TerminateProcess(pi.Process, 1)
		return -1, errors.New("terminal lifecycle protection unavailable")
	}
	if _, e = windows.ResumeThread(pi.Thread); e != nil {
		return -1, e
	}
	if r.started != nil {
		r.started()
	}
	for {
		s, e := windows.WaitForSingleObject(pi.Process, 100)
		if e != nil {
			return -1, e
		}
		if s == windows.WAIT_OBJECT_0 {
			var code uint32
			e = windows.GetExitCodeProcess(pi.Process, &code)
			return int(code), e
		}
		if ctx.Err() != nil {
			windows.TerminateJobObject(job, 1)
			windows.WaitForSingleObject(pi.Process, 5000)
			return -1, ctx.Err()
		}
	}
}
