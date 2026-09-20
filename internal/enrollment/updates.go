package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"time"
)

const UpdateSchema = "pfremote.fleet-updates/v1"

type UpdateStateStore interface {
	LoadUpdateState(context.Context, string) ([]byte, error)
	SaveUpdateState(context.Context, string, []byte) error
}

type UpdateRequest struct {
	Schema    string `json:"schema_version"`
	DeviceID  string `json:"device_id"`
	RequestID string `json:"request_id"`
	Channel   string `json:"channel"`
	Platform  string `json:"platform"`
	Version   string `json:"version"`
	Status    string `json:"status"`
	Notify    bool   `json:"notify"`
	Signature string `json:"signature"`
}
type UpdateReport struct {
	DeviceID string    `json:"device_id"`
	Name     string    `json:"name"`
	Channel  string    `json:"channel"`
	Platform string    `json:"platform"`
	Version  string    `json:"version"`
	Status   string    `json:"status"`
	Seen     time.Time `json:"seen"`
}
type UpdateResponse struct {
	Schema   string         `json:"schema_version"`
	FabricID string         `json:"fabric_id"`
	Hint     uint64         `json:"hint"`
	Reports  []UpdateReport `json:"reports"`
}
type updateState struct {
	Hint       uint64                  `json:"hint"`
	LastNotice time.Time               `json:"last_notice"`
	Reports    map[string]UpdateReport `json:"reports"`
}

var updateVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]{0,63}$`)

func UpdateMessage(r UpdateRequest) []byte {
	return canonicalMessage(UpdateSchema, r.DeviceID, r.RequestID, r.Channel, r.Platform, r.Version, r.Status, strconv.FormatBool(r.Notify))
}
func (m *Manager) SyncUpdates(r UpdateRequest) (UpdateResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	reject := func() (UpdateResponse, error) {
		return UpdateResponse{}, fault("UPDATE_REQUEST_INVALID", "updates", "Update coordination is unavailable.", "Check the connection service and retry.")
	}
	if err := m.requireHealthy(); err != nil {
		return UpdateResponse{}, err
	}
	if r.Schema != UpdateSchema || validateRequestID(r.RequestID) != nil || (r.Channel != "preview" && r.Channel != "stable") || (r.Platform != "windows-x64" && r.Platform != "linux-x64") || !updateVersion.MatchString(r.Version) {
		return reject()
	}
	switch r.Status {
	case "current", "scheduled", "retry", "available", "incompatible", "downloading", "busy", "installing", "held", "bootstrap":
	default:
		return reject()
	}
	d := m.devices[r.DeviceID]
	if d == nil || d.status != "active" {
		return reject()
	}
	if err := verifySignature(d.publicKey, UpdateMessage(r), r.Signature, "updates"); err != nil {
		return UpdateResponse{}, err
	}
	if _, exists := m.deviceRequestIDs[r.DeviceID][r.RequestID]; exists {
		return UpdateResponse{}, replayFault("updates")
	}
	if m.store == nil {
		return reject()
	}
	updateStore, ok := m.store.(UpdateStateStore)
	if !ok {
		return reject()
	}
	payload, err := updateStore.LoadUpdateState(context.Background(), UpdateSchema)
	if err != nil {
		return reject()
	}
	state := updateState{Reports: map[string]UpdateReport{}}
	if len(payload) > 0 {
		if len(payload) > 1<<20 || json.Unmarshal(payload, &state) != nil || state.Reports == nil {
			return reject()
		}
	}
	now := m.now().UTC()
	old, exists := state.Reports[r.DeviceID]
	announce := r.Notify || !exists || old.Version != r.Version
	if announce && (state.LastNotice.IsZero() || now.Sub(state.LastNotice) >= 30*time.Second) {
		if state.Hint == ^uint64(0) {
			return reject()
		}
		state.Hint++
		state.LastNotice = now
	}
	state.Reports[r.DeviceID] = UpdateReport{DeviceID: r.DeviceID, Name: d.deviceName, Channel: r.Channel, Platform: r.Platform, Version: r.Version, Status: r.Status, Seen: now}
	reports := make([]UpdateReport, 0, len(state.Reports))
	for id, report := range state.Reports {
		if device := m.devices[id]; device == nil || device.status != "active" {
			delete(state.Reports, id)
		} else {
			reports = append(reports, report)
		}
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].DeviceID < reports[j].DeviceID })
	m.recordDeviceRequestID(r.DeviceID, r.RequestID)
	if err = m.persistLocked(); err != nil {
		return UpdateResponse{}, err
	}
	payload, err = json.Marshal(state)
	if err != nil || len(payload) > 1<<20 {
		return reject()
	}
	if err = updateStore.SaveUpdateState(context.Background(), UpdateSchema, payload); err != nil {
		m.persistenceFailed = true
		return UpdateResponse{}, persistenceFault()
	}
	return UpdateResponse{Schema: UpdateSchema, FabricID: m.fabricID, Hint: state.Hint, Reports: reports}, nil
}
func (c Client) SyncUpdates(ctx context.Context, r UpdateRequest) (UpdateResponse, error) {
	var response UpdateResponse
	if err := c.post(ctx, "/api/v1/updates/sync", r, &response); err != nil {
		return response, err
	}
	if response.Schema != UpdateSchema {
		return UpdateResponse{}, errors.New("unsupported update response")
	}
	return response, nil
}
