package activity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

type fakePersistence struct {
	loaded    []contracts.RecentSession
	recorded  []contracts.RecentSession
	loadErr   error
	recordErr error
}

func (f *fakePersistence) LoadRecentSessions(context.Context, int) ([]contracts.RecentSession, error) {
	return append([]contracts.RecentSession(nil), f.loaded...), f.loadErr
}

func (f *fakePersistence) RecordRecentSession(_ context.Context, entry contracts.RecentSession, _ int) error {
	f.recorded = append(f.recorded, entry)
	return f.recordErr
}

func TestStoreKeepsBoundedNewestFirstRedactedSessions(t *testing.T) {
	store := New(2)
	store.now = func() time.Time { return time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC) }
	store.Record("session-1", "pfremote://fabric-example/devices/device-a/capabilities/desktop-a", "open", "opened")
	store.Record("session-2", "pfremote://fabric-example/devices/device-a/capabilities/shell-a", "exec", "completed")
	store.Record("session-3", "pfremote://fabric-example/devices/device-b/capabilities/desktop-b", "open", "opened")
	entries := store.Snapshot()
	if len(entries) != 2 || entries[0].SessionID != "session-3" || entries[1].SessionID != "session-2" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestPersistentStoreLoadsAndRecordsRedactedSessions(t *testing.T) {
	started := time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC)
	persistence := &fakePersistence{loaded: []contracts.RecentSession{{
		SessionID: "session-before-restart", CanonicalTarget: "pfremote://fabric-example/devices/device-a/capabilities/shell-a",
		Action: "exec", Status: "completed", StartedAt: started,
	}}}
	store := NewPersistent(20, persistence)
	if entries := store.Snapshot(); len(entries) != 1 || entries[0].SessionID != "session-before-restart" {
		t.Fatalf("loaded entries = %#v", entries)
	}
	store.now = func() time.Time { return started.Add(time.Minute) }
	store.Record("session-after-restart", "pfremote://fabric-example/devices/device-a/capabilities/desktop-a", "open", "opened")
	if len(persistence.recorded) != 1 || persistence.recorded[0].SessionID != "session-after-restart" {
		t.Fatalf("recorded entries = %#v", persistence.recorded)
	}
}

func TestPersistentStoreKeepsRemoteActionsUsableWhenHistoryPersistenceFails(t *testing.T) {
	persistence := &fakePersistence{loadErr: errors.New("load failed"), recordErr: errors.New("record failed")}
	store := NewPersistent(20, persistence)
	store.Record("session-live", "pfremote://fabric-example/devices/device-a/capabilities/shell-a", "exec", "completed")
	entries := store.Snapshot()
	if len(entries) != 1 || entries[0].SessionID != "session-live" {
		t.Fatalf("in-memory entries = %#v", entries)
	}
}
