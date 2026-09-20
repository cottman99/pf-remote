package recovery

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSealOpenAndRejectWrongPasswordOrDamage(t *testing.T) {
	payload := testPayload()
	random := bytes.NewReader(bytes.Repeat([]byte{0x4a}, 64))
	encoded, err := seal(payload, "a long recovery password", 100_000, random, time.Date(2026, 8, 29, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := Open(encoded, "a long recovery password")
	if err != nil {
		t.Fatal(err)
	}
	if opened.FabricID != payload.FabricID || opened.OwnerDeviceID != payload.OwnerDeviceID {
		t.Fatalf("opened payload = %#v", opened)
	}
	if _, err := Open(encoded, "the wrong password"); !errors.Is(err, ErrCannotOpen) {
		t.Fatalf("wrong password error = %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope["ciphertext"] = "AAAA" + envelope["ciphertext"].(string)
	damaged, _ := json.Marshal(envelope)
	if _, err := Open(damaged, "a long recovery password"); !errors.Is(err, ErrCannotOpen) {
		t.Fatalf("damaged file error = %v", err)
	}
}

func TestSealRejectsWeakPasswordAndIncompletePayload(t *testing.T) {
	if _, err := Seal(testPayload(), "short"); err == nil {
		t.Fatal("expected short password rejection")
	}
	payload := testPayload()
	payload.Snapshot = nil
	if _, err := Seal(payload, "a long recovery password"); err == nil {
		t.Fatal("expected incomplete payload rejection")
	}
}

func testPayload() Payload {
	return Payload{
		SchemaVersion: PayloadSchema, FabricID: "fabric-test", OwnerDeviceID: "device-owner",
		DirectoryVersion: 2, GrantVersion: 3, ExportedAt: time.Date(2026, 8, 29, 3, 0, 0, 0, time.UTC),
		EnrollmentState: json.RawMessage(`{"schema_version":"pfremote.enrollment-state/v1"}`),
		Snapshot:        json.RawMessage(`{"schema_version":"pfremote.state-snapshot/v1"}`),
	}
}
