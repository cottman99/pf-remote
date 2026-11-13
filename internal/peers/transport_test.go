package peers

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/session"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type exchangeExecutor struct {
	t      *testing.T
	remote Service
	calls  int
}

func (e *exchangeExecutor) Execute(ctx context.Context, x session.Execution) (int, error) {
	e.t.Helper()
	e.calls++
	if len(x.Command) != 4 || x.Command[1] != "doctor" || x.Command[2] != "--peer-stdin" || x.Binding.Signature == "" {
		e.t.Fatal("unbounded or unauthenticated exchange")
	}
	b, err := io.ReadAll(x.Stdin)
	if err != nil {
		return 1, err
	}
	out, err := e.remote.Handle(ctx, b)
	if err != nil {
		return 1, err
	}
	err = json.NewEncoder(x.Stdout).Encode(out)
	return 0, err
}
func initService(t *testing.T, j Journal, s Signer, path string) Service {
	t.Helper()
	svc := Service{Path: path, Signer: s, Now: func() time.Time { return testTime }}
	for _, r := range []RequestData{{Operation: "bootstrap", Fabric: j.Fabric, Root: &j.Root}, {Operation: "import", Journal: &j}} {
		b, _ := json.Marshal(r)
		if _, e := svc.Handle(context.Background(), b); e != nil {
			t.Fatal(e)
		}
	}
	return svc
}
func TestAuthenticatedOfflineCatchupUsesFixedCommand(t *testing.T) {
	j, _, m, node := fixture(t)
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	wire := []byte{}
	for _, p := range [][]byte{[]byte("ssh-ed25519"), public} {
		wire = binary.BigEndian.AppendUint32(wire, uint32(len(p)))
		wire = append(wire, p...)
	}
	binding, e := shellbinding.Sign(j.Fabric, "shell-test", 1, []contracts.SSHHostKey{{Algorithm: "ssh-ed25519", PublicKey: base64.StdEncoding.EncodeToString(wire)}}, node)
	if e != nil {
		t.Fatal(e)
	}
	c := contracts.Capability{ID: "shell-test", DeviceID: node.DeviceID(), Alias: "shell", DisplayName: "Shell", Kind: contracts.CapabilityShell, State: "available", SSHBinding: binding}
	j = appendOK(t, j, Record{Kind: "capability", Capability: &c}, node)
	local := initService(t, j, m, filepath.Join(t.TempDir(), "local", "peer.db"))
	remote := initService(t, j, node, filepath.Join(t.TempDir(), "remote", "peer.db"))
	req, _ := json.Marshal(RequestData{Operation: "revoke-grant", Revoke: "grant-test"})
	if _, e := local.Handle(context.Background(), req); e != nil {
		t.Fatal(e)
	}
	endpoint := Endpoint{Target: "pfremote://" + j.Fabric + "/devices/" + node.DeviceID() + "/capabilities/shell-test", Route: route.Candidate{ID: "route-peer", Adapter: "lan", Network: "tcp", Address: "192.0.2.10", Port: 22}, User: "test-user", Binary: "pfremote"}
	exec := &exchangeExecutor{t: t, remote: remote}
	if e = local.Sync(context.Background(), endpoint, exec); e != nil {
		t.Fatal(e)
	}
	export, _ := json.Marshal(RequestData{Operation: "export"})
	value, e := remote.Handle(context.Background(), export)
	if e != nil {
		t.Fatal(e)
	}
	if visible(t, value.(Journal), m, testTime) != 0 {
		t.Fatal("offline recipient missed revocation")
	}
	if e = local.Sync(context.Background(), endpoint, exec); e != nil {
		t.Fatal(e)
	}
	endpoint.Target = "pfremote://fabric-other/devices/" + node.DeviceID() + "/capabilities/shell-test"
	if local.Sync(context.Background(), endpoint, exec) == nil || exec.calls != 2 {
		t.Fatal("wrong Fabric reached transport")
	}
}
