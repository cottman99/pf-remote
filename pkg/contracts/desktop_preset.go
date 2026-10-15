package contracts

// DesktopPreset is returned only through protected local settings actions.
// It never contains a password or travels to a Gateway or context export.
type DesktopPreset struct {
	SchemaVersion  string                  `json:"schema_version"`
	Username       string                  `json:"username"`
	PasswordSaved  bool                    `json:"password_saved"`
	FullScreen     bool                    `json:"full_screen"`
	DefaultDesktop bool                    `json:"default_desktop"`
	RequiresSetup  bool                    `json:"requires_setup,omitempty"`
	Display        *DesktopDisplaySettings `json:"display,omitempty"`
	Displays       *DesktopMonitorLayout   `json:"displays,omitempty"`
}

type DesktopPresetUpdate struct {
	Username       string                  `json:"username"`
	Password       string                  `json:"password,omitempty"`
	ClearPassword  bool                    `json:"clear_password,omitempty"`
	FullScreen     bool                    `json:"full_screen"`
	DefaultDesktop bool                    `json:"default_desktop"`
	Display        *DesktopDisplaySettings `json:"display,omitempty"`
}

// These settings and local monitor geometry never enter the catalog or Gateway.
type DesktopDisplaySettings struct {
	Mode       string `json:"mode"`
	MonitorIDs []int  `json:"monitor_ids,omitempty"`
	LayoutID   string `json:"layout_id,omitempty"`
}

type DesktopMonitor struct {
	ID     int `json:"id"`
	Left   int `json:"left"`
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
}

type DesktopMonitorLayout struct {
	ID       string           `json:"id"`
	Monitors []DesktopMonitor `json:"monitors"`
}
