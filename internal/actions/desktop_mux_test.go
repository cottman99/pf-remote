package actions

import (
	"context"
	"errors"
	"testing"
)

type targetedDesktop struct {
	target string
	calls  int
	result DesktopRunResult
}

func (r *targetedDesktop) HasTarget(target string) bool { return target == r.target }
func (r *targetedDesktop) Run(context.Context, string) (DesktopRunResult, error) {
	r.calls++
	return r.result, nil
}

type fallbackDesktop struct{ calls int }

func (r *fallbackDesktop) Run(context.Context, string) (DesktopRunResult, error) {
	r.calls++
	return DesktopRunResult{Protocol: "rdp"}, nil
}

func TestDesktopMuxUsesOnlyExactCanonicalOverride(t *testing.T) {
	override := &targetedDesktop{target: "pfremote://fabric/devices/device/capabilities/external", result: DesktopRunResult{Protocol: "external"}}
	fallback := &fallbackDesktop{}
	mux := DesktopMux{Override: override, Fallback: fallback}
	result, err := mux.Run(context.Background(), override.target)
	if err != nil || result.Protocol != "external" || override.calls != 1 || fallback.calls != 0 {
		t.Fatalf("result=%#v err=%v override=%d fallback=%d", result, err, override.calls, fallback.calls)
	}
	if _, err := mux.Run(context.Background(), "pfremote://fabric/devices/device/capabilities/other"); err != nil || fallback.calls != 1 {
		t.Fatalf("fallback err=%v calls=%d", err, fallback.calls)
	}
}

func TestDesktopMuxFailsClosedWithoutMatchingRunner(t *testing.T) {
	_, err := (DesktopMux{}).Run(context.Background(), "pfremote://fabric/devices/device/capabilities/desktop")
	var fault *Fault
	if !errors.As(err, &fault) || fault.Code != "DESKTOP_NOT_READY" {
		t.Fatalf("err=%#v", err)
	}
}
