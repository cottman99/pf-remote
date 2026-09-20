package enrollment

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

const userCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

const (
	maxOwnerReplayIDs      = 4096
	maxDeviceReplayIDs     = 1024
	maxDeviceReplayBuckets = 4096
	maxPendingActivations  = 256
	deviceReplayRetention  = 24 * time.Hour
)

type Manager struct {
	mu                 sync.Mutex
	now                func() time.Time
	random             io.Reader
	activationTTL      time.Duration
	pollingInterval    time.Duration
	fabricID           string
	ownerDeviceID      string
	ownerPublicKey     ed25519.PublicKey
	directoryVersion   uint64
	grantVersion       uint64
	activations        map[[sha256.Size]byte]*activation
	userCodes          map[[sha256.Size]byte][sha256.Size]byte
	devices            map[string]*deviceRecord
	ownerRequestIDs    map[string]struct{}
	ownerRequestOrder  []string
	deviceRequestIDs   map[string]map[string]struct{}
	deviceRequestOrder map[string][]string
	deviceRequestSince map[string]time.Time
	pendingLimit       int
	failedCodeWindow   time.Time
	failedCodeAttempts int
	store              StateStore
	persistenceFailed  bool
	shellBindings      map[string]contracts.SSHCapabilityBinding
}

type activation struct {
	deviceCodeHash [sha256.Size]byte
	userCodeHash   [sha256.Size]byte
	deviceID       string
	deviceName     string
	publicKey      ed25519.PublicKey
	expiresAt      time.Time
	interval       time.Duration
	lastPoll       time.Time
	status         string
}

type deviceRecord struct {
	deviceID   string
	deviceName string
	publicKey  ed25519.PublicKey
	status     string
}

func NewManager() *Manager {
	return newManager(time.Now, rand.Reader)
}

func newManager(now func() time.Time, random io.Reader) *Manager {
	return &Manager{
		now:                now,
		random:             random,
		activationTTL:      DefaultActivationTTL,
		pollingInterval:    DefaultPollingInterval,
		activations:        make(map[[sha256.Size]byte]*activation),
		userCodes:          make(map[[sha256.Size]byte][sha256.Size]byte),
		devices:            make(map[string]*deviceRecord),
		ownerRequestIDs:    make(map[string]struct{}),
		deviceRequestIDs:   make(map[string]map[string]struct{}),
		deviceRequestOrder: make(map[string][]string),
		deviceRequestSince: make(map[string]time.Time),
		pendingLimit:       maxPendingActivations,
		shellBindings:      make(map[string]contracts.SSHCapabilityBinding),
	}
}

func (m *Manager) InitializeOwner(request OwnerInitializationRequest) (OwnerInitializationResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthy(); err != nil {
		return OwnerInitializationResponse{}, err
	}
	if m.fabricID != "" {
		return OwnerInitializationResponse{}, fault("OWNER_ALREADY_INITIALIZED", "owner-initialize", "The Fabric already has an Owner.", "Use the existing Owner identity or the recovery workflow.")
	}
	if err := validateSchemaAndVersion(request.SchemaVersion, request.ClientVersion); err != nil {
		return OwnerInitializationResponse{}, err
	}
	if !validIdentifier(request.FabricID) {
		return OwnerInitializationResponse{}, fault("INVALID_FABRIC_ID", "owner-initialize", "The Fabric ID is invalid.", "Use a lowercase identifier containing letters, digits, and hyphens.")
	}
	publicKey, err := decodePublicKey(request.OwnerPublicKey)
	if err != nil {
		return OwnerInitializationResponse{}, err
	}
	if err := verifyDeviceBinding(request.OwnerDeviceID, publicKey); err != nil {
		return OwnerInitializationResponse{}, err
	}
	if err := verifySignature(publicKey, OwnerInitializationMessage(request), request.Signature, "owner-initialize"); err != nil {
		return OwnerInitializationResponse{}, err
	}
	m.fabricID = request.FabricID
	m.ownerDeviceID = request.OwnerDeviceID
	m.ownerPublicKey = append(ed25519.PublicKey(nil), publicKey...)
	m.directoryVersion = 1
	m.grantVersion = 1
	m.devices[request.OwnerDeviceID] = &deviceRecord{
		deviceID: request.OwnerDeviceID, deviceName: "Owner controller",
		publicKey: append(ed25519.PublicKey(nil), publicKey...), status: "active",
	}
	if err := m.persistDeviceStatusLocked(request.OwnerDeviceID, "active"); err != nil {
		return OwnerInitializationResponse{}, err
	}
	return OwnerInitializationResponse{
		SchemaVersion: SchemaVersion, FabricID: m.fabricID,
		OwnerDeviceID: m.ownerDeviceID, DirectoryVersion: m.directoryVersion,
	}, nil
}

