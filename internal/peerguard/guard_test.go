package peerguard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFloorPreservesLegacyUntilActivation(t *testing.T) {
	d := t.TempDir()
	t.Setenv("APPDATA", d)
	t.Setenv("XDG_CONFIG_HOME", d)
	if e := CheckVersion("0.1.0-alpha.118"); e != nil {
		t.Fatal(e)
	}
	p, e := DefaultDirectory()
	if e != nil {
		t.Fatal(e)
	}
	if e = Require(p); e != nil {
		t.Fatal(e)
	}
	for _, v := range []string{"0.1.0-alpha.118", "development", "0.1.0-alpha.118-peer.2", "../../119", "0.1.0-alpha.119-junk"} {
		if CheckVersion(v) == nil {
			t.Fatalf("accepted %s", v)
		}
	}
	for _, v := range []string{"0.1.0-alpha.119", "0.1.0-alpha.120"} {
		if e = CheckVersion(v); e != nil {
			t.Fatal(e)
		}
	}
	if e = Require(p); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(p, marker), []byte("bad"), 0600)
	if CheckVersion("0.1.0-alpha.119") == nil {
		t.Fatal("corrupt marker accepted")
	}
}
