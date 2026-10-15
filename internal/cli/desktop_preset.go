package cli

import (
	"context"
	"errors"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

func runPreset(ctx context.Context, client actionClient, target string, update *contracts.DesktopPresetUpdate) (contracts.DesktopPreset, error) {
	if c, ok := client.(interface {
		DesktopPreset(context.Context, string, *contracts.DesktopPresetUpdate) (contracts.DesktopPreset, error)
	}); ok {
		return c.DesktopPreset(ctx, target, update)
	}
	return contracts.DesktopPreset{}, errors.New("connection settings unavailable")
}