func (m *Manager) BeginDeviceAuthorization(request DeviceAuthorizationRequest) (DeviceAuthorizationResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthy(); err != nil {
		return DeviceAuthorizationResponse{}, err
	}
	if err := m.requireInitialized(); err != nil {
		return DeviceAuthorizationResponse{}, err
	}
	if err := validateSchemaAndVersion(request.SchemaVersion, request.ClientVersion); err != nil {
		return DeviceAuthorizationResponse{}, err
	}
	if err := validateRequestID(request.RequestID); err != nil {
		return DeviceAuthorizationResponse{}, err
	}
	if err := validateDeviceName(request.DeviceName); err != nil {
		return DeviceAuthorizationResponse{}, err
	}
	publicKey, err := decodePublicKey(request.DevicePublicKey)
	if err != nil {
		return DeviceAuthorizationResponse{}, err
	}
	if err := verifyDeviceBinding(request.DeviceID, publicKey); err != nil {
		return DeviceAuthorizationResponse{}, err
	}
	if err := verifySignature(publicKey, DeviceAuthorizationMessage(request), request.Signature, "device-authorization"); err != nil {
		return DeviceAuthorizationResponse{}, err
	}
	if existing, ok := m.devices[request.DeviceID]; ok && existing.status == "active" {
		return DeviceAuthorizationResponse{}, fault("DEVICE_ALREADY_ACTIVE", "device-authorization", "The Device is already active.", "Synchronize the existing Device identity instead of activating it again.")
	}
	now := m.now().UTC()
	if m.pruneExpiredState(now) {
		if err := m.persistLocked(); err != nil {
			return DeviceAuthorizationResponse{}, err
		}
	}
	if _, exists := m.deviceRequestIDs[request.DeviceID][request.RequestID]; exists {
		return DeviceAuthorizationResponse{}, replayFault("device-authorization")
	}
	if m.activationForDevice(request.DeviceID) != nil {
		return DeviceAuthorizationResponse{}, fault("ACTIVATION_ALREADY_PENDING", "device-authorization", "This Device already has a pending activation.", "Continue the existing activation or wait for it to expire.")
	}
	if len(m.activations) >= m.pendingLimit {
		return DeviceAuthorizationResponse{}, fault("ACTIVATION_CAPACITY_REACHED", "device-authorization", "The Gateway has reached its pending activation limit.", "Wait for existing activation requests to complete or expire, then retry.")
	}
	if m.deviceRequestIDs[request.DeviceID] == nil && len(m.deviceRequestIDs) >= maxDeviceReplayBuckets {
		return DeviceAuthorizationResponse{}, fault("ACTIVATION_CAPACITY_REACHED", "device-authorization", "The Gateway has reached its recent Device request limit.", "Wait for expired request history to be pruned, then retry.")
	}
	m.recordDeviceRequestID(request.DeviceID, request.RequestID)
	if err := m.persistLocked(); err != nil {
		return DeviceAuthorizationResponse{}, err
	}

	deviceCode, err := randomToken(m.random, 32)
	if err != nil {
		return DeviceAuthorizationResponse{}, fault("ENTROPY_UNAVAILABLE", "device-authorization", "A secure activation code could not be generated.", "Retry after restoring the operating-system random source.")
	}
	userCode, err := randomUserCode(m.random)
	if err != nil {
		return DeviceAuthorizationResponse{}, fault("ENTROPY_UNAVAILABLE", "device-authorization", "A secure user code could not be generated.", "Retry after restoring the operating-system random source.")
	}
	deviceHash := sha256.Sum256([]byte(deviceCode))
	userHash := sha256.Sum256([]byte(normalizeUserCode(userCode)))
	m.activations[deviceHash] = &activation{
		deviceCodeHash: deviceHash, userCodeHash: userHash, deviceID: request.DeviceID,
		deviceName: strings.TrimSpace(request.DeviceName), publicKey: append(ed25519.PublicKey(nil), publicKey...),
		expiresAt: now.Add(m.activationTTL), interval: m.pollingInterval, status: "pending",
	}
	m.userCodes[userHash] = deviceHash
	if err := m.persistLocked(); err != nil {
		return DeviceAuthorizationResponse{}, err
	}
	return DeviceAuthorizationResponse{
		SchemaVersion: SchemaVersion, DeviceCode: deviceCode, UserCode: userCode,
		VerificationURI: "/activate", VerificationURIComplete: "/activate?user_code=" + url.QueryEscape(userCode),
		ExpiresIn: int64(m.activationTTL / time.Second), Interval: int64(m.pollingInterval / time.Second),
	}, nil
}

