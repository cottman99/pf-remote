// Package desktopdisplay resolves local display choices without opening a remote session.
package desktopdisplay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

const (
	Window   = "window"
	Single   = "single"
	All      = "all"
	Selected = "selected"
)

var ErrSelection = errors.New("display selection is unavailable or changed")

func Normalize(value *contracts.DesktopDisplaySettings, fullScreen bool) (contracts.DesktopDisplaySettings, error) {
	if value == nil {
		mode := Window
		if fullScreen {
			mode = Single
		}
		return contracts.DesktopDisplaySettings{Mode: mode}, nil
	}
	d := *value
	d.MonitorIDs = append([]int(nil), value.MonitorIDs...)
	switch d.Mode {
	case Window, Single, All:
		if len(d.MonitorIDs) != 0 || d.LayoutID != "" {
			return d, ErrSelection
		}
	case Selected:
		if len(d.MonitorIDs) == 0 || len(d.MonitorIDs) > 16 || len(d.LayoutID) != 64 {
			return d, ErrSelection
		}
		if _, err := hex.DecodeString(d.LayoutID); err != nil {
			return d, ErrSelection
		}
		seen := map[int]bool{}
		for _, id := range d.MonitorIDs {
			if id < 0 || id > 63 || seen[id] {
				return d, ErrSelection
			}
			seen[id] = true
		}
	default:
		return d, ErrSelection
	}
	return d, nil
}

// ValidateSelection rejects stale IDs instead of allowing mstsc to fall back to all screens.
func ValidateSelection(d contracts.DesktopDisplaySettings, layout contracts.DesktopMonitorLayout) error {
	if _, err := Normalize(&d, true); err != nil {
		return err
	}
	if d.Mode != Selected {
		return nil
	}
	if layout.ID == "" || d.LayoutID != layout.ID {
		return ErrSelection
	}
	var monitors []contracts.DesktopMonitor
	for _, id := range d.MonitorIDs {
		found := false
		for _, m := range layout.Monitors {
			if id == m.ID {
				monitors = append(monitors, m)
				found = true
				break
			}
		}
		if !found {
			return ErrSelection
		}
	}
	// Contiguous means an edge is shared; touching only at a corner is insufficient.
	visited := make([]bool, len(monitors))
	visited[0] = true
	for changed := true; changed; {
		changed = false
		for i, a := range monitors {
			if !visited[i] {
				continue
			}
			for j, b := range monitors {
				if visited[j] {
					continue
				}
				xOverlap := max(a.Left, b.Left) < min(a.Right, b.Right)
				yOverlap := max(a.Top, b.Top) < min(a.Bottom, b.Bottom)
				if (xOverlap && (a.Top == b.Bottom || a.Bottom == b.Top)) || (yOverlap && (a.Left == b.Right || a.Right == b.Left)) {
					visited[j] = true
					changed = true
				}
			}
		}
	}
	for _, v := range visited {
		if !v {
			return ErrSelection
		}
	}
	return nil
}

// mstsc /l is the documented authority for RDP IDs. Display-device numbers and
// EnumDisplayMonitors order are not interchangeable with these identifiers.
var monitorLine = regexp.MustCompile(`(?m)^\s*(\d+)\s*:\s*(\d+)\s*[x×]\s*(\d+)\s*;\s*\(\s*(-?\d+)\s*,\s*(-?\d+)\s*,\s*(-?\d+)\s*,\s*(-?\d+)\s*\)\s*$`)
var monitorPrefix = regexp.MustCompile(`(?m)^\s*\d+\s*:`)

func parseMonitorList(text string) (contracts.DesktopMonitorLayout, error) {
	var layout contracts.DesktopMonitorLayout
	seen := map[int]bool{}
	matches := monitorLine.FindAllStringSubmatch(text, -1)
	if len(matches) != len(monitorPrefix.FindAllStringIndex(text, -1)) {
		return layout, ErrSelection
	}
	for _, match := range matches {
		n := make([]int, 7)
		for i := range n {
			v, err := strconv.Atoi(match[i+1])
			if err != nil || v < -131072 || v > 131072 {
				return layout, ErrSelection
			}
			n[i] = v
		}
		if n[0] > 63 || seen[n[0]] || n[1] <= 0 || n[2] <= 0 || n[5]-n[3]+1 != n[1] || n[6]-n[4]+1 != n[2] {
			return layout, ErrSelection
		}
		seen[n[0]] = true
		layout.Monitors = append(layout.Monitors, contracts.DesktopMonitor{ID: n[0], Left: n[3], Top: n[4], Right: n[5] + 1, Bottom: n[6] + 1})
	}
	if len(layout.Monitors) == 0 || len(layout.Monitors) > 16 {
		return layout, ErrSelection
	}
	sort.Slice(layout.Monitors, func(i, j int) bool { return layout.Monitors[i].ID < layout.Monitors[j].ID })
	b, _ := json.Marshal(layout.Monitors)
	h := sha256.Sum256(b)
	layout.ID = hex.EncodeToString(h[:])
	return layout, nil
}
