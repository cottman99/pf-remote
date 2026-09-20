package enrollment

import (
	"context"
	"crypto/ed25519"
	"errors"
	"github.com/cottman99/pf-remote/internal/state"
	"path/filepath"
	"testing"
	"time"
)

type updateTestStore struct {
	rows map[string][]byte
	fail bool
}

func TestUpdatesSQLiteKeepsEnrollmentAndOfflineHint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.db")
	store, err := state.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewPersistentManager(store)
	if err != nil {
		t.Fatal(err)
	}
	owner := makeKeyPair(t, 0x43)
	initializeOwner(t, manager, owner)
	first, err := manager.SyncUpdates(signedUpdate(owner, "sqlite-notice-0001", true))
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = state.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err = NewPersistentManager(store)
	if err != nil {
		t.Fatalf("enrollment broken by update state: %v", err)
	}
	second, err := manager.SyncUpdates(signedUpdate(owner, "sqlite-notice-0002", false))
	if err != nil || second.Hint != first.Hint || len(second.Reports) != 1 {
		t.Fatalf("offline catch-up lost: %+v %v", second, err)
	}
}

func (s *updateTestStore) LoadControlState(_ context.Context, key string) ([]byte, error) {
	return append([]byte{}, s.rows[key]...), nil
}
func (s *updateTestStore) SaveControlState(_ context.Context, key string, b []byte) error {
	if s.fail {
		return errors.New("disk failed")
	}
	s.rows[key] = append([]byte{}, b...)
	return nil
}
func (s *updateTestStore) SaveControlStateWithDeviceStatus(ctx context.Context, key string, b []byte, _ string, _ string, _ uint64, _ uint64) error {
	return s.SaveControlState(ctx, key, b)
}
func signedUpdate(k keyPair, id string, notify bool) UpdateRequest {
	r := UpdateRequest{Schema: UpdateSchema, DeviceID: k.deviceID, RequestID: id, Channel: "preview", Platform: "windows-x64", Version: "0.1.0-alpha.97", Status: "current", Notify: notify}
	r.Signature = EncodeSignature(ed25519.Sign(k.private, UpdateMessage(r)))
	return r
}
func TestUpdatesDurableHintReplayAndRevocation(t *testing.T) {
	store := &updateTestStore{rows: map[string][]byte{}}
	m, err := NewPersistentManager(store)
	if err != nil {
		t.Fatal(err)
	}
	owner := makeKeyPair(t, 0x41)
	initializeOwner(t, m, owner)
	req := signedUpdate(owner, "updates-first-0001", true)
	first, err := m.SyncUpdates(req)
	if err != nil || first.Hint != 1 || len(first.Reports) != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	restarted, err := NewPersistentManager(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.SyncUpdates(req); faultCode(err) != "REQUEST_REPLAYED" {
		t.Fatalf("replay %v", err)
	}
	got, err := restarted.SyncUpdates(signedUpdate(owner, "updates-next-0002", true))
	if err != nil || got.Hint != first.Hint {
		t.Fatalf("coalescing %+v %v", got, err)
	}
	restarted.now = func() time.Time { return first.Reports[0].Seen.Add(time.Minute) }
	got, err = restarted.SyncUpdates(signedUpdate(owner, "updates-next-0003", true))
	if err != nil || got.Hint != 2 {
		t.Fatalf("next %+v %v", got, err)
	}
	altered := signedUpdate(owner, "updates-next-0004", false)
	altered.Notify = true
	if _, err = restarted.SyncUpdates(altered); err == nil {
		t.Fatal("accepted unsigned notify alteration")
	}
	restarted.devices[owner.deviceID].status = "revoked"
	if _, err = restarted.SyncUpdates(signedUpdate(owner, "updates-next-0005", true)); err == nil {
		t.Fatal("accepted revoked reporter")
	}
}
func TestUpdatesPersistenceFailureDoesNotReportSuccess(t *testing.T) {
	store := &updateTestStore{rows: map[string][]byte{}}
	m, _ := NewPersistentManager(store)
	owner := makeKeyPair(t, 0x42)
	initializeOwner(t, m, owner)
	store.fail = true
	if _, err := m.SyncUpdates(signedUpdate(owner, "updates-failure-0001", true)); err == nil {
		t.Fatal("reported success after failed save")
	}
}
func (s *updateTestStore) LoadUpdateState(ctx context.Context, key string) ([]byte, error) {
	return s.LoadControlState(ctx, key)
}
func (s *updateTestStore) SaveUpdateState(ctx context.Context, key string, b []byte) error {
	return s.SaveControlState(ctx, key, b)
}
