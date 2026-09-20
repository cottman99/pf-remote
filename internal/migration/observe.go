package migration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/cottman99/pf-remote/pkg/contracts"
)

const (
	ObservationSchema = "pfremote.migration-observation/v1"
	ReportSchema      = "pfremote.migration-observation-report/v1"
	RollbackTrigger   = "new-path-failure-revocation-or-major-slowdown"
)

type Observation struct {
	SchemaVersion   string           `json:"schema_version"`
	Source          string           `json:"source"`
	ObservedAt      time.Time        `json:"observed_at"`
	RollbackTrigger string           `json:"rollback_trigger,omitempty"`
	Targets         []ObservedTarget `json:"targets"`
}

type ObservedTarget struct {
	MappingKey        string                   `json:"mapping_key"`
	ComputerName      string                   `json:"computer_name"`
	CapabilityName    string                   `json:"capability_name"`
	Kind              contracts.CapabilityKind `json:"kind"`
	Result            string                   `json:"result"`
	TimingClass       string                   `json:"timing_class"`
	FallbackRechecked bool                     `json:"fallback_rechecked,omitempty"`
}

type ObservationReport struct {
	SchemaVersion string             `json:"schema_version"`
	Overall       string             `json:"overall"`
	Summary       string             `json:"summary"`
	Targets       []TargetAssessment `json:"targets"`
}

type TargetAssessment struct {
	MappingKey     string `json:"mapping_key"`
	ComputerName   string `json:"computer_name"`
	CapabilityName string `json:"capability_name"`
	Status         string `json:"status"`
	Code           string `json:"code"`
	Summary        string `json:"summary"`
}

