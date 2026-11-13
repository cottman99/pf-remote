package windowsinstaller

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	d, e := os.MkdirTemp("", "pfremote-installer-test-profile-")
	if e != nil {
		panic(e)
	}
	os.Setenv("APPDATA", d)
	os.Setenv("XDG_CONFIG_HOME", d)
	code := m.Run()
	os.RemoveAll(d)
	os.Exit(code)
}
