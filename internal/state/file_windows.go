//go:build windows

package state

func protectStateDirectory(string) error { return nil }

func protectStateFile(string) error { return nil }
