package main

import (
	"context"
	"errors"
	"github.com/cottman99/pf-remote/internal/state"
	"testing"
	"time"
)

type renewalStore struct {
	observed            time.Time
	clockErr, commitErr error
	commits             int
}

func (s *renewalStore) ObserveAuthorizationTime(context.Context, time.Time) (time.Time, error) {
	return s.observed, s.clockErr
}
func (s *renewalStore) CommitSnapshot(context.Context, state.Snapshot) error {
	s.commits++
	return s.commitErr
}
func TestControlledConnectionRenewalSchedule(t *testing.T) {
	now := time.Now()
	if connectionSyncDue(false, false, false, now, now.Add(time.Hour-time.Second)) {
		t.Fatal("node refresh too early")
	}
	if !connectionSyncDue(false, false, false, now, now.Add(time.Hour)) {
		t.Fatal("node never refreshes")
	}
	if !connectionSyncDue(false, false, true, now, now.Add(5*time.Second)) {
		t.Fatal("node failure never retries")
	}
}
func TestRenewalCommitUsesPersistentClockAndPropagatesFailure(t *testing.T) {
	now := time.Now()
	snapshot := state.Snapshot{CapturedAt: now}
	for _, name := range []string{"success", "rollback", "clock-write", "snapshot-write"} {
		t.Run(name, func(t *testing.T) {
			s := &renewalStore{observed: now}
			switch name {
			case "rollback":
				s.observed = now.Add(time.Second)
			case "clock-write":
				s.clockErr = errors.New("write failed")
			case "snapshot-write":
				s.commitErr = errors.New("write failed")
			}
			err := commitSynchronizedSnapshot(context.Background(), s, snapshot, 0, true)
			if (err == nil) != (name == "success") {
				t.Fatalf("unexpected result %v", err)
			}
			if (name == "rollback" || name == "clock-write") && s.commits != 0 {
				t.Fatal("unsafe snapshot committed")
			}
			if name == "success" && s.commits != 1 {
				t.Fatal("zero-import renewal was not persisted")
			}
		})
	}
}
