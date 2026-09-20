// Package activity keeps a bounded, redacted view of recent PF Remote sessions.
package activity

import (
	"context"
	"sync"
	"time"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

// Persistence stores only the already-redacted recent-session contract. A
// persistence failure must never fail or delay the authorized remote action.
type Persistence interface {
	LoadRecentSessions(context.Context, int) ([]contracts.RecentSession, error)
	RecordRecentSession(context.Context, contracts.RecentSession, int) error
}

type Store struct {
	mu      sync.Mutex
	limit   int
	now     func() time.Time
	entries []contracts.RecentSession
	persist Persistence
}

func New(limit int) *Store {
	if limit < 1 {
		limit = 20
	}
	return &Store{limit: limit, now: time.Now}
}

func NewPersistent(limit int, persistence Persistence) *Store {
	store := New(limit)
	store.persist = persistence
	if persistence == nil {
		return store
	}
	entries, err := persistence.LoadRecentSessions(context.Background(), store.limit)
	if err == nil {
		store.entries = append([]contracts.RecentSession(nil), entries...)
	}
	return store
}

func (s *Store) Record(sessionID, canonicalTarget, action, status string) {
	if s == nil || sessionID == "" || canonicalTarget == "" || action == "" || status == "" {
		return
	}
	s.mu.Lock()
	entry := contracts.RecentSession{SessionID: sessionID, CanonicalTarget: canonicalTarget, Action: action, Status: status, StartedAt: s.now().UTC()}
	s.entries = append([]contracts.RecentSession{entry}, s.entries...)
	if len(s.entries) > s.limit {
		s.entries = s.entries[:s.limit]
	}
	persistence := s.persist
	limit := s.limit
	s.mu.Unlock()
	if persistence != nil {
		_ = persistence.RecordRecentSession(context.Background(), entry, limit)
	}
}

func (s *Store) Snapshot() []contracts.RecentSession {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]contracts.RecentSession(nil), s.entries...)
}