func CompareObservations(legacyInput, candidateInput []byte) (ObservationReport, error) {
	legacy, err := parseObservation(legacyInput, "legacy")
	if err != nil {
		return ObservationReport{}, err
	}
	candidate, err := parseObservation(candidateInput, "candidate")
	if err != nil {
		return ObservationReport{}, err
	}
	if candidate.RollbackTrigger != RollbackTrigger {
		return ObservationReport{}, errors.New("candidate observation has no supported rollback trigger")
	}
	if candidate.ObservedAt.Before(legacy.ObservedAt) {
		return ObservationReport{}, errors.New("candidate observation predates the legacy baseline")
	}
	legacyTargets := make(map[string]ObservedTarget, len(legacy.Targets))
	for _, target := range legacy.Targets {
		legacyTargets[target.MappingKey] = target
	}
	candidateTargets := make(map[string]ObservedTarget, len(candidate.Targets))
	for _, target := range candidate.Targets {
		candidateTargets[target.MappingKey] = target
	}
	keys := make([]string, 0, len(legacyTargets)+len(candidateTargets))
	seen := make(map[string]struct{})
	for key := range legacyTargets {
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	for key := range candidateTargets {
		if _, exists := seen[key]; !exists {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	report := ObservationReport{SchemaVersion: ReportSchema, Overall: "ready"}
	for _, key := range keys {
		oldTarget, oldExists := legacyTargets[key]
		newTarget, newExists := candidateTargets[key]
		assessment := assessTarget(key, oldTarget, oldExists, newTarget, newExists)
		report.Targets = append(report.Targets, assessment)
		if assessment.Status == "rollback-required" {
			report.Overall = "rollback-required"
		} else if assessment.Status == "incomplete" && report.Overall == "ready" {
			report.Overall = "incomplete"
		}
	}
	switch report.Overall {
	case "ready":
		report.Summary = "All observed actions match, and the old path was rechecked where rollback may be needed."
	case "rollback-required":
		report.Summary = "Keep using the old path for the affected actions; the new path did not meet the rollback rule."
	default:
		report.Summary = "Observation is incomplete. Keep the old path and collect the missing comparison before cutover."
	}
	return report, nil
}

func parseObservation(input []byte, expectedSource string) (Observation, error) {
	if len(input) == 0 || len(input) > MaxInputBytes {
		return Observation{}, errors.New("migration observation size is invalid")
	}
	var observation Observation
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&observation); err != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		observation.SchemaVersion != ObservationSchema || observation.Source != expectedSource || observation.ObservedAt.IsZero() ||
		len(observation.Targets) == 0 || len(observation.Targets) > 8192 {
		return Observation{}, errors.New("migration observation metadata is invalid")
	}
	seen := make(map[string]struct{}, len(observation.Targets))
	for _, target := range observation.Targets {
		if !validOpaqueRef(target.MappingKey) || !validDisplayName(target.ComputerName) || !validDisplayName(target.CapabilityName) ||
			(target.Kind != contracts.CapabilityShell && target.Kind != contracts.CapabilityDesktop) ||
			(target.Result != "ready" && target.Result != "revoked" && target.Result != "unavailable") ||
			(target.TimingClass != "fast" && target.TimingClass != "normal" && target.TimingClass != "slow" && target.TimingClass != "not-measured") {
			return Observation{}, errors.New("migration observation contains an invalid target")
		}
		if _, exists := seen[target.MappingKey]; exists {
			return Observation{}, errors.New("migration observation contains a duplicate target")
		}
		seen[target.MappingKey] = struct{}{}
		if expectedSource == "legacy" && target.FallbackRechecked {
			return Observation{}, errors.New("legacy observation cannot claim a fallback recheck")
		}
	}
	return observation, nil
}

func assessTarget(key string, legacy ObservedTarget, legacyExists bool, candidate ObservedTarget, candidateExists bool) TargetAssessment {
	name, capability := legacy.ComputerName, legacy.CapabilityName
	if !legacyExists {
		name, capability = candidate.ComputerName, candidate.CapabilityName
	}
	result := TargetAssessment{MappingKey: key, ComputerName: name, CapabilityName: capability}
	if !legacyExists || !candidateExists {
		result.Status = "incomplete"
		result.Code = "missing-side"
		result.Summary = "This action is missing from one side of the comparison. Keep the old path."
		return result
	}
	if legacy.ComputerName != candidate.ComputerName || legacy.CapabilityName != candidate.CapabilityName || legacy.Kind != candidate.Kind {
		result.Status = "rollback-required"
		result.Code = "identity-mismatch"
		result.Summary = "The computer or action identity does not match. Keep the old path."
		return result
	}
	if legacy.Result == "revoked" {
		if candidate.Result == "revoked" {
			result.Status = "ready"
			result.Code = "revocation-match"
			result.Summary = "Revocation matches on both paths."
		} else {
			result.Status = "rollback-required"
			result.Code = "revocation-mismatch"
			result.Summary = "The new path did not preserve revocation. Keep the old path and do not cut over."
		}
		return result
	}
	if legacy.Result != "ready" {
		result.Status = "incomplete"
		result.Code = "legacy-unavailable"
		result.Summary = "The old path was not ready, so this action cannot be compared safely."
		return result
	}
	if candidate.Result != "ready" {
		result.Status = "rollback-required"
		result.Code = "candidate-failure"
		result.Summary = "The new path failed while the old path worked. Keep the old path."
		return result
	}
	if !candidate.FallbackRechecked {
		result.Status = "rollback-required"
		result.Code = "fallback-not-rechecked"
		result.Summary = "The old path was not rechecked after the new-path test. Do not cut over."
		return result
	}
	if legacy.TimingClass == "not-measured" || candidate.TimingClass == "not-measured" {
		result.Status = "incomplete"
		result.Code = "timing-missing"
		result.Summary = "The timing comparison is missing. Keep the old path until this action is observed again."
		return result
	}
	if timingRank(candidate.TimingClass)-timingRank(legacy.TimingClass) >= 2 {
		result.Status = "rollback-required"
		result.Code = "major-slowdown"
		result.Summary = "The new path was materially slower. Keep the old path while this is investigated."
		return result
	}
	result.Status = "ready"
	result.Code = "match"
	result.Summary = fmt.Sprintf("%s on %s matched, and the old path remains available.", candidate.CapabilityName, candidate.ComputerName)
	return result
}

func timingRank(value string) int {
	switch value {
	case "fast":
		return 1
	case "normal":
		return 2
	case "slow":
		return 3
	default:
		return 0
	}
}
