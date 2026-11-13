package peers

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/catalog"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type testSigner struct{ k ed25519.PrivateKey }

func signer(t *testing.T) testSigner {
	t.Helper()
	_, k, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	return testSigner{k}
}
func (s testSigner) DeviceID() string {
	id, _ := identity.DeviceIDFromPublicKey(s.PublicKey())
	return id
}
func (s testSigner) PublicKey() ed25519.PublicKey  { return s.k.Public().(ed25519.PublicKey) }
func (s testSigner) Sign(b []byte) ([]byte, error) { return ed25519.Sign(s.k, b), nil }
func device(s Signer, alias string) contracts.Device {
	return contracts.Device{ID: s.DeviceID(), Alias: alias, DisplayName: alias, State: "online", IdentityPublicKey: base64.RawURLEncoding.EncodeToString(s.PublicKey())}
}

var testTime = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

func appendOK(t *testing.T, j Journal, r Record, s Signer) Journal {
	t.Helper()
	n, e := j.Append(r, s, testTime)
	if e != nil {
		t.Fatal(e)
	}
	return n
}

func fixture(t *testing.T) (Journal, testSigner, testSigner, testSigner) {
	t.Helper()
	root, manager, node := signer(t), signer(t), signer(t)
	j, e := New("fabric-test", device(root, "root"))
	if e != nil {
		t.Fatal(e)
	}
	d := device(manager, "manager")
	j = appendOK(t, j, Record{Kind: "manager", Device: &d}, root)
	req, e := Request(j.Fabric, "node", "Node", node, testTime)
	if e != nil {
		t.Fatal(e)
	}
	j = appendOK(t, j, Record{Kind: "member", Join: &req}, manager)
	c := contracts.Capability{ID: "shell-test", DeviceID: node.DeviceID(), Alias: "shell", DisplayName: "Shell", Kind: contracts.CapabilityShell, State: "available"}
	j = appendOK(t, j, Record{Kind: "capability", Capability: &c}, node)
	until := testTime.Add(time.Hour)
	g := contracts.Grant{ID: "grant-test", SubjectDeviceID: manager.DeviceID(), CapabilityID: c.ID, State: "active", ValidUntil: &until}
	j = appendOK(t, j, Record{Kind: "grant", Grant: &g}, manager)
	return j, root, manager, node
}

func visible(t *testing.T, j Journal, s Signer, now time.Time) int {
	t.Helper()
	snap, e := j.Snapshot(now)
	if e != nil {
		t.Fatal(e)
	}
	return len(catalog.New(snap.FabricID, snap.Devices, snap.Capabilities, snap.Grants, s.DeviceID(), snap.CapturedAt, func() time.Time { return now }).List())
}

