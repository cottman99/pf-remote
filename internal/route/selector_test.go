package route

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

type testProvider struct {
	acquisition Acquisition
	err         error
	calls       int
}

func TestReachableProviderLetsSmartSelectionSkipADeadEndpoint(t *testing.T) {
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadPort := dead.Addr().(*net.TCPAddr).Port
	_ = dead.Close()
	live, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	go func() {
		connection, acceptErr := live.Accept()
		if acceptErr == nil {
			_ = connection.Close()
		}
	}()
	lanAcquisition := &testAcquisition{candidate: Candidate{ID: "route-lan-dead", Adapter: "lan", Network: "tcp", Address: "127.0.0.1", Port: uint16(deadPort)}}
	tailscaleAcquisition := &testAcquisition{candidate: Candidate{ID: "route-tailscale-live", Adapter: "tailscale", Network: "tcp", Address: "127.0.0.1", Port: uint16(live.Addr().(*net.TCPAddr).Port)}}
	selector := Selector{Providers: []NamedProvider{
		{Adapter: "lan", Provider: ReachableProvider{Provider: &testProvider{acquisition: lanAcquisition}}},
		{Adapter: "tailscale", Provider: ReachableProvider{Provider: &testProvider{acquisition: tailscaleAcquisition}}},
	}}
	acquired, err := selector.Acquire(context.Background(), Request{})
	if err != nil || acquired.Candidate().Adapter != "tailscale" || lanAcquisition.closes != 1 {
		t.Fatalf("candidate=%#v dead_closes=%d err=%v", acquired, lanAcquisition.closes, err)
	}
}

func TestReachableProviderUsesMultipleSamplesAndRejectsAnUnstableEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for index := 0; index < 3; index++ {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			_ = connection.Close()
		}
	}()
	acquisition := &testAcquisition{candidate: Candidate{ID: "route-lan-sampled", Adapter: "lan", Network: "tcp", Address: "127.0.0.1", Port: uint16(listener.Addr().(*net.TCPAddr).Port)}}
	provider := ReachableProvider{Provider: &testProvider{acquisition: acquisition}, Samples: 3}
	acquired, err := provider.Acquire(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	quality := acquired.(interface{ RouteQuality() RouteQuality }).RouteQuality()
	if quality.Samples != 3 || quality.Successes != 3 || quality.Median <= 0 {
		t.Fatalf("quality=%#v", quality)
	}
}

func (p *testProvider) Acquire(context.Context, Request) (Acquisition, error) {
	p.calls++
	return p.acquisition, p.err
}

type testAcquisition struct {
	candidate Candidate
	closes    int
}

func (a *testAcquisition) Candidate() Candidate { return a.candidate }
func (a *testAcquisition) Close() error         { a.closes++; return nil }

func TestSelectorPrefersGatewayAndDoesNotProbeLaterRoutes(t *testing.T) {
	frp := &testProvider{acquisition: &testAcquisition{candidate: Candidate{ID: "route-frp-01", Adapter: "frp", Network: "tcp", Address: "127.0.0.1", Port: 2201}}}
	tailscale := &testProvider{acquisition: &testAcquisition{candidate: Candidate{ID: "route-ts-01", Adapter: "tailscale", Network: "tcp", Address: "192.0.2.20", Port: 22}}}
	selector := Selector{Providers: []NamedProvider{{Adapter: "frp", Provider: frp}, {Adapter: "tailscale", Provider: tailscale}}}
	acquired, err := selector.Acquire(context.Background(), Request{})
	if err != nil || acquired.Candidate().Adapter != "frp" || frp.calls != 1 || tailscale.calls != 0 {
		t.Fatalf("candidate=%#v err=%v calls=%d/%d", acquired.Candidate(), err, frp.calls, tailscale.calls)
	}
}

