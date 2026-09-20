package migration

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCompareObservations_ReadyWithFallbackAndRevocationParity(t *testing.T) {
	legacy := observationJSON(t, "legacy", "", []ObservedTarget{
		{MappingKey: "migration-target-alpha", ComputerName: "Lab computer", CapabilityName: "Current screen", Kind: "desktop", Result: "ready", TimingClass: "fast"},
		{MappingKey: "migration-target-beta", ComputerName: "Old computer", CapabilityName: "Automation", Kind: "shell", Result: "revoked", TimingClass: "not-measured"},
	})
	candidate := observationJSON(t, "candidate", RollbackTrigger, []ObservedTarget{
		{MappingKey: "migration-target-beta", ComputerName: "Old computer", CapabilityName: "Automation", Kind: "shell", Result: "revoked", TimingClass: "not-measured"},
		{MappingKey: "migration-target-alpha", ComputerName: "Lab computer", CapabilityName: "Current screen", Kind: "desktop", Result: "ready", TimingClass: "normal", FallbackRechecked: true},
	})
	report, err := CompareObservations(legacy, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if report.Overall != "ready" || len(report.Targets) != 2 || report.Targets[0].MappingKey != "migration-target-alpha" {
		t.Fatalf("report = %#v", report)
	}
}

func TestCompareObservations_RollsBackOnFailureRevocationMismatchFallbackOrMajorSlowdown(t *testing.T) {
	tests := []struct {
		name      string
		legacy    ObservedTarget
		candidate ObservedTarget
	}{
		{name: "candidate failure", legacy: observed("ready", "normal", false), candidate: observed("unavailable", "not-measured", true)},
		{name: "revocation mismatch", legacy: observed("revoked", "not-measured", false), candidate: observed("ready", "fast", false)},
		{name: "fallback absent", legacy: observed("ready", "normal", false), candidate: observed("ready", "normal", false)},
		{name: "major slowdown", legacy: observed("ready", "fast", false), candidate: observed("ready", "slow", true)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy := observationJSON(t, "legacy", "", []ObservedTarget{test.legacy})
			candidate := observationJSON(t, "candidate", RollbackTrigger, []ObservedTarget{test.candidate})
			report, err := CompareObservations(legacy, candidate)
			if err != nil {
				t.Fatal(err)
			}
			if report.Overall != "rollback-required" || report.Targets[0].Status != "rollback-required" {
				t.Fatalf("report = %#v", report)
			}
		})
	}
}

func TestCompareObservations_IsIncompleteForMissingSideAndRejectsUnknownFields(t *testing.T) {
	legacy := observationJSON(t, "legacy", "", []ObservedTarget{observed("ready", "normal", false)})
	candidate := observationJSON(t, "candidate", RollbackTrigger, []ObservedTarget{
		{MappingKey: "migration-target-other", ComputerName: "Other computer", CapabilityName: "Automation", Kind: "shell", Result: "ready", TimingClass: "normal", FallbackRechecked: true},
	})
	report, err := CompareObservations(legacy, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if report.Overall != "incomplete" || len(report.Targets) != 2 {
		t.Fatalf("report = %#v", report)
	}
	invalid := append(legacy[:len(legacy)-1], []byte(`,"password":"secret"}`)...)
	if _, err := CompareObservations(invalid, candidate); err == nil {
		t.Fatal("unknown secret field was accepted")
	}
}

func TestCompareObservations_IsIncompleteWhenReadyTimingWasNotMeasured(t *testing.T) {
	legacy := observationJSON(t, "legacy", "", []ObservedTarget{observed("ready", "not-measured", false)})
	candidate := observationJSON(t, "candidate", RollbackTrigger, []ObservedTarget{observed("ready", "normal", true)})
	report, err := CompareObservations(legacy, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if report.Overall != "incomplete" {
		t.Fatalf("report = %#v", report)
	}
}

func TestCompareObservations_RejectsControlCharactersInVisibleNames(t *testing.T) {
	target := observed("ready", "normal", false)
	target.ComputerName = "Lab computer\nFAKE DECISION"
	legacy := observationJSON(t, "legacy", "", []ObservedTarget{target})
	candidate := observationJSON(t, "candidate", RollbackTrigger, []ObservedTarget{observed("ready", "normal", true)})
	if _, err := CompareObservations(legacy, candidate); err == nil {
		t.Fatal("control characters in a visible name were accepted")
	}
}

func observed(result, timing string, fallback bool) ObservedTarget {
	return ObservedTarget{MappingKey: "migration-target-alpha", ComputerName: "Lab computer", CapabilityName: "Current screen", Kind: "desktop", Result: result, TimingClass: timing, FallbackRechecked: fallback}
}

func observationJSON(t *testing.T, source, trigger string, targets []ObservedTarget) []byte {
	t.Helper()
	encoded, err := json.Marshal(Observation{
		SchemaVersion: ObservationSchema, Source: source, ObservedAt: time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC),
		RollbackTrigger: trigger, Targets: targets,
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
