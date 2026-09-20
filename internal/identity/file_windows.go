//go:build windows

package identity

import "os"

func protectDirectory(string) error { return nil }

func validateStoredFile(os.FileInfo) error { return nil }

func syncDirectory(string) error { return nil }
