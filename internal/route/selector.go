package route

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"
)

type Diagnostic struct {
	Adapter string
	Status  string
	Code    string
	Latency string
}

type NamedProvider struct {
	Adapter  string
	Provider Provider
}

// ReachableProvider verifies that a configured direct endpoint is accepting a
// TCP connection before Smart connect commits the Session to it. This allows
// the selector to continue to the next independent path without changing any
// operating-system network state.
type ReachableProvider struct {
	Provider Provider
	Timeout  time.Duration
	Samples  int
}

type RouteQuality struct {
	Samples   int
	Successes int
	Median    time.Duration
}

type qualityReporter interface {
	RouteQuality() RouteQuality
}

type measuredAcquisition struct {
	Acquisition
	quality RouteQuality
}

func (a measuredAcquisition) RouteQuality() RouteQuality { return a.quality }

func (p ReachableProvider) Acquire(ctx context.Context, request Request) (Acquisition, error) {
	if p.Provider == nil {
		return nil, errors.New("route provider is unavailable")
	}
	acquired, err := p.Provider.Acquire(ctx, request)
	if err != nil || acquired == nil {
		return nil, errors.New("route candidate is unavailable")
	}
	candidate := acquired.Candidate()
	if candidate.Validate() != nil {
		_ = acquired.Close()
		return nil, errors.New("route candidate is invalid")
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 750 * time.Millisecond
	}
	samples := p.Samples
	if samples <= 0 {
		samples = 1
	}
	latencies := make([]time.Duration, 0, samples)
	successes := 0
	address := net.JoinHostPort(candidate.Address, strconv.Itoa(int(candidate.Port)))
	for index := 0; index < samples; index++ {
		started := time.Now()
		dialer := net.Dialer{Timeout: timeout}
		connection, dialErr := dialer.DialContext(ctx, candidate.Network, address)
		latencies = append(latencies, time.Since(started))
		if dialErr == nil {
			successes++
			_ = connection.Close()
		}
		if err := ctx.Err(); err != nil {
			_ = acquired.Close()
			return nil, err
		}
	}
	if successes*3 < samples*2 {
		_ = acquired.Close()
		return nil, errors.New("route endpoint is unreachable")
	}
	return measuredAcquisition{Acquisition: acquired, quality: RouteQuality{
		Samples: samples, Successes: successes, Median: medianDuration(latencies),
	}}, nil
}

// Selector tries independent providers in policy order before a Session starts.
// Once selected, the Session owns one acquisition and never migrates.
type Selector struct {
	Providers   []NamedProvider
	EvaluateAll bool
	Health      *HealthTracker
}

func (s Selector) Acquire(ctx context.Context, request Request) (Acquisition, error) {
	if s.EvaluateAll {
		return s.acquireBest(ctx, request)
	}
	var diagnostics []Diagnostic
	for _, entry := range s.Providers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.Provider == nil || entry.Adapter == "" {
			diagnostics = append(diagnostics, Diagnostic{Adapter: entry.Adapter, Status: "unavailable", Code: "NOT_CONFIGURED"})
			continue
		}
		started := time.Now()
		acquired, err := entry.Provider.Acquire(ctx, request)
		latency := latencyBucket(time.Since(started))
		if err != nil || acquired == nil {
			diagnostics = append(diagnostics, Diagnostic{Adapter: entry.Adapter, Status: "unavailable", Code: "ACQUIRE_FAILED", Latency: latency})
			continue
		}
		candidate := acquired.Candidate()
		if candidate.Adapter != entry.Adapter || candidate.Validate() != nil {
			_ = acquired.Close()
			diagnostics = append(diagnostics, Diagnostic{Adapter: entry.Adapter, Status: "unavailable", Code: "INVALID_CANDIDATE", Latency: latency})
			continue
		}
		diagnostics = append(diagnostics, Diagnostic{Adapter: entry.Adapter, Status: "selected", Code: "READY", Latency: latency})
		return &selection{Acquisition: acquired, diagnostics: diagnostics}, nil
	}
	return nil, errors.New("no authorized route candidate is available")
}