func (m *Manager) ApproveDevice(request OwnerApprovalRequest) (OwnerApprovalResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthy(); err != nil {
		return OwnerApprovalResponse{}, err
	}
	if err := m.requireInitialized(); err != nil {
		return OwnerApprovalResponse{}, err
	}
	if err := validateSchemaAndVersion(request.SchemaVersion, request.ClientVersion); err != nil {
		return OwnerApprovalResponse{}, err
	}
	if err := m.verifyOwnerAction(request.OwnerDeviceID, request.RequestID, OwnerApprovalMessage(request), request.Signature, "owner-approve"); err != nil {
		return OwnerApprovalResponse{}, err
	}
	rateLimitErr := m.checkApprovalRateLimit()
	if err := m.persistLocked(); err != nil {
		return OwnerApprovalResponse{}, err
	}
	if rateLimitErr != nil {
		return OwnerApprovalResponse{}, rateLimitErr
	}
	userHash := sha256.Sum256([]byte(normalizeUserCode(request.UserCode)))
	deviceHash, ok := m.userCodes[userHash]
	if !ok {
		m.failedCodeAttempts++
		if err := m.persistLocked(); err != nil {
			return OwnerApprovalResponse{}, err
		}
		return OwnerApprovalResponse{}, fault("INVALID_USER_CODE", "owner-approve", "The activation user code is invalid or expired.", "Check the code shown by the Device and try again.")
	}
	pending := m.activations[deviceHash]
	if pending == nil || !m.now().UTC().Before(pending.expiresAt) {
		m.deleteActivation(deviceHash, pending)
		if err := m.persistLocked(); err != nil {
			return OwnerApprovalResponse{}, err
		}
		return OwnerApprovalResponse{}, fault("ACTIVATION_EXPIRED", "owner-approve", "The activation request has expired.", "Start a new Device activation request.")
	}
	if pending.status != "pending" {
		return OwnerApprovalResponse{}, fault("ACTIVATION_NOT_PENDING", "owner-approve", "The activation request is no longer pending.", "Start a new activation request if the Device is not active.")
	}
	pending.status = "approved"
	if err := m.persistLocked(); err != nil {
		return OwnerApprovalResponse{}, err
	}
	return OwnerApprovalResponse{
		SchemaVersion: SchemaVersion, DeviceID: pending.deviceID,
		DeviceName: pending.deviceName, Status: pending.status,
	}, nil
}

func (m *Manager) PollDevice(request DevicePollRequest) (DeviceActivationResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthy(); err != nil {
		return DeviceActivationResponse{}, err
	}
	if err := m.requireInitialized(); err != nil {
		return DeviceActivationResponse{}, err
	}
	if err := validateSchemaAndVersion(request.SchemaVersion, request.ClientVersion); err != nil {
		return DeviceActivationResponse{}, err
	}
	deviceHash := sha256.Sum256([]byte(request.DeviceCode))
	pending := m.activations[deviceHash]
	if pending == nil {
		return DeviceActivationResponse{}, fault("INVALID_DEVICE_CODE", "device-poll", "The Device activation code is invalid.", "Start a new activation request.")
	}
	now := m.now().UTC()
	if !now.Before(pending.expiresAt) {
		m.deleteActivation(deviceHash, pending)
		if err := m.persistLocked(); err != nil {
			return DeviceActivationResponse{}, err
		}
		return DeviceActivationResponse{}, fault("ACTIVATION_EXPIRED", "device-poll", "The activation request has expired.", "Start a new Device activation request.")
	}
	if !pending.lastPoll.IsZero() && now.Sub(pending.lastPoll) < pending.interval {
		pending.interval += 5 * time.Second
		pending.lastPoll = now
		if err := m.persistLocked(); err != nil {
			return DeviceActivationResponse{}, err
		}
		return DeviceActivationResponse{}, fault("SLOW_DOWN", "device-poll", "The Device polled before the allowed interval.", fmt.Sprintf("Wait at least %d seconds before polling again.", int64(pending.interval/time.Second)))
	}
	pending.lastPoll = now
	if pending.status == "pending" {
		if err := m.persistLocked(); err != nil {
			return DeviceActivationResponse{}, err
		}
		return DeviceActivationResponse{}, fault("AUTHORIZATION_PENDING", "device-poll", "The Owner has not approved this Device yet.", "Wait for Owner approval, then poll again at the advertised interval.")
	}
	if pending.status != "approved" {
		return DeviceActivationResponse{}, fault("ACTIVATION_DENIED", "device-poll", "The Device activation was not approved.", "Start a new activation request and ask the Owner to approve it.")
	}
	m.devices[pending.deviceID] = &deviceRecord{
		deviceID: pending.deviceID, deviceName: pending.deviceName,
		publicKey: append(ed25519.PublicKey(nil), pending.publicKey...), status: "active",
	}
	m.directoryVersion++
	m.deleteActivation(deviceHash, pending)
	if err := m.persistDeviceStatusLocked(pending.deviceID, "active"); err != nil {
		return DeviceActivationResponse{}, err
	}
	return DeviceActivationResponse{
		SchemaVersion: SchemaVersion, FabricID: m.fabricID, DeviceID: pending.deviceID,
		Status: "active", DirectoryVersion: m.directoryVersion, GrantVersion: m.grantVersion,
	}, nil
}

