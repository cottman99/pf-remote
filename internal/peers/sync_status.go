package peers

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/targetref"
)

// Diagnostic metadata never renews authority and contains no route or key data.
func (s Service) recordSync(target string, success bool) {
	ref, e := targetref.Parse(target)
	if e != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	db, e := state.Open(s.Path)
	if e != nil {
		return
	}
	defer db.Close()
	for attempt := 0; attempt < 2; attempt++ {
		old, e := db.LoadControlState(ctx, Schema)
		if e != nil || len(old) == 0 {
			return
		}
		var p persisted
		if Decode(old, &p) != nil || p.Journal.Fabric != ref.FabricID {
			return
		}
		var value SyncStatus
		for _, v := range p.Sync {
			if v.Device == ref.DeviceID {
				value = v
			}
		}
		value.Device = ref.DeviceID
		value.Attempt = s.clock()
		value.Code = "retry"
		if success {
			now := value.Attempt
			value.Success = &now
			value.Code = "current"
		}
		updated := false
		for i := range p.Sync {
			if p.Sync[i].Device == value.Device {
				p.Sync[i] = value
				updated = true
			}
		}
		if !updated {
			if len(p.Sync) >= 32 {
				return
			}
			p.Sync = append(p.Sync, value)
		}
		sort.Slice(p.Sync, func(a, b int) bool { return p.Sync[a].Device < p.Sync[b].Device })
		next, e := json.Marshal(p)
		if e != nil {
			return
		}
		if db.CompareAndSwapControlState(ctx, Schema, old, next) == nil {
			return
		}
	}
}
