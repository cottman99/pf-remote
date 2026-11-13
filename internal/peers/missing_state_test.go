package peers

import (
	"context"
	"github.com/cottman99/pf-remote/internal/peerguard"
	"github.com/cottman99/pf-remote/internal/state"
	"path/filepath"
	"testing"
	"time"
)

func TestActivatedPeerStateCannotDisappearIntoLegacy(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "peer.db")
	if _, a, e := ActiveSnapshot(context.Background(), p, time.Now()); e != nil || a {
		t.Fatal("legacy compatibility changed")
	}
	if e := peerguard.Require(d); e != nil {
		t.Fatal(e)
	}
	if _, _, e := ActiveSnapshot(context.Background(), p, time.Now()); e == nil {
		t.Fatal("missing authority silently fell back")
	}
	db, e := state.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	if _, _, e = ActiveSnapshot(context.Background(), p, time.Now()); e == nil {
		t.Fatal("empty authority silently fell back")
	}
}
