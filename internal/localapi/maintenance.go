package localapi

import (
	"sync"
	"time"
)

// ActionGate stops new local remote actions while an updater hands off to Setup.
// The lease expires if Setup cannot start or the handoff is interrupted.
type ActionGate struct {
	mu     sync.Mutex
	active int
	until  time.Time
}

func (g *ActionGate) Enter() (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if time.Now().Before(g.until) {
		return nil, false
	}
	g.active++
	return func() { g.mu.Lock(); g.active--; g.mu.Unlock() }, true
}
func (g *ActionGate) BeginUpdate() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active != 0 || time.Now().Before(g.until) {
		return false
	}
	g.until = time.Now().Add(2 * time.Minute)
	return true
}
func (g *ActionGate) CancelUpdate() { g.mu.Lock(); g.until = time.Time{}; g.mu.Unlock() }
