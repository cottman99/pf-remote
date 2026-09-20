package releaseauth

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestConcurrentAcceptanceNeverLowersCheckpoint(t *testing.T) {
	m, p, key := fixture(t)
	path := filepath.Join(t.TempDir(), "trust.db")
	if err := Bootstrap(path, p.PublicKey, p.Channel, p.Platform); err != nil {
		t.Fatal(err)
	}
	first, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	low := signed(t, m, key)
	m.Sequence++
	high := signed(t, m, key)
	var wg sync.WaitGroup
	for i, s := range []*Store{first, second} {
		wg.Add(1)
		go func(s *Store, doc []byte) { defer wg.Done(); s.Accept(context.Background(), doc, p) }(s, [][]byte{low, high}[i])
	}
	wg.Wait()
	// A busy concurrent transaction may fail closed. Retrying the newest release
	// must succeed and must permanently prevent a stale writer accepting the old.
	if _, err := first.Accept(context.Background(), high, p); err != nil {
		t.Fatal(err)
	}
	_, err = second.Accept(context.Background(), low, p)
	expectCode(t, err, "RELEASE_REPLAY_REJECTED")
}

func TestDurableTrustRestartAndReplay(t *testing.T) {
	m, p, key := fixture(t)
	path := filepath.Join(t.TempDir(), "trust.db")
	if err := Bootstrap(path, p.PublicKey, p.Channel, p.Platform); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Accept(context.Background(), signed(t, m, key), p); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Accept(context.Background(), signed(t, m, key), p); err != nil {
		t.Fatal(err)
	}
	m.Sequence--
	_, err = s.Accept(context.Background(), signed(t, m, key), p)
	expectCode(t, err, "RELEASE_REPLAY_REJECTED")
	m.Sequence++
	m.Version = "0.1.0-alpha.95"
	_, err = s.Accept(context.Background(), signed(t, m, key), p)
	expectCode(t, err, "RELEASE_REPLAY_REJECTED")
	m.Sequence = ^uint64(0)
	if _, err := s.Accept(context.Background(), signed(t, m, key), p); err != nil {
		t.Fatal(err)
	}
}

func TestDurableTrustClockRollback(t *testing.T) {
	m, p, key := fixture(t)
	path := filepath.Join(t.TempDir(), "trust.db")
	if err := Bootstrap(path, p.PublicKey, p.Channel, p.Platform); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	future := p
	future.Now = m.ExpiresAt.Add(time.Hour)
	_, err = s.Accept(context.Background(), signed(t, m, key), future)
	expectCode(t, err, "RELEASE_TIME_INVALID")
	s.Close()
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.Accept(context.Background(), signed(t, m, key), p)
	expectCode(t, err, "RELEASE_TIME_INVALID")
}

func TestDurableTrustFailClosed(t *testing.T) {
	m, p, key := fixture(t)
	path := filepath.Join(t.TempDir(), "trust.db")
	if _, err := OpenStore(path); err == nil {
		t.Fatal("missing trust silently created")
	}
	if err := Bootstrap(path, p.PublicKey, p.Channel, p.Platform); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(path, p.PublicKey, p.Channel, p.Platform); err == nil {
		t.Fatal("bootstrap replaced trust")
	}
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	wrong := p
	wrong.Platform = "linux-x64"
	_, err = s.Accept(context.Background(), signed(t, m, key), wrong)
	expectCode(t, err, "TRUST_STATE_INVALID")
	if _, err = s.db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	r, err := s.Accept(context.Background(), signed(t, m, key), p)
	expectCode(t, err, "TRUST_STATE_WRITE_FAILED")
	if r.Checkpoint().Sequence != 0 {
		t.Fatal("release returned without durable commit")
	}
	s.Close()
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path)
	if err != nil {
		return
	}
	defer s.Close()
	_, err = s.Accept(context.Background(), signed(t, m, key), p)
	if err == nil {
		t.Fatal("corrupt trust accepted")
	}
}
