package contracts

// DesktopPreset is returned only through protected local settings actions.
// It never contains a password or travels to a Gateway or context export.
type DesktopPreset struct {
	SchemaVersion  string `json:"schema_version"`
	Username       string `json:"username"`
	PasswordSaved  bool   `json:"password_saved"`
	FullScreen     bool   `json:"full_screen"`
	DefaultDesktop bool   `json:"default_desktop"`
	RequiresSetup  bool   `json:"requires_setup,omitempty"`
}

type DesktopPresetUpdate struct {
	Username       string `json:"username"`
	Password       string `json:"password,omitempty"`
	ClearPassword  bool   `json:"clear_password,omitempty"`
	FullScreen     bool   `json:"full_screen"`
	DefaultDesktop bool   `json:"default_desktop"`
}