func (m *Manager) RevokeDevice(request OwnerRevocationRequest) (OwnerRevocationResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthy(); err != nil {
		return OwnerRevocationResponse{}, err
	}
	if err := m.requireInitialized(); err != nil {
		return OwnerRevocationResponse{}, err
	}
	if err := validateSchemaAndVersion(request.SchemaVersion, request.ClientVersion); err != nil {
		return OwnerRevocationResponse{}, err
	}
	if err := m.verifyOwnerAction(request.OwnerDeviceID, request.RequestID, OwnerRevocationMessage(request), request.Signature, "owner-revoke"); err != nil {
		return OwnerRevocationResponse{}, err
	}
	if err := m.persistLocked(); err != nil {
		return OwnerRevocationResponse{}, err
	}
	if request.DeviceID == m.ownerDeviceID {
		return OwnerRevocationResponse{}, fault("OWNER_DEVICE_REQUIRES_RECOVERY", "owner-revoke", "The active Owner Device cannot revoke itself.", "Use the recovery workflow to replace the Owner Device.")
	}
	device := m.devices[request.DeviceID]
	if device == nil {
		return OwnerRevocationResponse{}, fault("DEVICE_NOT_FOUND", "owner-revoke", "The Device is not registered in this Fabric.", "Refresh the directory and choose an existing Device.")
	}
	if device.status == "revoked" {
		return OwnerRevocationResponse{}, fault("DEVICE_ALREADY_REVOKED", "owner-revoke", "The Device is already revoked.", "No further action is required.")
	}
	device.status = "revoked"
	m.removeShellBindingsForDevice(request.DeviceID)
	m.directoryVersion++
	m.grantVersion++
	if err := m.persistDeviceStatusLocked(request.DeviceID, "revoked"); err != nil {
		return OwnerRevocationResponse{}, err
	}
	return OwnerRevocationResponse{
		SchemaVersion: SchemaVersion, DeviceID: request.DeviceID, Status: device.status,
		DirectoryVersion: m.directoryVersion, GrantVersion: m.grantVersion,
	}, nil
}

