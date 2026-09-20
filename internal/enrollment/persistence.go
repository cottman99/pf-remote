package enrollment

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"time"

	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

const PersistentStateSchema = "pfremote.enrollment-state/v1"

type StateStore interface {
	LoadControlState(context.Context, string) ([]byte, error)
	SaveControlState(context.Context, string, []byte) error
	SaveControlStateWithDeviceStatus(context.Context, string, []byte, string, string, uint64, uint64) error
}

type deviceStatusUpdate struct {
	deviceID string
	status   string
}

type persistentState struct {
	SchemaVersion      string                           `json:"schema_version"`
	FabricID           string                           `json:"fabric_id,omitempty"`
	OwnerDeviceID      string                           `json:"owner_device_id,omitempty"`
	OwnerPublicKey     string                           `json:"owner_public_key,omitempty"`
	DirectoryVersion   uint64                           `json:"directory_version"`
	GrantVersion       uint64                           `json:"grant_version"`
	Activations        []persistentActivation           `json:"activations,omitempty"`
	Devices            []persistentDevice               `json:"devices,omitempty"`
	ShellBindings      []contracts.SSHCapabilityBinding `json:"shell_bindings,omitempty"`
	OwnerRequestIDs    []string                         `json:"owner_request_ids,omitempty"`
	DeviceRequestIDs   map[string][]string              `json:"device_request_ids,omitempty"`
	FailedCodeWindow   *time.Time                       `json:"failed_code_window,omitempty"`
	FailedCodeAttempts int                              `json:"failed_code_attempts"`
}

type persistentActivation struct {
	DeviceCodeHash string        `json:"device_code_hash"`
	UserCodeHash   string        `json:"user_code_hash"`
	DeviceID       string        `json:"device_id"`
	DeviceName     string        `json:"device_name"`
	PublicKey      string        `json:"public_key"`
	ExpiresAt      time.Time     `json:"expires_at"`
	Interval       time.Duration `json:"interval"`
	LastPoll       time.Time     `json:"last_poll,omitempty"`
	Status         string        `json:"status"`
}

type persistentDevice struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
	Status     string `json:"status"`
}

type RecoverySummary struct {
	FabricID         string
	OwnerDeviceID    string
	DirectoryVersion uint64
	GrantVersion     uint64
	DeviceStatuses   map[string]string
}

// PrepareRecoveryState keeps durable identity and authorization metadata while
// dropping short-lived activation, replay and rate-limit records.
func PrepareRecoveryState(payload []byte) ([]byte, RecoverySummary, error) {
	manager := newManager(time.Now, rand.Reader)
	if err := manager.restore(payload); err != nil || manager.fabricID == "" {
		return nil, RecoverySummary{}, errors.New("enrollment recovery state is invalid")
	}
	state := manager.persistenceSnapshot()
	state.Activations = nil
	state.OwnerRequestIDs = nil
	state.DeviceRequestIDs = nil
	state.FailedCodeWindow = nil
	state.FailedCodeAttempts = 0
	sort.Slice(state.Devices, func(left, right int) bool { return state.Devices[left].DeviceID < state.Devices[right].DeviceID })
	encoded, err := json.Marshal(state)
	if err != nil {
		return nil, RecoverySummary{}, errors.New("encode enrollment recovery state")
	}
	verified := newManager(time.Now, rand.Reader)
	if err := verified.restore(encoded); err != nil {
		return nil, RecoverySummary{}, errors.New("enrollment recovery state is invalid")
	}
	summary := RecoverySummary{
		FabricID: manager.fabricID, OwnerDeviceID: manager.ownerDeviceID,
		DirectoryVersion: manager.directoryVersion, GrantVersion: manager.grantVersion,
		DeviceStatuses: make(map[string]string, len(manager.devices)),
	}
	for deviceID, device := range manager.devices {
		summary.DeviceStatuses[deviceID] = device.status
	}
	return encoded, summary, nil
}

