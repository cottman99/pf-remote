// Package updatecheck runs independent per-host discovery. It never installs or
// executes downloaded content, and hints never carry a URL or release authority.
package updatecheck

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/cottman99/pf-remote/internal/releaseauth"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type Discover func(context.Context) (releaseauth.Release, error)

type Monitor struct {
	mu                  sync.RWMutex
	checkMu             sync.Mutex
	discover            Discover
	current             string
	dataEpoch, protocol uint64
	check               contracts.Check
	failures            int
	hints               chan struct{}
	Apply               func(context.Context, releaseauth.Release) string
}

func New(discover Discover, current string, dataEpoch, protocol uint64) *Monitor {
	m := &Monitor{discover: discover, current: current, dataEpoch: dataEpoch, protocol: protocol, hints: make(chan struct{}, 1)}
	m.check = contracts.Check{Name: "updates", Status: "pending", Code: "scheduled", Summary: "Background update check is scheduled"}
	if discover == nil {
		m.check.Status = "skip"
		m.check.Code = "bootstrap"
		m.check.Summary = "Automatic updates require a trusted bootstrap installation"
	}
	return m
}

func (m *Monitor) Status() contracts.Check { m.mu.RLock(); defer m.mu.RUnlock(); return m.check }

// Hint coalesces notices; the run loop rate-limits them independently of senders.
func (m *Monitor) Hint() {
	select {
	case m.hints <- struct{}{}:
	default:
	}
}

func (m *Monitor) Check(ctx context.Context) bool {
	m.checkMu.Lock()
	defer m.checkMu.Unlock()
	if m.discover == nil {
		return false
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r, err := m.discover(bounded)
	if err == nil && r.Checkpoint().Sequence == 0 {
		err = errors.New("unverified release")
	}
	m.mu.Lock()
	if ctx.Err() != nil {
		m.mu.Unlock()
		return false
	}
	if err != nil {
		if m.failures < 16 {
			m.failures++
		}
		m.check = contracts.Check{Name: "updates", Status: "pending", Code: "retry", Summary: "Update check could not complete; it will retry automatically"}
		m.mu.Unlock()
		return false
	}
	m.failures = 0
	summary := "A verified update is available; installation safety checks are required"
	status := "pending"
	code := "available"
	if r.Metadata().Version == m.current {
		summary = "This computer is on the latest verified release"
		status = "pass"
		code = "current"
	} else if !r.Compatible(m.dataEpoch, m.protocol) {
		code = "incompatible"
		summary = "An update is available but requires a compatibility review"
	}
	m.check = contracts.Check{Name: "updates", Status: status, Code: code, Summary: summary}
	m.mu.Unlock()
	if code == "available" && m.Apply != nil {
		m.setCode("downloading")
		applyContext, cancelApply := context.WithTimeout(ctx, 15*time.Minute)
		result := m.Apply(applyContext, r)
		cancelApply()
		m.setCode(result)
		if result == "busy" || result == "retry" {
			m.mu.Lock()
			m.failures = 1
			m.mu.Unlock()
			return false
		}
	}
	return true
}

func (m *Monitor) setCode(code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.check.Code = code
	m.check.Status = "pending"
	m.check.Summary = "Automatic update: " + code
}

func (m *Monitor) nextDelay() time.Duration {
	m.mu.RLock()
	failures := m.failures
	m.mu.RUnlock()
	delay := 6 * time.Hour
	if failures > 0 {
		delay = time.Minute * time.Duration(1<<min(failures-1, 9))
		if delay > 6*time.Hour {
			delay = 6 * time.Hour
		}
	}
	// Positive jitter avoids shortening the minimum request interval.
	return delay + time.Duration(rand.Int64N(int64(delay/5)))
}

func (m *Monitor) Run(ctx context.Context) {
	if m.discover == nil {
		return
	}
	timer := time.NewTimer(time.Minute + time.Duration(rand.Int64N(int64(time.Minute))))
	defer timer.Stop()
	var notBefore time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.hints:
			if time.Now().Before(notBefore) {
				continue
			}
		case <-timer.C:
		}
		success := m.Check(ctx)
		delay := m.nextDelay()
		notBefore = time.Now().Add(time.Minute)
		if !success {
			notBefore = time.Now().Add(delay)
		}
		timer.Reset(delay)
	}
}