func TestSelectorFallsBackBeforeSessionAndReturnsSafeDiagnostics(t *testing.T) {
	frp := &testProvider{err: errors.New("private relay address")}
	tailscale := &testProvider{acquisition: &testAcquisition{candidate: Candidate{ID: "route-ts-01", Adapter: "tailscale", Network: "tcp", Address: "192.0.2.20", Port: 22}}}
	selector := Selector{Providers: []NamedProvider{{Adapter: "frp", Provider: frp}, {Adapter: "tailscale", Provider: tailscale}}}
	acquired, err := selector.Acquire(context.Background(), Request{})
	if err != nil || acquired.Candidate().Adapter != "tailscale" {
		t.Fatal(err)
	}
	diagnostics := acquired.(interface{ Diagnostics() []Diagnostic }).Diagnostics()
	if len(diagnostics) != 2 || diagnostics[0].Code != "ACQUIRE_FAILED" || diagnostics[1].Status != "selected" {
		t.Fatalf("diagnostics=%#v", diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "private relay address" {
			t.Fatalf("private error leaked: %#v", diagnostics)
		}
	}
}

type qualityAcquisition struct {
	*testAcquisition
	quality RouteQuality
}

func (a qualityAcquisition) RouteQuality() RouteQuality { return a.quality }

func TestSmartSelectorMeasuresAllRoutesAndChoosesMateriallyBetterQuality(t *testing.T) {
	lan := &testProvider{acquisition: qualityAcquisition{
		testAcquisition: &testAcquisition{candidate: Candidate{ID: "route-lan-01", Adapter: "lan", Network: "tcp", Address: "192.0.2.10", Port: 3389}},
		quality:         RouteQuality{Samples: 3, Successes: 2, Median: 40 * time.Millisecond},
	}}
	tailscale := &testProvider{acquisition: qualityAcquisition{
		testAcquisition: &testAcquisition{candidate: Candidate{ID: "route-ts-01", Adapter: "tailscale", Network: "tcp", Address: "192.0.2.20", Port: 3389}},
		quality:         RouteQuality{Samples: 3, Successes: 3, Median: 75 * time.Millisecond},
	}}
	selector := Selector{EvaluateAll: true, Health: NewHealthTracker(8), Providers: []NamedProvider{
		{Adapter: "lan", Provider: lan},
		{Adapter: "tailscale", Provider: tailscale},
	}}
	acquired, err := selector.Acquire(context.Background(), Request{CanonicalTarget: "pfremote://fabric-test/devices/device-01/capabilities/desktop-01"})
	if err != nil || acquired.Candidate().Adapter != "tailscale" || lan.calls != 1 || tailscale.calls != 1 {
		t.Fatalf("candidate=%#v calls=%d/%d err=%v", acquired, lan.calls, tailscale.calls, err)
	}
	diagnostics := acquired.(interface{ Diagnostics() []Diagnostic }).Diagnostics()
	if len(diagnostics) != 2 || diagnostics[0].Code != "NOT_SELECTED" || diagnostics[1].Status != "selected" {
		t.Fatalf("diagnostics=%#v", diagnostics)
	}
}

func TestSmartSelectorKeepsPolicyOrderWithinSmallLatencyMargin(t *testing.T) {
	lan := &testProvider{acquisition: qualityAcquisition{
		testAcquisition: &testAcquisition{candidate: Candidate{ID: "route-lan-02", Adapter: "lan", Network: "tcp", Address: "192.0.2.10", Port: 3389}},
		quality:         RouteQuality{Samples: 3, Successes: 3, Median: 45 * time.Millisecond},
	}}
	tailscale := &testProvider{acquisition: qualityAcquisition{
		testAcquisition: &testAcquisition{candidate: Candidate{ID: "route-ts-02", Adapter: "tailscale", Network: "tcp", Address: "192.0.2.20", Port: 3389}},
		quality:         RouteQuality{Samples: 3, Successes: 3, Median: 30 * time.Millisecond},
	}}
	selector := Selector{EvaluateAll: true, Providers: []NamedProvider{
		{Adapter: "lan", Provider: lan},
		{Adapter: "tailscale", Provider: tailscale},
	}}
	acquired, err := selector.Acquire(context.Background(), Request{CanonicalTarget: "pfremote://fabric-test/devices/device-01/capabilities/desktop-02"})
	if err != nil || acquired.Candidate().Adapter != "lan" {
		t.Fatalf("candidate=%#v err=%v", acquired, err)
	}
}

func TestSelectorFailsWhenCanceledOrNoCandidate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &testProvider{}
	selector := Selector{Providers: []NamedProvider{{Adapter: "frp", Provider: provider}}}
	if _, err := selector.Acquire(ctx, Request{}); !errors.Is(err, context.Canceled) || provider.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, provider.calls)
	}
	if _, err := selector.Acquire(context.Background(), Request{}); err == nil {
		t.Fatal("empty candidate set succeeded")
	}
}

func TestNewAcquisitionSelectsAgain(t *testing.T) {
	frp := &testProvider{err: errors.New("offline")}
	tailscale := &testProvider{acquisition: &testAcquisition{candidate: Candidate{ID: "route-ts-01", Adapter: "tailscale", Network: "tcp", Address: "192.0.2.20", Port: 22}}}
	selector := Selector{Providers: []NamedProvider{{Adapter: "frp", Provider: frp}, {Adapter: "tailscale", Provider: tailscale}}}
	first, err := selector.Acquire(context.Background(), Request{})
	if err != nil || first.Candidate().Adapter != "tailscale" {
		t.Fatal(err)
	}
	frp.err = nil
	frp.acquisition = &testAcquisition{candidate: Candidate{ID: "route-frp-02", Adapter: "frp", Network: "tcp", Address: "127.0.0.1", Port: 2202}}
	second, err := selector.Acquire(context.Background(), Request{})
	if err != nil || second.Candidate().Adapter != "frp" {
		t.Fatalf("second=%#v err=%v", second, err)
	}
}

type fixedEndpointResolver struct {
	endpoint Endpoint
	err      error
}

func (r fixedEndpointResolver) ResolveEndpoint(context.Context, Request) (Endpoint, error) {
	return r.endpoint, r.err
}

func TestEndpointProvidersAreReadOnlyIndependentCandidates(t *testing.T) {
	request := Request{CanonicalTarget: "pfremote://fabric-test/devices/device-target/capabilities/shell-main", AuthorizationExpiry: time.Now().Add(time.Hour)}
	for _, adapter := range []string{"tailscale", "lan"} {
		provider := EndpointProvider{Adapter: adapter, Resolver: fixedEndpointResolver{endpoint: Endpoint{ID: "route-" + adapter + "-01", Address: "192.0.2.30", Port: 22}}}
		acquired, err := provider.Acquire(context.Background(), request)
		if err != nil || acquired.Candidate().Adapter != adapter {
			t.Fatalf("adapter=%s candidate=%#v err=%v", adapter, acquired.Candidate(), err)
		}
	}
}