type providerResult struct {
	index       int
	adapter     string
	acquisition Acquisition
	diagnostic  Diagnostic
	quality     RouteQuality
	score       time.Duration
}

func (s Selector) acquireBest(ctx context.Context, request Request) (Acquisition, error) {
	results := make(chan providerResult, len(s.Providers))
	for index, entry := range s.Providers {
		if entry.Provider == nil || entry.Adapter == "" {
			results <- providerResult{index: index, adapter: entry.Adapter, diagnostic: Diagnostic{Adapter: entry.Adapter, Status: "unavailable", Code: "NOT_CONFIGURED"}}
			continue
		}
		go func(index int, entry NamedProvider) {
			started := time.Now()
			acquired, err := entry.Provider.Acquire(ctx, request)
			elapsed := time.Since(started)
			result := providerResult{index: index, adapter: entry.Adapter, acquisition: acquired,
				diagnostic: Diagnostic{Adapter: entry.Adapter, Status: "available", Code: "READY", Latency: latencyBucket(elapsed)},
				quality:    RouteQuality{Samples: 1, Successes: 1, Median: elapsed}}
			if err != nil || acquired == nil {
				if acquired != nil {
					_ = acquired.Close()
				}
				result.acquisition = nil
				result.quality = RouteQuality{Samples: 1, Successes: 0, Median: elapsed}
				result.diagnostic.Status = "unavailable"
				result.diagnostic.Code = "ACQUIRE_FAILED"
				results <- result
				return
			}
			candidate := acquired.Candidate()
			if candidate.Adapter != entry.Adapter || candidate.Validate() != nil {
				_ = acquired.Close()
				result.acquisition = nil
				result.quality.Successes = 0
				result.diagnostic.Status = "unavailable"
				result.diagnostic.Code = "INVALID_CANDIDATE"
				results <- result
				return
			}
			if measured, ok := acquired.(qualityReporter); ok {
				result.quality = measured.RouteQuality()
				result.diagnostic.Latency = latencyBucket(result.quality.Median)
			}
			results <- result
		}(index, entry)
	}

	ordered := make([]providerResult, len(s.Providers))
	for range s.Providers {
		result := <-results
		ordered[result.index] = result
	}
	if err := ctx.Err(); err != nil {
		for _, result := range ordered {
			if result.acquisition != nil {
				_ = result.acquisition.Close()
			}
		}
		return nil, err
	}

	selected := -1
	bestScore := time.Duration(1<<63 - 1)
	for index := range ordered {
		result := &ordered[index]
		if s.Health != nil {
			stable := result.quality.Successes == result.quality.Samples && result.quality.Successes > 0
			s.Health.Record(request.CanonicalTarget, result.adapter, stable, result.quality.Median)
		}
		if result.acquisition == nil {
			continue
		}
		result.score = qualityScore(result.quality)
		if s.Health != nil {
			result.score += s.Health.Penalty(request.CanonicalTarget, result.adapter)
		}
		// Preserve the declared policy order when measured choices are within a
		// human-imperceptible margin. A materially faster or more stable later
		// route can still win.
		if selected < 0 || result.score+20*time.Millisecond < bestScore {
			selected = index
			bestScore = result.score
		}
	}
	if selected < 0 {
		return nil, errors.New("no authorized route candidate is available")
	}
	diagnostics := make([]Diagnostic, 0, len(ordered))
	for index, result := range ordered {
		if result.acquisition != nil {
			if index == selected {
				result.diagnostic.Status = "selected"
			} else {
				result.diagnostic.Status = "available"
				result.diagnostic.Code = "NOT_SELECTED"
				_ = result.acquisition.Close()
			}
		}
		diagnostics = append(diagnostics, result.diagnostic)
	}
	return &selection{Acquisition: ordered[selected].acquisition, diagnostics: diagnostics}, nil
}

