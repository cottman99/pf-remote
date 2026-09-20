//go:build !windows

package identity

import (
	"errors"
	"fmt"
	"os"
)

func protectDirectory(path string) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("protect identity directory: %w", safeError(err))
	}
	return nil
}

func validateStoredFile(info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("identity store permissions allow access outside the current user")
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open identity directory for sync: %w", safeError(err))
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync identity directory: %w", safeError(err))
	}
	return nil
}
