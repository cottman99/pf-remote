// Package peers verifies replicated management authority independently of transport.
package peers

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"time"

	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

const Schema = "pfremote.peer-journal/v1"
const MaxRecords = 16384
const MaxBytes = 8 << 20
const GrantTTL = 7 * 24 * time.Hour

type Signer interface {
	DeviceID() string
	PublicKey() ed25519.PublicKey
	Sign([]byte) ([]byte, error)
}

type Join struct {
	Fabric    string           `json:"fabric"`
	Device    contracts.Device `json:"device"`
	Nonce     string           `json:"nonce"`
	Issued    time.Time        `json:"issued"`
	Expires   time.Time        `json:"expires"`
	Signature string           `json:"signature"`
}

type Record struct {
	Schema         string                `json:"schema"`
	Fabric         string                `json:"fabric"`
	Issuer         string                `json:"issuer"`
	Sequence       uint64                `json:"sequence"`
	Previous       string                `json:"previous"`
	Issued         time.Time             `json:"issued"`
	Kind           string                `json:"kind"`
	Device         *contracts.Device     `json:"device,omitempty"`
	Join           *Join                 `json:"join,omitempty"`
	Capability     *contracts.Capability `json:"capability,omitempty"`
	Grant          *contracts.Grant      `json:"grant,omitempty"`
	Baseline       *state.Snapshot       `json:"baseline,omitempty"`
	Revoke         string                `json:"revoke,omitempty"`
	RevokedThrough uint64                `json:"revoked_through,omitempty"`
	RevokedDigest  string                `json:"revoked_digest,omitempty"`
	Signature      string                `json:"signature"`
}

type Journal struct {
	Schema  string           `json:"schema"`
	Fabric  string           `json:"fabric"`
	Root    contracts.Device `json:"root"`
	Records []Record         `json:"records"`
}

var ErrInvalid = errors.New("peer records failed identity, authority or chain validation")
var ErrConflict = errors.New("peer issuer equivocated; synchronization requires repair")

