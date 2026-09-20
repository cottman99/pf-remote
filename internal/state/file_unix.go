//go:build !windows

package state

import (
	"fmt"
	"os"
)

func protectStateDirectory(path string) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("protect state directory: %w", safeError(err))
	}
	return nil
}

func protectStateFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("protect state database: %w", safeError(err))
	}
	return nil
}
