//go:build windows

package powerresume

import (
	"errors"

	"golang.org/x/sys/windows"
)

const executionStateSystemRequired = 0x00000001

var setThreadExecutionState = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")

func Refresh() error {
	result, _, _ := setThreadExecutionState.Call(executionStateSystemRequired)
	if result == 0 {
		return errors.New("refresh Windows wake recovery")
	}
	return nil
}
