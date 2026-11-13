package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

func TestDesktopStatusCacheExpiryNamespaceAndIsolation(t *testing.T) {
	c := newDesktopStatusCache(time.Minute)
	now := time.Unix(100, 0)
	c.now = func() time.Time { return now }
	calls := 0
	probe := func(string) []contracts.RouteOption {
		calls++
		return []contracts.RouteOption{{Adapter: "lan", Status: "available", Order: 1}}
	}
	f := c.wrap("fabric-test:1", probe)
	f("target-a")[0].Status = "changed"
	if got := f("target-a"); calls != 1 || got[0].Status != "available" {
		t.Fatalf("cache result leaked or probe repeated: %d %v", calls, got)
	}
	c.wrap("fabric-test:2", probe)("target-a")
	f("target-b")
	if calls != 3 {
		t.Fatalf("namespace/target not isolated: %d", calls)
	}
	now = now.Add(time.Minute)
	f("target-a")
	if calls != 4 {
		t.Fatalf("expired result reused: %d", calls)
	}
	if c.wrap("x", nil) != nil {
		t.Fatal("nil provider must stay nil")
	}
}

func TestDesktopStatusCacheCoalescesConcurrentReads(t *testing.T) {
	c := newDesktopStatusCache(time.Minute)
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	f := c.wrap("fabric-test:1", func(string) []contracts.RouteOption {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return []contracts.RouteOption{{Adapter: "frp", Status: "unavailable"}}
	})
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if f("target")[0].Status != "unavailable" {
				t.Error("wrong result")
			}
		}()
	}
	<-started
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("concurrent probes: %d", calls.Load())
	}
}

func TestDesktopStatusCacheBoundAndProviderPanic(t *testing.T) {
	c := newDesktopStatusCache(time.Minute)
	f := c.wrap("fabric-test:1", func(string) []contracts.RouteOption { return nil })
	for i := range 300 {
		f(fmt.Sprint(i))
	}
	if len(c.entries) != 256 {
		t.Fatalf("unbounded cache: %d", len(c.entries))
	}
	c = newDesktopStatusCache(time.Minute)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("provider panic missing")
			}
		}()
		c.wrap("x", func(string) []contracts.RouteOption { panic("synthetic") })("target")
	}()
	if len(c.entries) != 0 {
		t.Fatal("panic left a blocked cache entry")
	}
	c.wrap("x", func(string) []contracts.RouteOption { return nil })("target")
}
