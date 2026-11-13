// Package peerguard prevents supported maintenance from losing peer authority.
package peerguard

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const marker = "peer-authority-required-v1"

func DefaultDirectory() (string, error) {
	p, e := os.UserConfigDir()
	return filepath.Join(p, "PF Remote", "state"), e
}
func Require(directory string) error {
	if e := os.MkdirAll(directory, 0700); e != nil {
		return e
	}
	p := filepath.Join(directory, marker)
	if b, e := os.ReadFile(p); e == nil {
		if string(b) != "pfremote.peer-authority/v1\n" {
			return errors.New("invalid peer authority floor")
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if _, e = f.WriteString("pfremote.peer-authority/v1\n"); e != nil {
		return e
	}
	return f.Sync()
}
func Required() (bool, error) {
	p, e := DefaultDirectory()
	if e != nil {
		return false, e
	}
	return RequiredAt(p)
}
func RequiredAt(p string) (bool, error) {
	b, e := os.ReadFile(filepath.Join(p, marker))
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if string(b) != "pfremote.peer-authority/v1\n" {
		return false, errors.New("invalid peer authority floor")
	}
	return true, nil
}

var versionPattern = regexp.MustCompile(`^0\.1\.0-alpha\.([0-9]+)$`)

func CheckVersion(v string) error {
	required, e := Required()
	if e != nil {
		return e
	}
	if !required {
		return nil
	}
	m := versionPattern.FindStringSubmatch(strings.TrimPrefix(v, "v"))
	if len(m) == 2 {
		n, e := strconv.Atoi(m[1])
		if e == nil && n >= 119 {
			return nil
		}
	}
	return errors.New("peer authority requires a compatible release; legacy downgrade refused")
}
