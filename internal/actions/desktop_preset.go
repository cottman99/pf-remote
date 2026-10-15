package actions

import (
	"github.com/cottman99/pf-remote/internal/desktopdisplay"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type DesktopPresetStore interface {
	ReadPreset(string, string, string) (contracts.DesktopPreset, error)
	SavePreset(string, string, string, contracts.DesktopPresetUpdate) (contracts.DesktopPreset, error)
}

func (s Service) DesktopPreset(input string, update *contracts.DesktopPresetUpdate) (contracts.DesktopPreset, error) {
	i, err := s.Inspect(input)
	if err != nil || !i.Target.Granted || i.Target.Authorization.Status != "active" {
		return contracts.DesktopPreset{}, &Fault{Code: "TARGET_NOT_FOUND", Stage: "authorize", Summary: "This desktop is no longer authorized.", Remediation: "Refresh the computer list."}
	}
	t := i.Target
	p := t.Capability.DesktopProfile
	if t.Capability.Kind != contracts.CapabilityDesktop || p == nil || p.Protocol != "rdp" || s.DesktopPresets == nil {
		return contracts.DesktopPreset{}, &Fault{Code: "DESKTOP_PRESET_UNSUPPORTED", Stage: "setup", Summary: "Connection settings are unavailable for this desktop.", Remediation: "Choose an RDP desktop."}
	}
	var v contracts.DesktopPreset
	discover := s.DesktopDisplays
	if discover == nil {
		discover = desktopdisplay.Discover
	}
	var layout contracts.DesktopMonitorLayout
	if update == nil || update.Display != nil && update.Display.Mode == desktopdisplay.Selected {
		layout, _ = discover()
	}
	if update != nil && update.Display != nil {
		d, displayErr := desktopdisplay.Normalize(update.Display, update.FullScreen)
		if displayErr != nil || desktopdisplay.ValidateSelection(d, layout) != nil {
			return v, &Fault{Code: "DESKTOP_DISPLAY_CHANGED", Stage: "setup", Summary: "Choose available adjacent screens for this connection.", Remediation: "Refresh the screen list in connection settings, or choose single screen."}
		}
	}
	if update == nil {
		v, err = s.DesktopPresets.ReadPreset(t.Canonical, t.Device.IdentityPublicKey, p.SessionSelector)
	} else {
		v, err = s.DesktopPresets.SavePreset(t.Canonical, t.Device.IdentityPublicKey, p.SessionSelector, *update)
	}
	if err != nil {
		return v, &Fault{Code: "DESKTOP_PRESET_INVALID", Stage: "setup", Summary: "The connection settings could not be loaded or saved.", Remediation: "Check the account settings and save again."}
	}
	if len(layout.Monitors) != 0 {
		v.Displays = &layout
	}
	return v, nil
}
