package main

import (
	"sync"
	"time"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

type desktopStatusKey struct{ namespace, target string }
type desktopStatusEntry struct {
	ready   chan struct{}
	options []contracts.RouteOption
	expires time.Time
}

// Cache only presentation hints. Catalog authorization is evaluated on every
// request and actual connections still acquire and validate fresh routes.
type desktopStatusCache struct {
	mu      sync.Mutex
	entries map[desktopStatusKey]*desktopStatusEntry
	ttl     time.Duration
	now     func() time.Time
}

func newDesktopStatusCache(ttl time.Duration) *desktopStatusCache {
	return &desktopStatusCache{entries: make(map[desktopStatusKey]*desktopStatusEntry), ttl: ttl, now: time.Now}
}

func (c *desktopStatusCache) wrap(namespace string, probe func(string) []contracts.RouteOption) func(string) []contracts.RouteOption {
	if probe == nil {
		return nil
	}
	return func(target string) []contracts.RouteOption {
		key := desktopStatusKey{namespace, target}
		for {
			c.mu.Lock()
			if e := c.entries[key]; e != nil {
				select {
				case <-e.ready:
					if c.now().Before(e.expires) {
						result := append([]contracts.RouteOption(nil), e.options...)
						c.mu.Unlock()
						return result
					}
					delete(c.entries, key)
				default:
					c.mu.Unlock()
					<-e.ready
					continue
				}
			}
			// Keep memory bounded even if targets or directory versions churn.
			if len(c.entries) >= 256 {
				for k, e := range c.entries {
					select {
					case <-e.ready:
						if !c.now().Before(e.expires) {
							delete(c.entries, k)
						}
					default:
					}
				}
				if len(c.entries) >= 256 {
					c.mu.Unlock()
					return probe(target)
				}
			}
			e := &desktopStatusEntry{ready: make(chan struct{})}
			c.entries[key] = e
			c.mu.Unlock()
			// Release waiters even if a provider panics; never cache that result.
			func() {
				completed := false
				defer func() {
					c.mu.Lock()
					if !completed {
						delete(c.entries, key)
					}
					close(e.ready)
					c.mu.Unlock()
				}()
				options := probe(target)
				c.mu.Lock()
				e.options = append([]contracts.RouteOption(nil), options...)
				e.expires = c.now().Add(c.ttl)
				c.mu.Unlock()
				completed = true
			}()
			return append([]contracts.RouteOption(nil), e.options...)
		}
	}
}