func NewPersistentManager(store StateStore) (*Manager, error) {
	if store == nil {
		return nil, errors.New("enrollment state store is required")
	}
	manager := newManager(time.Now, rand.Reader)
	manager.store = store
	payload, err := store.LoadControlState(context.Background(), PersistentStateSchema)
	if err != nil {
		return nil, errors.New("load enrollment state")
	}
	if len(payload) == 0 {
		return manager, nil
	}
	if err := manager.restore(payload); err != nil {
		return nil, err
	}
	if manager.pruneExpiredState(manager.now().UTC()) {
		if err := manager.persistLocked(); err != nil {
			return nil, err
		}
	}
	return manager, nil
}

func (m *Manager) persistLocked() error {
	return m.persistWithDeviceStatusLocked(nil)
}

func (m *Manager) persistDeviceStatusLocked(deviceID, status string) error {
	return m.persistWithDeviceStatusLocked(&deviceStatusUpdate{deviceID: deviceID, status: status})
}

func (m *Manager) persistWithDeviceStatusLocked(update *deviceStatusUpdate) error {
	if m.store == nil {
		return nil
	}
	payload, err := json.Marshal(m.persistenceSnapshot())
	if err != nil {
		m.persistenceFailed = true
		return persistenceFault()
	}
	if update == nil {
		err = m.store.SaveControlState(context.Background(), PersistentStateSchema, payload)
	} else {
		err = m.store.SaveControlStateWithDeviceStatus(
			context.Background(), PersistentStateSchema, payload, update.deviceID, update.status,
			m.directoryVersion, m.grantVersion,
		)
	}
	if err != nil {
		m.persistenceFailed = true
		return persistenceFault()
	}
	return nil
}

func (m *Manager) requireHealthy() error {
	if m.persistenceFailed {
		return persistenceFault()
	}
	return nil
}

func persistenceFault() error {
	return fault("STATE_PERSISTENCE_FAILED", "state", "Enrollment state could not be saved safely.", "Restore local state storage, then restart the Gateway.")
}

func (m *Manager) persistenceSnapshot() persistentState {
	state := persistentState{
		SchemaVersion: PersistentStateSchema, FabricID: m.fabricID, OwnerDeviceID: m.ownerDeviceID,
		OwnerPublicKey: encodeBytes(m.ownerPublicKey), DirectoryVersion: m.directoryVersion,
		GrantVersion: m.grantVersion, DeviceRequestIDs: make(map[string][]string),
		FailedCodeAttempts: m.failedCodeAttempts,
	}
	if !m.failedCodeWindow.IsZero() {
		value := m.failedCodeWindow
		state.FailedCodeWindow = &value
	}
	for _, value := range m.activations {
		state.Activations = append(state.Activations, persistentActivation{
			DeviceCodeHash: encodeBytes(value.deviceCodeHash[:]), UserCodeHash: encodeBytes(value.userCodeHash[:]),
			DeviceID: value.deviceID, DeviceName: value.deviceName, PublicKey: encodeBytes(value.publicKey),
			ExpiresAt: value.expiresAt, Interval: value.interval, LastPoll: value.lastPoll, Status: value.status,
		})
	}
	for _, value := range m.devices {
		state.Devices = append(state.Devices, persistentDevice{
			DeviceID: value.deviceID, DeviceName: value.deviceName,
			PublicKey: encodeBytes(value.publicKey), Status: value.status,
		})
	}
	for _, value := range m.shellBindings {
		state.ShellBindings = append(state.ShellBindings, cloneShellBinding(value))
	}
	sort.Slice(state.ShellBindings, func(left, right int) bool {
		if state.ShellBindings[left].DeviceID != state.ShellBindings[right].DeviceID {
			return state.ShellBindings[left].DeviceID < state.ShellBindings[right].DeviceID
		}
		return state.ShellBindings[left].CapabilityID < state.ShellBindings[right].CapabilityID
	})
	for _, requestID := range m.ownerRequestOrder {
		state.OwnerRequestIDs = append(state.OwnerRequestIDs, requestID)
	}
	for deviceID, requestIDs := range m.deviceRequestOrder {
		for _, requestID := range requestIDs {
			state.DeviceRequestIDs[deviceID] = append(state.DeviceRequestIDs[deviceID], requestID)
		}
	}
	return state
}

