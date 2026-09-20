// Package linuxinstaller updates only the managed user-level PF Remote binaries.
package linuxinstaller

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var names = []string{"pfremote", "pfremoted", "pfremote-update"}

type journal struct {
	Existing map[string]bool `json:"existing"`
}

func atomicCopy(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(target), ".pfremote-update-")
	if err != nil {
		return err
	}
	temporary := out.Name()
	defer os.Remove(temporary)
	if _, err = io.Copy(out, in); err == nil {
		err = out.Chmod(0700)
	}
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporary, target)
}
func Recover(root string) error {
	work := filepath.Join(root, ".update")
	b, err := os.ReadFile(filepath.Join(work, "pending.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var j journal
	if json.Unmarshal(b, &j) != nil || len(j.Existing) != len(names) {
		return errors.New("invalid update recovery record")
	}
	for _, name := range names {
		if _, ok := j.Existing[name]; !ok {
			return errors.New("incomplete update recovery record")
		}
	}
	for _, name := range names {
		if j.Existing[name] {
			if err := atomicCopy(filepath.Join(work, name), filepath.Join(root, name)); err != nil {
				return err
			}
		} else {
			if err := os.Remove(filepath.Join(root, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return os.Remove(filepath.Join(work, "pending.json"))
}
func Upgrade(root, stage string, activate func() error) error {
	if err := Recover(root); err != nil {
		return err
	}
	work := filepath.Join(root, ".update")
	if err := os.MkdirAll(work, 0700); err != nil {
		return err
	}
	j := journal{Existing: map[string]bool{}}
	for _, name := range names {
		j.Existing[name] = false
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err == nil {
			if !info.Mode().IsRegular() {
				return errors.New("managed binary is not a regular file")
			}
			if err = atomicCopy(path, filepath.Join(work, name)); err != nil {
				return err
			}
			j.Existing[name] = true
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	b, _ := json.Marshal(j)
	f, err := os.OpenFile(filepath.Join(work, "pending.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	f.Close()
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		if err := Recover(root); err != nil {
			return errors.New("update failed; recovery pending")
		}
		if err := activate(); err != nil {
			return errors.New("previous binaries restored; service recovery pending")
		}
		return cause
	}
	for _, name := range names {
		if err = atomicCopy(filepath.Join(stage, name+"-linux-x64"), filepath.Join(root, name)); err != nil {
			return rollback(err)
		}
	}
	if err = activate(); err != nil {
		return rollback(err)
	}
	return os.Remove(filepath.Join(work, "pending.json"))
}