func TestSecondManagerEnrollsWithRootAbsent(t *testing.T) {
	j, _, manager, _ := fixture(t)
	if visible(t, j, manager, testTime) != 1 {
		t.Fatal("approved node unavailable")
	}
}
func TestDuplicateDoesNotRefreshAbsoluteGrant(t *testing.T) {
	j, _, m, _ := fixture(t)
	later := testTime.Add(2 * time.Hour)
	n, e := j.Merge(j, later)
	if e != nil {
		t.Fatal(e)
	}
	if len(n.Records) != len(j.Records) || visible(t, n, m, later) != 0 {
		t.Fatal("replay renewed authority")
	}
}
func TestUnauthorizedAndForgedRequests(t *testing.T) {
	j, root, m, node := fixture(t)
	outsider := signer(t)
	req, _ := Request(j.Fabric, "new-node", "New node", outsider, testTime)
	if _, e := j.Append(Record{Kind: "member", Join: &req}, node, testTime); e == nil {
		t.Fatal("member approved another member")
	}
	d := device(outsider, "new-manager")
	if _, e := j.Append(Record{Kind: "manager", Device: &d}, m, testTime); e == nil {
		t.Fatal("manager expanded admin set")
	}
	req.Device.DisplayName = "tampered"
	if _, e := j.Append(Record{Kind: "member", Join: &req}, root, testTime); e == nil {
		t.Fatal("forged request accepted")
	}
	req, _ = Request(j.Fabric, "new-node", "New node", outsider, testTime.Add(-time.Hour))
	if _, e := j.Append(Record{Kind: "member", Join: &req}, m, testTime); e == nil {
		t.Fatal("expired invitation accepted")
	}
}
func TestRevocationWinsAcrossOrderAndRestart(t *testing.T) {
	j, root, m, node := fixture(t)
	revoked := appendOK(t, j, Record{Kind: "revoke-device", Revoke: node.DeviceID()}, m)
	a, e := j.Merge(revoked, testTime)
	if e != nil {
		t.Fatal(e)
	}
	b, e := revoked.Merge(j, testTime)
	if e != nil {
		t.Fatal(e)
	}
	if visible(t, a, m, testTime) != 0 || visible(t, b, m, testTime) != 0 {
		t.Fatal("revocation lost")
	}
	b, e = j.Merge(appendOK(t, j, Record{Kind: "revoke-device", Revoke: m.DeviceID()}, root), testTime)
	if e != nil {
		t.Fatal(e)
	}
	if visible(t, b, m, testTime) != 0 {
		t.Fatal("revoked manager retained access")
	}
	data, _ := json.Marshal(a)
	var reload Journal
	if Decode(data, &reload) != nil || visible(t, reload, m, testTime) != 0 {
		t.Fatal("restart lost tombstone")
	}
}
func TestForkGapWrongRootAndFutureRejected(t *testing.T) {
	j, root, _, _ := fixture(t)
	a := appendOK(t, j, Record{Kind: "revoke-grant", Revoke: "grant-a"}, root)
	b := appendOK(t, j, Record{Kind: "revoke-grant", Revoke: "grant-b"}, root)
	if _, e := a.Merge(b, testTime); e != ErrConflict {
		t.Fatalf("fork=%v", e)
	}
	gap := j
	gap.Records = append([]Record(nil), j.Records...)
	gap.Records = gap.Records[1:]
	if _, e := gap.Snapshot(testTime); e == nil {
		t.Fatal("gap accepted")
	}
	wrong := j
	wrong.Root = device(signer(t), "wrong")
	if _, e := j.Merge(wrong, testTime); e == nil {
		t.Fatal("root changed")
	}
	future, e := j.Append(Record{Kind: "revoke-grant", Revoke: "grant-a"}, root, testTime.Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if _, e := j.Merge(future, testTime); e == nil {
		t.Fatal("future accepted")
	}
}
func TestForeignCapabilityAndUnboundedGrantRejected(t *testing.T) {
	j, root, m, node := fixture(t)
	c := contracts.Capability{ID: "shell-other", DeviceID: root.DeviceID(), Alias: "shell", Kind: contracts.CapabilityShell}
	if _, e := j.Append(Record{Kind: "capability", Capability: &c}, node, testTime); e == nil {
		t.Fatal("foreign capability accepted")
	}
	until := testTime.Add(GrantTTL + time.Second)
	g := contracts.Grant{ID: "grant-long", SubjectDeviceID: m.DeviceID(), CapabilityID: "shell-test", State: "active", ValidUntil: &until}
	if _, e := j.Append(Record{Kind: "grant", Grant: &g}, m, testTime); e == nil {
		t.Fatal("unbounded authority")
	}
}
func TestServiceAtomicImportActivationClockAndRollback(t *testing.T) {
	j, _, m, node := fixture(t)
	now := testTime
	svc := Service{Path: filepath.Join(t.TempDir(), "private", "peer.db"), Signer: m, Now: func() time.Time { return now }}
	call := func(req RequestData) error {
		b, _ := json.Marshal(req)
		_, e := svc.Handle(context.Background(), b)
		return e
	}
	if e := call(RequestData{Operation: "bootstrap", Fabric: j.Fabric, Root: &j.Root}); e != nil {
		t.Fatal(e)
	}
	if e := call(RequestData{Operation: "import", Journal: &j}); e != nil {
		t.Fatal(e)
	}
	if e := call(RequestData{Operation: "activate"}); e != nil {
		t.Fatal(e)
	}
	snap, active, e := ActiveSnapshot(context.Background(), svc.Path, now)
	if e != nil || !active || len(snap.Capabilities) != 1 {
		t.Fatalf("activation %v %v", active, e)
	}
	bad := j
	bad.Fabric = "fabric-other"
	if call(RequestData{Operation: "import", Journal: &bad}) == nil {
		t.Fatal("invalid import accepted")
	}
	snap, active, e = ActiveSnapshot(context.Background(), svc.Path, now)
	if e != nil || !active || len(snap.Capabilities) != 1 {
		t.Fatal("failed import destroyed state")
	}
	now = now.Add(2 * time.Hour)
	if _, _, e = ActiveSnapshot(context.Background(), svc.Path, now); e != nil {
		t.Fatal(e)
	}
	snap, _, e = ActiveSnapshot(context.Background(), svc.Path, testTime)
	if e != nil || !snap.CapturedAt.Equal(now) {
		t.Fatal("clock regressed")
	}
	if e = call(RequestData{Operation: "rollback"}); e != nil {
		t.Fatal(e)
	}
	if _, active, e = ActiveSnapshot(context.Background(), svc.Path, now); e != nil || active {
		t.Fatal("rollback failed")
	}
	now = testTime.Add(3 * time.Hour)
	if e = call(RequestData{Operation: "revoke-device", Revoke: node.DeviceID()}); e != nil {
		t.Fatal(e)
	}
	if call(RequestData{Operation: "rollback"}) == nil {
		t.Fatal("rollback bypassed revocation")
	}
}
func TestStrictDecode(t *testing.T) {
	for _, b := range []string{`{"unknown":true}`, `{} {}`, `[]`} {
		var j Journal
		if Decode([]byte(b), &j) == nil {
			t.Fatalf("accepted %s", b)
		}
	}
}

func TestRootMigrationPreservesTargetsAndOriginalExpiry(t *testing.T) {
	j, root, manager, node := fixture(t)
	snap, e := j.Snapshot(testTime)
	if e != nil {
		t.Fatal(e)
	}
	snap.Capabilities[0].Kind = contracts.CapabilityDesktop
	snap.Capabilities[0].DesktopProfile = &contracts.DesktopProfile{Protocol: "rdp", RenderingEnvironment: "virtual", Authentication: "windows-sso"}
	clean, e := New(j.Fabric, j.Root)
	if e != nil {
		t.Fatal(e)
	}
	clean = appendOK(t, clean, Record{Kind: "migration", Baseline: &snap}, root)
	if visible(t, clean, manager, testTime) != 1 {
		t.Fatal("existing target disappeared")
	}
	if visible(t, clean, manager, testTime.Add(2*time.Hour)) != 0 {
		t.Fatal("migration extended original grant")
	}
	if _, e := clean.Append(Record{Kind: "migration", Baseline: &snap}, root, testTime); e == nil {
		t.Fatal("second migration accepted")
	}
	if _, e := j.Append(Record{Kind: "migration", Baseline: &snap}, manager, testTime); e == nil {
		t.Fatal("non-root migration accepted")
	}
	clean = appendOK(t, clean, Record{Kind: "revoke-device", Revoke: node.DeviceID()}, root)
	if visible(t, clean, manager, testTime) != 0 {
		t.Fatal("migration defeated revocation")
	}
}

func TestManagerRenewalDoesNotNeedRootOrReviveRevocation(t *testing.T) {
	j, _, m, node := fixture(t)
	now := testTime
	svc := initService(t, j, m, filepath.Join(t.TempDir(), "private", "peer.db"))
	svc.Now = func() time.Time { return now }
	call := func(req RequestData) any {
		b, _ := json.Marshal(req)
		v, e := svc.Handle(context.Background(), b)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	call(RequestData{Operation: "activate"})
	now = now.Add(2 * time.Hour)
	call(RequestData{Operation: "renew"})
	renewed := call(RequestData{Operation: "export"}).(Journal)
	if visible(t, renewed, m, now) != 1 {
		t.Fatal("manager could not renew after offline expiry")
	}
	call(RequestData{Operation: "revoke-device", Revoke: node.DeviceID()})
	call(RequestData{Operation: "renew"})
	if visible(t, call(RequestData{Operation: "export"}).(Journal), m, now) != 0 {
		t.Fatal("renewal revived revoked node")
	}
}

func TestRevokedManagerCannotBackdateFreshRevocations(t *testing.T) {
	j, root, m, node := fixture(t)
	// A second manager independently admits the member so revoking the first
	// manager does not itself remove this member's legitimate admission receipt.
	second := signer(t)
	d := device(second, "second-manager")
	j = appendOK(t, j, Record{Kind: "manager", Device: &d}, root)
	join, _ := Request(j.Fabric, "node", "Node", node, testTime)
	j = appendOK(t, j, Record{Kind: "member", Join: &join}, second)
	before := appendOK(t, j, Record{Kind: "revoke-grant", Revoke: "grant-old"}, m)
	revoked := appendOK(t, before, Record{Kind: "revoke-device", Revoke: m.DeviceID()}, root)
	// Construct on the unaware branch, so local Append cannot hide the attack.
	malicious := appendOK(t, before, Record{Kind: "revoke-device", Revoke: node.DeviceID()}, m)
	merged, e := revoked.Merge(malicious, testTime)
	if e != nil {
		t.Fatal(e)
	}
	snap, e := merged.Snapshot(testTime)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range snap.Devices {
		if d.ID == node.DeviceID() && d.State == "revoked" {
			t.Fatal("revoked manager retained revocation authority")
		}
	}
	if _, e = merged.Append(Record{Kind: "revoke-grant", Revoke: "grant-more"}, m, testTime); e == nil {
		t.Fatal("revoked local signer mutated authority")
	}
}