func (m *Manager) restore(payload []byte) error {
	var state persistentState
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil || decoder.Decode(&struct{}{}) != io.EOF || state.SchemaVersion != PersistentStateSchema {
		return errors.New("enrollment state is invalid")
	}
	var ownerKey []byte
	if state.FabricID == "" {
		if state.OwnerDeviceID != "" || state.OwnerPublicKey != "" || state.DirectoryVersion != 0 || state.GrantVersion != 0 || len(state.Devices) != 0 || len(state.Activations) != 0 || len(state.ShellBindings) != 0 {
			return errors.New("uninitialized enrollment state is inconsistent")
		}
	} else {
		var err error
		ownerKey, err = decodeBytes(state.OwnerPublicKey, ed25519.PublicKeySize, ed25519.PublicKeySize)
		if err != nil || !validIdentifier(state.FabricID) || state.OwnerDeviceID == "" || state.DirectoryVersion == 0 || state.GrantVersion == 0 {
			return errors.New("enrollment state owner metadata is invalid")
		}
		if err := verifyDeviceBinding(state.OwnerDeviceID, ed25519.PublicKey(ownerKey)); err != nil {
			return errors.New("enrollment state owner binding is invalid")
		}
	}
	m.fabricID, m.ownerDeviceID = state.FabricID, state.OwnerDeviceID
	m.ownerPublicKey = ed25519.PublicKey(ownerKey)
	m.directoryVersion, m.grantVersion = state.DirectoryVersion, state.GrantVersion
	if state.FailedCodeWindow != nil {
		m.failedCodeWindow = state.FailedCodeWindow.UTC()
	}
	m.failedCodeAttempts = state.FailedCodeAttempts
	if len(state.Activations) > m.pendingLimit || len(state.DeviceRequestIDs) > maxDeviceReplayBuckets {
		return errors.New("enrollment state exceeds resource limits")
	}
	for _, value := range state.Devices {
		key, err := decodeBytes(value.PublicKey, ed25519.PublicKeySize, ed25519.PublicKeySize)
		if err != nil || value.DeviceID == "" || m.devices[value.DeviceID] != nil || (value.Status != "active" && value.Status != "revoked") {
			return errors.New("enrollment state Device is invalid")
		}
		if err := verifyDeviceBinding(value.DeviceID, ed25519.PublicKey(key)); err != nil {
			return errors.New("enrollment state Device binding is invalid")
		}
		m.devices[value.DeviceID] = &deviceRecord{deviceID: value.DeviceID, deviceName: value.DeviceName, publicKey: key, status: value.Status}
	}
	if len(state.ShellBindings) > maxPublishedShellBindings {
		return errors.New("enrollment state exceeds Shell capability limits")
	}
	for _, binding := range state.ShellBindings {
		device := m.devices[binding.DeviceID]
		key := shellBindingKey(binding.DeviceID, binding.CapabilityID)
		if device == nil || m.shellBindings[key].Signature != "" {
			return errors.New("enrollment state Shell capability is invalid")
		}
		publicDevice := contracts.Device{ID: device.deviceID, Alias: "verified-device", State: "online", IdentityPublicKey: EncodePublicKey(device.publicKey)}
		publicCapability := contracts.Capability{ID: binding.CapabilityID, DeviceID: device.deviceID, Alias: "verified-shell", Kind: contracts.CapabilityShell, State: "available", SSHBinding: &binding}
		if err := shellbinding.Verify(publicDevice, publicCapability, m.fabricID); err != nil {
			return errors.New("enrollment state Shell capability binding is invalid")
		}
		m.shellBindings[key] = cloneShellBinding(binding)
	}
	for _, value := range state.Activations {
		deviceHash, err := decodeHash(value.DeviceCodeHash)
		if err != nil {
			return errors.New("enrollment state activation is invalid")
		}
		userHash, err := decodeHash(value.UserCodeHash)
		if err != nil {
			return errors.New("enrollment state activation is invalid")
		}
		key, err := decodeBytes(value.PublicKey, ed25519.PublicKeySize, ed25519.PublicKeySize)
		if err != nil || value.DeviceID == "" || (value.Status != "pending" && value.Status != "approved") || value.Interval <= 0 || value.ExpiresAt.IsZero() {
			return errors.New("enrollment state activation is invalid")
		}
		if err := verifyDeviceBinding(value.DeviceID, ed25519.PublicKey(key)); err != nil {
			return errors.New("enrollment state activation binding is invalid")
		}
		if m.activations[deviceHash] != nil {
			return errors.New("enrollment state activation is duplicated")
		}
		pending := &activation{deviceCodeHash: deviceHash, userCodeHash: userHash, deviceID: value.DeviceID,
			deviceName: value.DeviceName, publicKey: key, expiresAt: value.ExpiresAt.UTC(), interval: value.Interval,
			lastPoll: value.LastPoll.UTC(), status: value.Status}
		m.activations[deviceHash] = pending
		m.userCodes[userHash] = deviceHash
	}
	for _, requestID := range state.OwnerRequestIDs {
		if validateRequestID(requestID) != nil {
			return errors.New("enrollment state Owner request ID is invalid")
		}
		if _, exists := m.ownerRequestIDs[requestID]; exists || len(m.ownerRequestOrder) >= maxOwnerReplayIDs {
			return errors.New("enrollment state Owner request IDs are invalid")
		}
		m.ownerRequestIDs[requestID] = struct{}{}
		m.ownerRequestOrder = append(m.ownerRequestOrder, requestID)
	}
	for deviceID, requestIDs := range state.DeviceRequestIDs {
		if deviceID == "" {
			return errors.New("enrollment state Device request map is invalid")
		}
		if len(requestIDs) > maxDeviceReplayIDs {
			return errors.New("enrollment state Device request IDs are invalid")
		}
		m.deviceRequestIDs[deviceID] = make(map[string]struct{}, len(requestIDs))
		m.deviceRequestSince[deviceID] = m.now().UTC()
		for _, requestID := range requestIDs {
			if validateRequestID(requestID) != nil {
				return errors.New("enrollment state Device request ID is invalid")
			}
			if _, exists := m.deviceRequestIDs[deviceID][requestID]; exists {
				return errors.New("enrollment state Device request IDs are duplicated")
			}
			m.deviceRequestIDs[deviceID][requestID] = struct{}{}
			m.deviceRequestOrder[deviceID] = append(m.deviceRequestOrder[deviceID], requestID)
		}
	}
	if state.FabricID != "" {
		owner := m.devices[state.OwnerDeviceID]
		if owner == nil || owner.status != "active" || !bytes.Equal(owner.publicKey, ownerKey) {
			return errors.New("enrollment state Owner Device is missing")
		}
	}
	return nil
}

func encodeBytes(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }

func decodeBytes(value string, minimum, maximum int) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) < minimum || len(decoded) > maximum {
		return nil, errors.New("invalid encoded bytes")
	}
	return decoded, nil
}

func decodeHash(value string) ([sha256.Size]byte, error) {
	var result [sha256.Size]byte
	decoded, err := decodeBytes(value, sha256.Size, sha256.Size)
	if err != nil {
		return result, err
	}
	copy(result[:], decoded)
	return result, nil
}
