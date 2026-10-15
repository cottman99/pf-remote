package desktopdisplay

import (
	"strings"
	"testing"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

const syntheticMonitors = "0: 1920 x 1080; (0, 0, 1919, 1079)\r\n2: 1920 x 1080; (1920, 0, 3839, 1079)\r\n5: 1920 x 1080; (3840, 0, 5759, 1079)"

func TestNativeIDsGeometryAndContiguousSelection(t *testing.T) {
	layout, err := parseMonitorList(syntheticMonitors)
	if err != nil || len(layout.Monitors) != 3 || layout.Monitors[1].ID != 2 || layout.Monitors[0].Right != 1920 {
		t.Fatal("native monitor IDs lost", err)
	}
	for _, ids := range [][]int{{0}, {2, 0}, {5, 2, 0}} {
		if err := ValidateSelection(contracts.DesktopDisplaySettings{Mode: Selected, MonitorIDs: ids, LayoutID: layout.ID}, layout); err != nil {
			t.Fatal("valid contiguous selection", ids, err)
		}
	}
	for _, ids := range [][]int{{0, 5}, {0, 9}, {2, 2}, {}} {
		if err := ValidateSelection(contracts.DesktopDisplaySettings{Mode: Selected, MonitorIDs: ids, LayoutID: layout.ID}, layout); err == nil {
			t.Fatal("invalid selection", ids)
		}
	}
	changed, _ := parseMonitorList(strings.ReplaceAll(syntheticMonitors, "2:", "3:"))
	if err := ValidateSelection(contracts.DesktopDisplaySettings{Mode: Selected, MonitorIDs: []int{0, 2}, LayoutID: layout.ID}, changed); err == nil {
		t.Fatal("stale topology used different monitor")
	}
	for _, input := range []string{"", "0: 1920 x 1080; (0, 0, 1920, 1080)", syntheticMonitors + "\n" + syntheticMonitors} {
		if _, err := parseMonitorList(input); err == nil {
			t.Fatal("ambiguous native output accepted")
		}
	}
}

func TestLegacyDisplayModesAndValidation(t *testing.T) {
	for _, full := range []bool{false, true} {
		d, err := Normalize(nil, full)
		if err != nil || (d.Mode == Single) != full {
			t.Fatal("legacy full screen migration")
		}
	}
	for _, d := range []contracts.DesktopDisplaySettings{{Mode: "bogus"}, {Mode: All, MonitorIDs: []int{0}}, {Mode: Selected, MonitorIDs: []int{0}, LayoutID: strings.Repeat("z", 64)}} {
		if _, err := Normalize(&d, true); err == nil {
			t.Fatal("unsafe setting accepted")
		}
	}
}
