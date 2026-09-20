package updatecheck

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/releaseauth"
)

func verified(t *testing.T, compat *releaseauth.Compatibility) releaseauth.Release {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m := releaseauth.Metadata{SchemaVersion: releaseauth.MetadataSchema, Product: "PF Remote", Version: "0.1.0-alpha.95", Channel: "preview", Platform: "windows-x64", Sequence: 1, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), Artifacts: []releaseauth.Artifact{{Name: "payload.zip", Size: 1, SHA256: strings.Repeat("0", 64)}}, Compatibility: compat}
	if compat != nil {
		m.SchemaVersion = releaseauth.MetadataSchemaV2
	}
	doc, err := releaseauth.Sign(m, func(b []byte) ([]byte, error) { return ed25519.Sign(key, b), nil })
	if err != nil {
		t.Fatal(err)
	}
	r, err := releaseauth.Verify(doc, releaseauth.Policy{PublicKey: pub, Channel: m.Channel, Platform: m.Platform, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDiscoveryStatusAndBackoff(t *testing.T) {
	r := verified(t, &releaseauth.Compatibility{DataEpoch: 1, ProtocolMin: 1, ProtocolMax: 1})
	fail := true
	m := New(func(context.Context) (releaseauth.Release, error) {
		if fail {
			return releaseauth.Release{}, errors.New("private transport detail")
		}
		return r, nil
	}, "0.1.0-alpha.94", 1, 1)
	if m.Check(context.Background()) {
		t.Fatal("failed transport accepted")
	}
	if strings.Contains(m.Status().Summary, "private") {
		t.Fatal("unsafe error exposed")
	}
	if d := m.nextDelay(); d < time.Minute || d > 72*time.Second {
		t.Fatalf("bad first retry %v", d)
	}
	for i := 0; i < 20; i++ {
		m.Check(context.Background())
	}
	if d := m.nextDelay(); d < 6*time.Hour || d > 432*time.Minute {
		t.Fatalf("unbounded retry %v", d)
	}
	fail = false
	if !m.Check(context.Background()) || !strings.Contains(m.Status().Summary, "verified update") {
		t.Fatal(m.Status())
	}
	m.current = r.Metadata().Version
	if !m.Check(context.Background()) || m.Status().Status != "pass" {
		t.Fatal(m.Status())
	}
}

func TestMissingCompatibilityAndDisabledBootstrap(t *testing.T) {
	r := verified(t, nil)
	m := New(func(context.Context) (releaseauth.Release, error) { return r, nil }, "0.1.0-alpha.94", 1, 1)
	m.Check(context.Background())
	if !strings.Contains(m.Status().Summary, "compatibility review") {
		t.Fatal(m.Status())
	}
	disabled := New(nil, "development", 1, 1)
	if disabled.Status().Status != "skip" {
		t.Fatal(disabled.Status())
	}
	for i := 0; i < 100; i++ {
		disabled.Hint()
	}
	if len(disabled.hints) != 1 {
		t.Fatal("hints not coalesced")
	}
	disabled.Run(context.Background())
}

func TestCancelledCheckDoesNotOverwriteStatus(t *testing.T) {
	m := New(func(ctx context.Context) (releaseauth.Release, error) {
		<-ctx.Done()
		return releaseauth.Release{}, ctx.Err()
	}, "development", 1, 1)
	before := m.Status()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.Check(ctx)
	if m.Status() != before {
		t.Fatal("shutdown changed status")
	}
}

func TestRunHonorsFailureBackoffDespiteHintsAndStops(t *testing.T) {
	calls := make(chan struct{}, 10)
	m := New(func(context.Context) (releaseauth.Release, error) {
		calls <- struct{}{}
		return releaseauth.Release{}, errors.New("offline")
	}, "development", 1, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	m.Hint()
	go func() { m.Run(ctx); close(done) }()
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("hint did not trigger discovery")
	}
	for i := 0; i < 100; i++ {
		m.Hint()
	}
	select {
	case <-calls:
		t.Fatal("hint flood bypassed failure backoff")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("poller did not stop")
	}
}
