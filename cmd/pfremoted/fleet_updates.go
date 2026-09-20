package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/internal/updatecheck"
)

type fleetStatus struct {
	Schema  string                    `json:"schema_version"`
	Status  string                    `json:"status"`
	Reports []enrollment.UpdateReport `json:"reports"`
}
type fleetUpdates struct {
	mu        sync.Mutex
	operation sync.Mutex
	store     enrollment.UpdateStateStore
	signer    shellbinding.DeviceSigner
	monitor   *updatecheck.Monitor
	status    fleetStatus
}

func (f *fleetUpdates) run(ctx context.Context) {
	f.sync(ctx, false)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.sync(ctx, false)
		}
	}
}
func (f *fleetUpdates) sync(ctx context.Context, notify bool) error {
	f.operation.Lock()
	defer f.operation.Unlock()
	fail := func() error {
		f.mu.Lock()
		f.status.Status = "unavailable"
		f.mu.Unlock()
		return errors.New("update coordination is unavailable")
	}
	settings, err := loadConnectionSettings()
	if err != nil || settings.GatewayURL == "" {
		return fail()
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return fail()
	}
	r := enrollment.UpdateRequest{Schema: enrollment.UpdateSchema, DeviceID: f.signer.DeviceID(), RequestID: "updates-" + hex.EncodeToString(id), Channel: updateChannel, Platform: runtime.GOOS + "-x64", Version: releaseVersion, Status: f.monitor.Status().Code, Notify: notify}
	signature, err := f.signer.Sign(enrollment.UpdateMessage(r))
	if err != nil {
		return fail()
	}
	r.Signature = enrollment.EncodeSignature(signature)
	response, err := settings.gatewayClient().SyncUpdates(ctx, r)
	if err != nil || response.FabricID != settings.FabricID {
		return fail()
	}
	var cursor struct {
		Fabric string
		Hint   uint64
	}
	data, err := f.store.LoadUpdateState(ctx, "pfremote.update-cursor/v1")
	if err != nil {
		return fail()
	}
	if len(data) > 0 && json.Unmarshal(data, &cursor) != nil {
		return fail()
	}
	if cursor.Fabric != response.FabricID || cursor.Hint < response.Hint {
		cursor.Fabric = response.FabricID
		cursor.Hint = response.Hint
		data, _ = json.Marshal(cursor)
		if err = f.store.SaveUpdateState(ctx, "pfremote.update-cursor/v1", data); err != nil {
			return fail()
		}
		f.monitor.Hint()
	}
	f.mu.Lock()
	f.status = fleetStatus{Schema: enrollment.UpdateSchema, Status: "connected", Reports: response.Reports}
	f.mu.Unlock()
	return nil
}
func (f *fleetUpdates) action(ctx context.Context, action string) (any, error) {
	switch action {
	case "check-updates":
		f.monitor.Hint()
	case "notify-updates":
		if err := f.sync(ctx, true); err != nil {
			return nil, err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.status
	s.Schema = enrollment.UpdateSchema
	if s.Status == "" {
		s.Status = "unavailable"
	}
	s.Reports = append([]enrollment.UpdateReport{}, s.Reports...)
	return s, nil
}