func (m *Manager) SyncVersion(request VersionSyncRequest) (VersionSyncResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireHealthy(); err != nil {
		return VersionSyncResponse{}, err
	}
	if err := m.requireInitialized(); err != nil {
		return VersionSyncResponse{}, err
	}
	if err := validateSchemaAndVersion(request.SchemaVersion, request.ClientVersion); err != nil {
		return VersionSyncResponse{}, err
	}
	if err := validateRequestID(request.RequestID); err != nil {
		return VersionSyncResponse{}, err
	}
	device := m.devices[request.DeviceID]
	if device == nil {
		return VersionSyncResponse{}, fault("DEVICE_NOT_FOUND", "version-sync", "The Device is not registered in this Fabric.", "Activate the Device before synchronizing versions.")
	}
	if err := verifySignature(device.publicKey, VersionSyncMessage(request), request.Signature, "version-sync"); err != nil {
		return VersionSyncResponse{}, err
	}
	if device.status == "revoked" {
		return VersionSyncResponse{}, fault("DEVICE_REVOKED", "version-sync", "The Device identity has been revoked.", "Ask the Owner to activate a new Device identity.")
	}
	if m.deviceRequestIDs[request.DeviceID] == nil {
		m.deviceRequestIDs[request.DeviceID] = make(map[string]struct{})
	}
	if _, exists := m.deviceRequestIDs[request.DeviceID][request.RequestID]; exists {
		return VersionSyncResponse{}, replayFault("version-sync")
	}
	m.recordDeviceRequestID(request.DeviceID, request.RequestID)
	if err := m.persistLocked(); err != nil {
		return VersionSyncResponse{}, err
	}
	return VersionSyncResponse{
		SchemaVersion: SchemaVersion, FabricID: m.fabricID, DeviceID: request.DeviceID,
		DeviceStatus: device.status, DirectoryVersion: m.directoryVersion,
		GrantVersion: m.grantVersion, SupportedClientMajor: SupportedClientMajor,
	}, nil
}

func (m *Manager) verifyOwnerAction(ownerDeviceID, requestID string, message []byte, signature, stage string) error {
	if ownerDeviceID != m.ownerDeviceID {
		return fault("OWNER_IDENTITY_MISMATCH", stage, "The request is not from the active Owner Device.", "Use the initialized Owner Device identity.")
	}
	if err := validateRequestID(requestID); err != nil {
		return err
	}
	if err := verifySignature(m.ownerPublicKey, message, signature, stage); err != nil {
		return err
	}
	if _, exists := m.ownerRequestIDs[requestID]; exists {
		return replayFault(stage)
	}
	m.recordOwnerRequestID(requestID)
	return nil
}

func (m *Manager) recordOwnerRequestID(requestID string) {
	m.ownerRequestIDs[requestID] = struct{}{}
	m.ownerRequestOrder = append(m.ownerRequestOrder, requestID)
	if len(m.ownerRequestOrder) > maxOwnerReplayIDs {
		oldest := m.ownerRequestOrder[0]
		m.ownerRequestOrder = m.ownerRequestOrder[1:]
		delete(m.ownerRequestIDs, oldest)
	}
}

func (m *Manager) recordDeviceRequestID(deviceID, requestID string) {
	if m.deviceRequestIDs[deviceID] == nil {
		m.deviceRequestIDs[deviceID] = make(map[string]struct{})
		m.deviceRequestSince[deviceID] = m.now().UTC()
	}
	m.deviceRequestIDs[deviceID][requestID] = struct{}{}
	m.deviceRequestOrder[deviceID] = append(m.deviceRequestOrder[deviceID], requestID)
	if len(m.deviceRequestOrder[deviceID]) > maxDeviceReplayIDs {
		oldest := m.deviceRequestOrder[deviceID][0]
		m.deviceRequestOrder[deviceID] = m.deviceRequestOrder[deviceID][1:]
		delete(m.deviceRequestIDs[deviceID], oldest)
	}
}

func (m *Manager) requireInitialized() error {
	if m.fabricID == "" {
		return fault("OWNER_NOT_INITIALIZED", "owner", "The Fabric does not have an Owner yet.", "Initialize the Owner before activating Devices.")
	}
	return nil
}

func (m *Manager) checkApprovalRateLimit() error {
	now := m.now().UTC()
	if m.failedCodeWindow.IsZero() || now.Sub(m.failedCodeWindow) >= time.Minute {
		m.failedCodeWindow = now
		m.failedCodeAttempts = 0
	}
	if m.failedCodeAttempts >= 10 {
		return fault("USER_CODE_RATE_LIMITED", "owner-approve", "Too many invalid user-code attempts were made.", "Wait one minute before trying again.")
	}
	return nil
}

func (m *Manager) deleteActivation(deviceHash [sha256.Size]byte, pending *activation) {
	delete(m.activations, deviceHash)
	if pending != nil {
		delete(m.userCodes, pending.userCodeHash)
	}
}

func (m *Manager) activationForDevice(deviceID string) *activation {
	for _, pending := range m.activations {
		if pending.deviceID == deviceID {
			return pending
		}
	}
	return nil
}

