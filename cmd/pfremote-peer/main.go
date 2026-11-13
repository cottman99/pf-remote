// pfremote-peer prepares and validates peer migration beside the existing daemon.
// It uses the same protected identity and action core, never a copied private key.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/internal/catalog"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/localapi"
	"github.com/cottman99/pf-remote/internal/peers"
	"github.com/cottman99/pf-remote/internal/state"
)

var releaseVersion = "0.1.0-alpha.118-peer.2"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Peer migration failed; check identity, authority, signed records and protected local storage.")
		os.Exit(1)
	}
}
func run() error {
	idDefault, e := identity.DefaultPath()
	if e != nil {
		return e
	}
	stateDefault, e := state.DefaultPath()
	if e != nil {
		return e
	}
	identityPath := flag.String("identity", idDefault, "existing protected identity path; new nodes generate their own identity")
	statePath := flag.String("state", peers.PathForState(stateDefault), "protected peer state database")
	transports := flag.String("transports", peers.TransportPath(stateDefault), "private SSH transport settings")
	serve := flag.Bool("serve", false, "serve peer pilot on an explicitly isolated protected IPC endpoint")
	version := flag.Bool("version", false, "print candidate version")
	flag.Parse()
	if *version {
		fmt.Println(releaseVersion)
		return nil
	}
	if !filepath.IsAbs(*identityPath) || !filepath.IsAbs(*statePath) {
		return peers.ErrInvalid
	}
	store, e := identity.NewStore(*identityPath)
	if e != nil {
		return e
	}
	signer, e := store.LoadOrCreate()
	if e != nil {
		return e
	}
	svc := peers.Service{Path: *statePath, Signer: signer}
	if !*serve {
		raw, e := io.ReadAll(io.LimitReader(os.Stdin, 900001))
		if e != nil || len(raw) > 900000 {
			return peers.ErrInvalid
		}
		value, e := svc.Handle(context.Background(), raw)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(value)
	}
	// A pilot must not replace the running production pipe/socket.
	if os.Getenv("PFREMOTE_LOCAL_ENDPOINT") == "" {
		return peers.ErrInvalid
	}
	listener, _, e := localapi.Listen()
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	provider := func() (actions.Service, error) {
		snap, active, e := peers.ActiveSnapshot(ctx, *statePath, time.Now())
		if e != nil {
			return actions.Service{}, e
		}
		if !active {
			return actions.Service{}, peers.ErrInvalid
		}
		cfg, e := peers.LoadTransports(*transports)
		if e != nil {
			return actions.Service{}, e
		}
		cat := catalog.New(snap.FabricID, snap.Devices, snap.Capabilities, snap.Grants, signer.DeviceID(), snap.CapturedAt, func() time.Time {
			now := time.Now()
			if now.Before(snap.CapturedAt) {
				return snap.CapturedAt
			}
			return now
		})
		return actions.Service{DeviceID: signer.DeviceID(), Catalog: cat, RoutesConfigured: len(cfg.Peers) > 0, Shell: peers.ShellRunner{Config: cfg, Catalog: cat, Subject: signer.DeviceID()}}, nil
	}
	server := localapi.NewServerWithProvider(provider)
	server.Handler.Peers = svc.Handle
	go svc.RunSynchronization(ctx, *transports)
	return server.Serve(ctx, listener)
}
