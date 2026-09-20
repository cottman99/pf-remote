package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordingDesktopWritesExactCanonicalTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.json")
	target := "pfremote://fabric-demo/devices/device-compute/capabilities/desktop-main"
	result, err := (recordingDesktop{path: path}).Run(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Protocol != "rdp" || result.RenderingEnvironment != "virtual" {
		t.Fatalf("unexpected desktop result: %#v", result)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record actionRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	if record.Action != "open" || record.Target != target || len(record.Command) != 0 {
		t.Fatalf("unexpected desktop record: %#v", record)
	}
}
