package powerresume

import (
	"testing"
	"time"
)

func TestMonitorRefreshesOnlyAfterResumeSizedGap(t *testing.T) {
	start := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	monitor := NewMonitor(start)
	if monitor.Observe(start.Add(5 * time.Second)) {
		t.Fatal("ordinary ticker interval must not request wake recovery")
	}
	resumed := start.Add(time.Minute)
	if !monitor.Observe(resumed) {
		t.Fatal("resume recovery must refresh during the bounded hold")
	}
	for elapsed := 5 * time.Second; elapsed < 10*time.Minute; elapsed += 5 * time.Second {
		if !monitor.Observe(resumed.Add(elapsed)) {
			t.Fatal("resume recovery ended before the bounded hold")
		}
	}
	if monitor.Observe(resumed.Add(10 * time.Minute)) {
		t.Fatal("resume recovery must stop after the bounded hold")
	}
}
