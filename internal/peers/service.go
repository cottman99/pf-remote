package peers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/cottman99/pf-remote/internal/catalog"
	"github.com/cottman99/pf-remote/internal/peerguard"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type RequestData struct {
	Operation  string                `json:"operation"`
	Fabric     string                `json:"fabric,omitempty"`
	Root       *contracts.Device     `json:"root,omitempty"`
	Device     *contracts.Device     `json:"device,omitempty"`
	Alias      string                `json:"alias,omitempty"`
	Name       string                `json:"name,omitempty"`
	Join       *Join                 `json:"join,omitempty"`
	Capability *contracts.Capability `json:"capability,omitempty"`
	Grant      *contracts.Grant      `json:"grant,omitempty"`
	Revoke     string                `json:"revoke,omitempty"`
	Journal    *Journal              `json:"journal,omitempty"`
	Baseline   *state.Snapshot       `json:"baseline,omitempty"`
}

type persisted struct {
	Journal Journal      `json:"journal"`
	Active  bool         `json:"active"`
	Sync    []SyncStatus `json:"sync,omitempty"`
}

type SyncStatus struct {
	Device  string     `json:"device_id"`
	Attempt time.Time  `json:"last_attempt"`
	Success *time.Time `json:"last_success,omitempty"`
	Code    string     `json:"code"`
}

type Status struct {
	Schema       string       `json:"schema_version"`
	Initialized  bool         `json:"initialized"`
	Active       bool         `json:"active"`
	Records      int          `json:"records"`
	Devices      int          `json:"devices"`
	Capabilities int          `json:"capabilities"`
	Sync         []SyncStatus `json:"sync,omitempty"`
}

type Service struct {
	Path   string
	Signer Signer
	Now    func() time.Time
}

func PathForState(path string) string {
	return filepath.Join(filepath.Dir(path), "peer-management-v1.db")
}