func Decode(data []byte, dst any) error {
	if len(data) > MaxBytes {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil || d.Decode(&struct{}{}) != io.EOF {
		return ErrInvalid
	}
	return nil
}

func key(d contracts.Device) (ed25519.PublicKey, error) {
	b, e := base64.RawURLEncoding.Strict().DecodeString(d.IdentityPublicKey)
	if e != nil {
		return nil, ErrInvalid
	}
	id, e := identity.DeviceIDFromPublicKey(b)
	if e != nil || id != d.ID || d.Alias == "" || d.DisplayName == "" || d.State != "online" {
		return nil, ErrInvalid
	}
	return b, nil
}

func New(fabric string, root contracts.Device) (Journal, error) {
	j := Journal{Schema: Schema, Fabric: fabric, Root: root, Records: []Record{}}
	_, err := j.Snapshot(time.Now())
	return j, err
}

func joinBytes(j Join) []byte {
	j.Signature = ""
	b, _ := json.Marshal(j)
	return append([]byte("pfremote-peer-join/v1\n"), b...)
}
func recordBytes(r Record) []byte {
	r.Signature = ""
	b, _ := json.Marshal(r)
	return append([]byte("pfremote-peer-record/v1\n"), b...)
}
func Digest(r Record) string {
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func signature(k ed25519.PublicKey, b []byte, sig string) bool {
	s, e := base64.RawURLEncoding.Strict().DecodeString(sig)
	return e == nil && ed25519.Verify(k, b, s)
}

func Request(fabric, alias, name string, signer Signer, now time.Time) (Join, error) {
	nonce := make([]byte, 32)
	if _, e := rand.Read(nonce); e != nil {
		return Join{}, e
	}
	j := Join{Fabric: fabric, Device: contracts.Device{ID: signer.DeviceID(), Alias: alias, DisplayName: name, State: "online", IdentityPublicKey: base64.RawURLEncoding.EncodeToString(signer.PublicKey())}, Nonce: base64.RawURLEncoding.EncodeToString(nonce), Issued: now.UTC(), Expires: now.UTC().Add(10 * time.Minute)}
	s, e := signer.Sign(joinBytes(j))
	if e != nil {
		return Join{}, e
	}
	j.Signature = base64.RawURLEncoding.EncodeToString(s)
	return j, nil
}

func validJoin(j Join, fabric string, at time.Time) bool {
	k, e := key(j.Device)
	n, ne := base64.RawURLEncoding.Strict().DecodeString(j.Nonce)
	return e == nil && ne == nil && len(n) == 32 && j.Fabric == fabric && !j.Issued.IsZero() && !at.Before(j.Issued) && at.Before(j.Expires) && j.Expires.Sub(j.Issued) <= 10*time.Minute && signature(k, joinBytes(j), j.Signature)
}

// Append signs one local mutation. It returns a copy and never changes the caller
// on failure. Persistence must use a compare-and-swap transaction.
func (j Journal) Append(r Record, signer Signer, now time.Time) (Journal, error) {
	current, err := j.Snapshot(now)
	if err != nil {
		return Journal{}, err
	}
	allowed := false
	for _, d := range current.Devices {
		if d.ID == signer.DeviceID() && d.State != "revoked" {
			allowed = true
		}
	}
	if !allowed {
		return Journal{}, ErrInvalid
	}
	r.Schema = Schema
	r.Fabric = j.Fabric
	r.Issuer = signer.DeviceID()
	r.Issued = now.UTC()
	r.Sequence = 1
	r.Previous = ""
	if r.Kind == "revoke-device" && signer.DeviceID() == j.Root.ID {
		for _, p := range j.Records {
			if p.Issuer == r.Revoke && p.Sequence > r.RevokedThrough {
				r.RevokedThrough = p.Sequence
				r.RevokedDigest = Digest(p)
			}
		}
	}
	for _, p := range j.Records {
		if p.Issuer == r.Issuer && p.Sequence >= r.Sequence {
			r.Sequence = p.Sequence + 1
			r.Previous = Digest(p)
		}
	}
	s, e := signer.Sign(recordBytes(r))
	if e != nil {
		return Journal{}, e
	}
	r.Signature = base64.RawURLEncoding.EncodeToString(s)
	return j.Merge(Journal{Schema: Schema, Fabric: j.Fabric, Root: j.Root, Records: []Record{r}}, now)
}

func (j Journal) Merge(other Journal, now time.Time) (Journal, error) {
	if other.Schema != Schema || other.Fabric != j.Fabric || other.Root.ID != j.Root.ID || other.Root.IdentityPublicKey != j.Root.IdentityPublicKey {
		return Journal{}, ErrInvalid
	}
	result := j
	result.Records = append(append([]Record{}, j.Records...), other.Records...)
	sort.Slice(result.Records, func(a, b int) bool {
		x, y := result.Records[a], result.Records[b]
		if x.Issuer != y.Issuer {
			return x.Issuer < y.Issuer
		}
		return x.Sequence < y.Sequence
	})
	unique := make([]Record, 0, len(result.Records))
	for _, r := range result.Records {
		if len(unique) > 0 {
			p := unique[len(unique)-1]
			if p.Issuer == r.Issuer && p.Sequence == r.Sequence {
				if Digest(p) != Digest(r) {
					return Journal{}, ErrConflict
				}
				continue
			}
		}
		unique = append(unique, r)
	}
	result.Records = unique
	if _, e := result.Snapshot(now); e != nil {
		return Journal{}, e
	}
	return result, nil
}

// Snapshot derives effective authority from signatures. Revocations are checked
// after structural validation so arrival order cannot resurrect an issuer.
func (j Journal) Snapshot(now time.Time) (state.Snapshot, error) {
	fail := func() (state.Snapshot, error) { return state.Snapshot{}, ErrInvalid }
	if j.Schema != Schema || len(j.Records) > MaxRecords {
		return fail()
	}
	j.Records = append([]Record(nil), j.Records...)
	sort.Slice(j.Records, func(a, b int) bool {
		if j.Records[a].Issuer != j.Records[b].Issuer {
			return j.Records[a].Issuer < j.Records[b].Issuer
		}
		return j.Records[a].Sequence < j.Records[b].Sequence
	})
	rootKey, e := key(j.Root)
	if e != nil {
		return fail()
	}
	devices := map[string]contracts.Device{j.Root.ID: j.Root}
	managers := map[string]bool{j.Root.ID: true}
	var baseline *state.Snapshot
	baselineRevoked := map[string]bool{}
	for _, r := range j.Records {
		if r.Kind == "migration" {
			if baseline != nil || r.Issuer != j.Root.ID || r.Baseline == nil || r.Baseline.FabricID != j.Fabric || r.Baseline.CapturedAt.After(r.Issued) || r.Baseline.Validate() != nil || !signature(rootKey, recordBytes(r), r.Signature) {
				return fail()
			}
			baseline = r.Baseline
			for _, d := range baseline.Devices {
				if d.State == "revoked" {
					baselineRevoked[d.ID] = true
				}
				d.State = "online"
				if _, err := key(d); err != nil {
					return fail()
				}
				if d.ID == j.Root.ID && d.IdentityPublicKey != j.Root.IdentityPublicKey {
					return fail()
				}
				devices[d.ID] = d
			}
			if baselineRevoked[j.Root.ID] {
				return fail()
			}
			for _, c := range baseline.Capabilities {
				if c.Kind == contracts.CapabilityShell && c.SSHBinding == nil {
					return fail()
				}
			}
		}
	}
	// Only root appointments establish manager authority, never self-assertions.
	for _, r := range j.Records {
		if r.Kind == "manager" {
			if r.Issuer != j.Root.ID || r.Device == nil || !signature(rootKey, recordBytes(r), r.Signature) {
				return fail()
			}
			if _, e := key(*r.Device); e != nil {
				return fail()
			}
			devices[r.Device.ID] = *r.Device
			managers[r.Device.ID] = true
		}
	}
	for _, r := range j.Records {
		if r.Kind == "member" {
			d, ok := devices[r.Issuer]
			k, e := key(d)
			if !ok || e != nil || !managers[r.Issuer] || r.Join == nil || !validJoin(*r.Join, j.Fabric, r.Issued) || !signature(k, recordBytes(r), r.Signature) {
				return fail()
			}
			if existing, ok := devices[r.Join.Device.ID]; ok && existing.IdentityPublicKey != r.Join.Device.IdentityPublicKey {
				return fail()
			}
			devices[r.Join.Device.ID] = r.Join.Device
		}
	}
	heads := map[string]Record{}
	revoked := map[string]bool{}
	for id := range baselineRevoked {
		revoked[id] = true
	}
	revokedGrants := map[string]bool{}
	cuts := map[string]uint64{}
	records := append([]Record(nil), j.Records...)
	sort.Slice(records, func(a, b int) bool {
		if records[a].Issuer != records[b].Issuer {
			return records[a].Issuer < records[b].Issuer
		}
		return records[a].Sequence < records[b].Sequence
	})
	for _, r := range records {
		d, ok := devices[r.Issuer]
		k, e := key(d)
		if !ok || e != nil || r.Schema != Schema || r.Fabric != j.Fabric || r.Issued.IsZero() || r.Issued.After(now.Add(5*time.Minute)) || !signature(k, recordBytes(r), r.Signature) {
			return fail()
		}
		p, has := heads[r.Issuer]
		if !has && (r.Sequence != 1 || r.Previous != "") || has && (r.Sequence != p.Sequence+1 || r.Previous != Digest(p) || r.Issued.Before(p.Issued)) {
			return fail()
		}
		heads[r.Issuer] = r
		fields := 0
		if r.Device != nil {
			fields++
		}
		if r.Join != nil {
			fields++
		}
		if r.Capability != nil {
			fields++
		}
		if r.Grant != nil {
			fields++
		}
		if r.Baseline != nil {
			fields++
		}
		if r.Revoke != "" {
			fields++
		}
		if fields != 1 {
			return fail()
		}
		switch r.Kind {
		case "migration":
			if r.Baseline == nil || r.Issuer != j.Root.ID {
				return fail()
			}
		case "manager":
			if r.Device == nil || r.Issuer != j.Root.ID {
				return fail()
			}
		case "member":
			if r.Join == nil || !managers[r.Issuer] {
				return fail()
			}
		case "capability":
			if r.Capability == nil || r.Capability.DeviceID != r.Issuer {
				return fail()
			}
		case "grant":
			if r.Grant == nil || !managers[r.Issuer] || r.Grant.State != "active" || r.Grant.ValidUntil == nil || !r.Grant.ValidUntil.After(r.Issued) || r.Grant.ValidUntil.Sub(r.Issued) > GrantTTL {
				return fail()
			}
		case "revoke-device":
			if r.Revoke == "" || !managers[r.Issuer] || r.Revoke == j.Root.ID || managers[r.Revoke] && r.Issuer != j.Root.ID {
				return fail()
			}
			if r.Issuer == j.Root.ID {
				if r.RevokedThrough == 0 && r.RevokedDigest != "" {
					return fail()
				}
				found := r.RevokedThrough == 0
				for _, p := range records {
					if p.Issuer == r.Revoke && p.Sequence == r.RevokedThrough && Digest(p) == r.RevokedDigest {
						found = true
					}
				}
				if !found {
					return fail()
				}
				revoked[r.Revoke] = true
				if _, ok := cuts[r.Revoke]; !ok {
					cuts[r.Revoke] = r.RevokedThrough
				}
			} else if r.RevokedThrough != 0 || r.RevokedDigest != "" {
				return fail()
			}
		case "revoke-grant":
			if r.Revoke == "" || !managers[r.Issuer] {
				return fail()
			}
			if r.Issuer == j.Root.ID {
				revokedGrants[r.Revoke] = true
			}
		default:
			return fail()
		}
		if r.Kind != "revoke-device" && (r.RevokedThrough != 0 || r.RevokedDigest != "") {
			return fail()
		}
	}
	// Root freezes the revoked manager's witnessed chain. Later signed negative
	// records cannot acquire authority by lying about their issuance timestamp.
	for _, r := range records {
		if r.Issuer == j.Root.ID || revoked[r.Issuer] && r.Sequence > cuts[r.Issuer] {
			continue
		}
		if r.Kind == "revoke-device" {
			revoked[r.Revoke] = true
		}
		if r.Kind == "revoke-grant" {
			revokedGrants[r.Revoke] = true
		}
	}
	// Admission receipts from revoked managers are no longer sufficient. Root
	// appointments remain independent; another active manager may admit a member.
	admitted := map[string]bool{j.Root.ID: true}
	if baseline != nil {
		for _, d := range baseline.Devices {
			admitted[d.ID] = true
		}
	}
	for id := range managers {
		if !revoked[id] {
			admitted[id] = true
		}
	}
	for _, r := range records {
		if r.Kind == "member" && !revoked[r.Issuer] {
			admitted[r.Join.Device.ID] = true
		}
	}
	caps := map[string]contracts.Capability{}
	grants := map[string]contracts.Grant{}
	grantIssuers := map[string]string{}
	if baseline != nil {
		for _, c := range baseline.Capabilities {
			caps[c.ID] = c
		}
		for _, g := range baseline.Grants {
			until := baseline.CapturedAt.Add(GrantTTL)
			if g.ValidUntil != nil && g.ValidUntil.Before(until) {
				until = *g.ValidUntil
			}
			g.ValidUntil = &until
			if g.State == "revoked" || revokedGrants[g.ID] {
				g.State = "revoked"
				revokedGrants[g.ID] = true
			}
			grants[g.ID] = g
			grantIssuers[g.ID] = j.Root.ID
		}
	}
	for _, r := range records {
		if r.Kind == "capability" {
			c := *r.Capability
			if old, ok := caps[c.ID]; ok && old.DeviceID != c.DeviceID {
				return fail()
			}
			if old, ok := caps[c.ID]; ok && old.SSHBinding != nil && (c.SSHBinding == nil || c.SSHBinding.BindingVersion < old.SSHBinding.BindingVersion || c.SSHBinding.BindingVersion == old.SSHBinding.BindingVersion && c.SSHBinding.Signature != old.SSHBinding.Signature) {
				return fail()
			}
			caps[c.ID] = c
		}
		if r.Kind == "grant" {
			g := *r.Grant
			if issuer, ok := grantIssuers[g.ID]; ok && issuer != r.Issuer {
				return fail()
			}
			grantIssuers[g.ID] = r.Issuer
			if old, ok := grants[g.ID]; ok && (old.SubjectDeviceID != g.SubjectDeviceID || old.CapabilityID != g.CapabilityID) {
				return fail()
			}
			if revokedGrants[g.ID] || revoked[r.Issuer] {
				g.State = "revoked"
			}
			grants[g.ID] = g
		}
	}
	snapshot := state.Snapshot{SchemaVersion: state.SnapshotSchema, FabricID: j.Fabric, DirectoryVersion: uint64(len(records)) + 1, GrantVersion: uint64(len(records)) + 1, CapturedAt: now.UTC()}
	for id, d := range devices {
		if revoked[id] || !admitted[id] {
			d.State = "revoked"
		}
		snapshot.Devices = append(snapshot.Devices, d)
	}
	for _, c := range caps {
		snapshot.Capabilities = append(snapshot.Capabilities, c)
	}
	for _, g := range grants {
		snapshot.Grants = append(snapshot.Grants, g)
	}
	sort.Slice(snapshot.Devices, func(a, b int) bool { return snapshot.Devices[a].ID < snapshot.Devices[b].ID })
	sort.Slice(snapshot.Capabilities, func(a, b int) bool { return snapshot.Capabilities[a].ID < snapshot.Capabilities[b].ID })
	sort.Slice(snapshot.Grants, func(a, b int) bool { return snapshot.Grants[a].ID < snapshot.Grants[b].ID })
	if e := snapshot.Validate(); e != nil {
		return fail()
	}
	return snapshot, nil
}