type selection struct {
	Acquisition
	diagnostics []Diagnostic
}

func (s *selection) Diagnostics() []Diagnostic {
	return append([]Diagnostic(nil), s.diagnostics...)
}

func (s *selection) RouteQuality() RouteQuality {
	if measured, ok := s.Acquisition.(qualityReporter); ok {
		return measured.RouteQuality()
	}
	return RouteQuality{Samples: 1, Successes: 1}
}

type healthSample struct {
	success bool
	latency time.Duration
}

type HealthTracker struct {
	mu      sync.Mutex
	limit   int
	samples map[string][]healthSample
}

func NewHealthTracker(limit int) *HealthTracker {
	if limit <= 0 {
		limit = 12
	}
	return &HealthTracker{limit: limit, samples: make(map[string][]healthSample)}
}

func (h *HealthTracker) Record(target, adapter string, success bool, latency time.Duration) {
	if h == nil || target == "" || adapter == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	key := target + "\x00" + adapter
	values := append(h.samples[key], healthSample{success: success, latency: latency})
	if len(values) > h.limit {
		values = append([]healthSample(nil), values[len(values)-h.limit:]...)
	}
	h.samples[key] = values
}

func (h *HealthTracker) Penalty(target, adapter string) time.Duration {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	values := h.samples[target+"\x00"+adapter]
	if len(values) == 0 {
		return 0
	}
	failures := 0
	for _, value := range values {
		if !value.success {
			failures++
		}
	}
	return time.Duration(failures) * 400 * time.Millisecond / time.Duration(len(values))
}

func qualityScore(quality RouteQuality) time.Duration {
	if quality.Samples <= 0 {
		return time.Hour
	}
	failures := quality.Samples - quality.Successes
	if failures < 0 {
		failures = 0
	}
	return quality.Median + time.Duration(failures)*250*time.Millisecond
}

func medianDuration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	copyOfValues := append([]time.Duration(nil), values...)
	for index := 1; index < len(copyOfValues); index++ {
		for current := index; current > 0 && copyOfValues[current] < copyOfValues[current-1]; current-- {
			copyOfValues[current], copyOfValues[current-1] = copyOfValues[current-1], copyOfValues[current]
		}
	}
	return copyOfValues[len(copyOfValues)/2]
}

func latencyBucket(value time.Duration) string {
	switch {
	case value < 50*time.Millisecond:
		return "under-50ms"
	case value < 200*time.Millisecond:
		return "50-199ms"
	case value < time.Second:
		return "200-999ms"
	default:
		return "1s-or-more"
	}
}

type Endpoint struct {
	ID      string
	Address string
	Port    uint16
}

type EndpointResolver interface {
	ResolveEndpoint(context.Context, Request) (Endpoint, error)
}

// EndpointProvider turns read-only discovery into a process-scoped candidate.
// It never changes operating-system network configuration.
type EndpointProvider struct {
	Adapter  string
	Resolver EndpointResolver
}

func (p EndpointProvider) Acquire(ctx context.Context, request Request) (Acquisition, error) {
	if p.Resolver == nil || (p.Adapter != "tailscale" && p.Adapter != "lan") {
		return nil, errors.New("endpoint route provider is not configured")
	}
	endpoint, err := p.Resolver.ResolveEndpoint(ctx, request)
	if err != nil {
		return nil, errors.New("route endpoint is unavailable")
	}
	candidate := Candidate{ID: endpoint.ID, Adapter: p.Adapter, Network: "tcp", Address: endpoint.Address, Port: endpoint.Port}
	if err := candidate.Validate(); err != nil {
		return nil, errors.New("route endpoint is invalid")
	}
	return staticAcquisition{candidate: candidate}, nil
}

type staticAcquisition struct{ candidate Candidate }

func (a staticAcquisition) Candidate() Candidate { return a.candidate }
func (staticAcquisition) Close() error           { return nil }
