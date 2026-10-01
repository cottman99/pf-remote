package desktop

import "context"

type audioCaptureKey struct{}

// WithAudioCapture carries explicit, request-scoped microphone consent through
// the same target authorization and route selection as an ordinary open.
// It is never persisted or inferred from another desktop session.
func WithAudioCapture(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, audioCaptureKey{}, enabled)
}

func AudioCaptureRequested(ctx context.Context) bool {
	enabled, _ := ctx.Value(audioCaptureKey{}).(bool)
	return enabled
}