func (m *Manager) pruneExpiredState(now time.Time) bool {
	changed := false
	for deviceHash, pending := range m.activations {
		if !now.Before(pending.expiresAt) {
			m.deleteActivation(deviceHash, pending)
			changed = true
		}
	}
	for deviceID, since := range m.deviceRequestSince {
		if _, registered := m.devices[deviceID]; registered || m.activationForDevice(deviceID) != nil || now.Sub(since) < deviceReplayRetention {
			continue
		}
		delete(m.deviceRequestIDs, deviceID)
		delete(m.deviceRequestOrder, deviceID)
		delete(m.deviceRequestSince, deviceID)
		changed = true
	}
	return changed
}

func validateSchemaAndVersion(schema, version string) error {
	if schema != SchemaVersion {
		return fault("UNSUPPORTED_SCHEMA", "version", "The enrollment schema version is unsupported.", "Update PF Remote components together.")
	}
	major, err := parseMajorVersion(version)
	if err != nil || major != SupportedClientMajor {
		return fault("UNSUPPORTED_CLIENT_VERSION", "version", "The PF Remote client major version is unsupported.", "Use a client with major version 1.")
	}
	return nil
}

func parseMajorVersion(version string) (int, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return 0, errors.New("version must contain major, minor, and patch numbers")
	}
	for _, part := range parts {
		if part == "" {
			return 0, errors.New("version component is empty")
		}
		for _, value := range part {
			if value < '0' || value > '9' {
				return 0, errors.New("version component is not numeric")
			}
		}
	}
	return strconv.Atoi(parts[0])
}

func validateRequestID(value string) error {
	if len(value) < 16 || len(value) > 128 {
		return fault("INVALID_REQUEST_ID", "proof", "The signed request ID is invalid.", "Generate a fresh random request ID between 16 and 128 characters.")
	}
	for _, character := range value {
		if !(character == '-' || character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9') {
			return fault("INVALID_REQUEST_ID", "proof", "The signed request ID is invalid.", "Use only URL-safe letters, digits, hyphens, and underscores.")
		}
	}
	return nil
}

func validateDeviceName(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len([]rune(trimmed)) > 80 {
		return fault("INVALID_DEVICE_NAME", "device-authorization", "The Device name is empty or too long.", "Use a name between 1 and 80 characters.")
	}
	for _, character := range trimmed {
		if unicode.IsControl(character) {
			return fault("INVALID_DEVICE_NAME", "device-authorization", "The Device name contains control characters.", "Use a printable Device name.")
		}
	}
	return nil
}

func validIdentifier(value string) bool {
	if len(value) < 3 || len(value) > 64 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if !(character == '-' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func decodePublicKey(encoded string) (ed25519.PublicKey, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, fault("INVALID_PUBLIC_KEY", "proof", "The Device public key is invalid.", "Provide a base64url-encoded Ed25519 public key.")
	}
	return ed25519.PublicKey(decoded), nil
}

func verifyDeviceBinding(deviceID string, publicKey ed25519.PublicKey) error {
	expected, err := identity.DeviceIDFromPublicKey(publicKey)
	if err != nil || deviceID != expected {
		return fault("DEVICE_IDENTITY_MISMATCH", "proof", "The Device ID does not match its public key.", "Recompute the Device ID from the protected identity public key.")
	}
	return nil
}

func verifySignature(publicKey ed25519.PublicKey, message []byte, encoded, stage string) error {
	signature, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || !identity.Verify(publicKey, message, signature) {
		return fault("INVALID_SIGNATURE", stage, "The signed identity proof is invalid.", "Sign the canonical request with the matching protected Device identity.")
	}
	return nil
}

func randomToken(source io.Reader, bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := io.ReadFull(source, buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func randomUserCode(source io.Reader) (string, error) {
	buffer := make([]byte, 8)
	if _, err := io.ReadFull(source, buffer); err != nil {
		return "", err
	}
	for index := range buffer {
		buffer[index] = userCodeAlphabet[int(buffer[index])%len(userCodeAlphabet)]
	}
	return string(buffer[:4]) + "-" + string(buffer[4:]), nil
}

func normalizeUserCode(value string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
}

func replayFault(stage string) *Fault {
	return fault("REQUEST_REPLAYED", stage, "The signed request ID has already been used.", "Generate a fresh request ID and sign the new request.")
}

func fault(code, stage, summary, remediation string) *Fault {
	return &Fault{Code: code, Stage: stage, Summary: summary, Remediation: remediation}
}

func EncodePublicKey(publicKey ed25519.PublicKey) string {
	return base64.RawURLEncoding.EncodeToString(publicKey)
}

func EncodeSignature(signature []byte) string {
	return base64.RawURLEncoding.EncodeToString(signature)
}
