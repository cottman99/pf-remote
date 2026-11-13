//go:build windows

package openssh

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"
)

// Never display a test console on the user's input desktop.
func TestConsoleLifecycleOnIsolatedDesktop(t *testing.T) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	name := "PFRemoteTerminalTest" + time.Now().Format("150405000000000")
	wide, _ := windows.UTF16PtrFromString(name)
	desk, _, err := user32.NewProc("CreateDesktopW").Call(uintptr(unsafe.Pointer(wide)), 0, 0, 0, 0x10000000, 0)
	if desk == 0 {
		t.Fatal(err)
	}
	defer user32.NewProc("CloseDesktop").Call(desk)
	binary := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	started := 0
	runner := consoleRunner{desktop: "WinSta0\\" + name, started: func() { started++ }}
	code, e := runner.Run(context.Background(), binary, []string{"/d", "/c", "exit 7"}, nil, nil, nil)
	if e != nil || code != 7 || started != 1 {
		t.Fatalf("exit=%d started=%d error=%v", code, started, e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	code, e = runner.Run(ctx, binary, []string{"/d", "/c", "pause"}, nil, nil, nil)
	if !errors.Is(e, context.DeadlineExceeded) || code != -1 {
		t.Fatalf("cancellation exit=%d error=%v", code, e)
	}
	// Optional live acceptance reads private argv outside source; never log it.
	if path := os.Getenv("PFREMOTE_TEST_TERMINAL_SSH_ARGS"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("live SSH input unavailable")
		}
		var args []string
		if json.Unmarshal(data, &args) != nil || len(args) < 2 {
			t.Fatal("invalid live SSH input")
		}
		ssh, err := exec.LookPath(args[0])
		if err != nil {
			t.Fatal("OpenSSH unavailable")
		}
		args = append(args[1:], "-tt", "test -t 0 && test -t 1")
		live, finish := context.WithTimeout(context.Background(), 20*time.Second)
		defer finish()
		code, err := runner.Run(live, ssh, args, nil, nil, nil)
		if err != nil || code != 0 {
			t.Fatalf("live console PTY failed: exit=%d error=%v", code, err)
		}
	}
}
