//go:build !windows

package windowsinstaller

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func lockFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func replaceState(source, target string) error {
	if err := os.Rename(source, target); err != nil {
		return err
	}
	f, err := os.Open(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
