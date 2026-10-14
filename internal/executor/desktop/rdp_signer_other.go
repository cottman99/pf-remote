//go:build !windows

package desktop

import "context"

func signRDPProfile(context.Context, string) error { return nil }
