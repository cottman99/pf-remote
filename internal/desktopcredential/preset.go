package desktopcredential

import (
	"encoding/json"
	"errors"
	"github.com/cottman99/pf-remote/internal/desktopdisplay"
	"github.com/cottman99/pf-remote/pkg/contracts"
	"github.com/cottman99/pf-remote/pkg/targetref"
	"strings"
	"unicode/utf16"
)

type RDPPreset struct {
	Target     string                            `json:"target"`
	Identity   string                            `json:"identity"`
	Selector   string                            `json:"selector"`
	Username   string                            `json:"username"`
	Password   string                            `json:"password"`
	FullScreen bool                              `json:"full_screen"`
	Display    *contracts.DesktopDisplaySettings `json:"display,omitempty"`
}
type presetDocument struct {
	Schema   string               `json:"schema"`
	Device   string               `json:"device"`
	Default  string               `json:"default"`
	Profiles map[string]RDPPreset `json:"profiles"`
}

func presetDevice(target string) (string, error) {
	r, err := targetref.Parse(target)
	if err != nil {
		return "", errors.New("invalid preset target")
	}
	return r.FabricID + "/" + r.DeviceID, nil
}
func (s *Store) readPresets(target string) (presetDocument, error) {
	device, err := presetDevice(target)
	if err != nil {
		return presetDocument{}, err
	}
	d := presetDocument{Schema: "pfremote.desktop-presets/v1", Device: device, Profiles: map[string]RDPPreset{}}
	b, err := s.Load("rdp-presets/" + device)
	defer clear(b)
	if errors.Is(err, ErrNotFound) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	if json.Unmarshal(b, &d) != nil || d.Schema != "pfremote.desktop-presets/v1" || d.Device != device || d.Profiles == nil {
		return d, errors.New("invalid protected presets")
	}
	return d, nil
}
func (s *Store) LoadRDP(target, identity, selector string) (RDPPreset, error) {
	s.presetMu.Lock()
	defer s.presetMu.Unlock()
	d, err := s.readPresets(target)
	if err != nil {
		return RDPPreset{}, err
	}
	p, ok := d.Profiles[target]
	if !ok {
		return RDPPreset{}, ErrNotFound
	}
	if p.Target != target || p.Identity != identity || p.Selector != selector {
		return RDPPreset{}, errors.New("preset identity changed")
	}
	display, err := desktopdisplay.Normalize(p.Display, p.FullScreen)
	if err != nil {
		return RDPPreset{}, err
	}
	p.Display = &display
	p.FullScreen = display.Mode != desktopdisplay.Window
	return p, nil
}
func (s *Store) ReadPreset(target, identity, selector string) (contracts.DesktopPreset, error) {
	s.presetMu.Lock()
	defer s.presetMu.Unlock()
	v := contracts.DesktopPreset{SchemaVersion: "pfremote.desktop-preset/v1", FullScreen: true, Display: &contracts.DesktopDisplaySettings{Mode: desktopdisplay.Single}}
	d, err := s.readPresets(target)
	if err != nil {
		return v, err
	}
	p, ok := d.Profiles[target]
	if !ok {
		return v, nil
	}
	if p.Target != target || p.Identity != identity || p.Selector != selector {
		// Reveal none of the old account data. The settings UI can now replace
		// this binding after the owner enters the new computer's account.
		v.RequiresSetup = true
		return v, nil
	}
	v.Username = p.Username
	v.PasswordSaved = p.Password != ""
	display, err := desktopdisplay.Normalize(p.Display, p.FullScreen)
	if err != nil {
		return v, err
	}
	v.Display = &display
	v.FullScreen = display.Mode != desktopdisplay.Window
	v.DefaultDesktop = d.Default == target
	return v, nil
}
func (s *Store) SavePreset(target, identity, selector string, u contracts.DesktopPresetUpdate) (contracts.DesktopPreset, error) {
	s.presetMu.Lock()
	defer s.presetMu.Unlock()
	v := contracts.DesktopPreset{}
	if len(u.Username) > 256 || strings.ContainsAny(u.Username, "\r\n\x00") || strings.ContainsRune(u.Password, 0) || len(utf16.Encode([]rune(u.Password))) > 1200 || u.Password != "" && strings.TrimSpace(u.Username) == "" || u.ClearPassword && u.Password != "" {
		return v, errors.New("invalid account settings")
	}
	d, err := s.readPresets(target)
	if err != nil {
		return v, err
	}
	old := d.Profiles[target]
	p := RDPPreset{Target: target, Identity: identity, Selector: selector, Username: strings.TrimSpace(u.Username), FullScreen: u.FullScreen, Password: u.Password}
	displayUpdate := u.Display
	// An older settings client may update an account without understanding the
	// display extension. Preserve it unless its legacy full-screen flag changes.
	if displayUpdate == nil && old.Target == target && old.Identity == identity && old.Selector == selector && old.FullScreen == u.FullScreen {
		displayUpdate = old.Display
	}
	display, err := desktopdisplay.Normalize(displayUpdate, u.FullScreen)
	if err != nil {
		return v, err
	}
	p.Display = &display
	p.FullScreen = display.Mode != desktopdisplay.Window
	if u.Password == "" && !u.ClearPassword && old.Target == target && old.Identity == identity && old.Selector == selector && old.Username == p.Username {
		p.Password = old.Password
	}
	d.Profiles[target] = p
	if old.Password != "" && (old.Password != p.Password || old.Username != p.Username) {
		if err := clearRDPCredentials(target); err != nil {
			return v, err
		}
	}
	if u.DefaultDesktop {
		d.Default = target
	} else if d.Default == target {
		d.Default = ""
	}
	b, err := json.Marshal(d)
	if err != nil {
		return v, err
	}
	defer clear(b)
	if err = s.Save("rdp-presets/"+d.Device, string(b)); err != nil {
		return v, err
	}
	return contracts.DesktopPreset{SchemaVersion: "pfremote.desktop-preset/v1", Username: p.Username, PasswordSaved: p.Password != "", FullScreen: p.FullScreen, Display: p.Display, DefaultDesktop: d.Default == target}, nil
}
