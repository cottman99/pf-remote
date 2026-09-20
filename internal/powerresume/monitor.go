package powerresume

import "time"

const (
	defaultGap  = 30 * time.Second
	defaultHold = 10 * time.Minute
)

type Monitor struct {
	last         time.Time
	holdUntil    time.Time
	suspendGap   time.Duration
	recoveryHold time.Duration
}

func NewMonitor(now time.Time) *Monitor {
	return &Monitor{last: now, suspendGap: defaultGap, recoveryHold: defaultHold}
}

// Observe returns true while the daemon should refresh Windows' system-required
// idle timer. A large wall-clock gap is treated as resume evidence; one-shot
// refreshes avoid leaving a persistent power request behind after a crash.
func (m *Monitor) Observe(now time.Time) bool {
	if m == nil {
		return false
	}
	if !m.last.IsZero() && now.Sub(m.last) >= m.suspendGap {
		m.holdUntil = now.Add(m.recoveryHold)
	}
	m.last = now
	return !m.holdUntil.IsZero() && now.Before(m.holdUntil)
}