func (s Service) clock() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) Handle(ctx context.Context, raw json.RawMessage) (any, error) {
	var req RequestData
	if Decode(raw, &req) != nil {
		return nil, ErrInvalid
	}
	if req.Operation == "join" {
		return Request(req.Fabric, req.Alias, req.Name, s.Signer, s.clock())
	}
	db, e := state.Open(s.Path)
	if e != nil {
		return nil, e
	}
	defer db.Close()
	now, e := db.ObserveAuthorizationTime(ctx, s.clock())
	if e != nil {
		return nil, e
	}
	old, e := db.LoadControlState(ctx, Schema)
	if e != nil {
		return nil, e
	}
	var p persisted
	if len(old) > 0 {
		if Decode(old, &p) != nil {
			return nil, ErrInvalid
		}
		if _, e = p.Journal.Snapshot(now); e != nil {
			return nil, e
		}
	}
	switch req.Operation {
	case "status":
		out := Status{Schema: Schema, Initialized: len(old) > 0, Active: p.Active, Records: len(p.Journal.Records)}
		out.Sync = p.Sync
		if len(old) > 0 {
			snap, _ := p.Journal.Snapshot(now)
			out.Devices = len(snap.Devices)
			out.Capabilities = len(snap.Capabilities)
		}
		return out, nil
	case "bootstrap":
		if len(old) > 0 || req.Root == nil {
			return nil, ErrInvalid
		}
		p.Journal, e = New(req.Fabric, *req.Root)
	case "export":
		if len(old) == 0 {
			return nil, ErrInvalid
		}
		return p.Journal, nil
	case "catalog":
		if len(old) == 0 {
			return nil, ErrInvalid
		}
		snap, err := p.Journal.Snapshot(now)
		if err != nil {
			return nil, err
		}
		return catalog.New(snap.FabricID, snap.Devices, snap.Capabilities, snap.Grants, s.Signer.DeviceID(), snap.CapturedAt, func() time.Time { return now }).List(), nil
	default:
		if len(old) == 0 {
			return nil, errors.New("peer management is not initialized")
		}
		switch req.Operation {
		case "import", "exchange":
			if req.Journal == nil {
				return nil, ErrInvalid
			}
			p.Journal, e = p.Journal.Merge(*req.Journal, now)
		case "activate":
			snap, err := p.Journal.Snapshot(now)
			if err != nil {
				return nil, err
			}
			present := false
			for _, d := range snap.Devices {
				if d.ID == s.Signer.DeviceID() && d.State != "revoked" {
					present = true
				}
			}
			if !present {
				return nil, ErrInvalid
			}
			if err := peerguard.Require(filepath.Dir(s.Path)); err != nil {
				return nil, err
			}
			p.Active = true
		case "rollback":
			// Legacy authority cannot override known revocations. Recovery needs an
			// explicitly reconciled legacy snapshot, not this convenience operation.
			for _, r := range p.Journal.Records {
				if r.Baseline != nil {
					for _, d := range r.Baseline.Devices {
						if d.State == "revoked" {
							return nil, ErrInvalid
						}
					}
					for _, g := range r.Baseline.Grants {
						if g.State == "revoked" {
							return nil, ErrInvalid
						}
					}
				}
				if r.Kind == "revoke-device" || r.Kind == "revoke-grant" {
					return nil, errors.New("rollback requires revocation-aware legacy recovery")
				}
			}
			p.Active = false
		case "appoint":
			if req.Device == nil {
				return nil, ErrInvalid
			}
			p.Journal, e = p.Journal.Append(Record{Kind: "manager", Device: req.Device}, s.Signer, now)
		case "migrate":
			if req.Baseline == nil {
				return nil, ErrInvalid
			}
			p.Journal, e = p.Journal.Append(Record{Kind: "migration", Baseline: req.Baseline}, s.Signer, now)
		case "approve":
			if req.Join == nil {
				return nil, ErrInvalid
			}
			p.Journal, e = p.Journal.Append(Record{Kind: "member", Join: req.Join}, s.Signer, now)
		case "publish":
			if req.Capability == nil {
				return nil, ErrInvalid
			}
			if req.Capability.Kind == contracts.CapabilityShell && req.Capability.SSHBinding == nil {
				keys, err := shellbinding.DiscoverSystemHostKeys()
				if err != nil {
					return nil, err
				}
				var version uint64 = 1
				for _, r := range p.Journal.Records {
					if r.Capability != nil && r.Capability.ID == req.Capability.ID && r.Capability.SSHBinding != nil && r.Capability.SSHBinding.BindingVersion >= version {
						version = r.Capability.SSHBinding.BindingVersion + 1
					}
				}
				binding, err := shellbinding.Sign(p.Journal.Fabric, req.Capability.ID, version, keys, s.Signer)
				if err != nil {
					return nil, err
				}
				req.Capability.SSHBinding = binding
			}
			p.Journal, e = p.Journal.Append(Record{Kind: "capability", Capability: req.Capability}, s.Signer, now)
		case "renew":
			if !p.Active {
				return nil, ErrInvalid
			}
			snap, err := p.Journal.Snapshot(now)
			if err != nil {
				return nil, err
			}
			live := map[string]bool{}
			for _, d := range snap.Devices {
				live[d.ID] = d.State != "revoked"
			}
			owners := map[string]string{}
			for _, c := range snap.Capabilities {
				owners[c.ID] = c.DeviceID
			}
			issuers := map[string]string{}
			for _, r := range p.Journal.Records {
				if r.Baseline != nil {
					for _, g := range r.Baseline.Grants {
						issuers[g.ID] = p.Journal.Root.ID
					}
				}
				if r.Grant != nil {
					issuers[r.Grant.ID] = r.Issuer
				}
			}
			for _, g := range snap.Grants {
				if issuers[g.ID] != s.Signer.DeviceID() || g.State != "active" || !live[g.SubjectDeviceID] || !live[owners[g.CapabilityID]] || g.ValidUntil == nil || g.ValidUntil.After(now.Add(48*time.Hour)) {
					continue
				}
				until := now.Add(GrantTTL)
				g.ValidUntil = &until
				p.Journal, err = p.Journal.Append(Record{Kind: "grant", Grant: &g}, s.Signer, now)
				if err != nil {
					return nil, err
				}
			}
		case "grant":
			if req.Grant == nil {
				return nil, ErrInvalid
			}
			p.Journal, e = p.Journal.Append(Record{Kind: "grant", Grant: req.Grant}, s.Signer, now)
		case "revoke-device", "revoke-grant":
			p.Journal, e = p.Journal.Append(Record{Kind: req.Operation, Revoke: req.Revoke}, s.Signer, now)
		default:
			return nil, ErrInvalid
		}
	}
	if e != nil {
		return nil, e
	}
	next, e := json.Marshal(p)
	if e != nil || len(next) > MaxBytes {
		return nil, ErrInvalid
	}
	if e = db.CompareAndSwapControlState(ctx, Schema, old, next); e != nil {
		return nil, e
	}
	if req.Operation == "exchange" {
		return p.Journal, nil
	}
	return Status{Schema: Schema, Initialized: true, Active: p.Active, Records: len(p.Journal.Records)}, nil
}

// ActiveSnapshot never falls back after a corrupt activated peer store. Missing
// store is the only implicit legacy case; rollback is an explicit local action.
func ActiveSnapshot(ctx context.Context, path string, now time.Time) (state.Snapshot, bool, error) {
	required, guardErr := peerguard.RequiredAt(filepath.Dir(path))
	if guardErr != nil {
		return state.Snapshot{}, false, guardErr
	}
	info, e := os.Lstat(path)
	if errors.Is(e, os.ErrNotExist) {
		if required {
			return state.Snapshot{}, false, ErrInvalid
		}
		return state.Snapshot{}, false, nil
	}
	if e != nil || !info.Mode().IsRegular() {
		return state.Snapshot{}, false, ErrInvalid
	}
	db, e := state.Open(path)
	if e != nil {
		return state.Snapshot{}, false, e
	}
	defer db.Close()
	effective, e := db.ObserveAuthorizationTime(ctx, now)
	if e != nil {
		return state.Snapshot{}, false, e
	}
	b, e := db.LoadControlState(ctx, Schema)
	if e != nil {
		return state.Snapshot{}, false, e
	}
	if len(b) == 0 {
		if required {
			return state.Snapshot{}, false, ErrInvalid
		}
		return state.Snapshot{}, false, nil
	}
	var p persisted
	if Decode(b, &p) != nil {
		return state.Snapshot{}, false, ErrInvalid
	}
	snap, e := p.Journal.Snapshot(effective)
	if e != nil {
		return state.Snapshot{}, false, e
	}
	return snap, p.Active, nil
}
