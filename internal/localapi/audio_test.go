package localapi

import (
	"context"
	"testing"

	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/internal/desktop"
)

type audioDesktop struct {
	capture bool
	target  string
}

func (a *audioDesktop) Run(ctx context.Context, target string) (actions.DesktopRunResult, error) {
	a.capture = desktop.AudioCaptureRequested(ctx)
	a.target = target
	return actions.DesktopRunResult{SessionID: "synthetic-audio-session", Protocol: "rdp"}, nil
}

func TestMicrophoneConsentIsExplicitAndRequestScoped(t *testing.T) {
	service := actions.New()
	runner := &audioDesktop{}
	service.Desktop = runner
	handler := Handler{Service: service}
	for _, capture := range []bool{false, true, false} {
		response := handler.Handle(Request{SchemaVersion: SchemaVersion, Action: "open", Target: "compute-node/desktop", AudioCapture: capture})
		if response.Error != nil || runner.capture != capture || runner.target == "compute-node/desktop" {
			t.Fatalf("capture=%v response=%#v", capture, response)
		}
	}
	for i := range service.Catalog.Capabilities {
		if service.Catalog.Capabilities[i].ID == "desktop-main" {
			service.Catalog.Capabilities[i].DesktopProfile.Protocol = "vnc"
		}
	}
	response := (Handler{Service: service}).Handle(Request{SchemaVersion: SchemaVersion, Action: "open", Target: "compute-node/desktop", AudioCapture: true})
	if response.Error == nil || response.Error.Code != "DESKTOP_AUDIO_UNSUPPORTED" {
		t.Fatalf("response=%#v", response)
	}
}
